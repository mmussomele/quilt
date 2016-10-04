package cluster

import (
	"errors"
	"time"

	"github.com/NetSys/quilt/cluster/acl"
	"github.com/NetSys/quilt/cluster/amazon"
	"github.com/NetSys/quilt/cluster/foreman"
	"github.com/NetSys/quilt/cluster/google"
	"github.com/NetSys/quilt/cluster/machine"
	"github.com/NetSys/quilt/cluster/vagrant"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/util"
	log "github.com/Sirupsen/logrus"
)

type provider interface {
	List() ([]machine.Machine, error)

	Boot([]machine.Machine) error

	Stop([]machine.Machine) error

	SetACLs([]acl.ACL) error
}

// Store the providers in a variable so we can change it in the tests
var allProviders = []db.Provider{db.Amazon, db.Google, db.Vagrant}

type cluster struct {
	namespace string
	conn      db.Conn
	providers map[db.Provider]provider
}

var myIP = util.MyIP
var sleep = time.Sleep

// Run continually checks 'conn' for cluster changes and recreates the cluster as
// needed.
func Run(conn db.Conn) {
	var clst *cluster
	clusterConn := conn.Restrict(db.ClusterTable, db.MachineTable, db.ACLTable)
	for range clusterConn.TriggerTick(30).C {
		clst = updateCluster(conn, clst)

		// Somewhat of a crude rate-limit of once every five seconds to avoid
		// stressing out the cloud providers with too many API calls.
		sleep(5 * time.Second)
	}
}

func updateCluster(conn db.Conn, clst *cluster) *cluster {
	namespace, err := conn.GetClusterNamespace()
	if err != nil {
		return clst
	}

	if clst == nil || clst.namespace != namespace {
		clst = newCluster(conn, namespace)
		clst.runOnce()
		foreman.Init(clst.conn)
	}

	clst.runOnce()
	foreman.RunOnce(clst.conn)

	return clst
}

func newCluster(conn db.Conn, namespace string) *cluster {
	clst := &cluster{
		namespace: namespace,
		conn:      conn,
		providers: make(map[db.Provider]provider),
	}

	for _, p := range allProviders {
		prvdr, err := newProvider(p, namespace)
		if err != nil {
			log.Debugf("Failed to connect to provider %s: %s", p, err)
		} else {
			clst.providers[p] = prvdr
		}
	}

	return clst
}

func (clst cluster) runOnce() {
	/* Each iteration of this loop does the following:
	 *
	 * - Get the current set of machines and ACLs from the cloud provider.
	 * - Get the current policy from the database.
	 * - Compute a diff.
	 * - Update the cloud provider accordingly.
	 *
	 * Updating the cloud provider may have consequences (creating machines for
	 * instances) that should be reflected in the database.  Therefore, if updates
	 * are necessary the code loops so that database can be updated before the next
	 * runOnce() call.  Once the loop as converged, it then updates the cluster ACLs
	 * before finally exiting. */
	for i := 0; i < 2; i++ {
		jr, err := clst.join()
		if err != nil {
			return
		}

		if len(jr.boot) == 0 && len(jr.terminate) == 0 {
			// ACLs must be processed after Quilt learns about what machines
			// are in the cloud.  If we didn't, inter-machine ACLs could get
			// removed when the Quilt controller restarts, even if there are
			// running cloud machines that still need to communicate.
			clst.syncACLs(jr.acl.Admin, jr.acl.ApplicationPorts, jr.machines)
			return
		}

		clst.updateCloud(jr.boot, true)
		clst.updateCloud(jr.terminate, false)
	}
}

func (clst cluster) updateCloud(machines []machine.Machine, boot bool) {
	if len(machines) == 0 {
		return
	}

	actionString := "halt"
	if boot {
		actionString = "boot"
	}

	log.WithField("count", len(machines)).
		Infof("Attempt to %s machines.", actionString)

	noFailures := true
	groupedMachines := groupBy(machines)
	for p, providerMachines := range groupedMachines {
		providerInst, ok := clst.providers[p]
		if !ok {
			noFailures = false
			log.Warnf("Provider %s is unavailable.", p)
			continue
		}
		var err error
		if boot {
			err = providerInst.Boot(providerMachines)
		} else {
			err = providerInst.Stop(providerMachines)
		}
		if err != nil {
			noFailures = false
			log.WithError(err).
				Warnf("Unable to %s machines on %s.", actionString, p)
		}
	}

	if noFailures {
		log.Infof("Successfully %sed machines.", actionString)
	} else {
		log.Infof("Due to failures, sleeping for 1 minute")
		sleep(60 * time.Second)
	}
}

type joinResult struct {
	machines []db.Machine
	acl      db.ACL

	boot      []machine.Machine
	terminate []machine.Machine
}

