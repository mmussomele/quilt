package client

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/pb"
	"github.com/NetSys/quilt/db"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	// The timeout for making requests to the daemon once we've connected.
	requestTimeout = time.Minute

	// The timeout for connecting to the daemon.
	connectTimeout = 5 * time.Second
)

// Client provides methods to interact with the Quilt daemon.
type Client interface {
	// Close the grpc connection.
	Close() error

	// QueryMachines retrieves the machines tracked by the Quilt daemon.
	QueryMachines() ([]db.Machine, error)

	// QueryContainers retrieves the containers tracked by the Quilt daemon.
	QueryContainers() ([]db.Container, error)

	// QueryEtcd retrieves the etcd information tracked by the Quilt daemon.
	QueryEtcd() ([]db.Etcd, error)

	// RunStitch makes a request to the Quilt daemon to execute the given stitch.
	RunStitch(stitch string) error
}

type clientImpl struct {
	pbClient pb.APIClient
	cc       *grpc.ClientConn
}

// New creates a new Quilt client connected to `lAddr`.
func New(lAddr string) (Client, error) {
	proto, addr, err := api.ParseListenAddress(lAddr)
	if err != nil {
		return nil, err
	}

	dialer := func(dialAddr string, t time.Duration) (net.Conn, error) {
		return net.DialTimeout(proto, dialAddr, t)
	}
	cc, err := grpc.Dial(addr, grpc.WithDialer(dialer), grpc.WithInsecure(),
		grpc.WithBlock(), grpc.WithTimeout(connectTimeout))
	if err != nil {
		return nil, err
	}

	pbClient := pb.NewAPIClient(cc)
	return clientImpl{
		pbClient: pbClient,
		cc:       cc,
	}, nil
}

func query(pbClient pb.APIClient, table db.TableType) (interface{}, error) {
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	reply, err := pbClient.Query(ctx, &pb.DBQuery{Table: string(table)})
	if err != nil {
		return nil, err
	}

	replyBytes := []byte(reply.TableContents)
	switch table {
	case db.MachineTable:
		var machines []db.Machine
		if err := json.Unmarshal(replyBytes, &machines); err != nil {
			return nil, err
		}
		return machines, nil
	case db.ContainerTable:
		var containers []db.Container
		if err := json.Unmarshal(replyBytes, &containers); err != nil {
			return nil, err
		}
		return containers, nil
	case db.EtcdTable:
		var etcds []db.Etcd
		if err := json.Unmarshal(replyBytes, &etcds); err != nil {
			return nil, err
		}
		return etcds, nil
	default:
		panic(fmt.Sprintf("unsupported table type: %s", table))
	}
}

// Close the grpc connection.
func (c clientImpl) Close() error {
	return c.cc.Close()
}

// QueryMachines retrieves the machines tracked by the Quilt daemon.
func (c clientImpl) QueryMachines() ([]db.Machine, error) {
	rows, err := query(c.pbClient, db.MachineTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Machine), nil
}

// QueryContainers retrieves the containers tracked by the Quilt daemon.
func (c clientImpl) QueryContainers() ([]db.Container, error) {
	rows, err := query(c.pbClient, db.ContainerTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Container), nil
}

// QueryEtcd retrieves the etcd information tracked by the Quilt daemon.
func (c clientImpl) QueryEtcd() ([]db.Etcd, error) {
	rows, err := query(c.pbClient, db.EtcdTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Etcd), nil
}

// RunStitch makes a request to the Quilt daemon to execute the given stitch.
func (c clientImpl) RunStitch(stitch string) error {
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	_, err := c.pbClient.Run(ctx, &pb.RunRequest{Stitch: stitch})
	return err
}
