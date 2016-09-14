package command

import (
	"errors"
	"os/exec"
	"reflect"
	"testing"

	"github.com/spf13/afero"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/util"
)

func TestMachineFlags(t *testing.T) {
	t.Parallel()

	expHost := "IP"

	machineCmd := Machine{}
	err := machineCmd.Parse([]string{"-H", expHost})

	if err != nil {
		t.Errorf("Unexpected error when parsing container args: %s", err.Error())
		return
	}

	if machineCmd.host != expHost {
		t.Errorf("Expected machine command to parse arg %s, but got %s",
			expHost, machineCmd.host)
	}
}

func TestMachineOutput(t *testing.T) {
	t.Parallel()

	res := machinesStr([]db.Machine{{
		ID:       1,
		Role:     db.Master,
		Provider: "Amazon",
		Region:   "us-west-1",
		Size:     "m4.large",
		PublicIP: "8.8.8.8",
	}})

	exp := "Machine-1{Master, Amazon us-west-1 m4.large, PublicIP=8.8.8.8}\n"
	if res != exp {
		t.Errorf("\nGot: %s\nExp: %s\n", res, exp)
	}
}

func TestContainerFlags(t *testing.T) {
	t.Parallel()

	expHost := "IP"

	containerCmd := Container{}
	err := containerCmd.Parse([]string{"-H", expHost})

	if err != nil {
		t.Errorf("Unexpected error when parsing container args: %s", err.Error())
		return
	}

	if containerCmd.host != expHost {
		t.Errorf("Expected container command to parse arg %s, but got %s",
			expHost, containerCmd.host)
	}
}

func TestContainerOutput(t *testing.T) {
	t.Parallel()

	res := containersStr([]db.Container{{ID: 1, Command: []string{"cmd", "arg"}}})
	exp := "Container-1{run  cmd arg}\n"
	if res != exp {
		t.Errorf("Expected container command to print %s, but got %s.", exp, res)
	}
}

func checkGetParsing(t *testing.T, args []string, expImport string, expErr error) {
	getCmd := Get{}
	err := getCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing get args: %s", err.Error())
		return
	}

	if getCmd.importPath != expImport {
		t.Errorf("Expected get command to parse arg %s, but got %s",
			expImport, getCmd.importPath)
	}
}

func TestGetFlags(t *testing.T) {
	t.Parallel()

	expImport := "spec"
	checkGetParsing(t, []string{"-import", expImport}, expImport, nil)
	checkGetParsing(t, []string{expImport}, expImport, nil)
	checkGetParsing(t, []string{}, "", errors.New("no import specified"))
}

func checkRunParsing(t *testing.T, args []string, expStitch string, expErr error) {
	runCmd := Run{}
	err := runCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing run args: %s", err.Error())
		return
	}

	if runCmd.stitch != expStitch {
		t.Errorf("Expected run command to parse arg %s, but got %s",
			expStitch, runCmd.stitch)
	}
}

func TestRunFlags(t *testing.T) {
	t.Parallel()

	expStitch := "spec"
	checkRunParsing(t, []string{"-stitch", expStitch}, expStitch, nil)
	checkRunParsing(t, []string{expStitch}, expStitch, nil)
	checkRunParsing(t, []string{}, "", errors.New("no spec specified"))
}

func checkStopParsing(t *testing.T, args []string, expNamespace string, expErr error) {
	stopCmd := Stop{}
	err := stopCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing stop args: %s", err.Error())
		return
	}

	if stopCmd.namespace != expNamespace {
		t.Errorf("Expected stop command to parse arg %s, but got %s",
			expNamespace, stopCmd.namespace)
	}
}

func TestStopFlags(t *testing.T) {
	t.Parallel()

	expNamespace := "namespace"
	checkStopParsing(t, []string{"-namespace", expNamespace}, expNamespace, nil)
	checkStopParsing(t, []string{expNamespace}, expNamespace, nil)
}

func checkSSHParsing(t *testing.T, args []string, expMachine int,
	expSSHArgs []string, expErr error) {

	sshCmd := SSH{}
	err := sshCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing ssh args: %s", err.Error())
		return
	}

	if sshCmd.targetMachine != expMachine {
		t.Errorf("Expected ssh command to parse target machine %d, but got %d",
			expMachine, sshCmd.targetMachine)
	}

	if !reflect.DeepEqual(sshCmd.sshArgs, expSSHArgs) {
		t.Errorf("Expected ssh command to parse SSH args %v, but got %v",
			expSSHArgs, sshCmd.sshArgs)
	}
}

