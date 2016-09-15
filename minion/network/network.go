// Package network manages the network services of the application dataplane.  This
// means ensuring that containers can find and communicate with each other in accordance
// with the policy specification.  It achieves this by manipulating IP addresses and
// hostnames within the containers, Open vSwitch on each running worker, and the OVN
// controller.
package network

import (
	"fmt"
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/minion/docker"
	"github.com/NetSys/quilt/minion/ovsdb"
	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"
)

const labelMac = "0a:00:00:00:00:00"
const lSwitch = "quilt"
const quiltBridge = "quilt-int"
const ovnBridge = "br-int"

// GatewayIP is the address of the border router in the logical network.
const GatewayIP = "10.0.0.1"

const gatewayMAC = "02:00:0a:00:00:01"

type dbport struct {
	bridge string
	ip     string
	mac    string
}

// dbslice is a wrapper around []dbport to allow us to perform a join
type dbslice []dbport

// Run blocks implementing the network services.
func Run(conn db.Conn, dk docker.Client) {
	for {
		odb, err := ovsdb.Open()
		if err == nil {
			odb.Close()
			break
		}
		log.WithError(err).Debug("Could not connect to ovsdb-server.")
		time.Sleep(5 * time.Second)
	}

	loopLog := util.NewLoopTimeLogger("Network")
	for range conn.TriggerTick(30, db.MinionTable, db.ContainerTable,
		db.ConnectionTable, db.LabelTable, db.EtcdTable).C {

		loopLog.LogLoopStart()
		runWorker(conn, dk)
		runMaster(conn)
		loopLog.LogLoopEnd()
	}
}

// The leader of the cluster is responsible for properly configuring OVN northd for
// container networking.  This simply means creating a logical port for each container
// and label.  The specialized OpenFlow rules Quilt requires are managed by the workers
// individuallly.
func runMaster(conn db.Conn) {
	var leader bool
	var labels []db.Label
	var containers []db.Container
	var connections []db.Connection
	conn.Transact(func(view db.Database) error {
		leader = view.EtcdLeader()

		labels = view.SelectFromLabel(func(label db.Label) bool {
			return label.IP != ""
		})

		containers = view.SelectFromContainer(func(dbc db.Container) bool {
			return dbc.Mac != "" && dbc.IP != ""
		})

		connections = view.SelectFromConnection(nil)
		return nil
	})

	if !leader {
		return
	}

	var dbData []dbport
	for _, l := range labels {
		if l.MultiHost {
			dbData = append(dbData, dbport{
				bridge: lSwitch,
				ip:     l.IP,
				mac:    labelMac,
			})
		}
	}
	for _, c := range containers {
		dbData = append(dbData, dbport{bridge: lSwitch, ip: c.IP, mac: c.Mac})
	}

	ovsdbClient, err := ovsdb.Open()
	if err != nil {
		log.WithError(err).Error("Failed to connect to OVSDB.")
		return
	}
	defer ovsdbClient.Close()

	ovsdbClient.CreateLogicalSwitch(lSwitch)
	lports, err := ovsdbClient.ListLogicalPorts(lSwitch)
	if err != nil {
		log.WithError(err).Error("Failed to list OVN ports.")
		return
	}

	portKey := func(val interface{}) interface{} {
		port := val.(ovsdb.LPort)
		return fmt.Sprintf("bridge:%s\nname:%s", port.Bridge, port.Name)
	}

	dbKey := func(val interface{}) interface{} {
		dbPort := val.(dbport)
		return fmt.Sprintf("bridge:%s\nname:%s", dbPort.bridge, dbPort.ip)
	}

	_, ovsps, dbps := join.HashJoin(ovsdb.LPortSlice(lports), dbslice(dbData),
		portKey, dbKey)

	for _, dbp := range dbps {
		lport := dbp.(dbport)
		log.WithField("IP", lport.ip).Info("New logical port.")
		err := ovsdbClient.CreateLogicalPort(lport.bridge, lport.ip, lport.mac,
			lport.ip)
		if err != nil {
			log.WithError(err).Warnf("Failed to create port %s.", lport.ip)
		}
	}

	for _, ovsp := range ovsps {
		lport := ovsp.(ovsdb.LPort)
		log.Infof("Delete logical port %s.", lport.Name)
		if err := ovsdbClient.DeleteLogicalPort(lSwitch, lport); err != nil {
			log.WithError(err).Warn("Failed to delete logical port.")
		}
	}

	updateACLs(connections, labels, containers)
}

