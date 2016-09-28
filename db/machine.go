package db

import (
	"fmt"
	"sort"
	"strings"
)

// Machine represents a physical or virtual machine operated by a cloud provider on
// which containers may be run.
type Machine struct {
	ID int //Database ID

	/* Populated by the policy engine. */
	Role     Role
	Provider Provider
	Region   string
	Size     string
	DiskSize int
	SSHKeys  []string `rowStringer:"omit"`

	/* Populated by the cloud provider. */
	CloudID   string //Cloud Provider ID
	PublicIP  string
	PrivateIP string

	/* Populated by the foreman. */
	Connected bool // Whether the minion on this machine has connected back.
}

// InsertMachine creates a new Machine and inserts it into 'db'.
func (db Database) InsertMachine() Machine {
	result := Machine{ID: db.nextID()}
	db.insert(result)
	return result
}

// SelectFromMachine gets all machines in the database that satisfy the 'check'.
func (db Database) SelectFromMachine(check func(Machine) bool) []Machine {
	machineTable := db.accessTable(MachineTable)
	result := []Machine{}
	for _, row := range machineTable.rows {
		if check == nil || check(row.(Machine)) {
			result = append(result, row.(Machine))
		}
	}
	return result
}

// SelectFromMachine gets all machines in the database that satisfy 'check'.
func (cn Conn) SelectFromMachine(check func(Machine) bool) []Machine {
	var machines []Machine
	cn.Txn(MachineTable).Run(func(view Database) error {
		machines = view.SelectFromMachine(check)
		return nil
	})
	return machines
}

func (m Machine) getID() int {
	return m.ID
}

func (m Machine) String() string {
	var tags []string

	if m.Role != "" {
		tags = append(tags, string(m.Role))
	}

	tags = append(tags, string(m.Provider)+" "+m.Region+" "+m.Size)

	if m.CloudID != "" {
		tags = append(tags, m.CloudID)
	}

	if m.PublicIP != "" {
		tags = append(tags, "PublicIP="+m.PublicIP)
	}

	if m.PrivateIP != "" {
		tags = append(tags, "PrivateIP="+m.PrivateIP)
	}

	if m.DiskSize != 0 {
		tags = append(tags, fmt.Sprintf("Disk=%dGB", m.DiskSize))
	}

	if m.Connected {
		tags = append(tags, "Connected")
	}

	return fmt.Sprintf("Machine-%d{%s}", m.ID, strings.Join(tags, ", "))
}

func (m Machine) less(arg row) bool {
	l, r := m, arg.(Machine)
	upl := l.PublicIP != "" && l.PrivateIP != ""
	upr := r.PublicIP != "" && r.PrivateIP != ""
	downl := l.PublicIP == "" && l.PrivateIP == ""
	downr := r.PublicIP == "" && r.PrivateIP == ""

	switch {
	case l.Role != r.Role:
		return l.Role == Master || r.Role == ""
	case upl != upr:
		return upl
	case downl != downr:
		return !downl
	case l.ID != r.ID:
		return l.ID < r.ID
	default:
		return l.CloudID < r.CloudID
	}
}

// SortMachines returns a slice of machines sorted according to the default database
// sort order.
func SortMachines(machines []Machine) []Machine {
	rows := make([]row, 0, len(machines))
	for _, m := range machines {
		rows = append(rows, m)
	}

	sort.Sort(rowSlice(rows))

	machines = make([]Machine, 0, len(machines))
	for _, r := range rows {
		machines = append(machines, r.(Machine))
	}

	return machines
}

// MachineSlice is an alias for []Machine to allow for joins
type MachineSlice []Machine

// Get returns the value contained at the given index
func (ms MachineSlice) Get(ii int) interface{} {
	return ms[ii]
}

// Len returns the number of items in the slice
func (ms MachineSlice) Len() int {
	return len(ms)
}
