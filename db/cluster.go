package db

import (
	"errors"
	"log"
)

// A Cluster is a group of Machines which can operate containers.
type Cluster struct {
	ID int

	Namespace string // Cloud Provider Namespace
	Spec      string `rowStringer:"omit"`
}

// InsertCluster creates a new Cluster and interts it into 'db'.
func (db Database) InsertCluster() Cluster {
	result := Cluster{ID: db.nextID()}
	db.insert(result)
	return result
}

// SelectFromCluster gets all clusters in the database that satisfy 'check'.
func (db Database) SelectFromCluster(check func(Cluster) bool) []Cluster {
	clusterTable := db.accessTable(ClusterTable)
	result := []Cluster{}
	for _, row := range clusterTable.rows {
		if check == nil || check(row.(Cluster)) {
			result = append(result, row.(Cluster))
		}
	}
	return result
}

// GetCluster gets the cluster from the database. There should only ever be a single
// cluster.
func (db Database) GetCluster() (Cluster, error) {
	clusters := db.SelectFromCluster(nil)
	numClusters := len(clusters)
	if numClusters == 1 {
		return clusters[0], nil
	} else if numClusters > 1 {
		log.Panicf("Found %d clusters, there should be 1", numClusters)
	}
	return Cluster{}, errors.New("no clusters found")
}

// GetClusterNamespace returns the namespace of the single cluster object in the cluster
// table.  Otherwise it returns an error.
func (db Database) GetClusterNamespace() (string, error) {
	clst, err := db.GetCluster()
	if err != nil {
		return "", err
	}
	return clst.Namespace, nil
}

// GetClusterNamespace returns the namespace of the single cluster object in the cluster
// table.  Otherwise it returns an error.
func (conn Conn) GetClusterNamespace() (namespace string, err error) {
	conn.Transact(func(db Database) error {
		namespace, err = db.GetClusterNamespace()
		return nil
	})
	return
}

func (c Cluster) getID() int {
	return c.ID
}

func (c Cluster) tt() TableType {
	return ClusterTable
}

func (c Cluster) String() string {
	return defaultString(c)
}

func (c Cluster) less(r row) bool {
	return c.ID < r.(Cluster).ID
}
