package db

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestMachine(t *testing.T) {
	conn := New()

	var m Machine
	err := conn.Transact(func(db Database) error {
		m = db.InsertMachine()
		return nil
	})
	if err != nil {
		t.FailNow()
	}

	if m.ID != 1 || m.Role != None || m.CloudID != "" || m.PublicIP != "" ||
		m.PrivateIP != "" {
		t.Errorf("Invalid Machine: %s", spew.Sdump(m))
		return
	}

	old := m

	m.Role = Worker
	m.CloudID = "something"
	m.PublicIP = "1.2.3.4"
	m.PrivateIP = "5.6.7.8"

	err = conn.Transact(func(db Database) error {
		if err := SelectMachineCheck(db, nil, []Machine{old}); err != nil {
			return err
		}

		db.Commit(m)

		if err := SelectMachineCheck(db, nil, []Machine{m}); err != nil {
			return err
		}

		db.Remove(m)

		if err := SelectMachineCheck(db, nil, []Machine{}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
		return
	}
}

func TestMachineSelect(t *testing.T) {
	conn := New()
	regions := []string{"here", "there", "anywhere", "everywhere"}

	var machines []Machine
	conn.Transact(func(db Database) error {
		for i := 0; i < 4; i++ {
			m := db.InsertMachine()
			m.Region = regions[i]
			db.Commit(m)
			machines = append(machines, m)
		}
		return nil
	})

	err := conn.Transact(func(db Database) error {
		err := SelectMachineCheck(db, func(m Machine) bool {
			return m.Region == "there"
		}, []Machine{machines[1]})
		if err != nil {
			return err
		}

		err = SelectMachineCheck(db, func(m Machine) bool {
			return m.Region != "there"
		}, []Machine{machines[0], machines[2], machines[3]})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
		return
	}
}

func TestMachineString(t *testing.T) {
	m := Machine{}

	got := m.String()
	exp := "Machine-0{  }"
	if got != exp {
		t.Errorf("\nGot: %s\nExp: %s", got, exp)
	}

	m = Machine{
		ID:        1,
		CloudID:   "CloudID1234",
		Provider:  "Amazon",
		Region:    "us-west-1",
		Size:      "m4.large",
		PublicIP:  "1.2.3.4",
		PrivateIP: "5.6.7.8",
		DiskSize:  56,
		Connected: true,
	}
	got = m.String()
	exp = "Machine-1{Amazon us-west-1 m4.large, CloudID1234, PublicIP=1.2.3.4," +
		" PrivateIP=5.6.7.8, Disk=56GB, Connected}"
	if got != exp {
		t.Errorf("\nGot: %s\nExp: %s", got, exp)
	}
}

func TestRestrictConnBasic(t *testing.T) {
	conn := New()
	conn.Transact(func(view Database) error {
		m := view.InsertMachine()
		m.Provider = "Amazon"
		view.Commit(m)

		return nil
	})

	conn.Restrict(MachineTable).Transact(func(view Database) error {
		machines := view.SelectFromMachine(func(m Machine) bool {
			return true
		})

		if len(machines) != 1 {
			t.Fatal("No machines in DB, should be 1")
		}
		if machines[0].Provider != "Amazon" {
			t.Fatal("Machine provider is not Amazon")
		}

		return nil
	})
}

// Regular connections have access to all tables of the database
func TestConnNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("Conn panicked on valid transaction")
		}
	}()

	conn := New()
	conn.Transact(func(view Database) error {
		view.InsertEtcd()
		view.InsertLabel()
		view.InsertMinion()
		view.InsertMachine()
		view.InsertCluster()
		view.InsertPlacement()
		view.InsertContainer()
		view.InsertConnection()
		view.InsertACL()

		return nil
	})
}

// Connections should not panic when accessing tables in their allowed set
func TestRestrictConnNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("Restricted conn panicked on valid transaction")
		}
	}()

	conn := New().Restrict(MachineTable, ClusterTable)
	conn.Transact(func(view Database) error {
		view.InsertMachine()
		view.InsertCluster()

		return nil
	})
}

// Connections should panic when accessing tables not in their allowed set
func TestRestrictConnPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Restricted conn didn't panic on invalid transaction")
		}
	}()

	conn := New().Restrict(MachineTable, ClusterTable)
	conn.Transact(func(view Database) error {
		view.InsertEtcd()

		return nil
	})
}

// This test and the test below cover an edge case where a restricted connection is
// restricted further. This really shouldn't be done, but it should behave correctly
// anyway.
func TestRestrictRecurseNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("Restricted conn panicked on valid restriction")
		}
	}()

	c1 := New().Restrict(MachineTable, ClusterTable)
	c1.Restrict(MachineTable) // should be fine, c1 can access MachineTable
}

func TestRestrictRecursePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Restricted conn didn't panic on invalid restriction")
		}
	}()

	c1 := New().Restrict(MachineTable, ClusterTable)
	c1.Restrict(EtcdTable) // can't restrict to a table you can't access
}

// Connections with restricted table sets should be able to run concurrently if their
// table sets do not overlap.
func TestRestrictConcurrent(t *testing.T) {
	// Run the deadlock test multiple times to increase the odds of detecting a race
	// condition
	for i := 0; i < 10; i++ {
		checkIndependentTransacts(t)
	}
}

// returns false when the transactions deadlock
func checkIndependentTransacts(t *testing.T) {
	transactOneStart := make(chan struct{})
	transactTwoDone := make(chan struct{})
	done := make(chan struct{})
	doneRoutines := make(chan struct{})
	defer close(doneRoutines)

	subConnOne, subConnTwo := getRandomRestrictedConns(New())
	one := func() {
		subConnOne.Transact(func(view Database) error {
			close(transactOneStart)
			select {
			case <-transactTwoDone:
				break
			case <-doneRoutines:
				return nil // break out of this if it times out
			}
			return nil
		})

		close(done)
	}

	two := func() {
		// Wait for either the first transact to start or for timeout
		select {
		case <-transactOneStart:
			break
		case <-doneRoutines:
			return // break out of this if it times out
		}

		subConnTwo.Transact(func(view Database) error {
			return nil
		})

		close(transactTwoDone)
	}

	go one()
	go two()
	timeout := time.After(time.Second)
	select {
	case <-timeout:
		t.Fatal("Transactions deadlocked")
	case <-done:
		return
	}
}

// Test that transactions with overlapping table sets run sequentially.
func TestRestrictSequential(t *testing.T) {
	// Run the sequential test multiple times to increase the odds of detecting a
	// race condition
	for i := 0; i < 10; i++ {
		checkRestrictSequential(t)
	}
}

func checkRestrictSequential(t *testing.T) {
	subConnOne, subConnTwo := getRandomRestrictedConns(New(),
		pickTwoTables(map[TableType]struct{}{})...)

	done := make(chan struct{})
	defer close(done)
	results := make(chan int)
	defer close(results)

	oneStarted := make(chan struct{})
	one := func() {
		subConnOne.Transact(func(view Database) error {
			close(oneStarted)
			time.Sleep(100 * time.Millisecond)
			select {
			case results <- 1:
				return nil
			case <-done:
				return nil
			}
		})
	}

	two := func() {
		subConnTwo.Transact(func(view Database) error {
			select {
			case results <- 2:
				return nil
			case <-done:
				return nil
			}
		})
	}

	check := make(chan bool)
	defer close(check)
	go func() {
		first := <-results
		second := <-results

		check <- (first == 1 && second == 2)
	}()

	go one()
	<-oneStarted
	go two()

	timeout := time.After(time.Second)
	select {
	case <-timeout:
		t.Fatal("Transactions timed out")
	case success := <-check:
		if !success {
			t.Fatal("Transactions ran concurrently")
		}
	}
}

func getRandomRestrictedConns(conn Conn, tables ...TableType) (Conn, Conn) {
	taken := map[TableType]struct{}{}
	firstTables := pickTwoTables(taken)
	secondTables := pickTwoTables(taken)

	firstTables = append(firstTables, tables...)
	secondTables = append(secondTables, tables...)

	return conn.Restrict(firstTables...), conn.Restrict(secondTables...)
}

func pickTwoTables(taken map[TableType]struct{}) []TableType {
	tableCount := int32(len(allTables))
	chosen := []TableType{}
	for len(chosen) < 2 {
		tt := allTables[rand.Int31n(tableCount)]
		if _, ok := taken[tt]; ok {
			continue
		}

		taken[tt] = struct{}{}
		chosen = append(chosen, tt)
	}

	return chosen
}

