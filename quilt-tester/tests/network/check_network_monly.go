package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var defaultContainers = map[string]struct{}{
	"ovn-controller": {},
	"swarm":          {},
	"etcd":           {},
	"ovs-vswitchd":   {},
	"ovsdb-server":   {},
	"minion":         {},
}

func main() {
	// Grab the container with master
	output, err := exec.Command("swarm", "ps", "-a").Output()
	if err != nil {
		panic(err)
	}

	containerStr := string(output)
	fmt.Println("Output of swarm ps -a:")
	fmt.Println(containerStr)

	containers := strings.Split(containerStr, "\n")

	var failed bool
	for _, line := range containers[1:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		cid := fields[len(fields)-1]
		if _, isDefault := defaultContainers[strings.Split(cid, "/")[1]]; isDefault {
			continue
		}
		if containerFailed := pingAll(cid); containerFailed {
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}

func pingAll(cid string) bool {
	fmt.Printf("Starting test on %s\n", cid)
	hosts, err := exec.Command("swarm", "exec", "-t", cid, "cat /etc/hosts").Output()
	if err != nil {
		fmt.Printf(".. FAILED, couldn't get the container's hosts: %s\n", err.Error())
		return true
	}

	var failed bool
	for _, line := range strings.Split(string(hosts), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.HasPrefix(fields[1], "ip6-") {
			continue
		}
		fmt.Printf(".. Pinging %s\n", fields[1])
		ping := exec.Command("swarm", "exec", "-t", cid,
			fmt.Sprintf("ping %s -c 5", fields[1]))
		if err := ping.Run(); err != nil {
			failed = true
			fmt.Printf(".... FAILED: %s.\n", err.Error())
		}
	}

	return failed
}
