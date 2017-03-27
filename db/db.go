package db

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// The Database is the central storage location for all state in the system.  The policy
// engine populates the database with a preferred state of the world, while various
// modules flesh out that policy with actual implementation details.
type Database struct {
	tables  map[TableType]*table
	idAlloc *idCounter
}

type row interface {
	less(row) bool
	String() string
	getID() int
}

// A Conn is a database handle on which Transactions may be created.
type Conn struct {
	db        Database
	callbacks *callbackList
}

// A Transaction is a database handle on which transactions may be executed.
type Transaction struct {
	db Database
}

// An idCounter is a wrapper around the global DB id providing concurrency safe use
type idCounter struct {
	sync.Mutex
	curID int
}

// A callbackList is a wrapper around a list of callbacks providing concurrency safe use
type callbackList struct {
	sync.Mutex
	list []*Callback
}

// New creates a connection to a brand new database.
func New() Conn {
	db := Database{make(map[TableType]*table), &idCounter{}}
	for _, t := range AllTables {
		db.tables[t] = newTable(t)
	}

	cn := Conn{db: db, callbacks: &callbackList{}}
	cn.runLogger()
	go cn.runCallbackTimer()
	return cn
}

// Txn creates a new Transaction object connected to the same database, but with
// restricted access to only the given tables.
func (cn Conn) Txn(tables ...TableType) Transaction {
	// The Transaction has the same database data, just a subset of the tables.
	db := Database{make(map[TableType]*table), cn.db.idAlloc}
	for _, t := range tables {
		db.tables[t] = cn.db.accessTable(t)
	}

	return Transaction{db: db}
}

// Run executes database transactions.  It takes a closure, 'do', which is operates
// on its 'db' argument.  Transactions may be concurrent, but only if they operate on
// independent sets of tables. Otherwise, each transaction runs sequentially on it's
// database without conflicting with other transactions.
func (tr Transaction) Run(do func(db Database) error) error {
	tr.lockTables()
	defer tr.unlockTables()

	err := do(tr.db)
	var alertTables []*table
	for _, table := range tr.db.tables {
		if table.shouldAlert {
			alertTables = append(alertTables, table)
			table.shouldAlert = false
		}
	}

	for _, table := range alertTables {
		table.alert()
	}
	return err
}

// Lock all tables needed by the Transaction to perform a transact. Locking tables in
// sorted order avoids deadlock between two transactionss requesting intersecting sets of
// tables.
func (tr Transaction) lockTables() {
	tables := tableSlice{}
	for tt := range tr.db.tables {
		tables = append(tables, tt)
	}
	sort.Sort(tables)

	for _, tt := range tables {
		tr.db.tables[tt].Lock()
	}
}

// Unlock all tables needed by the Transaction to perform a transact. Unlock order is
// irrelevant.
func (tr Transaction) unlockTables() {
	for _, t := range tr.db.tables {
		t.Unlock()
	}
}

func (db Database) insert(r row) {
	table := db.accessTable(getTableType(r))
	table.shouldAlert = true
	table.rows[r.getID()] = r
}

// Commit updates the database with the data contained in row.
func (db Database) Commit(r row) {
	rid := r.getID()
	table := db.accessTable(getTableType(r))
	old := table.rows[rid]

	if reflect.TypeOf(old) != reflect.TypeOf(r) {
		panic("Type Error")
	}

	if table.shouldAlert || !reflect.DeepEqual(r, old) {
		table.rows[rid] = r
		table.shouldAlert = true
	}
}

// Remove deletes row from the database.
func (db Database) Remove(r row) {
	table := db.accessTable(getTableType(r))
	delete(table.rows, r.getID())
	table.shouldAlert = true
}

func (db Database) nextID() int {
	db.idAlloc.Lock()
	defer db.idAlloc.Unlock()

	db.idAlloc.curID++
	return db.idAlloc.curID
}

// There is no need to lock the DB when accessing tables, since each db has a
// separate map that it reads from, and they are never written to except at creation.
// The only thing that gets written to are the db tables, but those get locked before
// use, and this function can only access locked tables anyway.
func (db Database) accessTable(tt TableType) *table {
	dbTable, ok := db.tables[tt]
	if !ok {
		panic("No access to table: " + tt)
	}

	return dbTable
}

type tableSlice []TableType

func (tables tableSlice) Len() int {
	return len(tables)
}

func (tables tableSlice) Swap(i, j int) {
	tables[i], tables[j] = tables[j], tables[i]
}

func (tables tableSlice) Less(i, j int) bool {
	return tables[i] < tables[j]
}

type rowSlice []row

func (rows rowSlice) Len() int {
	return len(rows)
}

func (rows rowSlice) Swap(i, j int) {
	rows[i], rows[j] = rows[j], rows[i]
}

func (rows rowSlice) Less(i, j int) bool {
	return rows[i].less(rows[j])
}

func defaultString(r row) string {
	trow := reflect.TypeOf(r)
	vrow := reflect.ValueOf(r)

	var tags []string
	for i := 0; i < trow.NumField(); i++ {
		formatString := trow.Field(i).Tag.Get("rowStringer")
		if trow.Field(i).Name == "ID" || formatString == "omit" {
			continue
		}
		if formatString == "" {
			formatString = fmt.Sprintf("%s=%%s", trow.Field(i).Name)
		}
		fieldString := fmt.Sprint(vrow.Field(i).Interface())
		if fieldString == "" || fieldString == "0" {
			continue
		}
		tags = append(tags, fmt.Sprintf(formatString, fieldString))
	}

	id := vrow.FieldByName("ID").Int()
	return fmt.Sprintf("%s-%d{%s}", trow.Name(), id, strings.Join(tags, ", "))
}

func getTableType(r row) TableType {
	return TableType(reflect.TypeOf(r).String())
}
