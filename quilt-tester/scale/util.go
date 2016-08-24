package main

import (
	"fmt"
	"os/exec"
)

func ssh(hostIP string, command ...string) *exec.Cmd {
	args := []string{"-o", "UserKnownHostsFile=/dev/null", "-o",
		"StrictHostKeyChecking=no", fmt.Sprintf("quilt@%s", hostIP)}
	args = append(args, command...)
	return exec.Command("ssh", args...)
}