func TestSSHFlags(t *testing.T) {
	t.Parallel()

	checkSSHParsing(t, []string{"1"}, 1, []string{}, nil)
	sshArgs := []string{"-i", "~/.ssh/key"}
	checkSSHParsing(t, append([]string{"1"}, sshArgs...), 1, sshArgs, nil)
	checkSSHParsing(t, []string{}, 0, nil,
		errors.New("must specify a target machine"))
}

func checkExecParsing(t *testing.T, args []string, expContainer int,
	expKey string, expCmd string, expErr error) {

	execCmd := Exec{}
	err := execCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing exec args: %s", err.Error())
		return
	}

	if execCmd.targetContainer != expContainer {
		t.Errorf("Expected exec command to parse target container %d, but got %d",
			expContainer, execCmd.targetContainer)
	}

	if execCmd.command != expCmd {
		t.Errorf("Expected exec command to parse command %s, but got %s",
			expCmd, execCmd.command)
	}

	if execCmd.privateKey != expKey {
		t.Errorf("Expected exec command to parse private key %s, but got %s",
			expKey, execCmd.privateKey)
	}
}

func TestExecFlags(t *testing.T) {
	t.Parallel()

	checkExecParsing(t, []string{"1", "sh"}, 1, "", "sh", nil)
	checkExecParsing(t, []string{"-i", "key", "1", "sh"}, 1, "key", "sh", nil)
	checkExecParsing(t, []string{"1", "cat /etc/hosts"}, 1, "",
		"cat /etc/hosts", nil)
	checkExecParsing(t, []string{"1"}, 0, "", "",
		errors.New("must specify a target container and command"))
	checkExecParsing(t, []string{}, 0, "", "",
		errors.New("must specify a target container and command"))
}

type mockClient struct {
	machineReturn   []db.Machine
	containerReturn []db.Container
	etcdReturn      []db.Etcd
	runStitchArg    string
}

func (c *mockClient) QueryMachines() ([]db.Machine, error) {
	return c.machineReturn, nil
}

func (c *mockClient) QueryContainers() ([]db.Container, error) {
	return c.containerReturn, nil
}

func (c *mockClient) QueryEtcd() ([]db.Etcd, error) {
	return c.etcdReturn, nil
}

func (c *mockClient) Close() error {
	return nil
}

func (c *mockClient) RunStitch(stitch string) error {
	c.runStitchArg = stitch
	return nil
}

func TestStopNamespace(t *testing.T) {
	c := &mockClient{}
	getClient = func(host string) (client.Client, error) {
		return c, nil
	}

	stopCmd := &Stop{namespace: "namespace"}
	stopCmd.Run()
	expStitch := `setNamespace("namespace");`
	if c.runStitchArg != expStitch {
		t.Error("stop command invoked Quilt with the wrong stitch")
	}

	stopCmd = &Stop{}
	stopCmd.Run()
	expStitch = ""
	if c.runStitchArg != expStitch {
		t.Error("stop command invoked Quilt with the wrong stitch")
	}
}

func TestRunSpec(t *testing.T) {
	c := &mockClient{}
	getClient = func(host string) (client.Client, error) {
		return c, nil
	}
	util.AppFs = afero.NewMemMapFs()

	stitchPath := "test.spec"
	testSpec := `new Docker("nginx");`
	util.WriteFile(stitchPath, []byte(testSpec), 0644)

	runCmd := &Run{stitch: stitchPath}
	runCmd.Run()

	expStitch := `importSources = {};` + testSpec
	if c.runStitchArg != expStitch {
		t.Error("run command invoked Quilt with the wrong stitch")
	}
}

