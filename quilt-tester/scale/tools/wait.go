package tools

import (
	"fmt"

	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/db"

	log "github.com/Sirupsen/logrus"
)

// ContainersBooted returns a func() bool that returns true when all expected containers
// (described by expCounts) have booted.
func ContainersBooted(minionList []string, expCounts map[string]int) func() bool {
	expTotal := 0
	for _, count := range expCounts {
		expTotal += count
	}

	return func() bool {
		counts := map[string]int{}
		containers := GetContainers(minionList)
		for _, c := range containers {
			counts[c.Image]++
		}

		// Don't report progress when waiting for containers to shutdown
		if expTotal > 0 {
			foundCount := len(containers)
			percComplete := 100 * float64(foundCount) / float64(expTotal)
			fmt.Printf("Booting containers... (%.2f%%)\r", percComplete)
		}

		if len(expCounts) != len(counts) {
			return false
		}

		for image, expCount := range expCounts {
			if count, ok := counts[image]; !ok || count != expCount {
				return false
			}
		}

		return true
	}
}

// MachinesBooted returns a func() bool that returns true when all expected machines have
// connected. If ipOnly is true, it only waits for all expected machines to get public ip
// adresses.
func MachinesBooted(localClient client.Client, ipOnly bool) func() bool {
	return func() bool {
		machines, err := localClient.QueryMachines()
		if err != nil {
			log.Error("Failed to query machines.")
			return false
		}

		if len(machines) == 0 {
			return false
		}

		if !ipOnly {
			return allHaveConnected(machines)
		}
		return allHavePublicIPs(machines)
	}
}

func allHaveConnected(machines []db.Machine) bool {
	for _, m := range machines {
		if !m.Connected {
			return false
		}
	}
	return true
}

func allHavePublicIPs(machines []db.Machine) bool {
	for _, m := range machines {
		if m.PublicIP == "" {
			return false
		}
	}
	return true
}
