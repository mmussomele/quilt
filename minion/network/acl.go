package network

import (
	"fmt"
	"strings"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/minion/ovsdb"

	log "github.com/Sirupsen/logrus"
)

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

	generator := newACLGenerator(labels)

	connKey := func(c interface{}) interface{} {
		return generator.create(c.(db.Connection))
	}
	ovsdbKey := func(ovsdbIntf interface{}) interface{} {
		ovsdbACL := ovsdbIntf.(ovsdb.Acl).Core
		return acl{
			drop:  ovsdbACL.Action == "drop",
			match: ovsdbACL.Match,
		}
	}

	connections = append(connections, dropConnection)
	_, toCreate, toDelete := join.HashJoin(db.ConnectionSlice(connections),
		ovsdbACLSlice(ovsdbACLs), connKey, ovsdbKey)

	for _, acl := range toDelete {
		if err := ovsdbClient.DeleteACL(lSwitch, acl.(ovsdb.Acl)); err != nil {
			log.WithError(err).Warn("Error deleting ACL")
		}
	}

	for _, connIntf := range toCreate {
		aclCore := generator.create(connIntf.(db.Connection)).toOvsdb()
		if err := ovsdbClient.CreateACL(lSwitch, aclCore.Direction,
			aclCore.Priority, aclCore.Match, aclCore.Action); err != nil {
			log.WithError(err).Warn("Error adding ACL")
		}
	}
}

type acl struct {
	drop  bool
	match string
}

func (a acl) toOvsdb() ovsdb.AclCore {
	priority := 1
	action := "allow"
	if a.drop {
		// Allow ACLs supersede drop ACLs.
		priority = 0
		action = "drop"
	}
	return ovsdb.AclCore{
		Priority:  priority,
		Match:     a.match,
		Action:    action,
		Direction: "to-lport",
	}
}

var dropConnection = db.Connection{ID: -1}

type aclGenerator map[string]db.Label

func newACLGenerator(labels []db.Label) aclGenerator {
	labelMap := map[string]db.Label{}
	for _, l := range labels {
		labelMap[l.Label] = l
	}
	return aclGenerator(labelMap)
}

func (generator aclGenerator) create(c db.Connection) acl {
	if c.ID == dropConnection.ID {
		return acl{
			drop:  true,
			match: "ip",
		}
	}

	fromIPs := generator[c.From].ContainerIPs
	toIPs := append(generator[c.To].ContainerIPs, generator[c.To].IP)
	return acl{
		match: or(
			and(
				and(fromAny(fromIPs), toAny(toIPs)),
				portConstraint(c.MinPort, c.MaxPort, "dst")),
			and(
				and(fromAny(toIPs), toAny(fromIPs)),
				portConstraint(c.MinPort, c.MaxPort, "src"))),
	}
}

func portConstraint(minPort, maxPort int, direction string) string {
	return fmt.Sprintf("(icmp || %[1]d <= udp.%[2]s <= %[3]d || "+
		"%[1]d <= tcp.%[2]s <= %[3]d)", minPort, direction, maxPort)
}

func fromAny(ips []string) string {
	return any("ip4.src==%s", ips)
}

func toAny(ips []string) string {
	return any("ip4.dst==%s", ips)
}

func any(fmtr string, args []string) string {
	var matches []string
	for _, arg := range args {
		matches = append(matches, fmt.Sprintf(fmtr, arg))
	}
	return or(matches...)
}

func or(predicates ...string) string {
	return "(" + strings.Join(predicates, " || ") + ")"
}

func and(predicates ...string) string {
	return "(" + strings.Join(predicates, " && ") + ")"
}

// ovsdbACLSlice is a wrapper around []ovsdb.Acl to allow us to perform a join
type ovsdbACLSlice []ovsdb.Acl

// Len returns the length of the slice
func (slc ovsdbACLSlice) Len() int {
	return len(slc)
}

// Get returns the element at index i of the slice
func (slc ovsdbACLSlice) Get(i int) interface{} {
	return slc[i]
}
