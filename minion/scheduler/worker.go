package scheduler

import (
	"sync"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/minion/docker"
	"github.com/NetSys/quilt/util"
	log "github.com/Sirupsen/logrus"
)

const labelKey = "quilt"
const labelValue = "scheduler"
const labelPair = labelKey + "=" + labelValue
const dockerRunGoroutineLimit = 32

func runWorker(conn db.Conn, dk docker.Client, myIP string) {
	if myIP == "" {
		return
	}

	filter := map[string][]string{"label": {labelPair}}
	dkcs, err := dk.List(filter)
	if err != nil {
		log.WithError(err).Warning("Failed to list docker containers.")
		return
	}

	conn.Transact(func(view db.Database) error {
		dbcs := view.SelectFromContainer(func(dbc db.Container) bool {
			return dbc.Minion == myIP
		})

		changed := syncWorker(dk, dbcs, dkcs)
		for _, dbc := range changed {
			view.Commit(dbc)
		}
		return nil
	})
}

func syncWorker(dk docker.Client, dbcs []db.Container,
	dkcs []docker.Container) (changed []db.Container) {

	pairs, dbci, dkci := join.Join(dbcs, dkcs, syncJoinScore)

	for _, i := range dkci {
		dkc := i.(docker.Container)
		log.WithField("container", dkc.ID).Info("Remove container")
		if err := dk.RemoveID(dkc.ID); err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"id":    dkc.ID,
			}).Warning("Failed to remove container.")
		}
	}

	// Start a bunch of goroutines listening for db.Containers
	dbcChannel := make(chan db.Container)
	pairChannels := []chan join.Pair{}
	for i := 0; i < dockerRunGoroutineLimit; i++ {
		pairChannels = append(pairChannels, dockerRun(dk, dbcChannel))
	}
	pairOutput := merge(pairChannels)

	// Send a bunch of db.Containers to the previously started goroutines
	for _, i := range dbci {
		dbcChannel <- i.(db.Container)
	}
	close(dbcChannel)

	// Collect the results of booting the docker containers
	for p := range pairOutput {
		pairs = append(pairs, p)
	}

	for _, pair := range pairs {
		dbc := pair.L.(db.Container)
		dkc := pair.R.(docker.Container)

		if dbc.DockerID != dkc.ID {
			dbc.DockerID = dkc.ID
			dbc.Pid = dkc.Pid
			changed = append(changed, dbc)
		}
	}

	return changed
}

func dockerRun(dk docker.Client, in chan db.Container) chan join.Pair {
	out := make(chan join.Pair)
	go func() {
		defer close(out)
		for dbc := range in {
			log.WithField("container", dbc).Info("Start container")
			id, err := dk.Run(docker.RunOptions{
				Image:  dbc.Image,
				Args:   dbc.Command,
				Env:    dbc.Env,
				Labels: map[string]string{labelKey: labelValue},
			})
			if err != nil {
				log.WithFields(log.Fields{
					"error":     err,
					"container": dbc,
				}).WithError(err).Warning("Failed to run container", dbc)
				continue
			}

			dkc, err := dk.Get(id)
			if err != nil {
				log.WithFields(log.Fields{
					"error":     err,
					"container": dbc,
				}).WithError(err).Warning("Failed to get container", dbc)
				continue
			}

			out <- join.Pair{L: dbc, R: dkc}
		}
	}()

	return out
}

func merge(channels []chan join.Pair) chan join.Pair {
	var wg sync.WaitGroup
	out := make(chan join.Pair)

	wg.Add(len(channels))
	go func() {
		wg.Wait()
		close(out)
	}()

	collect := func(vals chan join.Pair) {
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

// TODO: Find a way to turn this into a HashJoin
//	- Maybe join on cmd1, then cmd2 and then join on those?
func syncJoinScore(left, right interface{}) int {
	dbc := left.(db.Container)
	dkc := right.(docker.Container)

	// Depending on the container, the command in the database could be
	// either The command plus it's arguments, or just it's arguments.  To
	// handle that case, we check both.
	cmd1 := dkc.Args
	cmd2 := append([]string{dkc.Path}, dkc.Args...)
	dbcCmd := dbc.Command

	for key, value := range dbc.Env {
		if dkc.Env[key] != value {
			return -1
		}
	}

	switch {
	case dbc.Image != dkc.Image:
		return -1
	case len(dbcCmd) != 0 &&
		!util.StrSliceEqual(dbcCmd, cmd1) &&
		!util.StrSliceEqual(dbcCmd, cmd2):
		return -1
	case dbc.DockerID == dkc.ID:
		return 0
	default:
		return 1
	}
}
