package supervisor

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/minion/docker"
	"github.com/vishvananda/netlink"

	log "github.com/Sirupsen/logrus"
)

const (
	// Etcd is the name etcd cluster store container.
	Etcd = "etcd"

	// Ovncontroller is the name of the OVN controller container.
	Ovncontroller = "ovn-controller"

	// Ovnnorthd is the name of the OVN northd container.
	Ovnnorthd = "ovn-northd"

	// Ovsdb is the name of the OVSDB container.
	Ovsdb = "ovsdb-server"

	// Ovsvswitchd is the name of the ovs-vswitchd container.
	Ovsvswitchd = "ovs-vswitchd"

	// Registry is the name of the registry container.
	Registry = "registry"
)

const ovsImage = "quilt/ovs"

// The tunneling protocol to use between machines.
// "stt" and "geneve" are supported.
const tunnelingProtocol = "stt"

var images = map[string]string{
	Etcd:          "quay.io/coreos/etcd:v3.0.2",
	Ovncontroller: ovsImage,
	Ovnnorthd:     ovsImage,
	Ovsdb:         ovsImage,
	Ovsvswitchd:   ovsImage,
	Registry:      "registry:2",
}

const etcdHeartbeatInterval = "500"
const etcdElectionTimeout = "5000"

var conn db.Conn
var dk docker.Client
var role db.Role
var oldEtcdIPs []string
var oldIP string

// Run blocks implementing the supervisor module.
func Run(_conn db.Conn, _dk docker.Client, _role db.Role) {
	conn = _conn
	dk = _dk
	role = _role

	imageSet := map[string]struct{}{}
	for _, image := range images {
		imageSet[image] = struct{}{}
	}

	for image := range imageSet {
		go dk.Pull(image)
	}

	switch role {
	case db.Master:
		runMaster()
	case db.Worker:
		runWorker()
	}
}

// run calls out to the Docker client to run the container specified by name.
func run(name string, args ...string) {
	log.Debugf("Entering run for container: %s", name)
	isRunning, err := dk.IsRunning(name)
	if err != nil {
		log.WithError(err).Warnf("could not check running status of %s.", name)
		return
	}
	if isRunning {
		log.Debugf("Container %s is already running.", name)
		return
	}

	ro := docker.RunOptions{
		Name:        name,
		Image:       images[name],
		Args:        args,
		NetworkMode: "host",
		VolumesFrom: []string{"minion"},
	}

	if name == Ovsvswitchd {
		ro.Privileged = true
	}

	log.Infof("Start Container: %s", name)
	_, err = dk.Run(ro)
	if err != nil {
		log.WithError(err).Warnf("Failed to run %s.", name)
	}
}

// Remove removes the docker container specified by name.
func Remove(name string) {
	log.WithField("name", name).Info("Removing container")
	err := dk.Remove(name)
	if err != nil && err != docker.ErrNoSuchContainer {
		log.WithError(err).Warnf("Failed to remove %s.", name)
	}
}

func initialClusterString(etcdIPs []string) string {
	var initialCluster []string
	for _, ip := range etcdIPs {
		initialCluster = append(initialCluster,
			fmt.Sprintf("%s=http://%s:2380", nodeName(ip), ip))
	}
	return strings.Join(initialCluster, ",")
}

func nodeName(IP string) string {
	return fmt.Sprintf("master-%s", IP)
}

// execRun() is a global variable so that it can be mocked out by the unit tests.
var execRun = func(name string, arg ...string) error {
	return exec.Command(name, arg...).Run()
}

var linkByName = netlink.LinkByName
var linkSetUp = netlink.LinkSetUp
var addrAdd = netlink.AddrAdd