func updateACLs(connections []db.Connection, labels []db.Label,
	containers []db.Container) {
	// Get the ACLs currently stored in the database.
	ovsdbClient, err := ovsdb.Open()
	if err != nil {
		log.WithError(err).Error("Failed to connect to OVSDB.")
		return
	}
	defer ovsdbClient.Close()

	ovsdbACLs, err := ovsdbClient.ListACLs(lSwitch)
	if err != nil {
		log.WithError(err).Error("Failed to list ACLS.")
		return
	}

	// Generate the ACLs that should be in the database.
	conns := getACLConnections(connections, labels, containers)
	coreACLs := generateACLs(conns)

	for _, acl := range ovsdbACLs {
		core := acl.Core
		if _, ok := coreACLs[core]; ok {
			delete(coreACLs, core)
			continue
		}

		if err := ovsdbClient.DeleteACL(lSwitch, acl); err != nil {
			log.WithError(err).Warn("Error deleting ACL")
		}
	}

	for aclCore := range coreACLs {
		if err := ovsdbClient.CreateACL(lSwitch, aclCore.Direction,
			aclCore.Priority, aclCore.Match, aclCore.Action); err != nil {
			log.WithError(err).Warn("Error adding ACL")
		}
	}
}

type aclConnection struct {
	fromIPs []string
	toIPs   []string
	minPort int
	maxPort int
}

func getACLConnections(connections []db.Connection, labels []db.Label,
	containers []db.Container) []aclConnection {

	labelMap := map[string]db.Label{}
	for _, l := range labels {
		labelMap[l.Label] = l
	}

	labelDbcMap := map[string][]db.Container{}
	for _, dbc := range containers {
		for _, l := range dbc.Labels {
			labelDbcMap[l] = append(labelDbcMap[l], dbc)
		}
	}

	var res []aclConnection
	for _, conn := range connections {
		var fromIPs []string
		for _, fromDbc := range labelDbcMap[conn.From] {
			fromIP := fromDbc.IP
			if fromIP == "" {
				continue
			}
			fromIPs = append(fromIPs, fromIP)
		}

		var toIPs []string
		toLabel := labelMap[conn.To]
		for _, toIP := range append(toLabel.ContainerIPs, toLabel.IP) {
			if toIP == "" {
				continue
			}
			toIPs = append(toIPs, toIP)
		}

		if len(toIPs) == 0 || len(fromIPs) == 0 {
			continue
		}

		res = append(res, aclConnection{
			fromIPs: fromIPs,
			toIPs:   toIPs,
			minPort: conn.MinPort,
			maxPort: conn.MaxPort,
		})
	}

	return res
}

func (c aclConnection) acls() (acls []string) {
	matchFmt := fmt.Sprintf("ip4.src==%%[1]s && ip4.dst==%%[3]s && "+
		"(%[1]d <= udp.%%[2]s <= %[2]d || %[1]d <= tcp.%%[2]s <= %[2]d)",
		c.minPort, c.maxPort)
	icmpFmt := "ip4.src==%s && ip4.dst==%s && icmp"
	for _, fromIP := range c.fromIPs {
		for _, toIP := range c.toIPs {
			acls = append(acls,
				fmt.Sprintf(matchFmt, fromIP, "dst", toIP),
				fmt.Sprintf(matchFmt, toIP, "src", fromIP),
				fmt.Sprintf(icmpFmt, fromIP, toIP),
				fmt.Sprintf(icmpFmt, toIP, fromIP))
		}
	}
	return acls
}

func generateACLs(connections []aclConnection) map[ovsdb.AclCore]struct{} {
	coreACLs := map[ovsdb.AclCore]struct{}{
		// Drop all ip traffic by default.
		ovsdb.AclCore{
			Priority:  0,
			Match:     "ip",
			Action:    "drop",
			Direction: "to-lport",
		}: {},
		ovsdb.AclCore{
			Priority:  0,
			Match:     "ip",
			Action:    "drop",
			Direction: "from-lport",
		}: {},
	}
	for _, c := range connections {
		for _, match := range c.acls() {
			coreACLs[ovsdb.AclCore{
				Priority:  1,
				Direction: "to-lport",
				Action:    "allow",
				Match:     match,
			}] = struct{}{}
			coreACLs[ovsdb.AclCore{
				Priority:  1,
				Direction: "from-lport",
				Action:    "allow",
				Match:     match,
			}] = struct{}{}
		}
	}

	return coreACLs
}

// Len returns the length of the slice
func (dbs dbslice) Len() int {
	return len(dbs)
}

// Get returns the element at index i of the slice
func (dbs dbslice) Get(i int) interface{} {
	return dbs[i]
}
