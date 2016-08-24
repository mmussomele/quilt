package tools

import (
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/stitch"

	log "github.com/Sirupsen/logrus"
)

// LoadSpec reads in the given spec and returns it with imports resolved and the given
// namespace appended.
func LoadSpec(path, namespace string) (string, error) {
	flatSpec, err := stitch.Compile(path, stitch.DefaultImportGetter)
	if err != nil {
		return path, err
	}

	return flatSpec + "\n" + fmt.Sprintf(`setNamespace("%s");`, namespace), nil
}

// ReadFile reads and returns the contents of the file located at the path given.
func ReadFile(path string) (string, error) {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(contents), nil
}

// WriteResults writes the timing results to the given path, appending optionally.
func WriteResults(path string, data []string, appendToFile bool) error {
	var fileOpenFlag = os.O_RDWR | os.O_CREATE
	if appendToFile {
		fileOpenFlag |= os.O_APPEND
	}

	outFile, err := os.OpenFile(path, fileOpenFlag, 0666)
	if err != nil {
		return err
	}
	defer outFile.Close()

	dataWriter := csv.NewWriter(outFile)
	if err := dataWriter.Write(data); err != nil {
		return err
	}

	dataWriter.Flush()
	return nil
}

// SaveLogs copies the scale tester and quilt logs to a folder named by the current date
// and time. It also copies the logs from each of the minions to that folder.
func SaveLogs(localClient client.Client, quiltLog, scaleLog string, failed bool) {
	now := time.Now().Format("Jan_02_2006-15.04.05")
	status := "Success"
	if failed {
		status = "Failed"
	}

	now = filepath.Join(webRoot, fmt.Sprintf("%s-%s", status, now))
	if err := os.MkdirAll(now, 0777); err != nil {
		log.WithError(err).Error("Failed to create log store directory")
		return
	}

	quiltLogStore := filepath.Join(now, "quilt-logs")
	if err := os.Rename(quiltLog, quiltLogStore); err != nil {
		log.WithError(err).Error("Failed to copy quilt logs")
	}

	scaleLogStore := filepath.Join(now, "scale-logs")
	if err := os.Rename(scaleLog, scaleLogStore); err != nil {
		log.WithError(err).Error("Failed to copy scale logs")
	}

	// We don't want to lose logs made after storing the logs, so change the logger
	// output location
	newScaleLogFile, err := os.OpenFile(scaleLogStore, os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		log.WithError(err).Error("Failed to open new scale log")
	} else {
		log.SetOutput(newScaleLogFile)
	}

	machines, err := localClient.QueryMachines()
	if err != nil {
		log.WithError(err).Error("Failed to get machines to save logs")
		return
	}

	for _, m := range machines {
		logFile := fmt.Sprintf("%s-%d", m.Role, m.ID)
		logStore := filepath.Join(now, logFile)

		logs, err := SSH(m.PublicIP,
			strings.Fields("docker logs minion")...).CombinedOutput()
		if err != nil {
			log.WithError(err).Errorf("Failed to get machine %d logs", m.ID)
		}

		if err := ioutil.WriteFile(logStore, logs, 0666); err != nil {
			log.WithError(err).Errorf("Failed writing machine %d logs", m.ID)
		}
	}
}