func TestGetLeaderClient(t *testing.T) {
	passedClient := &mockClient{}
	getClient = func(host string) (client.Client, error) {
		switch host {
		// One machine doesn't know the LeaderIP
		case api.RemoteAddress("8.8.8.8"):
			return &mockClient{
				etcdReturn: []db.Etcd{
					{
						LeaderIP: "",
					},
				},
			}, nil
		// The other machine knows the LeaderIP
		case api.RemoteAddress("9.9.9.9"):
			return &mockClient{
				etcdReturn: []db.Etcd{
					{
						LeaderIP: "leader",
					},
				},
			}, nil
		// The leader. getLeaderClient() should return an instance of this.
		case api.RemoteAddress("leader"):
			return passedClient, nil
		default:
			t.Errorf("Unexpected call to getClient with host %s", host)
			t.Fail()
		}
		panic("unreached")
	}

	localClient := &mockClient{
		machineReturn: []db.Machine{
			{
				PublicIP: "8.8.8.8",
			},
			{
				PublicIP: "9.9.9.9",
			},
			{
				PublicIP:  "leader",
				PrivateIP: "leader-priv",
			},
		},
	}
	res, err := getLeaderClient(localClient)
	if err != nil {
		t.Errorf("Unexpected error when getting lead minion: %s", err.Error())
		return
	}

	if res != passedClient {
		t.Errorf("Didn't retrieve the proper client for the lead minion: "+
			"expected %v, got %v", passedClient, res)
	}
}

func TestNoLeader(t *testing.T) {
	getClient = func(host string) (client.Client, error) {
		// No client knows the leader IP.
		return &mockClient{
			etcdReturn: []db.Etcd{
				{
					LeaderIP: "",
				},
			},
		}, nil
	}

	localClient := &mockClient{
		machineReturn: []db.Machine{
			{
				PublicIP: "8.8.8.8",
			},
			{
				PublicIP: "9.9.9.9",
			},
		},
	}
	_, err := getLeaderClient(localClient)
	expErr := "no leader found"
	if err == nil {
		t.Errorf("Expected an error when the leader IP is not set.")
		return
	}

	if err.Error() != expErr {
		t.Errorf("Got wrong error: expected %s, got %s", expErr, err.Error())
	}
}

func TestSSHCommandCreation(t *testing.T) {
	exp := []string{"ssh", "quilt@host", "-o", "StrictHostKeyChecking=no",
		"-i", "~/.ssh/quilt"}
	res := ssh("host", []string{"-i", "~/.ssh/quilt"})
	if !reflect.DeepEqual(res.Args, exp) {
		t.Errorf("Bad SSH command creation: expected %v, got %v.", exp, res.Args)
	}
}

func TestExec(t *testing.T) {
	targetContainer := 1
	execCmd := Exec{
		privateKey:      "key",
		command:         "cat /etc/hosts",
		targetContainer: targetContainer,
		host:            api.DefaultSocket,
	}
	workerHost := "worker"
	getClient = func(host string) (client.Client, error) {
		switch host {
		// The local client. Used by getLeaderClient to figure out machine
		// information.
		case api.DefaultSocket:
			return &mockClient{
				machineReturn: []db.Machine{
					{
						PublicIP:  "leader",
						PrivateIP: "leader-priv",
					},
					{
						PrivateIP: "worker-priv",
						PublicIP:  workerHost,
					},
				},
			}, nil
		case api.RemoteAddress("leader"):
			return &mockClient{
				containerReturn: []db.Container{
					{
						StitchID: targetContainer,
						Minion:   "worker-priv",
					},
					{
						StitchID: 5,
						Minion:   "bad",
					},
				},
				etcdReturn: []db.Etcd{
					{
						LeaderIP: "leader",
					},
				},
			}, nil
		case api.RemoteAddress(workerHost):
			return &mockClient{
				containerReturn: []db.Container{
					{
						StitchID: targetContainer,
						DockerID: "foo",
					},
				},
			}, nil
		default:
			t.Errorf("Unexpected call to getClient with host %s", host)
			t.Fail()
		}
		panic("unreached")
	}

	expArgs := []string{"-t", "-i", "key", "docker exec -it foo cat /etc/hosts"}
	var sshCalled bool
	ssh = func(host string, args []string) *exec.Cmd {
		sshCalled = true
		if host != workerHost {
			t.Errorf("Bad ssh host: expected %s, but got %s",
				workerHost, host)
		}
		if !reflect.DeepEqual(args, expArgs) {
			t.Errorf("Bad ssh args: expected %v, but got %v", expArgs, args)
		}
		return &exec.Cmd{}
	}

	execCmd.Run()

	if !sshCalled {
		t.Errorf("Never tried SSHing")
	}
}
