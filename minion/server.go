package minion

import (
	"net"
	"sort"
	"strings"
	"time"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/minion/pb"
	"github.com/quilt/quilt/version"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
)

type server struct {
	db.Conn
}

func minionServerRun(conn db.Conn) {
	var sock net.Listener
	server := server{conn}
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
	req *pb.Request) (*pb.MinionConfig, error) {

	var cfg pb.MinionConfig
	if !version.Compatible(req.Version) {
		return &cfg, version.NewError(req.Version)
	}

	m := s.MinionSelf()
	cfg.Role = db.RoleToPB(m.Role)
	cfg.PrivateIP = m.PrivateIP
	cfg.Spec = m.Spec
	cfg.Provider = m.Provider
	cfg.Size = m.Size
	cfg.Region = m.Region
	cfg.AuthorizedKeys = strings.Split(m.AuthorizedKeys, "\n")

	s.Txn(db.EtcdTable).Run(func(view db.Database) error {
		if etcdRow, err := view.GetEtcd(); err == nil {
			cfg.EtcdMembers = etcdRow.EtcdIPs
		}
		return nil
	})

	return &cfg, nil
}

func (s server) SetMinionConfig(ctx context.Context,
	msg *pb.MinionConfig) (*pb.Reply, error) {

	if !version.Compatible(msg.Version) {
		return &pb.Reply{}, version.NewError(msg.Version)
	}

	go s.Txn(db.EtcdTable,
		db.MinionTable).Run(func(view db.Database) error {

		minion := view.MinionSelf()
		minion.PrivateIP = msg.PrivateIP
		minion.Spec = msg.Spec
		minion.Provider = msg.Provider
		minion.Size = msg.Size
		minion.Region = msg.Region
		minion.FloatingIP = msg.FloatingIP
		minion.AuthorizedKeys = strings.Join(msg.AuthorizedKeys, "\n")
		minion.Self = true
		view.Commit(minion)

		etcdRow, err := view.GetEtcd()
		if err != nil {
			log.Info("Received boot etcd request.")
			etcdRow = view.InsertEtcd()
		}

		etcdRow.EtcdIPs = msg.EtcdMembers
		sort.Strings(etcdRow.EtcdIPs)
		view.Commit(etcdRow)

		return nil
	})
	return &pb.Reply{}, nil
}
