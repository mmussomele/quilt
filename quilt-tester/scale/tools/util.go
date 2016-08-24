package tools

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/quilt-tester/util"

	log "github.com/Sirupsen/logrus"
)

var (
	webRoot  = filepath.Join("/var/www/quilt-tester", "scale")
	done     = make(chan struct{}, 1)
	shutdown = false

	// ErrTimeout is a timeout error
	ErrTimeout = errors.New("timed out")
	// ErrShutdown is an error returned when a shutdown signal was received
	ErrShutdown = errors.New("received shutdown command")
)

func alertShutdown() {
	select {
	case done <- struct{}{}:
	default: // Already a shutdown signal in queue, no need for another
	}
}

func shouldShutdown() bool {
	select {
	case <-done:
		shutdown = true
	default:
	}

	return shutdown
}

// WaitForShutdown waits for either pred to return true, a shutdown signal or timeout. An
// error is returned in either of the latter two cases.
func WaitForShutdown(pred func() bool, timeout time.Duration) error {
	timeoutChan := time.After(timeout)
	for {
		if shouldShutdown() {
			return errors.New("machines changed while waiting")
		}

		select {
		case <-timeoutChan:
			return errors.New("timed out")
		default:
			if pred() {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
}

// SSH returns an *exec.Cmd that will ssh into the given host and execute the command
// described by the command parameter.
func SSH(hostIP string, command ...string) *exec.Cmd {
	args := []string{"-o", "UserKnownHostsFile=/dev/null", "-o",
		"StrictHostKeyChecking=no", fmt.Sprintf("quilt@%s", hostIP)}
	args = append(args, command...)
	return exec.Command("ssh", args...)
}

// CreateLogFile creates a read-write-able file at the given path.
func CreateLogFile(path string) (*os.File, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.Create(path)
	} else if err != nil {
		return nil, err
	}

	return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0666)
}

// GetMachineIPs gets the public IP addresses of all the current quilt machines.
func GetMachineIPs(localClient client.Client) ([]string, []string, error) {
	machines, err := localClient.QueryMachines()
	if err != nil {
		return nil, nil, err
	}

	masters := []string{}
	workers := []string{}
	for _, m := range machines {
		if m.Role == db.Worker {
			workers = append(workers, m.PublicIP)
		} else if m.Role == db.Master {
			masters = append(masters, m.PublicIP)
		}
	}

	return masters, workers, nil
}

// GetIPMap gets a map of PublicIP to PrivateIP for each quilt machine. (TODO: Remove?)
func GetIPMap(localClient client.Client) (map[string]string, error) {
	machines, err := localClient.QueryMachines()
	if err != nil {
		return nil, err
	}

	ipMap := map[string]string{}
	for _, m := range machines {
		ipMap[m.PublicIP] = m.PrivateIP
	}

	return ipMap, nil
}

// CleanupRequest holds fields needed to properly clean up the scale tester state.
type CleanupRequest struct {
	Command     *exec.Cmd
	Namespace   string
	LocalClient client.Client
	Stop        bool
	ExitCode    int
	Message     string
	Time        time.Time
}

// Cleanup cleans up the scale tester state and reports any errors encountered.
func Cleanup(request CleanupRequest) {
	defer os.Exit(request.ExitCode)

	// Report a reasonable time if the caller forgot
	if request.Time.IsZero() {
		request.Time = time.Now()
	}

	// Post to slack if the scale tester exited with an error
	if request.ExitCode != 0 {
		log.Info("Posting error to slack")
		slackError(request)
	} else {
		slackSuccess(request)
	}

	// Attempt to use the current quilt daemon to stop the namespace, then kill the
	// quilt daemon regardless of success.
	log.Info("Cleaning up scale tester state")
	if request.Stop {
		stop(request.Namespace, request.LocalClient)
	}
	request.LocalClient.Close()
	request.Command.Process.Kill()

	// Occasionally the quilt daemon doesn't clean up the socket for some reason.
	// TODO: Investigate why
	_, defaultSocket, _ := api.ParseListenAddress(api.DefaultSocket)
	os.Remove(defaultSocket)
}

func url() string {
	return fmt.Sprintf("http://%s/%s", os.Getenv("MY_IP"), webRoot)
}

func slackSuccess(request CleanupRequest) {
	msg := fmt.Sprintf("The scale tester <%s|passed>!", url())
	slack(msg, request.Message, false)
}

func slackError(request CleanupRequest) {
	msg := fmt.Sprintf("[%s] <!channel> The scale tester encounter an <%s|error>.",
		request.Time.Format("Jan-02-2006 15:04:05"), url())
	slack(msg, request.Message, true)
}

func slack(pretext, message string, failure bool) {
	user, err := user.Current()
	if err != nil {
		log.WithError(err).Error("Failed to get current user")
		return
	}

	slackFile := filepath.Join(user.HomeDir, ".slack_hook")
	slackHook, err := ReadFile(slackFile)
	if err != nil {
		log.WithError(err).Error("Failed to load slack hook URL")
		return
	}

	slackPost := util.ToPost(true, "quilt-testing", pretext, message)
	err = util.Slack(slackHook, slackPost)
	if err != nil {
		log.WithError(err).Error("Failed to post to slack")
	}
}

// stop attempts to use the quilt client to stop the current namespace but makes no
// attempts to check that it worked. That is up to the user to check.
func stop(namespace string, localClient client.Client) {
	emptySpec := "setAdminACL([]);\n" +
		fmt.Sprintf(`setNamespace("%s");`, namespace)

	log.Infof(`Stopping namespace "%s"`, namespace)
	if err := localClient.RunStitch(emptySpec); err != nil {
		log.WithError(err).Error("Failed to stop machines")
		return
	}

	log.Info("Waiting for machines to shutdown")
	time.Sleep(time.Minute)
}

// ListenForMachineChange listens for the machines in the localClient to change. If they
// do, it signals a shutdown.
func ListenForMachineChange(localClient client.Client) {
	originalMachines, err := localClient.QueryMachines()
	if err != nil {
		log.WithError(err).Error("Failed to get db machines")
		alertShutdown()
		return
	}

	for {
		time.Sleep(time.Minute)
		currentMachines, err := localClient.QueryMachines()
		if err != nil {
			continue
		}

		if machinesChanged(originalMachines, currentMachines) {
			alertShutdown()
			return
		}
	}
}

func machinesChanged(oldMachines []db.Machine, newMachines []db.Machine) bool {
	oldSet := map[int]struct{}{}
	newSet := map[int]struct{}{}

	for _, m := range oldMachines {
		oldSet[m.ID] = struct{}{}
	}
	for _, m := range newMachines {
		newSet[m.ID] = struct{}{}
	}

	if len(oldSet) != len(newSet) {
		return true
	}

	for id := range oldSet {
		if _, ok := newSet[id]; !ok {
			return true
		}
	}

	return false
}
