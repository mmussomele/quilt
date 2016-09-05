package tools

import (
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

// CleanupError cleans up the scale tester when it fails with an error.
func CleanupError(cmd *exec.Cmd, namespace string, lc client.Client) {
	// On error, always stop namespace and exit with code 1
	cleanup(cmd, namespace, lc, true, 1)
}

// CleanupNoError cleans up the scale tester when it fails without an error.
func CleanupNoError(cmd *exec.Cmd, namespace string, lc client.Client, stop bool) {
	cleanup(cmd, namespace, lc, stop, 0)
}

func cleanup(cmd *exec.Cmd, namespace string,
	localClient client.Client, stopMachines bool, exitCode int) {

	log.Info("Cleaning up scale tester state")
	defer os.Exit(exitCode)

	// Post to slack if the scale tester exited with an error
	if exitCode != 0 {
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

		pretext := "<@mmussomele> The scale tester needs help."
		slackPost := util.ToPost(true, "quilt-testing", pretext, "")
		err = util.Slack(slackHook, slackPost)
		if err != nil {
			log.WithError(err).Error("Failed to post to slack")
		}
	}

	// Attempt to use the current quilt daemon to stop the namespace, then kill the
	// quilt daemon regardless of success.
	if stopMachines {
		stop(namespace, localClient)
	}
	localClient.Close()
	cmd.Process.Kill()

	// Occasionally the quilt daemon doesn't clean up the socket for some reason.
	// TODO: Investigate why
	_, defaultSocket, _ := api.ParseListenAddress(api.DefaultSocket)
	os.Remove(defaultSocket)
}

// stop attempts to use the quilt client to stop the current namespace but makes no
// attempts to check that it worked. That is up to the user to check.
func stop(namespace string, localClient client.Client) {
	emptySpec := "AdminACL = [];\n" +
		fmt.Sprintf(`Namespace = "%s";`, namespace)

	log.Infof(`Stopping namespace "%s"`, namespace)
	if err := localClient.RunStitch(emptySpec); err != nil {
		log.WithError(err).Error("Failed to stop machines")
		return
	}

	log.Info("Waiting for machines to shutdown")
	time.Sleep(time.Minute)
}
