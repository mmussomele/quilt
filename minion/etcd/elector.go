package etcd

import (
	"time"

	"github.com/coreos/etcd/client"
	"github.com/quilt/quilt/db"

	log "github.com/Sirupsen/logrus"
)

const electionTTL = 30
const leaderKey = "/leader"

// Run blocks implementing leader election.
func runElection(conn db.Conn, store Store) {
	watch := store.Watch(leaderKey, 1*time.Second)
	conn.RegisterCallback(campaign(conn, store), "Campaign", electionTTL/2,
		db.EtcdTable).RegisterTrigger(watch)

	tickRate := electionTTL
	if tickRate > 30 {
		tickRate = 30
	}

	// These callbacks can run in any order, so watchLeader's is registered second so
	// its initial run of its do() doesn't block campaign.
	conn.RegisterCallback(watchLeader(conn, store), "Watch Leader", tickRate,
		db.EtcdTable).RegisterTrigger(watch)
}

func watchLeader(conn db.Conn, store Store) func() {
	do := func() {
		leader, _ := store.Get(leaderKey)
		conn.Txn(db.EtcdTable).Run(func(view db.Database) error {
			etcdRows := view.SelectFromEtcd(nil)
			if len(etcdRows) == 1 {
				etcdRows[0].LeaderIP = leader
				view.Commit(etcdRows[0])
			}
			return nil
		})
	}

	// do must be executed once before it should be triggered by a callback, so we
	// call it before returning.
	do()
	return do
}

func campaign(conn db.Conn, store Store) func() {
	return func() {
		etcdRows := conn.SelectFromEtcd(nil)

		minion := conn.MinionSelf()
		master := minion.Role == db.Master && len(etcdRows) == 1

		if !master {
			return
		}

		IP := minion.PrivateIP
		if IP == "" {
			return
		}

		ttl := electionTTL * time.Second

		var err error
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

	conn.Txn(db.EtcdTable).Run(func(view db.Database) error {
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
