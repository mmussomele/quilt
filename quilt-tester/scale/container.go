package main

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

type scaleContainer struct {
	ip    string
	image string
	name  string
}

func getContainers(minionList []string) []scaleContainer {
	channels := []chan scaleContainer{}
	for _, minion := range minionList {
		channels = append(channels, queryContainers(minion))
	}

	out := mergeContainers(channels)
	containers := []scaleContainer{}
	for container := range out {
		containers = append(containers, container)
	}

	return containers
}

func queryContainers(host string) chan scaleContainer {
	args := []string{"docker", "ps", "-a"}
	out := make(chan scaleContainer)
	go func() {
		defer close(out)
		output, err := ssh(host, args...).Output()
		if err != nil {
			return
		}

		containers := bytes.Split(output, []byte{'\n'})
		for _, cont := range containers {
			container, err := parseContainer(cont)
			if err != nil {
				continue
			}

			container.ip = host
			out <- container
		}
	}()

	return out
}

func mergeContainers(channels []chan scaleContainer) chan scaleContainer {
	var wg sync.WaitGroup
	out := make(chan scaleContainer)

	wg.Add(len(channels))
	go func() {
		wg.Wait()
		close(out)
	}()

	collect := func(vals chan scaleContainer) {
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

func parseContainer(container []byte) (scaleContainer, error) {
	containersRegex := `[a-f0-9]+\s+(\S+)\s+".+"\s+(?:(?:\w+\s)+\s+){2}\s+([\w\-]+)`
	containerMatch := regexp.MustCompile(containersRegex)

	groups := containerMatch.FindSubmatch(container)
	if len(groups) != 3 {
		return scaleContainer{}, fmt.Errorf("malformed container: %s", container)
	}

	image, name := string(groups[1]), string(groups[2])
	if _, ok := defaultContainers[name]; ok {
		return scaleContainer{}, fmt.Errorf("default container: %s", container)
	}

	return scaleContainer{image: image, name: name}, nil
}
