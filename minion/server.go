package minion

import (
	"net"
	"sort"
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/pb"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
)

type server struct {
	db.Conn
	minion *db.Minion
}

func minionServerRun(conn db.Conn) {
	var sock net.Listener
	server := server{conn, &db.Minion{}}
	for {
		var err error
		sock, err = net.Listen("tcp", ":9999")
		if err != nil {
			log.WithError(err).Error("Failed to open socket.")
		} else {
			break
		}

		time.Sleep(30 * time.Second)
	}

	s := grpc.NewServer()
	pb.RegisterMinionServer(s, server)
	s.Serve(sock)
}

func (s server) GetMinionConfig(cts context.Context,
	_ *pb.Request) (*pb.MinionConfig, error) {

	var cfg pb.MinionConfig
	if s.minion.Self {
		cfg.Role = db.RoleToPB(s.minion.Role)
		cfg.PrivateIP = s.minion.PrivateIP
		cfg.PublicIP = s.minion.PublicIP
		cfg.Spec = s.minion.Spec
		cfg.Provider = s.minion.Provider
		cfg.Size = s.minion.Size
		cfg.Region = s.minion.Region
	} else {
		cfg.Role = db.RoleToPB(db.None)
	}

	return &cfg, nil
}

func (s server) SetMinionConfig(ctx context.Context,
	msg *pb.MinionConfig) (*pb.Reply, error) {
	go s.Transact(func(view db.Database) error {
		minion, err := view.MinionSelf()
		if err != nil {
			log.Info("Received initial configuation.")
			minion = view.InsertMinion()
		}

		s.minion.ID = minion.ID
		s.minion.Role = db.PBToRole(msg.Role)
		s.minion.PrivateIP = msg.PrivateIP
		s.minion.PublicIP = msg.PublicIP
		s.minion.Spec = msg.Spec
		s.minion.Provider = msg.Provider
		s.minion.Size = msg.Size
		s.minion.Region = msg.Region
		s.minion.Self = true
		view.Commit(*s.minion)

		return nil
	})

	return &pb.Reply{Success: true}, nil
}

func (s server) BootEtcd(ctx context.Context,
	members *pb.EtcdMembers) (*pb.Reply, error) {
	go s.Transact(func(view db.Database) error {
		etcdSlice := view.SelectFromEtcd(nil)
		var etcdRow db.Etcd
		switch len(etcdSlice) {
		case 0:
			log.Info("Received boot etcd request.")
			etcdRow = view.InsertEtcd()
		case 1:
			etcdRow = etcdSlice[0]
		default:
			panic("Not Reached")
		}

		etcdRow.EtcdIPs = members.IPs
		sort.Strings(etcdRow.EtcdIPs)
		view.Commit(etcdRow)

		return nil
	})

	return &pb.Reply{Success: true}, nil
}
