package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/db"
	testUtil "github.com/NetSys/quilt/quilt-tester/util"
	"github.com/NetSys/quilt/stitch"
	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"
)

// Reserved namespace for the scale tester. If you are developing the scale tester,
// use the `-namespace=<your_namespace>` command line flag to ensure you don't interfere
// with the tester.
// We want the namespace to be deterministic so the user can use `quilt stop` to halt the
// namespace if the scale tester fails for some reason.
const scaleNamespace = "scale-bd89e4c89f4d384e7afb155a3af99d8a6f4f5a06a9fecf0b6d220eb66e"

// The time limit for how long containers take to boot.
const bootLimit = time.Hour

func main() {
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(util.Formatter{})
	flag.Usage = func() {
		flag.PrintDefaults()
		fmt.Println("\npreboot-stitch, stich and out-file are required flags.")
	}

	// TODO: After the paper deadline, change this so you just point to a folder that
	// is assumed to have certain specs inside of it, with an optional postboot spec
	prebootFlag := flag.String("preboot-stitch", "", "spec to boot machines only")
	specFlag := flag.String("stitch", "", "spec to boot containers after machines")
	postbootFlag := flag.String("postboot-stitch", "", "spec to boot n+1th")

	// Only used to override the default namespace when developing on the scale
	// tester.
	namespaceFlag := flag.String("namespace", scaleNamespace,
		"namespace to run scale tests on")
	outputFlag := flag.String("out-file", "", "output file")
	logFlag := flag.String("log-file", "/dev/null", "log file")
	quiltLogFlag := flag.String("quilt-log-file", "/dev/null", "quilt log file")
	appendFlag := flag.Bool("append", false, "if given, append to output file")
	nostopFlag := flag.Bool("nostop", false, "if given, don't try to stop machines")
	ipOnlyFlag := flag.Bool("ip-only", false, "if given, only wait for machines to"+
		" get public IPs")
	flag.Parse()

	namespace := *namespaceFlag
	prebootPath := *prebootFlag
	specPath := *specFlag
	postbootPath := *postbootFlag
	outputFile := *outputFlag
	logOutputPath := *logFlag
	stopMachines := !*nostopFlag
	ipOnly := *ipOnlyFlag

	if prebootPath == "" {
		log.Error("No preboot spec supplied.")
		usage()
	}

	if specPath == "" {
		log.Error("No main spec supplied")
		usage()
	}

	if outputFile == "" {
		log.Error("No output file specified")
		usage()
	}

	logFile, err := createLogFile(logOutputPath)
	if err != nil {
		log.WithError(err).Fatal("Failed to open log file")
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Info("Starting the quilt daemon.")
	quiltLogFile := fmt.Sprintf("-log-file=%s", *quiltLogFlag)
	cmd := exec.Command("quilt", quiltLogFile, "daemon")
	if err := cmd.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start quilt")
	}
	time.Sleep(5 * time.Second) // Give quilt a while to start up

	log.Info("Getting local client")
	localClient, err := client.New(api.DefaultSocket)
	if err != nil {
		log.WithError(err).Error("Failed to get quiltctl client")
		cmd.Process.Kill()
		os.Exit(1)
	}

	// Cleanup the scale tester if we're interrupted.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGHUP)
	go func(c chan os.Signal) {
		sig := <-c
		fmt.Printf("Caught signal %s: shutting down.\n", sig)
		cleanupNoError(cmd, namespace, localClient, true)
		os.Exit(0)
	}(sigc)

	// Load specs early so that we fail early if they aren't good
	log.Info("Loading preboot stitch")
	flatPreSpec, err := loadSpec(prebootPath, namespace)
	if err != nil {
		log.WithError(err).WithField("spec", prebootPath).Error("Failed to load")
		cmd.Process.Kill()
		os.Exit(1)
	}

	log.Info("Loading main stitch")
	flatSpec, err := loadSpec(specPath, namespace)
	if err != nil {
		log.WithError(err).WithField("spec", specPath).Error("Failed to load")
		cmd.Process.Kill()
		os.Exit(1)
	}

	flatPostSpec := ""
	if postbootPath == "" {
		log.Info("No postboot spec provided.")
	} else {
		log.Info("Loading postboot stitch")
		flatPostSpec, err = loadSpec(postbootPath, namespace)
		if err != nil {
			log.WithError(err).WithField("spec",
				postbootPath).Error("Failed to load")
			cmd.Process.Kill()
			os.Exit(1)
		}
	}

	log.Info("Running preboot stitch")
	if err := localClient.RunStitch(flatPreSpec); err != nil {
		log.WithError(err).Error("Failed to run preboot stitch")
		cmd.Process.Kill()
		os.Exit(1)
	}
	defer cleanupError(cmd, namespace, localClient)

	err = testUtil.WaitFor(machinesBooted(localClient, ipOnly), 10*time.Minute)
	if err != nil {
		log.WithError(err).Error("Failed to boot machines")
		return
	}
	log.Info("Machines have booted successfully")

	workerIPs, err := getWorkerIPs(localClient)
	if err != nil {
		log.WithError(err).Error("Failed to get worker IPs")
		return
	}

	// When running multiple scale tester instances back to back, if we don't wait
	// for old containers to shutdown we get bad times
	log.Info("Waiting for (potential) old containers to shut down")
	err = testUtil.WaitFor(containersBooted(workerIPs, map[string]int{}), bootLimit)
	if err != nil {
		log.WithError(err).Error("Containers took too long to shut down")
		return
	}

	err = timeStitch(flatSpec, "main", outputFile, *appendFlag, localClient)
	if err != nil {
		return
	}

	// If we aren't running a post boot timing test, just exit
	if flatPostSpec == "" {
		cleanupNoError(cmd, namespace, localClient, stopMachines)
	}

	err = timeStitch(flatPostSpec, "+1", outputFile, *appendFlag, localClient)
	if err != nil {
		return
	}

	cleanupNoError(cmd, namespace, localClient, stopMachines)
}

