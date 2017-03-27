package supervisor

import (
	"fmt"
	"net"
	"time"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/minion/ipdef"
	"github.com/quilt/quilt/util"
	"github.com/vishvananda/netlink"

	log "github.com/Sirupsen/logrus"
)

func runWorker() {
	setupWorker()
	conn.RegisterCallback(runWorkerOnce, "Supervisor", 0,
		db.MinionTable, db.EtcdTable)
}

func setupWorker() {
	run(Ovsdb, "ovsdb-server")
	run(Ovsvswitchd, "ovs-vswitchd")

	for {
		err := setupBridge()
		if err == nil {
			break
		}
		log.WithError(err).Warnf("Failed to exec in %s.", Ovsvswitchd)
		time.Sleep(5 * time.Second)
	}

	ip := net.IPNet{IP: ipdef.GatewayIP, Mask: ipdef.QuiltSubnet.Mask}
	for {
		err := cfgGateway("quilt-int", ip)
		if err == nil {
			break
		}
		log.WithError(err).Error("Failed to configure quilt-int.")
		time.Sleep(5 * time.Second)
	}
}

func runWorkerOnce() {
	minion := conn.MinionSelf()

	var etcdRow db.Etcd
	if etcdRows := conn.SelectFromEtcd(nil); len(etcdRows) == 1 {
		etcdRow = etcdRows[0]
	}

	etcdIPs := etcdRow.EtcdIPs
	leaderIP := etcdRow.LeaderIP
	IP := minion.PrivateIP

	if !util.StrSliceEqual(oldEtcdIPs, etcdIPs) {
		Remove(Etcd)
	}

	oldEtcdIPs = etcdIPs
	oldIP = IP

	run(Etcd, fmt.Sprintf("--initial-cluster=%s", initialClusterString(etcdIPs)),
		"--heartbeat-interval="+etcdHeartbeatInterval,
		"--election-timeout="+etcdElectionTimeout,
		"--proxy=on")

	run(Ovsdb, "ovsdb-server")
	run(Ovsvswitchd, "ovs-vswitchd")

	if leaderIP == "" || IP == "" {
		return
	}

	err := execRun("ovs-vsctl", "set", "Open_vSwitch", ".",
		fmt.Sprintf("external_ids:ovn-remote=\"tcp:%s:6640\"", leaderIP),
		fmt.Sprintf("external_ids:ovn-encap-ip=%s", IP),
		fmt.Sprintf("external_ids:ovn-encap-type=\"%s\"", tunnelingProtocol),
		fmt.Sprintf("external_ids:api_server=\"http://%s:9000\"", leaderIP),
		fmt.Sprintf("external_ids:system-id=\"%s\"", IP))
	if err != nil {
		log.WithError(err).Warnf("Failed to exec in %s.", Ovsvswitchd)
		return
	}

	/* The ovn controller doesn't support reconfiguring ovn-remote mid-run.
	 * So, we need to restart the container when the leader changes. */
	Remove(Ovncontroller)
	run(Ovncontroller, "ovn-controller")
}

func setupBridge() error {
	gwMac := ipdef.IPToMac(ipdef.GatewayIP)
	return execRun("ovs-vsctl", "add-br", "quilt-int",
		"--", "set", "bridge", "quilt-int", "fail_mode=secure",
		fmt.Sprintf("other_config:hwaddr=\"%s\"", gwMac))
}

func cfgGatewayImpl(name string, ip net.IPNet) error {
	link, err := linkByName(name)
	if err != nil {
		return fmt.Errorf("no such interface: %s (%s)", name, err)
	}

	if err := linkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up link: %s (%s)", name, err)
	}

	if err := addrAdd(link, &netlink.Addr{IPNet: &ip}); err != nil {
		return fmt.Errorf("failed to set address: %s (%s)", name, err)
	}

	return nil
}

var cfgGateway = cfgGatewayImpl
