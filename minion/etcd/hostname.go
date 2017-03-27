package etcd

import (
	"encoding/json"
	"fmt"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/join"
)

const hostnamePath = "/hostnames"

func runHostnameOnce(conn db.Conn, store Store) error {
	etcdStr, err := readEtcdNode(store, hostnamePath)
	if err != nil {
		return fmt.Errorf("etcd read error: %s", err)
	}

	if conn.EtcdLeader() {
		hostnames := db.HostnameSlice(conn.SelectFromHostname(nil))
		err := writeEtcdSlice(store, hostnamePath, etcdStr, hostnames)
		if err != nil {
			return fmt.Errorf("etcd write error: %s", err)
		}
	} else {
		var etcdHostnames []db.Hostname
		json.Unmarshal([]byte(etcdStr), &etcdHostnames)
		conn.Txn(db.HostnameTable).Run(func(view db.Database) error {
			joinHostnames(view, etcdHostnames)
			return nil
		})
	}

	return nil
}

func joinHostnames(view db.Database, etcdHostnames []db.Hostname) {
	key := func(iface interface{}) interface{} {
		h := iface.(db.Hostname)
		h.ID = 0
		return h
	}
	_, dbIfaces, etcdIfaces := join.HashJoin(
		db.HostnameSlice(view.SelectFromHostname(nil)),
		db.HostnameSlice(etcdHostnames), key, key)

	for _, iface := range dbIfaces {
		view.Remove(iface.(db.Hostname))
	}

	for _, iface := range etcdIfaces {
		etcdHostname := iface.(db.Hostname)
		dbHostname := view.InsertHostname()
		etcdHostname.ID = dbHostname.ID
		view.Commit(etcdHostname)
	}
}
