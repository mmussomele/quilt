package tools

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
)

var defaultContainers = map[string]struct{}{
	"ovn-controller": {},
	"swarm":          {},
	"etcd":           {},
	"ovs-vswitchd":   {},
	"ovsdb-server":   {},
	"minion":         {},
}

// ScaleContainer contains the IP, Image and Name of a container.
type ScaleContainer struct {
	IP      string
	Image   string
	Name    string
	Started bool
}

// GetContainers fetches all of the current non-quilt containers from the workers.
func GetContainers(minionList []string) []ScaleContainer {
	channels := []chan ScaleContainer{}
	for _, minion := range minionList {
		channels = append(channels, queryContainers(minion))
	}

	out := mergeContainers(channels)
	containers := []ScaleContainer{}
	for container := range out {
		containers = append(containers, container)
	}

	return containers
}

func queryContainers(host string) chan ScaleContainer {
	args := []string{"docker", "ps", "-a"}
	out := make(chan ScaleContainer)
	go func() {
		defer close(out)
		output, err := SSH(host, args...).Output()
		if err != nil {
			return
		}

		containers := bytes.Split(output, []byte{'\n'})
		for _, cont := range containers {
			container, err := parseContainer(cont)
			if err != nil {
				continue
			}

			container.IP = host
			out <- container
		}
	}()

	return out
}

func mergeContainers(channels []chan ScaleContainer) chan ScaleContainer {
	var wg sync.WaitGroup
	out := make(chan ScaleContainer)

	wg.Add(len(channels))
	go func() {
		wg.Wait()
		close(out)
	}()

	collect := func(vals chan ScaleContainer) {
		for val := range vals {
			out <- val
		}
		wg.Done()
	}

	for _, c := range channels {
		go collect(c)
	}

	return out
}

// KillContainers kills all non-quilt containers on the given minions.
func KillContainers(minionList []string) error {
	dockerRM := `docker rm -f %s`
	containers := GetContainers(minionList)
	removeCommands := map[string][]string{}
	for _, container := range containers {
		removeCommands[container.IP] = append(removeCommands[container.IP],
			fmt.Sprintf(dockerRM, container.Name))
	}

	channels := []chan error{}
	for host, cmds := range removeCommands {
		channels = append(channels, killContainers(host, cmds))
	}

	out := mergeErrors(channels)
	lastErr := error(nil)
	for err := range out {
		if err != nil {
			log.WithError(err).Error("Failed to remove containers.")
			lastErr = err
		}
	}

	return lastErr
}

func killContainers(host string, cmds []string) chan error {
	out := make(chan error)
	log.Infof("Tearing down containers on host quilt@%s", host)
	go func() {
		defer close(out)
		for _, cmd := range cmds {
			out <- SSH(host, strings.Fields(cmd)...).Run()
		}
	}()

	return out
}

func mergeErrors(channels []chan error) chan error {
	var wg sync.WaitGroup
	out := make(chan error)

	wg.Add(len(channels))
	go func() {
		wg.Wait()
		close(out)
	}()

	collect := func(vals chan error) {
		for val := range vals {
			out <- val
		}
		wg.Done()
	}

	for _, c := range channels {
		go collect(c)
	}

	return out
}

func parseContainer(container []byte) (ScaleContainer, error) {
	containersRegex := `[a-f0-9]+\s+(\S+)\s+".+"\s+(?:\w+\s)+\s+` +
		`((?:\w+\s)+\s+)\s+([\w\-]+)`
	containerMatch := regexp.MustCompile(containersRegex)

	groups := containerMatch.FindSubmatch(container)
	if len(groups) != 4 {
		return ScaleContainer{}, fmt.Errorf("malformed container: %s", container)
	}

	image, status, name := string(groups[1]), string(groups[2]), string(groups[3])
	if _, ok := defaultContainers[name]; ok {
		return ScaleContainer{}, fmt.Errorf("default container: %s", container)
	}

	started := strings.HasPrefix(status, "Up ")
	return ScaleContainer{Image: image, Name: name, Started: started}, nil
}
