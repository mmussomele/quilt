package tools

import (
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/NetSys/quilt/stitch"
)

// LoadSpec reads in the given spec and returns it with imports resolved and the given
// namespace appended.
func LoadSpec(path, namespace string) (string, error) {
	flatSpec, err := stitch.Compile(path, stitch.DefaultImportGetter)
	if err != nil {
		return path, err
	}

	return flatSpec + fmt.Sprintf("\nNamespace = \"%s\";", namespace), nil
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
