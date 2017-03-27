package supervisor

import (
	"fmt"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/util"
)

func runMaster() {
	run(Ovsdb, "ovsdb-server")
	run(Registry)
	conn.RegisterCallback(runMasterOnce, "Supervisor", 0,
		db.MinionTable, db.EtcdTable)
}

func runMasterOnce() {
	minion := conn.MinionSelf()

	var etcdRow db.Etcd
	if etcdRows := conn.SelectFromEtcd(nil); len(etcdRows) == 1 {
		etcdRow = etcdRows[0]
	}

	IP := minion.PrivateIP
	etcdIPs := etcdRow.EtcdIPs
	leader := etcdRow.Leader

	if oldIP != IP || !util.StrSliceEqual(oldEtcdIPs, etcdIPs) {
		Remove(Etcd)
	}

	oldEtcdIPs = etcdIPs
	oldIP = IP

	if IP == "" || len(etcdIPs) == 0 {
		return
	}

	run(Etcd, fmt.Sprintf("--name=master-%s", IP),
		fmt.Sprintf("--initial-cluster=%s", initialClusterString(etcdIPs)),
		fmt.Sprintf("--advertise-client-urls=http://%s:2379", IP),
		fmt.Sprintf("--listen-peer-urls=http://%s:2380", IP),
		fmt.Sprintf("--initial-advertise-peer-urls=http://%s:2380", IP),
		"--listen-client-urls=http://0.0.0.0:2379",
		"--heartbeat-interval="+etcdHeartbeatInterval,
		"--initial-cluster-state=new",
		"--election-timeout="+etcdElectionTimeout)

	run(Ovsdb, "ovsdb-server")
	run(Registry)

	if leader {
		/* XXX: If we fail to boot ovn-northd, we should give up
		* our leadership somehow.  This ties into the general
		* problem of monitoring health. */
		run(Ovnnorthd, "ovn-northd")
	} else {
		Remove(Ovnnorthd)
	}
}