func TestTrigger(t *testing.T) {
	conn := New()
	machineConn := conn.Restrict(MachineTable)
	clusterConn := conn.Restrict(ClusterTable)

	mt := machineConn.Trigger()
	mt2 := machineConn.Trigger()
	ct := clusterConn.Trigger()
	ct2 := clusterConn.Trigger()

	triggerNoRecv(t, mt)
	triggerNoRecv(t, mt2)
	triggerNoRecv(t, ct)
	triggerNoRecv(t, ct2)

	err := conn.Transact(func(db Database) error {
		db.InsertMachine()
		return nil
	})
	if err != nil {
		t.Fail()
		return
	}

	triggerRecv(t, mt)
	triggerRecv(t, mt2)
	triggerNoRecv(t, ct)
	triggerNoRecv(t, ct2)

	mt2.Stop()
	err = conn.Transact(func(db Database) error {
		db.InsertMachine()
		return nil
	})
	if err != nil {
		t.Fail()
		return
	}
	triggerRecv(t, mt)
	triggerNoRecv(t, mt2)

	mt.Stop()
	ct.Stop()
	ct2.Stop()

	fast := machineConn.TriggerTick(1)
	triggerRecv(t, fast)
	triggerRecv(t, fast)
	triggerRecv(t, fast)
}

func TestTriggerTickStop(t *testing.T) {
	conn := New()

	mt := conn.Restrict(MachineTable).TriggerTick(100)

	// The initial tick.
	triggerRecv(t, mt)

	triggerNoRecv(t, mt)
	err := conn.Transact(func(db Database) error {
		db.InsertMachine()
		return nil
	})
	if err != nil {
		t.Fail()
		return
	}

	triggerRecv(t, mt)

	mt.Stop()
	err = conn.Transact(func(db Database) error {
		db.InsertMachine()
		return nil
	})
	if err != nil {
		t.Fail()
		return
	}
	triggerNoRecv(t, mt)
}

func triggerRecv(t *testing.T, trig Trigger) {
	select {
	case <-trig.C:
	case <-time.Tick(5 * time.Second):
		t.Error("Expected Receive")
	}
}

func triggerNoRecv(t *testing.T, trig Trigger) {
	select {
	case <-trig.C:
		t.Error("Unexpected Receive")
	case <-time.Tick(25 * time.Millisecond):
	}
}

func SelectMachineCheck(db Database, do func(Machine) bool, expected []Machine) error {
	query := db.SelectFromMachine(do)
	sort.Sort(mSort(expected))
	sort.Sort(mSort(query))
	if !reflect.DeepEqual(expected, query) {
		return fmt.Errorf("unexpected query result: %s\nExpected %s",
			spew.Sdump(query), spew.Sdump(expected))
	}

	return nil
}

type prefixedString struct {
	prefix string
	str    string
}

func (ps prefixedString) String() string {
	return ps.prefix + ps.str
}

type testStringerRow struct {
	ID         int
	FieldOne   string
	FieldTwo   int `rowStringer:"omit"`
	FieldThree int `rowStringer:"three: %s"`
	FieldFour  prefixedString
	FieldFive  int
}

func (r testStringerRow) String() string {
	return ""
}

func (r testStringerRow) getID() int {
	return -1
}

func (r testStringerRow) less(arg row) bool {
	return true
}

func TestStringer(t *testing.T) {
	testRow := testStringerRow{
		ID:         5,
		FieldOne:   "one",
		FieldThree: 3,

		// Should always omit.
		FieldTwo: 2,

		// Should evaluate String() method.
		FieldFour: prefixedString{"pre", "foo"},

		// Should omit because value is zero value.
		FieldFive: 0,
	}
	exp := "testStringerRow-5{FieldOne=one, three: 3, FieldFour=prefoo}"
	actual := defaultString(testRow)
	if exp != actual {
		t.Errorf("Bad defaultStringer output: expected %q, got %q.", exp, actual)
	}
}

func TestSortContainers(t *testing.T) {
	containers := []Container{
		{StitchID: 3},
		{StitchID: 5},
		{StitchID: 5},
		{StitchID: 1},
	}
	expected := []Container{
		{StitchID: 1},
		{StitchID: 3},
		{StitchID: 5},
		{StitchID: 5},
	}

	if !reflect.DeepEqual(SortContainers(containers), expected) {
		t.Errorf("Bad Container Sort: expected %q, got %q", expected, containers)
	}
}

func TestGetClusterNamespace(t *testing.T) {
	conn := New()

	ns, err := conn.GetClusterNamespace()
	assert.NotNil(t, err)
	assert.Exactly(t, ns, "")

	conn.Transact(func(view Database) error {
		clst := view.InsertCluster()
		clst.Namespace = "test"
		view.Commit(clst)
		return nil
	})

	ns, err = conn.GetClusterNamespace()
	assert.NoError(t, err)
	assert.Exactly(t, ns, "test")
}

type mSort []Machine

func (machines mSort) sort() {
	sort.Stable(machines)
}

func (machines mSort) Len() int {
	return len(machines)
}

func (machines mSort) Swap(i, j int) {
	machines[i], machines[j] = machines[j], machines[i]
}

func (machines mSort) Less(i, j int) bool {
	return machines[i].ID < machines[j].ID
}
