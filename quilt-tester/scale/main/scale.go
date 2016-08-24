package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/quilt-tester/scale"
	"github.com/NetSys/quilt/quilt-tester/scale/tools"
	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"
)

const (
	// Reserved namespace for the scale tester. If you are developing the scale tester,
	// use the `-namespace=<your_namespace>` command line flag to ensure you don't interfere
	// with the tester.
	// We want the namespace to be deterministic so the user can use `quilt stop` to halt the
	// namespace if the scale tester fails for some reason.
	scaleNamespace = "scale-bd89e4c89f4d384e7afb155a3af99d8a6f4f5a06a9fecf0b6d220eb66e"

	prebootSpec  = "pre.spec"
	mainSpec     = "main.spec"
	postbootSpec = "post.spec"
)

func main() {
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(util.Formatter{})
	flag.Usage = func() {
		flag.PrintDefaults()
		fmt.Println("\npreboot-stitch, stich and out-file are required flags.")
	}

	specFolderFlag := flag.String("test-files", "", "folder containing test specs")
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

	if *specFolderFlag == "" {
		log.Error("No spec folder supplied.")
		usage()
	}

	// We don't care why it failed (i.e. file not found), just that it can't be read
	prePath := filepath.Join(*specFolderFlag, prebootSpec)
	if _, err := os.Stat(prePath); err != nil {
		log.WithError(err).Error("Failed to load pre spec")
		usage()
	}

	mainPath := filepath.Join(*specFolderFlag, mainSpec)
	if _, err := os.Stat(mainPath); err != nil {
		log.WithError(err).Error("Failed to load main spec")
		usage()
	}

	postPath := filepath.Join(*specFolderFlag, postbootSpec)
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		log.Info("No post spec given. Post test will be skipped.")
		postPath = ""
	} else if err != nil {
		log.WithError(err).Error("Failed to load post spec")
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

	params := scale.Params{
		Command:        cmd,
		LocalClient:    localClient,
		PrebootPath:    prePath,
		SpecPath:       mainPath,
		PostbootPath:   postPath,
		Namespace:      *namespaceFlag,
		OutputFile:     *outputFlag,
		LogFile:        *logFlag,
		QuiltLogFile:   *quiltLogFlag,
		AppendToOutput: *appendFlag,
		IpOnly:         *ipOnlyFlag,
		Debug:          *debugFlag,
		Stop:           !*nostopFlag,
	}

	scale.Run(params)
}

func usage() {
	flag.Usage()
	os.Exit(1)
}
