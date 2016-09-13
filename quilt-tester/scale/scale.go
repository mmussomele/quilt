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
	"github.com/NetSys/quilt/quilt-tester/scale/tools"
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

	logFile, err := tools.CreateLogFile(logOutputPath)
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

	cleanupReq := tools.CleanupRequest{
		Command:     cmd,
		Namespace:   namespace,
		LocalClient: localClient,
		Stop:        true,
		ExitCode:    1,
		Message:     "timed out",
	}

	// Cleanup the scale tester if we're interrupted.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGHUP)
	go func(c chan os.Signal) {
		sig := <-c
		fmt.Printf("Caught signal %s: shutting down.\n", sig)
		cleanReq := cleanupReq
		cleanReq.ExitCode = 0
		cleanReq.Message = "keyboard interrupt"
		tools.Cleanup(cleanReq)
	}(sigc)

	// Load specs early so that we fail early if they aren't good
	log.Info("Loading preboot stitch")
	flatPreSpec, err := tools.LoadSpec(prebootPath, namespace)
	if err != nil {
		log.WithError(err).WithField("spec", prebootPath).Error("Failed to load")
		cmd.Process.Kill()
		os.Exit(1)
	}

	log.Info("Loading main stitch")
	flatSpec, err := tools.LoadSpec(specPath, namespace)
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
		flatPostSpec, err = tools.LoadSpec(postbootPath, namespace)
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
	defer tools.Cleanup(cleanupReq)

	err = testUtil.WaitFor(tools.MachinesBooted(localClient, ipOnly), 10*time.Minute)
	if err != nil {
		log.WithError(err).Error("Failed to boot machines")
		return
	}
	log.Info("Machines have booted successfully")

	_, workerIPs, err := tools.GetMachineIPs(localClient)
	if err != nil {
		log.WithError(err).Error("Failed to get worker IPs")
		return
	}

	// Listen for the machines in the db to change. If they do, signal a shutdown.
	go tools.ListenForMachineChange(localClient)

	// When running multiple scale tester instances back to back, if we don't wait
	// for old containers to shutdown we get bad times
	log.Info("Waiting for (potential) old containers to shut down")
	err = tools.WaitForShutdown(tools.ContainersBooted(workerIPs,
		map[string]int{}), bootLimit)
	if err != nil {
		log.WithError(err).Error("Containers took too long to shut down")
		return
	}

	err = timeStitch(flatSpec, "main", outputFile,
		*appendFlag, localClient, cleanupReq)
	if err != nil {
		if err == tools.ErrShutdown {
			cleanupShutdown(err, cleanupReq)
		}
		return
	}

	cleanupReq.ExitCode = 0
	cleanupReq.Message = "exiting normally"
	cleanupReq.Stop = stopMachines

	// If we aren't running a post boot timing test, just exit
	if flatPostSpec == "" {
		tools.Cleanup(cleanupReq)
	}

	err = timeStitch(flatPostSpec, "+1", outputFile,
		*appendFlag, localClient, cleanupReq)
	if err != nil {
		if err == tools.ErrShutdown {
			cleanupShutdown(err, cleanupReq)
		}
		return
	}

	tools.Cleanup(cleanupReq)
}

func timeStitch(flatSpec, name, outputFile string,
	appendData bool, localClient client.Client,
	cleanReq tools.CleanupRequest) error {

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

	_, workerIPs, err := tools.GetMachineIPs(localClient)
	if err != nil {
		log.WithError(err).Error("Failed to get worker IPs")
		return err
	}

	log.Info("Waiting for containers to boot")
	err = tools.WaitForShutdown(tools.ContainersBooted(workerIPs, expCounts),
		bootLimit)
	if err != nil {
		log.WithError(err).Error("Scale testing timed out")
		return err
	}
	log.Info("Containers successfully booted")

	log.Info("Gathering timestamps")
	endTimestamp, err := tools.GetLastTimestamp(workerIPs, time.Hour)
	if err != nil {
		log.WithError(err).Error("Fail to collect timestamps")
		return err
	}

	bootTime := endTimestamp.Sub(startTimestamp)
	numContainers := strconv.Itoa(len(expectedContainers))
	data := []string{numContainers, bootTime.String()}

	dataFile := fmt.Sprintf("%s-%s", outputFile, name)
	err = tools.WriteResults(dataFile, data, appendData)
	if err != nil {
		log.WithError(err).Errorf("Failed to write to output file. "+
			"Time to boot %s containers was %v.", numContainers, bootTime)
		return err
	}

	log.Infof("Took %v to boot %s containers.", bootTime, numContainers)
	return nil
}

func cleanupShutdown(err error, cleanReq tools.CleanupRequest) {
	cleanReq.ExitCode = 2
	cleanReq.Message = err.Error()
	cleanReq.Stop = true
	tools.Cleanup(cleanReq)
}

func queryStitchContainers(spec string) ([]stitch.Container, error) {
	handle, err := stitch.New(spec, stitch.DefaultImportGetter)
	if err != nil {
		return nil, err
	}
	return handle.QueryContainers(), nil
}

func usage() {
	flag.Usage()
	os.Exit(1)
}
