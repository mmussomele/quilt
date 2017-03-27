package etcd

import (
	"encoding/json"
	"fmt"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/join"
)

const connectionPath = "/connections"

func runConnectionOnce(conn db.Conn, store Store) error {
	etcdStr, err := readEtcdNode(store, connectionPath)
	if err != nil {
		return fmt.Errorf("etcd read error: %s", err)
	}

	if conn.EtcdLeader() {
		slice := db.ConnectionSlice(conn.SelectFromConnection(nil))
		err = writeEtcdSlice(store, connectionPath, etcdStr, slice)
		if err != nil {
			return fmt.Errorf("etcd write error: %s", err)
		}
	} else {
		var etcdConns []db.Connection
		json.Unmarshal([]byte(etcdStr), &etcdConns)
		conn.Txn(db.ConnectionTable).Run(func(view db.Database) error {
			joinConnections(view, etcdConns)
			return nil
		})
	}

	return nil
}

func joinConnections(view db.Database, etcdConns []db.Connection) {
	key := func(iface interface{}) interface{} {
		conn := iface.(db.Connection)
		conn.ID = 0
		return conn
	}

	_, connIfaces, etcdConnIfaces := join.HashJoin(
		db.ConnectionSlice(view.SelectFromConnection(nil)),
		db.ConnectionSlice(etcdConns), key, key)

	for _, conn := range connIfaces {
		view.Remove(conn.(db.Connection))
	}

	for _, etcdConnIface := range etcdConnIfaces {
		etcdConn := etcdConnIface.(db.Connection)
		conn := view.InsertConnection()
		etcdConn.ID = conn.ID
		view.Commit(etcdConn)
	}
}
