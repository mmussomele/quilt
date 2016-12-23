package db

import (
	"fmt"
	"sort"
	"strings"

	"github.com/NetSys/quilt/util"
)

// A Container row is created for each container specified by the policy.  Each row will
// eventually be instantiated within its corresponding cluster.
// Used only by the minion.
type Container struct {
	ID int // The database ID of this container.

	Pid        int    // The PID of the running container.
	IP         string // The virtual IP address of this container.
	Mac        string // The virtual MAC address of this container.
	Minion     string // The private IP of the minion this container is running on.
	EndpointID string // The ID libnetwork has assigned to this container's veth.
	StitchID   int    // A unique ID given to this container by the stitch compiler.
	DockerID   string
	Image      string
	Command    []string
	Labels     []string
	Env        map[string]string
}

// ContainerSlice is an alias for []Container to allow for joins
type ContainerSlice []Container

// InsertContainer creates a new container row and inserts it into the database.
func (db Database) InsertContainer() Container {
	result := Container{ID: db.nextID()}
	db.insert(result)
	return result
}

// SelectFromContainer gets all containers in the database that satisfy 'check'.
func (db Database) SelectFromContainer(check func(Container) bool) []Container {
	containerTable := db.accessTable(ContainerTable)
	var result []Container
	for _, row := range containerTable.rows {
		if check == nil || check(row.(Container)) {
			result = append(result, row.(Container))
		}
	}

	return result
}

// SelectFromContainer gets all containers in the database that satisfy the 'check'.
func (conn Conn) SelectFromContainer(check func(Container) bool) []Container {
	var containers []Container
	conn.Txn(ContainerTable).Run(func(view Database) error {
		containers = view.SelectFromContainer(check)
		return nil
	})
	return containers
}

func (c Container) getID() int {
	return c.ID
}

func (c Container) String() string {
	cmdStr := strings.Join(append([]string{"run", c.Image}, c.Command...), " ")
	tags := []string{cmdStr}

	if c.DockerID != "" {
		id := util.ShortUUID(c.DockerID)
		tags = append(tags, fmt.Sprintf("DockerID: %s", id))
	}

	if c.Minion != "" {
		tags = append(tags, fmt.Sprintf("Minion: %s", c.Minion))
	}

	if c.StitchID != 0 {
		tags = append(tags, fmt.Sprintf("StitchID: %d", c.StitchID))
	}

	if c.Pid != 0 {
		tags = append(tags, fmt.Sprintf("Pid: %d", c.Pid))
	}

	if c.IP != "" {
		tags = append(tags, fmt.Sprintf("IP: %s", c.IP))
	}

	if c.Mac != "" {
		tags = append(tags, fmt.Sprintf("Mac: %s", c.Mac))
	}

	if len(c.Labels) > 0 {
		tags = append(tags, fmt.Sprintf("Labels: %s", c.Labels))
	}

	if len(c.Env) > 0 {
		tags = append(tags, fmt.Sprintf("Env: %s", c.Env))
	}

	return fmt.Sprintf("Container-%d{%s}", c.ID, strings.Join(tags, ", "))
}

// SortContainers returns a slice of containers sorted according to the default database
// sort order.
func SortContainers(containers []Container) []Container {
	rows := make([]row, 0, len(containers))
	for _, m := range containers {
		rows = append(rows, m)
	}

	sort.Sort(rowSlice(rows))

	containers = make([]Container, 0, len(containers))
	for _, r := range rows {
		containers = append(containers, r.(Container))
	}

	return containers
}

func (c Container) less(r row) bool {
	return c.StitchID < r.(Container).StitchID
}

// Get returns the value contained at the given index
func (cs ContainerSlice) Get(ii int) interface{} {
	return cs[ii]
}

// Len returns the number of items in the slice
func (cs ContainerSlice) Len() int {
	return len(cs)
}
