package network

import (
	"fmt"
	"strings"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/minion/ovsdb"

	log "github.com/Sirupsen/logrus"
)

func updateACLs(connections []db.Connection, labels []db.Label) {
	// Get the ACLs currently stored in the database.
	ovsdbClient, err := ovsdb.Open()
	if err != nil {
		log.WithError(err).Error("Failed to connect to OVSDB.")
		return
	}
	defer ovsdbClient.Close()

	syncAddressSets(ovsdbClient, labels)
	syncACLs(ovsdbClient, connections)
}

type addressSetKey struct {
	name      string
	addresses string
}

func newAddressSetKey(name string, addresses []string) addressSetKey {
	return addressSetKey{
		name:      name,
		addresses: strings.Join(addresses, " "),
	}
}

func unique(lst []string) (uniq []string) {
	set := make(map[string]struct{})
	for _, elem := range lst {
		set[elem] = struct{}{}
	}

	for elem := range set {
		uniq = append(uniq, elem)
	}
	return uniq
}

func syncAddressSets(ovsdbClient ovsdb.Client, labels []db.Label) {
	ovsdbAddresses, err := ovsdbClient.ListAddressSets(lSwitch)
	if err != nil {
		log.WithError(err).Error("Failed to list address sets.")
		return
	}

	labelKey := func(intf interface{}) interface{} {
		lbl := intf.(db.Label)
		return newAddressSetKey(lbl.Label, unique(append(lbl.ContainerIPs, lbl.IP)))
	}
	ovsdbKey := func(intf interface{}) interface{} {
		addrSet := intf.(ovsdb.AddressSet)
		return newAddressSetKey(addrSet.Name, addrSet.Addresses)
	}
	_, toCreate, toDelete := join.HashJoin(db.LabelSlice(labels),
		addressSlice(ovsdbAddresses), labelKey, ovsdbKey)

	for _, addr := range toDelete {
		if err := ovsdbClient.DeleteAddressSet(lSwitch, addr.(ovsdb.AddressSet).Name); err != nil {
			log.WithError(err).Warn("Error deleting address set.")
		}
	}

	for _, labelIntf := range toCreate {
		label := labelIntf.(db.Label)
		if err := ovsdbClient.CreateAddressSet(lSwitch, label.Label, unique(append(label.ContainerIPs, label.IP))); err != nil {
			log.WithError(err).Warn("Error adding address set.")
		}
	}
}

type aclKey struct {
	drop  bool
	match string
}

func syncACLs(ovsdbClient ovsdb.Client, connections []db.Connection) {
	ovsdbACLs, err := ovsdbClient.ListACLs(lSwitch)
	if err != nil {
		log.WithError(err).Error("Failed to list ACLs.")
		return
	}

	expACLs := []aclKey{
		{
			match: "ip",
			drop:  true,
		},
	}
	for _, conn := range connections {
		expACLs = append(expACLs, aclKey{
			match: matchString(conn),
		})
	}

	ovsdbKey := func(ovsdbIntf interface{}) interface{} {
		ovsdbACL := ovsdbIntf.(ovsdb.Acl).Core
		return aclKey{
			drop:  ovsdbACL.Action == "drop",
			match: ovsdbACL.Match,
		}
	}

	_, toCreate, toDelete := join.HashJoin(aclKeySlice(expACLs),
		ovsdbACLSlice(ovsdbACLs), nil, ovsdbKey)

	for _, acl := range toDelete {
		if err := ovsdbClient.DeleteACL(lSwitch, acl.(ovsdb.Acl)); err != nil {
			log.WithError(err).Warn("Error deleting ACL")
		}
	}

	for _, intf := range toCreate {
		acl := intf.(aclKey)
		priority := 1
		// XXX: If we can figure out `allow-related`, we should be able to create
		// simpler match rules (they wouldn't have to account for return traffic).
		action := "allow"
		if acl.drop {
			// Allow ACLs supersede drop ACLs.
			priority = 0
			action = "drop"
		}
		if err := ovsdbClient.CreateACL(lSwitch, "to-lport",
			priority, acl.match, action); err != nil {
			log.WithError(err).Warn("Error adding ACL")
		}
	}
}

func matchString(c db.Connection) string {
	return or(
		and(
			and(from(c.From), to(c.To)),
			portConstraint(c.MinPort, c.MaxPort, "dst")),
		and(
			and(from(c.To), to(c.From)),
			portConstraint(c.MinPort, c.MaxPort, "src")))
}

func portConstraint(minPort, maxPort int, direction string) string {
	return fmt.Sprintf("(icmp || %[1]d <= udp.%[2]s <= %[3]d || "+
		"%[1]d <= tcp.%[2]s <= %[3]d)", minPort, direction, maxPort)
}

func from(tgt string) string {
	return fmt.Sprintf("ip4.src == $%s", tgt)
}

func to(tgt string) string {
	return fmt.Sprintf("ip4.dst == $%s", tgt)
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

type addressSlice []ovsdb.AddressSet

// Len returns the length of the slice
func (slc addressSlice) Len() int {
	return len(slc)
}

// Get returns the element at index i of the slice
func (slc addressSlice) Get(i int) interface{} {
	return slc[i]
}

type aclKeySlice []aclKey

// Len returns the length of the slice
func (slc aclKeySlice) Len() int {
	return len(slc)
}

// Get returns the element at index i of the slice
func (slc aclKeySlice) Get(i int) interface{} {
	return slc[i]
}
