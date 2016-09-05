package tools

import (
	"bytes"
	"fmt"
	"regexp"
	"sync"
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
	IP    string
	Image string
	Name  string
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

func parseContainer(container []byte) (ScaleContainer, error) {
	containersRegex := `[a-f0-9]+\s+(\S+)\s+".+"\s+(?:(?:\w+\s)+\s+){2}\s+([\w\-]+)`
	containerMatch := regexp.MustCompile(containersRegex)

	groups := containerMatch.FindSubmatch(container)
	if len(groups) != 3 {
		return ScaleContainer{}, fmt.Errorf("malformed container: %s", container)
	}

	image, name := string(groups[1]), string(groups[2])
	if _, ok := defaultContainers[name]; ok {
		return ScaleContainer{}, fmt.Errorf("default container: %s", container)
	}

	return ScaleContainer{Image: image, Name: name}, nil
}
