package supervisor

import (
	"fmt"
	"net"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/minion/docker"
)

type testCtx struct {
	fd    fakeDocker
	execs [][]string

	conn  db.Conn
	trigg chan struct{}
}

func initTest(r db.Role) *testCtx {
	conn = db.New()
	md, _dk := docker.NewMock()
	ctx := testCtx{fakeDocker{_dk, md}, nil, conn, make(chan struct{})}
	conn.RegisterCallback(func() {
		ctx.trigg <- struct{}{}
	}, "", 0, db.MinionTable, db.EtcdTable)

	role = r
	dk = ctx.fd.Client

	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		m := view.InsertMinion()
		m.Self = true
		view.Commit(m)
		e := view.InsertEtcd()
		view.Commit(e)
		return nil
	})

	execRun = func(name string, args ...string) error {
		ctx.execs = append(ctx.execs, append([]string{name}, args...))
		return nil
	}

	cfgGateway = func(name string, ip net.IPNet) error {
		execRun("cfgGateway", ip.String())
		return nil
	}

	return &ctx
}

func (ctx *testCtx) run() {
	ctx.execs = nil
	select {
	case <-ctx.trigg:
	}

	switch role {
	case db.Master:
		runMasterOnce()
	case db.Worker:
		runWorkerOnce()
	}
}

type fakeDocker struct {
	docker.Client
	md *docker.MockClient
}

func (f fakeDocker) running() map[string][]string {
	containers, _ := f.List(nil)

	res := map[string][]string{}
	for _, c := range containers {
		res[c.Name] = c.Args
	}
	return res
}

func etcdArgsMaster(ip string, etcdIPs []string) []string {
	return []string{
		fmt.Sprintf("--name=master-%s", ip),
		fmt.Sprintf("--initial-cluster=%s", initialClusterString(etcdIPs)),
		fmt.Sprintf("--advertise-client-urls=http://%s:2379", ip),
		fmt.Sprintf("--listen-peer-urls=http://%s:2380", ip),
		fmt.Sprintf("--initial-advertise-peer-urls=http://%s:2380", ip),
		"--listen-client-urls=http://0.0.0.0:2379",
		"--heartbeat-interval=500",
		"--initial-cluster-state=new",
		"--election-timeout=5000",
	}
}
