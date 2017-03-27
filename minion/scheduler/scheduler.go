// Package scheduler is respnosible for deciding on which minion to place each container
// in the cluster.  It does this by updating each container in the Database with the
// PrivateIP of the minion it's assigned to, or the empty string if no assignment could
// be made.  Worker nodes then read these assignments form Etcd, and boot the containers
// that they are instructed to.
package scheduler

import (
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/minion/docker"
	"github.com/quilt/quilt/minion/network/plugin"
)

// Run blocks implementing the scheduler module.
func Run(conn db.Conn, dk docker.Client) {
	bootWait(conn)

	err := dk.ConfigureNetwork(plugin.NetworkName)
	if err != nil {
		log.WithError(err).Fatal("Failed to configure network plugin")
	}

	conn.RegisterCallback(func() {
		minion := conn.MinionSelf()

		if minion.Role == db.Worker {
			runWorker(conn, dk, minion.PrivateIP)
		} else if minion.Role == db.Master {
			runMaster(conn)
		}
	}, "Scheduler", 60, db.MinionTable, db.ContainerTable,
		db.PlacementTable, db.EtcdTable)
}

func bootWait(conn db.Conn) {
	for workerCount := 0; workerCount <= 0; {
		workerCount = 0
		for _, m := range conn.SelectFromMinion(nil) {
			if m.Role == db.Worker {
				workerCount++
			}
		}
		time.Sleep(30 * time.Second)
	}
}
