package server

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/NetSys/quilt/api/pb"
	"github.com/NetSys/quilt/db"
)

func checkQuery(t *testing.T, s server, table db.TableType, exp string) {
	reply, err := s.Query(context.Background(),
		&pb.DBQuery{Table: string(table)})
	if err != nil {
		t.Errorf("Unexpected error: %s\n", err.Error())
		return
	}

	if exp != reply.TableContents {
		t.Errorf(`Bad query response: expected "%s", got "%s".`,
			exp, reply.TableContents)
	}
}

func TestMachineResponse(t *testing.T) {
	t.Parallel()

	conn := db.New()
	conn.Transact(func(view db.Database) error {
		m := view.InsertMachine()
		m.Role = db.Master
		m.Provider = db.AmazonSpot
		m.Size = "size"
		m.PublicIP = "8.8.8.8"
		m.PrivateIP = "9.9.9.9"
		view.Commit(m)

		return nil
	})

	exp := `[{"ID":1,"Role":"Master","Provider":"AmazonSpot","Region":"",` +
		`"Size":"size","DiskSize":0,"SSHKeys":null,"CloudID":"",` +
		`"PublicIP":"8.8.8.8","PrivateIP":"9.9.9.9","Connected":false}]`

	checkQuery(t, server{conn, nil}, db.MachineTable, exp)
}

func TestContainerResponse(t *testing.T) {
	t.Parallel()

	conn := db.New()
	conn.Transact(func(view db.Database) error {
		c := view.InsertContainer()
		c.DockerID = "docker-id"
		c.Image = "image"
		c.Command = []string{"cmd", "arg"}
		c.Labels = []string{"labelA", "labelB"}
		view.Commit(c)

		return nil
	})

	exp := `[{"ID":1,"Pid":0,"IP":"","Mac":"","Minion":"",` +
		`"DockerID":"docker-id","StitchID":0,"Image":"image",` +
		`"Command":["cmd","arg"],"Labels":["labelA","labelB"],"Env":null}]`

	checkQuery(t, server{conn, nil}, db.ContainerTable, exp)
}

/* XXX: Fix these tests after we fix the server Run method
func TestBadStitch(t *testing.T) {
	conn := db.New()
	s := server{dbConn: conn}

	badStitch := `anUndefinedVariable`

	_, err := s.Run(context.Background(),
		&pb.RunRequest{Stitch: badStitch})

	if err == nil {
		t.Error("Expected error from bad stitch.")
		return
	}

	expErr := "ReferenceError: 'anUndefinedVariable' is not defined"
	if err.Error() != expErr {
		t.Errorf("Expected run error %s, but got %s\n", expErr, err.Error())
	}
}

func TestRun(t *testing.T) {
	conn := db.New()
	s := server{dbConn: conn}

	createMachineStitch :=
		`deployMachines(new Machine(
			{provider: "AmazonSpot", size: "m4.large", role: "Master"}));
		deployMachines(new Machine(
			{provider: "AmazonSpot", size: "m4.large", role: "Worker"}));`

	_, err := s.Run(context.Background(),
		&pb.RunRequest{Stitch: createMachineStitch})

	if err != nil {
		t.Errorf("Unexpected error when running stich: %s\n", err.Error())
		return
	}

	var machines []db.Machine
	conn.Transact(func(view db.Database) error {
		machines = view.SelectFromMachine(nil)
		return nil
	})

	if len(machines) != 2 {
		t.Errorf("Two machines should have been created by running the stitch, "+
			"but we found: %v\n", machines)
	}
}
*/
