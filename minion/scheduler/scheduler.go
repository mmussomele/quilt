// Package scheduler is respnosible for deciding on which minion to place each container
// in the cluster.  It does this by updating each container in the Database with the
// PrivateIP of the minion it's assigned to, or the empty string if no assignment could
// be made.  Worker nodes then read these assignments form Etcd, and boot the containers
// that they are instructed to.
package scheduler

import (
	"net"
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/docker"
	"github.com/NetSys/quilt/minion/network/plugin"
	"github.com/NetSys/quilt/util"
	log "github.com/Sirupsen/logrus"
)

// Run blocks implementing the scheduler module.
func Run(conn db.Conn, dk docker.Client) {
	conn = conn.Restrict(db.MinionTable, db.ContainerTable, db.PlacementTable,
		db.EtcdTable)

	bootWait(conn)

	subnet := getMinionSubnet(conn)
	err := dk.ConfigureNetwork(plugin.NetworkName, subnet)
	if err != nil {
		log.WithError(err).Fatal("Failed to configure network plugin")
	}

	loopLog := util.NewEventTimer("Scheduler")
	for range conn.TriggerTick(60).C {
		loopLog.LogStart()
		minion, err := conn.MinionSelf()
		if err != nil {
			log.WithError(err).Warn("Missing self in the minion table.")
			continue
		}

		if minion.Role == db.Worker {
			subnet = updateNetwork(conn, dk, subnet)
			runWorker(conn, dk, minion.PrivateIP, subnet)
		} else if minion.Role == db.Master {
			runMaster(conn)
		}
		loopLog.LogEnd()
	}
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

func getMinionSubnet(conn db.Conn) net.IPNet {
	for {
		minion, err := conn.MinionSelf()
		if err != nil {
			log.WithError(err).Debug("Failed to get self")
		} else if minion.PrivateIP == "" {
			log.Error("This minion has no PrivateIP")
		} else if minion.Subnet == "" {
			log.Debug("This minion has no subnet yet")
		} else {
			_, subnet, err := net.ParseCIDR(minion.Subnet)
			if err != nil {
				log.WithError(err).Errorf("Malformed subnet: %s",
					minion.Subnet)
			}
			return *subnet
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func updateNetwork(conn db.Conn, dk docker.Client, subnet net.IPNet) net.IPNet {

	newSubnet := getMinionSubnet(conn)
	if subnet.String() == newSubnet.String() {
		return subnet
	}

	err := dk.ConfigureNetwork(plugin.NetworkName, newSubnet)
	if err != nil {
		log.WithError(err).Fatal("Failed to configure network plugin")
		return subnet
	}

	return newSubnet
}
