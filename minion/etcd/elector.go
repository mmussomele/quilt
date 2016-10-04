package etcd

import (
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/coreos/etcd/client"

	log "github.com/Sirupsen/logrus"
)

const electionTTL = 30
const dir = "/minion"
const leaderKey = dir + "/leader"

// Run blocks implementing leader election.
func runElection(conn db.Conn, store Store) {
	go watchLeader(conn, store)
	campaign(conn, store)
}

func watchLeader(conn db.Conn, store Store) {
	tickRate := electionTTL
	if tickRate > 30 {
		tickRate = 30
	}

	conn = conn.Restrict(db.EtcdTable)
	watch := store.Watch(leaderKey, 1*time.Second)
	trigg := conn.TriggerTick(tickRate)
	for {
		leader, _ := store.Get(leaderKey)
		conn.Transact(func(view db.Database) error {
			etcdRows := view.SelectFromEtcd(nil)
			if len(etcdRows) == 1 {
				etcdRows[0].LeaderIP = leader
				view.Commit(etcdRows[0])
			}
			return nil
		})

		select {
		case <-watch:
		case <-trigg.C:
		}
	}
}

func campaign(conn db.Conn, store Store) {
	conn = conn.Restrict(db.EtcdTable, db.MinionTable)
	watch := store.Watch(leaderKey, 1*time.Second)
	trigg := conn.Restrict(db.EtcdTable).TriggerTick(electionTTL / 2)

	for {
		select {
		case <-watch:
		case <-trigg.C:
		}

		etcdRows := conn.SelectFromEtcd(nil)

		minion, err := conn.MinionSelf()
		master := err == nil && minion.Role == db.Master && len(etcdRows) == 1

		if !master {
			continue
		}

		IP := minion.PrivateIP
		if IP == "" {
			continue
		}

		ttl := electionTTL * time.Second

		if etcdRows[0].Leader {
			err = store.Refresh(leaderKey, IP, ttl)
		} else {
			err = store.Create(leaderKey, IP, ttl)
		}

		if err == nil {
			commitLeader(conn, true, IP)
		} else {
			clientErr, ok := err.(client.Error)
			if !ok || clientErr.Code != client.ErrorCodeNodeExist {
				log.WithError(err).Warn("Error setting leader key")
				commitLeader(conn, false, "")

				// Give things a chance to settle down.
				time.Sleep(electionTTL * time.Second)
			} else {
				commitLeader(conn, false)
			}
		}
	}
}

func commitLeader(conn db.Conn, leader bool, ip ...string) {
	if len(ip) > 1 {
		panic("Not Reached")
	}

	conn.Transact(func(view db.Database) error {
		etcdRows := view.SelectFromEtcd(nil)
		if len(etcdRows) == 1 {
			etcdRows[0].Leader = leader

			if len(ip) == 1 {
				etcdRows[0].LeaderIP = ip[0]
			}

			view.Commit(etcdRows[0])
		}
		return nil
	})
}
