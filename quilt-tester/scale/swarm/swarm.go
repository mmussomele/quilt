package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/quilt-tester/scale/tools"
	testUtil "github.com/NetSys/quilt/quilt-tester/util"
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
	}

	// TODO: After the paper deadline, change this so you just point to a folder that
	// is assumed to have certain specs inside of it, with an optional postboot spec
	prebootFlag := flag.String("preboot-stitch", "", "spec to boot machines only")
	containerFlag := flag.Int("containers", -1, "how many containers to boot")
	imageFlag := flag.String("image", "", "the container image to boot")

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
	numContainers := *containerFlag
	image := *imageFlag

	outputFile := *outputFlag
	logOutputPath := *logFlag
	stopMachines := !*nostopFlag
	ipOnly := *ipOnlyFlag

	if outputFile == "" {
		log.Error("No output file specified")
		usage()
	}

	if prebootPath == "" {
		log.Error("No preboot spec supplied.")
		usage()
	}

	if image == "" {
		log.Error("No image supplied")
		usage()
	}

	if numContainers == -1 {
		log.Error("No container count specified")
		usage()
	}

	if numContainers < 1 {
		log.Error("Container count must be positive")
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

	log.Info("Running preboot stitch")
	if err := localClient.RunStitch(flatPreSpec); err != nil {
		log.WithError(err).Error("Failed to run preboot stitch")
		cmd.Process.Kill()
		os.Exit(1)
	}
	defer tools.Cleanup(cleanupReq)

	err = testUtil.WaitFor(tools.MachinesBooted(localClient, ipOnly), 20*time.Minute)
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
	err = tools.WaitForShutdown(cleanupReq, tools.ContainersBooted(workerIPs,
		map[string]int{}), bootLimit)
	if err != nil {
		log.WithError(err).Error("Containers took too long to shut down")
		return
	}

	err = swarmBoot(numContainers, image, "main",
		outputFile, *appendFlag, localClient, cleanupReq)
	if err != nil {
		if err == tools.ErrShutdown {
			cleanupShutdown(err, cleanupReq)
		}
		return
	}

	err = swarmBoot(numContainers+1, image, "+1",
		outputFile, *appendFlag, localClient, cleanupReq)
	if err != nil {
		if err == tools.ErrShutdown {
			cleanupShutdown(err, cleanupReq)
		}
		return
	}

	err = swarmKill(localClient)
	if err != nil {
		return
	}

	cleanupReq.ExitCode = 0
	cleanupReq.Message = "exiting normally"
	cleanupReq.Stop = stopMachines
	tools.Cleanup(cleanupReq)
}

func swarmBoot(containers int, image, name,
	outputFile string, appendData bool,
	localClient client.Client, cleanupReq tools.CleanupRequest) error {

	// Gather information on the expected state of the system from the main stitch
	expCounts := map[string]int{image: containers}
	numContainers := containers
	if name == "+1" {
		containers = 1
	}

	masterIPs, workerIPs, err := tools.GetMachineIPs(localClient)
	if err != nil {
		log.WithError(err).Error("Failed to get worker IPs")
		return err
	}

	if len(masterIPs) != 1 {
		return fmt.Errorf("expected 1 master, found %d", len(masterIPs))
	}

	// TODO: Chunk this by every 100 containers to reduce the parallel load on the
	// master when booting
	masterIP := masterIPs[0]
	swarmRun := `swarm run -d -e 'PURE_SWARM_MODE=1' --net=host %s`
	swarmCmd := fmt.Sprintf(swarmRun, image)

	utc, _ := time.LoadLocation("UTC")
	startTimestamp := time.Now().In(utc)

	log.Infof("Booting %d containers of image '%s'", containers, image)
	for containers > 0 {
		bootContainers := 100
		if containers < bootContainers {
			bootContainers = containers
		}

		containers -= bootContainers
		log.Infof("Telling swarm to boot %d containers.", bootContainers)
		bootCmd := fmt.Sprintf("for _ in {0..%d}; do %s & done",
			bootContainers-1, swarmCmd)
		_, err = tools.SSH(masterIP, strings.Fields(bootCmd)...).Output()
		if err != nil {
			log.WithError(err).Error("Failed to signal swarm to boot " +
				"containers")
			return err
		}
	}

	log.Info("Waiting for containers to boot")
	err = tools.WaitForShutdown(cleanupReq, tools.ContainersBooted(workerIPs,
		expCounts), bootLimit)
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
	data := []string{strconv.Itoa(numContainers), bootTime.String()}

	dataFile := fmt.Sprintf("%s-%s", outputFile, name)
	err = tools.WriteResults(dataFile, data, appendData)
	if err != nil {
		log.WithError(err).Errorf("Failed to write to output file. "+
			"Time to boot %d containers was %v.", numContainers, bootTime)
		return err
	}

	log.Infof("Took %v to boot %d containers.", bootTime, numContainers)
	return nil
}

func cleanupShutdown(err error, cleanReq tools.CleanupRequest) {
	cleanReq.ExitCode = 2
	cleanReq.Message = err.Error()
	cleanReq.Stop = true
	tools.Cleanup(cleanReq)
}

func swarmKill(localClient client.Client) error {
	log.Info("Tearing down swarm containers")
	masterIPs, workerIPs, err := tools.GetMachineIPs(localClient)
	if err != nil {
		return err
	}

	if len(masterIPs) != 1 {
		return fmt.Errorf("expected 1 master, found %d", len(masterIPs))
	}
	masterIP := masterIPs[0]

	containers := tools.GetContainers(workerIPs)
	publicPrivate, err := tools.GetIPMap(localClient)
	if err != nil {
		return err
	}

	joinCmdTemplate := `%s ; %s`
	killContainerCmd := `swarm rm -f ip-%s/%s`
	command := ""
	for _, container := range containers {
		// to teardown containers with swarm, we need their private IPs
		privateIP := publicPrivate[container.IP]
		privateIP = strings.Replace(privateIP, ".", "-", -1)
		killCmd := fmt.Sprintf(killContainerCmd, privateIP, container.Name)
		if command == "" {
			command = killCmd
		} else {
			command = fmt.Sprintf(joinCmdTemplate, command, killCmd)
		}
	}

	_, err = tools.SSH(masterIP, strings.Fields(command)...).Output()
	if err != nil {
		return err
	}

	return nil
}

func usage() {
	flag.Usage()
	os.Exit(1)
}
