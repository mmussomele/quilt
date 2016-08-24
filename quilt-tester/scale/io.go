package main

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/NetSys/quilt/stitch"
)

func loadSpec(path, namespace string) (string, error) {
	flatSpec, err := stitch.Compile(path, stitch.DefaultImportGetter)
	if err != nil {
		return path, err
	}

	return flatSpec + fmt.Sprintf("\nNamespace = \"%s\";", namespace), nil
}

func writeResults(path string, data []string, appendToFile bool) error {
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
