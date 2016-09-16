package network

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/ovsdb"
)

type lportslice []ovsdb.LPort

func (lps lportslice) Len() int {
	return len(lps)
}

func (lps lportslice) Less(i, j int) bool {
	return lps[i].Name < lps[j].Name
}

func (lps lportslice) Swap(i, j int) {
	lps[i], lps[j] = lps[j], lps[i]
}

func TestRunMaster(t *testing.T) {
	client := ovsdb.NewFakeOvsdbClient()
	client.CreateLogicalSwitch(lSwitch)
	conn := db.New()
	ovsdb.Open = func() (ovsdb.Client, error) {
		return client, nil
	}

	expPorts := []ovsdb.LPort{}
	conn.Transact(func(view db.Database) error {
		etcd := view.InsertEtcd()
		etcd.Leader = true
		view.Commit(etcd)

		for i := 0; i < 3; i++ {
			si := strconv.Itoa(i)
			l := view.InsertLabel()
			l.IP = fmt.Sprintf("0.0.0.%s", si)
			l.MultiHost = true
			view.Commit(l)
			expPorts = append(expPorts, ovsdb.LPort{
				Bridge:    lSwitch,
				Name:      l.IP,
				Addresses: []string{labelMac, l.IP},
			})
		}

		for i := 3; i < 5; i++ {
			si := strconv.Itoa(i)
			c := view.InsertContainer()
			c.IP = fmt.Sprintf("0.0.0.%s", si)
			c.Mac = fmt.Sprintf("00:00:00:00:00:0%s", si)
			view.Commit(c)
			expPorts = append(expPorts, ovsdb.LPort{
				Bridge:    lSwitch,
				Name:      c.IP,
				Addresses: []string{c.Mac, c.IP},
			})
		}

		return nil
	})

	for i := 1; i < 6; i++ {
		si := strconv.Itoa(i)
		mac := fmt.Sprintf("00:00:00:00:00:0%s", si)
		if i < 3 {
			mac = labelMac
		}
		ip := fmt.Sprintf("0.0.0.%s", si)
		client.CreateLogicalPort(lSwitch, ip, mac, ip)
	}

	runMaster(conn)

	lports, err := client.ListLogicalPorts(lSwitch)
	if err != nil {
		t.Fatal("failed to fetch logical ports from mock client")
	}

	if len(lports) != len(expPorts) {
		t.Fatalf("wrong number of logical ports. Got %d, expected %d.",
			len(lports), len(expPorts))
	}

	sort.Sort(lportslice(lports))
	sort.Sort(lportslice(expPorts))
	for i, port := range expPorts {
		lport := lports[i]
		if lport.Bridge != port.Bridge || lport.Name != port.Name {
			t.Fatalf("Incorrect port %v, expected %v.", lport, port)
		}
	}
}

func checkACLCount(t *testing.T, client ovsdb.Client,
	connections []db.Connection, expCount int) {

	updateACLs(connections, allLabels, allContainers)
	if acls, _ := client.ListACLs(lSwitch); len(acls) != expCount {
		t.Errorf("Wrong number of ACLs: expected %d, got %d.",
			expCount, len(acls))
	}
}

func TestACLUpdate(t *testing.T) {
	client := ovsdb.NewFakeOvsdbClient()
	client.CreateLogicalSwitch(lSwitch)
	ovsdb.Open = func() (ovsdb.Client, error) {
		return client, nil
	}
	redBlueConnection := db.Connection{
		From:    "red",
		To:      "blue",
		MinPort: 80,
		MaxPort: 80,
	}
	redYellowConnection := db.Connection{
		From:    "redBlue",
		To:      "redBlue",
		MinPort: 80,
		MaxPort: 81,
	}
	checkACLCount(t, client, []db.Connection{redBlueConnection}, 2)
	checkACLCount(t, client,
		[]db.Connection{redBlueConnection, redYellowConnection}, 3)
	checkACLCount(t, client, []db.Connection{redYellowConnection}, 2)
	checkACLCount(t, client, nil, 1)
}

func TestACLGeneration(t *testing.T) {
	testGenerator := newACLGenerator(allLabels)

	exp := acl{
		match: "((((ip4.src==100.1.1.3) && (ip4.dst==100.1.1.1 || ip4.dst==100.1.1.2 || ip4.dst==13.13.13.13)) && (icmp || 80 <= udp.dst <= 81 || 80 <= tcp.dst <= 81)) || (((ip4.src==100.1.1.1 || ip4.src==100.1.1.2 || ip4.src==13.13.13.13) && (ip4.dst==100.1.1.3)) && (icmp || 80 <= udp.src <= 81 || 80 <= tcp.src <= 81)))",
	}

	actual := testGenerator.create(db.Connection{
		From:    "yellow",
		To:      "redBlue",
		MinPort: 80,
		MaxPort: 81,
	})
	if !reflect.DeepEqual(actual, exp) {
		t.Errorf("Bad ACL generation: expected %v, got %v",
			exp, actual)
	}
}

var redLabelIP = "8.8.8.8"
var blueLabelIP = "9.9.9.9"
var yellowLabelIP = "10.10.10.10"
var redBlueLabelIP = "13.13.13.13"
var redContainerIP = "100.1.1.1"
var blueContainerIP = "100.1.1.2"
var yellowContainerIP = "100.1.1.3"

var redLabel = db.Label{
	Label:        "red",
	IP:           redLabelIP,
	ContainerIPs: []string{redContainerIP},
}
var blueLabel = db.Label{
	Label:        "blue",
	IP:           blueLabelIP,
	ContainerIPs: []string{blueContainerIP},
}
var yellowLabel = db.Label{
	Label:        "yellow",
	IP:           yellowLabelIP,
	ContainerIPs: []string{yellowContainerIP},
}
var redBlueLabel = db.Label{
	Label:        "redBlue",
	IP:           redBlueLabelIP,
	ContainerIPs: []string{redContainerIP, blueContainerIP},
}
var allLabels = []db.Label{redLabel, blueLabel, yellowLabel, redBlueLabel}

var redContainer = db.Container{
	IP:     redContainerIP,
	Labels: []string{"red", "redBlue"},
}
var blueContainer = db.Container{
	IP:     blueContainerIP,
	Labels: []string{"blue", "redBlue"},
}
var yellowContainer = db.Container{
	IP:     yellowContainerIP,
	Labels: []string{"yellow"},
}
var allContainers = []db.Container{redContainer, blueContainer, yellowContainer}
