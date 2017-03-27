package db

import (
	"fmt"
	"reflect"
	"sync"
)

// TableType represents a table in the database.
type TableType string

// ClusterTable is the type of the cluster table.
var ClusterTable = TableType(reflect.TypeOf(Cluster{}).String())

// MachineTable is the type of the machine table.
var MachineTable = TableType(reflect.TypeOf(Machine{}).String())

// ContainerTable is the type of the container table.
var ContainerTable = TableType(reflect.TypeOf(Container{}).String())

// MinionTable is the type of the minion table.
var MinionTable = TableType(reflect.TypeOf(Minion{}).String())

// ConnectionTable is the type of the connection table.
var ConnectionTable = TableType(reflect.TypeOf(Connection{}).String())

// LabelTable is the type of the label table.
var LabelTable = TableType(reflect.TypeOf(Label{}).String())

// EtcdTable is the type of the etcd table.
var EtcdTable = TableType(reflect.TypeOf(Etcd{}).String())

// PlacementTable is the type of the placement table.
var PlacementTable = TableType(reflect.TypeOf(Placement{}).String())

// ACLTable is the type of the ACL table.
var ACLTable = TableType(reflect.TypeOf(ACL{}).String())

// ImageTable is the type of the image table.
var ImageTable = TableType(reflect.TypeOf(Image{}).String())

// HostnameTable is the type of the Hostname table.
var HostnameTable = TableType(reflect.TypeOf(Hostname{}).String())

// AllTables is a slice of all the db TableTypes. It is used primarily for tests,
// where there is no reason to put lots of thought into which tables a Transaction
// should use.
var AllTables = []TableType{ClusterTable, MachineTable, ContainerTable, MinionTable,
	ConnectionTable, LabelTable, EtcdTable, PlacementTable, ACLTable, ImageTable,
	HostnameTable}

type table struct {
	name TableType
	rows map[int]row

	callbacks   []*Callback
	shouldAlert bool
	sync.Mutex
}

func newTable(t TableType) *table {
	return &table{
		name:        t,
		rows:        make(map[int]row),
		callbacks:   []*Callback{},
		shouldAlert: false,
	}
}

// alert will call of the Callbacks for the table. The caller must have a lock on the
// table before calling alert.
func (t *table) alert() {
	for _, c := range t.callbacks {
		select {
		case c.causes <- fmt.Sprintf(changeAction, t.name):
		default:
		}
	}
}
