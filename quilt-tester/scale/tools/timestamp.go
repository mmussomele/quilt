package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

// GetLastTimestamp gets the latest timestamp to be output by a non-quilt container.
func GetLastTimestamp(workers []string, timeLimit time.Duration) (time.Time, error) {
	timestamp := make(chan time.Time)
	done := make(chan struct{})
	go collectTimestamps(workers, timestamp, done)
	timeout := time.After(timeLimit)
	for {
		if shouldShutdown() {
			done <- struct{}{}
			return time.Time{}, ErrShutdown
		}

		select {
		case time := <-timestamp:
			return time, nil
		case <-timeout:
			done <- struct{}{}
			return time.Time{}, ErrTimeout
		default:
		}

		time.Sleep(time.Second)
	}
}

func collectTimestamps(workers []string, output chan time.Time, done chan struct{}) {
	// Everything is after the zero time
	latestTimestamp := time.Time{}
	containers := GetContainers(workers)

	containerCount := 0
	hostMap := map[string][]string{}
	for _, container := range containers {
		// Sometimes, containers show up in `docker ps` but don't start. This
		// skips those containers when collecting timestamps
		if !container.Started {
			continue
		}
		hostMap[container.IP] = append(hostMap[container.IP], container.Name)
		containerCount++
	}

	channels := []chan time.Time{}
	for ip, containers := range hostMap {
		channels = append(channels, queryTimestamps(ip, containers))
	}

	out := mergeTimestamps(channels)

	timestampCount := 0
	fmt.Printf("Collecting timestamps... (0.00%s)\r", "%") // go vet is dumb
Loop:
	for {
		select {
		case <-done:
			return
		case ts := <-out:
			// Since waiting for timestamps looks the same as a closed
			// channel from a blocking perspective, we need an explicit
			// signal that we're done.
			if ts.IsZero() {
				break Loop
			}

			if ts.After(latestTimestamp) {
				latestTimestamp = ts
			}

			timestampCount++
			percComplete := float64(timestampCount) / float64(containerCount)
			fmt.Printf("Collecting timestamps... (%.2f%%)\r",
				100*percComplete)
		}
	}
	fmt.Print("                                  \r")
	output <- latestTimestamp
}

// When querying timestamps, we can only have one ssh connection to each host at a time,
// so we can parallelize across multiple hosts, but have to serialize for each host.
func queryTimestamps(minion string, containers []string) chan time.Time {
	out := make(chan time.Time)
	timestampRegex := regexp.MustCompile(`quilt_timestamp_unix=(\d+)\n`)
	cmdTemplate := `docker logs %s`

	go func() {
		defer close(out)

		collected := map[string]struct{}{}
		for len(collected) < len(containers) {
			for _, container := range containers {
				if _, ok := collected[container]; ok {
					continue
				}

				cmdStr := fmt.Sprintf(cmdTemplate, container)
				args := strings.Fields(cmdStr)
				cmd := SSH(minion, args...)
				output, err := cmd.Output()
				if err != nil {
					log.WithError(err).WithField("cmd",
						cmd).Errorf("ssh into %s:%s "+
						"failed", minion, container)
					continue
				}

				match := timestampRegex.FindSubmatch(output)
				if len(match) != 2 {
					continue
				}

				stamp := string(match[1])
				seconds, err := strconv.ParseInt(stamp, 10, 64)
				if err != nil {
					continue
				}

				out <- time.Unix(seconds, 0)
				collected[container] = struct{}{}
			}
		}
	}()

	return out
}

func mergeTimestamps(channels []chan time.Time) chan time.Time {
	var wg sync.WaitGroup
	out := make(chan time.Time)

	wg.Add(len(channels))
	go func() {
		wg.Wait()
		out <- time.Time{} // signal done
		close(out)
	}()

	collect := func(vals chan time.Time) {
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