func (clst cluster) join() (joinResult, error) {
	res := joinResult{}

	cloudMachines, err := clst.get()
	if err != nil {
		log.WithError(err).Error("Failed to list machines")
		return res, err
	}

	err = clst.conn.Transact(func(view db.Database) error {
		namespace, err := view.GetClusterNamespace()
		if err != nil {
			log.WithError(err).Error("Failed to get namespace")
			return err
		}

		if clst.namespace != namespace {
			err := errors.New("namespace change during a cluster run")
			log.WithError(err).Debug("Cluster run abort")
			return err
		}

		res.acl, err = view.GetACL()
		if err != nil {
			log.WithError(err).Error("Failed to get ACLs")
		}

		res.machines = view.SelectFromMachine(nil)

		var pairs []join.Pair
		pairs, res.boot, res.terminate = syncDB(cloudMachines, res.machines)
		for _, pair := range pairs {
			dbm := pair.L.(db.Machine)
			m := pair.R.(machine.Machine)

			dbm.CloudID = m.ID
			dbm.PublicIP = m.PublicIP
			dbm.PrivateIP = m.PrivateIP

			// We just booted the machine, can't possibly be connected.
			if dbm.PublicIP == "" {
				dbm.Connected = false
			}

			// If we overwrite the machine's size before the machine has
			// fully booted, the Stitch will flip it back immediately.
			if m.Size != "" {
				dbm.Size = m.Size
			}
			if m.DiskSize != 0 {
				dbm.DiskSize = m.DiskSize
			}
			dbm.Provider = m.Provider
			view.Commit(dbm)
		}
		return nil
	})
	return res, err
}

func (clst cluster) syncACLs(adminACLs []string, appACLs []db.PortRange,
	machines []db.Machine) {

	// Always allow traffic from the Quilt controller.
	ip, err := myIP()
	if err == nil {
		adminACLs = append(adminACLs, ip+"/32")
	} else {
		log.WithError(err).Error("Couldn't retrieve our IP address.")
	}

	var acls []acl.ACL
	for _, adminACL := range adminACLs {
		acls = append(acls, acl.ACL{
			CidrIP:  adminACL,
			MinPort: 1,
			MaxPort: 65535,
		})
	}
	for _, appACL := range appACLs {
		acls = append(acls, acl.ACL{
			CidrIP:  "0.0.0.0/0",
			MinPort: appACL.MinPort,
			MaxPort: appACL.MaxPort,
		})
	}

	// Providers with at least one machine.
	prvdrSet := map[db.Provider]struct{}{}
	for _, m := range machines {
		if m.PublicIP != "" {
			// XXX: Look into the minimal set of necessary ports.
			acls = append(acls, acl.ACL{
				CidrIP:  m.PublicIP + "/32",
				MinPort: 1,
				MaxPort: 65535,
			})
		}
		prvdrSet[m.Provider] = struct{}{}
	}

	for name, prvdr := range clst.providers {
		// For this providers with no specified machines, we remove all ACLs.
		// Otherwise we set acls to what's specified.
		var setACLs []acl.ACL
		if _, ok := prvdrSet[name]; ok {
			setACLs = acls
		}

		if err := prvdr.SetACLs(setACLs); err != nil {
			log.WithError(err).Warnf("Could not update ACLs on %s.", name)
		}
	}
}

func syncDB(cloudMachines []machine.Machine, dbMachines []db.Machine) (
	pairs []join.Pair, bootSet []machine.Machine, terminateSet []machine.Machine) {
	scoreFun := func(left, right interface{}) int {
		dbm := left.(db.Machine)
		m := right.(machine.Machine)

		switch {
		case dbm.Provider != m.Provider:
			return -1
		case m.Region != "" && dbm.Region != m.Region:
			return -1
		case m.Size != "" && dbm.Size != m.Size:
			return -1
		case m.DiskSize != 0 && dbm.DiskSize != m.DiskSize:
			return -1
		case dbm.CloudID == m.ID:
			return 0
		case dbm.PublicIP == m.PublicIP:
			return 1
		case dbm.PrivateIP == m.PrivateIP:
			return 2
		default:
			return 3
		}
	}

	pairs, dbmIface, cmIface := join.Join(dbMachines, cloudMachines, scoreFun)

	for _, cm := range cmIface {
		m := cm.(machine.Machine)
		terminateSet = append(terminateSet, m)
	}

	for _, dbm := range dbmIface {
		m := dbm.(db.Machine)
		bootSet = append(bootSet, machine.Machine{
			Size:     m.Size,
			Provider: m.Provider,
			Region:   m.Region,
			DiskSize: m.DiskSize,
			SSHKeys:  m.SSHKeys})
	}

	return pairs, bootSet, terminateSet
}

func (clst cluster) get() ([]machine.Machine, error) {
	var cloudMachines []machine.Machine
	for _, p := range clst.providers {
		providerMachines, err := p.List()
		if err != nil {
			return []machine.Machine{}, err
		}
		cloudMachines = append(cloudMachines, providerMachines...)
	}
	return cloudMachines, nil
}

func groupBy(machines []machine.Machine) map[db.Provider][]machine.Machine {
	machineMap := make(map[db.Provider][]machine.Machine)
	for _, m := range machines {
		if _, ok := machineMap[m.Provider]; !ok {
			machineMap[m.Provider] = []machine.Machine{}
		}
		machineMap[m.Provider] = append(machineMap[m.Provider], m)
	}

	return machineMap
}

func newProviderImpl(p db.Provider, namespace string) (provider, error) {
	switch p {
	case db.Amazon:
		return amazon.New(namespace)
	case db.Google:
		return google.New(namespace)
	case db.Vagrant:
		return vagrant.New(namespace)
	default:
		panic("Unimplemented")
	}
}

// Stored in a variable so it may be mocked out
var newProvider = newProviderImpl
