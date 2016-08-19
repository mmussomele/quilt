package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/pb"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/engine"
	"github.com/NetSys/quilt/stitch"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
)

type server struct {
	dbConn     db.Conn
	stitchChan chan string
}

// Run accepts incoming `quiltctl` connections and responds to them.
func Run(conn db.Conn, listenAddr string) error {
	proto, addr, err := api.ParseListenAddress(listenAddr)
	if err != nil {
		return err
	}

	var sock net.Listener
	apiServer := server{conn, make(chan string, 1)}
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

	go func() {
		for spec := range apiServer.stitchChan {
			compiledSpec, err := stitch.New(spec, stitch.DefaultImportGetter)
			if err != nil {
				log.WithError(err).WithField("spec",
					spec).Error("Failed to compile")
				continue
			}

			err = engine.UpdatePolicy(apiServer.dbConn, compiledSpec)
			if err != nil {
				log.WithError(err).Error("Failed to update policy")
			}
		}
	}()

	s := grpc.NewServer()
	pb.RegisterAPIServer(s, apiServer)
	s.Serve(sock)

	return nil
}

func (s server) Query(cts context.Context, query *pb.DBQuery) (*pb.QueryReply, error) {
	var rows interface{}
	err := s.dbConn.Transact(func(view db.Database) error {
		switch db.TableType(query.Table) {
		case db.MachineTable:
			rows = view.SelectFromMachine(nil)
		case db.ContainerTable:
			rows = view.SelectFromContainer(nil)
		case db.EtcdTable:
			rows = view.SelectFromEtcd(nil)
		default:
			return fmt.Errorf("unrecognized table: %s", query.Table)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	json, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}

	return &pb.QueryReply{TableContents: string(json)}, nil
}

// XXX: Right now, this just sends the stitch to a channel to be compiled since a stitch
// can take longer than the timeout to compile, so it's rather hard to test. We should
// rework it to be able to provide feedback on the actual compiling of the stitch.
func (s server) Run(cts context.Context, runReq *pb.RunRequest) (*pb.RunReply, error) {
	s.stitchChan <- runReq.Stitch
	return &pb.RunReply{}, nil
}