func timeStitch(flatSpec, name, outputFile string,
	appendData bool, localClient client.Client) error {

	// Gather information on the expected state of the system from the main stitch
	log.Infof("Querying %s stitch containers", name)
	expectedContainers, err := queryStitchContainers(flatSpec)
	if err != nil {
		log.WithError(err).WithField("spec", flatSpec).Error("Failed to get " +
			"expected containers from spec")
		return err
	}

	expCounts := map[string]int{}
	for _, c := range expectedContainers {
		expCounts[c.Image]++
	}

	log.Infof("Running %s stitch", name)
	if err := localClient.RunStitch(flatSpec); err != nil {
		log.WithError(err).Error("Failed to run stitch")
		return err
	}

	utc, _ := time.LoadLocation("UTC")
	startTimestamp := time.Now().In(utc)

	workerIPs, err := getWorkerIPs(localClient)
	if err != nil {
		log.WithError(err).Error("Failed to get worker IPs")
		return err
	}

	log.Info("Waiting for containers to boot")
	err = testUtil.WaitFor(containersBooted(workerIPs, expCounts), bootLimit)
	if err != nil {
		log.WithError(err).Error("Scale testing timed out")
		return err
	}
	log.Info("Containers successfully booted")

	log.Info("Gathering timestamps")
	endTimestamp := getLastTimestamp(workerIPs)

	bootTime := endTimestamp.Sub(startTimestamp)
	numContainers := strconv.Itoa(len(expectedContainers))
	data := []string{numContainers, bootTime.String()}

	dataFile := fmt.Sprintf("%s-%s", outputFile, name)
	err = writeResults(dataFile, data, appendData)
	if err != nil {
		log.WithError(err).Errorf("Failed to write to output file. "+
			"Time to boot %s containers was %v.", numContainers, bootTime)
		return err
	}

	log.Infof("Took %v to boot %s containers.", bootTime, numContainers)
	return nil
}

func createLogFile(path string) (*os.File, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.Create(path)
	} else if err != nil {
		return nil, err
	}

	return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0666)
}

func queryStitchContainers(spec string) ([]stitch.Container, error) {
	handle, err := stitch.New(spec, stitch.DefaultImportGetter)
	if err != nil {
		return nil, err
	}
	return handle.QueryContainers(), nil
}

func getWorkerIPs(localClient client.Client) ([]string, error) {
	machines, err := localClient.QueryMachines()
	if err != nil {
		return nil, err
	}

	workers := []string{}
	for _, m := range machines {
		if m.Role == db.Worker {
			workers = append(workers, m.PublicIP)
		}
	}

	return workers, nil
}

func cleanupError(cmd *exec.Cmd, namespace string, lc client.Client) {
	// On error, always stop namespace and exit with code 1
	cleanup(cmd, namespace, lc, true, 1)
}

func cleanupNoError(cmd *exec.Cmd, namespace string, lc client.Client, stop bool) {
	cleanup(cmd, namespace, lc, stop, 0)
}

func cleanup(cmd *exec.Cmd, namespace string,
	localClient client.Client, stopMachines bool, exitCode int) {

	log.Info("Cleaning up scale tester state")
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
	os.Exit(exitCode)
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

func usage() {
	flag.Usage()
	os.Exit(1)
}
