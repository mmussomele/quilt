package check

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/NetSys/quilt/api/client"

	"github.com/NetSys/quilt/quilt-tester/scale"

	log "github.com/Sirupsen/logrus"
)

const (
	scaleSocket   = "unix:///tmp/quilt_scale.sock"
	prePath       = "/tmp/scale/specs/pre.spec"
	mainPath      = "/tmp/scale/specs/main.spec"
	postPath      = "/tmp/scale/specs/post.spec"
	outFile       = "/dev/null"
	logFile       = "/tmp/scale/scale-log"
	quiltLogFile  = "/tmp/scale/scale-quilt-log"
	quiltLogLevel = "debug"
)

// Used to run the scale tester for the integration tests
func Run() {
	sock := fmt.Sprintf("-H=%s", scaleSocket)
	quiltLog := fmt.Sprintf("log-file=%s", quiltLogFile)
	quiltLogLevel := fmt.Sprintf("log-level=%s", quiltLogLevel)
	cmd := exec.Command("quilt", quiltLog, quiltLogLevel, sock, "daemon")
	if err := cmd.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start quilt")
	}
	time.Sleep(time.Second) // Give quilt a while to start up

	localClient, err := client.New(scaleSocket)
	if err != nil {
		log.WithError(err).Error("Failed to get quiltctl client")
		cmd.Process.Kill()
		os.Exit(1)
	}

	params := scale.Params{
		Command:        cmd,
		LocalClient:    localClient,
		PrebootPath:    prePath,
		SpecPath:       mainPath,
		PostbootPath:   postPath,
		Namespace:      scale.Namespace,
		OutputFile:     outFile,
		LogFile:        logFile,
		QuiltLogFile:   quiltLogFile,
		AppendToOutput: false,
		IpOnly:         false,
		Debug:          true,
		Stop:           true,
	}

	scale.Run(params)
}
