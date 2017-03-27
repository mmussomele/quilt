package etcd

import (
	"time"

	"github.com/coreos/etcd/client"
	"github.com/quilt/quilt/db"

	log "github.com/Sirupsen/logrus"
)

// Run synchronizes state in `conn` with the Etcd cluster.
func Run(conn db.Conn) {
	store := NewStore()
	makeEtcdDir(minionPath, store, 0)
	runElection(conn, store)

	// Register the connection update callback.
	connectionWatch := store.Watch(connectionPath, 1*time.Second)
	conn.RegisterCallback(func() {
		if err := runConnectionOnce(conn, store); err != nil {
			log.WithError(err).Warn("Failed to sync connections with Etcd.")
		}
	}, "Etcd Connection", 60, db.ConnectionTable).RegisterTrigger(connectionWatch)

	// Register the container update callback.
	containerWatch := store.Watch(containerPath, 1*time.Second)
	conn.RegisterCallback(func() {
		if err := runContainerOnce(conn, store); err != nil {
			log.WithError(err).Warn("Failed to sync containers with Etcd.")
		}
	}, "Etcd Container", 60, db.ContainerTable).RegisterTrigger(containerWatch)

	// Register the hostname callback.
	hostnameWatch := store.Watch(hostnamePath, 1*time.Second)
	conn.RegisterCallback(func() {
		if err := runHostnameOnce(conn, store); err != nil {
			log.WithError(err).Warn("Failed to sync hostnames with Etcd")
		}
	}, "Etcd Hostname", 60, db.HostnameTable).RegisterTrigger(hostnameWatch)

	// Register the minion sync callback.
	conn.RegisterCallback(func() {
		writeMinion(conn, store)
		readMinion(conn, store)
	}, "Etcd Minion", minionTimeout/2, db.MinionTable)
}

func makeEtcdDir(dir string, store Store, ttl time.Duration) {
	for {
		err := createEtcdDir(dir, store, ttl)
		if err == nil {
			break
		}

		log.WithError(err).Debug("Failed to create etcd dir")
		time.Sleep(5 * time.Second)
	}
}

func createEtcdDir(dir string, store Store, ttl time.Duration) error {
	err := store.Mkdir(dir, ttl)
	if err == nil {
		return nil
	}

	// If the directory already exists, no need to create it
	etcdErr, ok := err.(client.Error)
	if ok && etcdErr.Code == client.ErrorCodeNodeExist {
		return store.RefreshDir(dir, ttl)
	}

	return err
}
