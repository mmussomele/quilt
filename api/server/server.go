package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/pb"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/ipdef"
	"github.com/NetSys/quilt/stitch"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
)

type server struct{}

// Run accepts incoming `quiltctl` connections and responds to them.
func Run(listenAddr string) error {
	proto, addr, err := api.ParseListenAddress(listenAddr)
	if err != nil {
		return err
	}

	var sock net.Listener
	apiServer := server{}
	for {
		sock, err = net.Listen(proto, addr)

		if err == nil {
			break
		}
		log.WithError(err).Error("Failed to open socket.")

		time.Sleep(30 * time.Second)
	}

	// Cleanup the socket if we're interrupted.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGHUP)
	go func(c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.\n", sig)
		sock.Close()
		os.Exit(0)
	}(sigc)

	s := grpc.NewServer()
	pb.RegisterAPIServer(s, apiServer)
	s.Serve(sock)

	return nil
}

func (s server) Query(cts context.Context, query *pb.DBQuery) (reply *pb.QueryReply,
	err error) {
	defer func() {
		// If the queried table is not valid, db.Open will panic.
		if r := recover(); r != nil {
			reply = nil
			err = fmt.Errorf("unrecognized table: %s", query.Table)
		}
	}()

	var rows interface{}
	// Only one table type will ever be accessed, so we can just open a connection
	// to that table.
	conn := db.Open(db.TableType(query.Table))
	conn.Transact(func(view db.Database) error {
		switch db.TableType(query.Table) {
		case db.MachineTable:
			rows = view.SelectFromMachine(nil)
		case db.ContainerTable:
			rows = view.SelectFromContainer(nil)
		case db.EtcdTable:
			rows = view.SelectFromEtcd(nil)
		case db.ConnectionTable:
			rows = view.SelectFromConnection(nil)
		case db.LabelTable:
			rows = view.SelectFromLabel(nil)
		case db.ClusterTable:
			rows = view.SelectFromCluster(nil)
		default:
			panic("not reached")
		}
		return nil
	})

	json, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}

	return &pb.QueryReply{TableContents: string(json)}, nil
}

func (s server) Deploy(cts context.Context, deployReq *pb.DeployRequest) (
	*pb.DeployReply, error) {

	stitch, err := stitch.FromJSON(deployReq.Deployment)
	if err != nil {
		return &pb.DeployReply{}, err
	}

	if len(stitch.Machines) > ipdef.MaxMinionCount {
		return &pb.DeployReply{}, fmt.Errorf("cannot boot more than %d "+
			"machines", ipdef.MaxMinionCount)
	}

	err = db.Open(db.ClusterTable).Transact(func(view db.Database) error {
		cluster, err := view.GetCluster()
		if err != nil {
			cluster = view.InsertCluster()
		}

		cluster.Spec = stitch.String()
		view.Commit(cluster)
		return nil
	})
	if err != nil {
		return &pb.DeployReply{}, err
	}

	// XXX: Remove this error when the Vagrant provider is done.
	for _, machine := range stitch.Machines {
		if machine.Provider == db.Vagrant {
			err = errors.New("The Vagrant provider is in development." +
				" The stitch will continue to run, but" +
				" probably won't work correctly.")
			return &pb.DeployReply{}, err
		}
	}

	return &pb.DeployReply{}, nil
}
