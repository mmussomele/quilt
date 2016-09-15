package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

type scaleParams struct {
	command        *exec.Cmd
	localClient    client.Client
	prebootPath    string
	specPath       string
	postbootPath   string
	namespace      string
	outputFile     string
	appendToOutput bool
	ipOnly         bool
}

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
	debugFlag := flag.Bool("debug", false, "if given, run in debug mode")
	flag.Parse()

	if *prebootFlag == "" {
		log.Error("No preboot spec supplied.")
		usage()
	}

	if *specFlag == "" {
		log.Error("No main spec supplied")
		usage()
	}

	if *outputFlag == "" {
		log.Error("No output file specified")
		usage()
	}

	logFile, err := tools.CreateLogFile(*logFlag)
	if err != nil {
		log.WithError(err).Fatal("Failed to open log file")
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	quiltLogLevel := "info"
	if *debugFlag {
		quiltLogLevel = "debug"
	}
	quiltLogLevel = fmt.Sprintf("-log-level=%s", quiltLogLevel)
	quiltLogFile := fmt.Sprintf("-log-file=%s", *quiltLogFlag)

	log.Info("Starting the quilt daemon.")
	cmd := exec.Command("quilt", quiltLogFile, quiltLogLevel, "daemon")
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

	params := scaleParams{
		command:        cmd,
		localClient:    localClient,
		prebootPath:    *prebootFlag,
		specPath:       *specFlag,
		postbootPath:   *postbootFlag,
		namespace:      *namespaceFlag,
		outputFile:     *outputFlag,
		appendToOutput: *appendFlag,
		ipOnly:         *ipOnlyFlag,
	}

	cleanupRequest := tools.CleanupRequest{
		Command:     cmd,
		Namespace:   params.namespace,
		LocalClient: localClient,
		Stop:        !*nostopFlag,
		ExitCode:    0,
		Message:     "",
	}

	err = runScale(params)
	if err != nil {
		log.WithError(err).Error("The scale tester exited with an error.")
		cleanupRequest.Stop = true
		cleanupRequest.ExitCode = 1
		cleanupRequest.Message = err.Error()
		cleanupRequest.Time = time.Now()
	}

	if err != nil || *debugFlag {
		tools.SaveLogs(localClient, *quiltLogFlag, *logFlag, err != nil)
	}

	tools.Cleanup(cleanupRequest)
}

func runScale(params scaleParams) error {
	// Load specs early so that we fail early if they aren't good
	log.Info("Loading preboot stitch")
	flatPreSpec, err := tools.LoadSpec(params.prebootPath, params.namespace)
	if err != nil {
		return fmt.Errorf("failed to load spec '%s': %s", params.prebootPath,
			err.Error())
	}

	log.Info("Loading main stitch")
	flatSpec, err := tools.LoadSpec(params.specPath, params.namespace)
	if err != nil {
		return fmt.Errorf("failed to load spec '%s': %s", params.specPath,
			err.Error())
	}

	flatPostSpec := ""
	if params.postbootPath == "" {
		log.Info("No postboot spec provided.")
	} else {
		log.Info("Loading postboot stitch")
		flatPostSpec, err = tools.LoadSpec(params.postbootPath, params.namespace)
		if err != nil {
			return fmt.Errorf("failed to load spec '%s': %s",
				params.postbootPath, err.Error())
		}
	}

	log.Info("Running preboot stitch")
	if err := params.localClient.RunStitch(flatPreSpec); err != nil {
		return fmt.Errorf("failed to run preboot stitch: %s", err.Error())
	}

	err = testUtil.WaitFor(tools.MachinesBooted(params.localClient, params.ipOnly),
		10*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to boot machines: %s", err.Error())
	}
	log.Info("Machines have booted successfully")

	_, workerIPs, err := tools.GetMachineIPs(params.localClient)
	if err != nil {
		return fmt.Errorf("failed to get worker IPs: %s", err.Error())
	}

	// Listen for the machines in the db to change. If they do, signal a shutdown.
	go tools.ListenForMachineChange(params.localClient)

	// When running multiple scale tester instances back to back, if we don't wait
	// for old containers to shutdown we get bad times
	log.Info("Waiting for (potential) old containers to shut down")
	err = tools.WaitForShutdown(tools.ContainersBooted(workerIPs,
		map[string]int{}), bootLimit)
	if err != nil {
		return fmt.Errorf("failed to shutdown containers: %s", err.Error())
	}

	err = timeStitch(flatSpec, "main", params.outputFile,
		params.appendToOutput, params.localClient)
	if err != nil {
		return fmt.Errorf("failed main boot run: [%s]", err.Error())
	}

	// If we aren't running a post boot timing test, just exit
	if flatPostSpec == "" {
		return nil
	}

	err = timeStitch(flatPostSpec, "+1", params.outputFile,
		params.appendToOutput, params.localClient)
	if err != nil {
		return fmt.Errorf("failed +1 boot run: [%s]", err.Error())
	}

	return nil
}

func timeStitch(flatSpec, name, outputFile string,
	appendData bool, localClient client.Client) error {

	// Gather information on the expected state of the system from the main stitch
	log.Infof("Querying %s stitch containers", name)
	expectedContainers, err := queryStitchContainers(flatSpec)
	if err != nil {
		return fmt.Errorf("failed to query spec containers: %s", err.Error())
	}

	expCounts := map[string]int{}
	for _, c := range expectedContainers {
		expCounts[c.Image]++
	}

	log.Infof("Running %s stitch", name)
	if err := localClient.RunStitch(flatSpec); err != nil {
		return fmt.Errorf("failed to run spec: %s", err.Error())
	}

	utc, _ := time.LoadLocation("UTC")
	startTimestamp := time.Now().In(utc)

	_, workerIPs, err := tools.GetMachineIPs(localClient)
	if err != nil {
		return fmt.Errorf("failed to get machine IPs: %s", err.Error())
	}

	log.Info("Waiting for containers to boot")
	err = tools.WaitForShutdown(tools.ContainersBooted(workerIPs, expCounts),
		bootLimit)
	if err != nil {
		return fmt.Errorf("failed to boot containers: %s", err.Error())
	}
	log.Info("Containers successfully booted")

	log.Info("Gathering timestamps")
	endTimestamp, err := tools.GetLastTimestamp(workerIPs, time.Hour)
	if err != nil {
		return fmt.Errorf("failed to gather timestamps: %s", err.Error())
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
