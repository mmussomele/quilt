package main

import (
	"fmt"
	"os"

	scale "github.com/NetSys/quilt/quilt-tester/scale/check"
	"github.com/NetSys/quilt/quilt-tester/tester"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Expected test type (normal or scale), got %v\n.",
			os.Args[1:])
		os.Exit(1)
	}

	testType := os.Args[1]
	switch testType {
	case "normal":
		tester.Run()
	case "scale":
		scale.Run()
	default:
		fmt.Printf("Unknown test type '%s', expected normal or scale", testType)
		os.Exit(1)
	}
}
