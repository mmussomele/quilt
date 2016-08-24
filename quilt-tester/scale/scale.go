package scale

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/quilt-tester/scale/tools"
	testUtil "github.com/NetSys/quilt/quilt-tester/util"
	"github.com/NetSys/quilt/stitch"

	log "github.com/Sirupsen/logrus"
)

// The time limit for how long containers take to boot.
const bootLimit = time.Hour

type Params struct {
	Command        *exec.Cmd
	LocalClient    client.Client
	PrebootPath    string
	SpecPath       string
	PostbootPath   string
	Namespace      string
	OutputFile     string
	LogFile        string
	QuiltLogFile   string
	AppendToOutput bool
	IpOnly         bool
	Debug          bool
	Stop           bool
}

func Run(params Params) {
	cleanupRequest := tools.CleanupRequest{
		Command:     params.Command,
		Namespace:   params.Namespace,
		LocalClient: params.LocalClient,
		Stop:        params.Stop,
		ExitCode:    0,
		Message:     "",
	}

	results, err := runScale(params)
	if err != nil {
		log.WithError(err).Error("The scale tester exited with an error.")
		cleanupRequest.Stop = true
		cleanupRequest.ExitCode = 1
		cleanupRequest.Time = time.Now()
	}

	if err != nil || params.Debug {
		tools.SaveLogs(params.LocalClient, params.QuiltLogFile,
			params.LogFile, err != nil)
	}

	cleanupRequest.Message = fmt.Sprintf("Results: %s\nError: %v", results, err)
	tools.Cleanup(cleanupRequest)
}

func runScale(params Params) (string, error) {
	// Load specs early so that we fail early if they aren't good
	log.Info("Loading preboot stitch")
	flatPreSpec, err := tools.LoadSpec(params.PrebootPath, params.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to load spec '%s': %s", params.PrebootPath,
			err.Error())
	}

	log.Info("Loading main stitch")
	flatSpec, err := tools.LoadSpec(params.SpecPath, params.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to load spec '%s': %s", params.SpecPath,
			err.Error())
	}

	flatPostSpec := ""
	if params.PostbootPath == "" {
		log.Info("No postboot spec provided.")
	} else {
		log.Info("Loading postboot stitch")
		flatPostSpec, err = tools.LoadSpec(params.PostbootPath, params.Namespace)
		if err != nil {
			return "", fmt.Errorf("failed to load spec '%s': %s",
				params.PostbootPath, err.Error())
		}
	}

	log.Info("Running preboot stitch")
	if err := params.LocalClient.RunStitch(flatPreSpec); err != nil {
		return "", fmt.Errorf("failed to run preboot stitch: %s", err.Error())
	}

	err = testUtil.WaitFor(tools.MachinesBooted(params.LocalClient, params.IpOnly),
		10*time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to boot machines: %s", err.Error())
	}
	log.Info("Machines have booted successfully")

	_, workerIPs, err := tools.GetMachineIPs(params.LocalClient)
	if err != nil {
		return "", fmt.Errorf("failed to get worker IPs: %s", err.Error())
	}

	// Listen for the machines in the db to change. If they do, signal a shutdown.
	go tools.ListenForMachineChange(params.LocalClient)

	// When running multiple scale tester instances back to back, if we don't wait
	// for old containers to shutdown we get bad times
	log.Info("Waiting for (potential) old containers to shut down")
	err = tools.WaitForShutdown(tools.ContainersBooted(workerIPs,
		map[string]int{}), bootLimit)
	if err != nil {
		return "", fmt.Errorf("failed to shutdown containers: %s", err.Error())
	}

	mainTime, containerCount, err := timeStitch(flatSpec, "main", params.OutputFile,
		params.AppendToOutput, params.LocalClient)
	if err != nil {
		return "", fmt.Errorf("failed main boot run: [%s]", err.Error())
	}

	resultString := fmt.Sprintf("Took %v time to boot %d containers.", mainTime,
		containerCount)
	// If we aren't running a post boot timing test, just exit
	if flatPostSpec == "" {
		return resultString, nil
	}

	plusTime, containerCount, err := timeStitch(flatPostSpec, "+1", params.OutputFile,
		params.AppendToOutput, params.LocalClient)
	if err != nil {
		return resultString, fmt.Errorf("failed +1 boot run: [%s]", err.Error())
	}

	return resultString + fmt.Sprintf("\nTook %v to boot %d containers.", plusTime,
		containerCount), nil
}

func timeStitch(flatSpec, name, outputFile string,
	appendData bool, localClient client.Client) (time.Duration, int, error) {

	// Gather information on the expected state of the system from the main stitch
	log.Infof("Querying %s stitch containers", name)
	expectedContainers, err := queryStitchContainers(flatSpec)
	if err != nil {
		return 0, 0,
			fmt.Errorf("failed to query spec containers: %s", err.Error())
	}

	expCounts := map[string]int{}
	for _, c := range expectedContainers {
		expCounts[c.Image]++
	}

	log.Infof("Running %s stitch", name)
	if err := localClient.RunStitch(flatSpec); err != nil {
		return 0, 0, fmt.Errorf("failed to run spec: %s", err.Error())
	}

	utc, _ := time.LoadLocation("UTC")
	startTimestamp := time.Now().In(utc)

	_, workerIPs, err := tools.GetMachineIPs(localClient)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get machine IPs: %s", err.Error())
	}

	log.Info("Waiting for containers to boot")
	err = tools.WaitForShutdown(tools.ContainersBooted(workerIPs, expCounts),
		bootLimit)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to boot containers: %s", err.Error())
	}
	log.Info("Containers successfully booted")

	log.Info("Gathering timestamps")
	endTimestamp, err := tools.GetLastTimestamp(workerIPs, time.Hour)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to gather timestamps: %s", err.Error())
	}

	bootTime := endTimestamp.Sub(startTimestamp)
	numContainers := strconv.Itoa(len(expectedContainers))
	data := []string{numContainers, bootTime.String()}

	dataFile := fmt.Sprintf("%s-%s", outputFile, name)
	err = tools.WriteResults(dataFile, data, appendData)
	if err != nil {
		log.WithError(err).Errorf("Failed to write to output file. "+
			"Time to boot %s containers was %v.", numContainers, bootTime)
		return bootTime, len(expectedContainers), err
	}

	log.Infof("Took %v to boot %s containers.", bootTime, numContainers)
	return bootTime, len(expectedContainers), nil
}

func queryStitchContainers(spec string) ([]stitch.Container, error) {
	handle, err := stitch.New(spec, stitch.DefaultImportGetter)
	if err != nil {
		return nil, err
	}
	return handle.QueryContainers(), nil
}
