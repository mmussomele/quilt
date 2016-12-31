package stitch

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestMachine(t *testing.T) {
	t.Parallel()

	checkMachines(t, `
	var deployment = createDeployment({
                namespace: "ns",
                adminACL: ["local"],
                regions: {"Amazon": ["us-west-2"]},
        });
	deployment.deploy([new Machine({
                role: "Worker",
                provider: "Amazon",
                region: "us-west-2",
                size: "m4.large",
                cpu: new Range(2, 4),
                ram: new Range(4, 8),
                diskSize: 32,
                sshKeys: ["key1", "key2"]
        })])`,
		[]Machine{
			{
				ID:       "abef9486d86c626156a54c2afb542cb469c412fb",
				Role:     "Worker",
				Provider: "Amazon",
				Region:   "us-west-2",
				Size:     "m4.large",
				CPU:      Range{2, 4},
				RAM:      Range{4, 8},
				DiskSize: 32,
				SSHKeys:  []string{"key1", "key2"},
			}})

	checkMachines(t, `
	var deployment = createDeployment({
                namespace: "ns",
                adminACL: ["local"],
                regions: {"Amazon": [""]},
        });
	var baseMachine = new Machine({provider: "Amazon"});
	deployment.deploy(baseMachine.asMaster().replicate(2));`,
		[]Machine{
			{
				ID:       "3b863122cff01d0e3a21e39df480a98950e40eca",
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{},
			},
			{
				ID:       "44b523a5a320f15bd45104776308c416b6a00832",
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{},
			},
		},
	)

	checkMachines(t, `
	var deployment = createDeployment({
                namespace: "ns",
                adminACL: ["local"],
                regions: {"Amazon": [""]},
        });
	var baseMachine = new Machine({provider: "Amazon"});
	var machines = baseMachine.asMaster().replicate(2);
	machines[0].sshKeys.push("key");
	deployment.deploy(machines);`,
		[]Machine{
			{
				ID:       "d614f26d7c29e10daae511e6187d8605ee2be23c",
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{"key"},
			},
			{
				ID:       "3b863122cff01d0e3a21e39df480a98950e40eca",
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{},
			},
		},
	)

	checkMachines(t, `var baseMachine = new Machine({
	  provider: "Amazon",
	  floatingIp: "xxx.xxx.xxx.xxx"
	});
	deployment.deploy(baseMachine.asMaster());`,
		[]Machine{
			{
				ID:         "b1b982307101a1ca4d5b5dde891bbe844d9c1df7",
				Role:       "Master",
				Provider:   "Amazon",
				FloatingIP: "xxx.xxx.xxx.xxx",
				SSHKeys:    []string{},
			},
		})
}

func TestContainer(t *testing.T) {
	t.Parallel()

	checkContainers(t, `deployment.deploy(new Service("foo", [
	new Container("image", ["arg1", "arg2"]).withEnv({"foo": "bar"})
	]));`,
		map[string]Container{
			"7bd352cc06b0bfa0d13a25e8b28a34878c8ae9ba": {
				ID:      "7bd352cc06b0bfa0d13a25e8b28a34878c8ae9ba",
				Image:   "image",
				Command: []string{"arg1", "arg2"},
				Env:     map[string]string{"foo": "bar"},
			},
		})

	checkContainers(t, `deployment.deploy(new Service("foo", [
	new Container("image", ["arg1", "arg2"])
	]));`,
		map[string]Container{
			"67a8499599527201fa1664f04ea78f0eabce03e1": {
				ID:      "67a8499599527201fa1664f04ea78f0eabce03e1",
				Image:   "image",
				Command: []string{"arg1", "arg2"},
				Env:     map[string]string{},
			},
		})

	checkContainers(t, `deployment.deploy(
		new Service("foo", [
		new Container("image")
		])
	);`,
		map[string]Container{
			"1d43c15752b0e45b650db4f60a24c796751f892b": {
				ID:      "1d43c15752b0e45b650db4f60a24c796751f892b",
				Image:   "image",
				Command: []string{},
				Env:     map[string]string{},
			},
		})

	checkContainers(t, `var c = new Container("image");
	c.env["foo"] = "bar";
	deployment.deploy(new Service("foo", [c]));`,
		map[string]Container{
			"9bde8c9286f7f90162669fd1443cb2a7112f2674": {
				ID:      "9bde8c9286f7f90162669fd1443cb2a7112f2674",
				Image:   "image",
				Command: []string{},
				Env:     map[string]string{"foo": "bar"},
			},
		})

	checkContainers(t, `deployment.deploy(
		new Service("foo", new Container("image", ["arg"]).replicate(2))
	);`,
		map[string]Container{
			"3ccc0897b40f7d93fc3a28c68110dfcc8a38546e": {
				ID:      "3ccc0897b40f7d93fc3a28c68110dfcc8a38546e",
				Image:   "image",
				Command: []string{"arg"},
				Env:     map[string]string{},
			},
			"8589230395202832f47a4e7440cfcd4361879a82": {
				ID:      "8589230395202832f47a4e7440cfcd4361879a82",
				Image:   "image",
				Command: []string{"arg"},
				Env:     map[string]string{},
			},
		})

	// Test changing attributes of replicated container.
	checkContainers(t, `var repl = new Container("image", ["arg"]).replicate(2);
	repl[0].env["foo"] = "bar";
	repl[0].command.push("changed");
	deployment.deploy(
		new Service("baz", repl)
	);`,
		map[string]Container{
			"ee4f124fe75dafe7950616b93267491505b2da53": {
				ID:      "ee4f124fe75dafe7950616b93267491505b2da53",
				Image:   "image",
				Command: []string{"arg", "changed"},
				Env: map[string]string{
					"foo": "bar",
				},
			},
			"8589230395202832f47a4e7440cfcd4361879a82": {
				ID:      "8589230395202832f47a4e7440cfcd4361879a82",
				Image:   "image",
				Command: []string{"arg"},
				Env:     map[string]string{},
			},
		})
}

func TestPlacement(t *testing.T) {
	t.Parallel()

	pre := `var target = new Service("target", []);
	var other = new Service("other", []);`
	post := `deployment.deploy(target);
	deployment.deploy(other);`
	checkPlacements(t, pre+`target.place(new LabelRule(true, other));`+post,
		[]Placement{
			{
				TargetLabel: "target",
				OtherLabel:  "other",
				Exclusive:   true,
			},
		})

	checkPlacements(t, pre+`target.place(new MachineRule(true,
	{size: "m4.large",
	region: "us-west-2",
	provider: "Amazon"}));`+post,
		[]Placement{
			{
				TargetLabel: "target",
				Exclusive:   true,
				Region:      "us-west-2",
				Provider:    "Amazon",
				Size:        "m4.large",
			},
		})

	checkPlacements(t, pre+`target.place(new MachineRule(true,
	{size: "m4.large",
	provider: "Amazon"}));`+post,
		[]Placement{
			{
				TargetLabel: "target",
				Exclusive:   true,
				Provider:    "Amazon",
				Size:        "m4.large",
			},
		})

	checkPlacements(t, pre+`target.place(new MachineRule(false,
	{floatingIp: "xxx.xxx.xxx.xxx"}));`+post,
		[]Placement{
			{
				TargetLabel: "target",
				Exclusive:   false,
				FloatingIP:  "xxx.xxx.xxx.xxx",
			},
		})
}

func TestLabel(t *testing.T) {
	t.Parallel()

	checkLabels(t, `deployment.deploy(
		new Service("web_tier", [new Container("nginx")])
	);`,
		map[string]Label{
			"web_tier": {
				Name: "web_tier",
				IDs: []string{
					"76e8c2b7cc361d3b56876afbe99abbd9ead4f497",
				},
				Annotations: []string{},
			},
		})

	checkLabels(t, `deployment.deploy(
		new Service("web_tier", [
		new Container("nginx"),
		new Container("nginx")
		])
	);`,
		map[string]Label{
			"web_tier": {
				Name: "web_tier",
				IDs: []string{
					"76e8c2b7cc361d3b56876afbe99abbd9ead4f497",
					"1c8527df6750f34b0936c855b0fefe23d3209162",
				},
				Annotations: []string{},
			},
		})

	// Conflicting label names.
	checkLabels(t, `deployment.deploy(new Service("foo", []));
	deployment.deploy(new Service("foo", []));`,
		map[string]Label{
			"foo": {
				Name:        "foo",
				IDs:         []string{},
				Annotations: []string{},
			},
			"foo2": {
				Name:        "foo2",
				IDs:         []string{},
				Annotations: []string{},
			},
		})

	expHostname := "foo.q"
	checkJavascript(t, `(function() {
		var foo = new Service("foo", []);
		return foo.hostname();
	})()`, expHostname)

	expChildren := []string{"1.foo.q", "2.foo.q"}
	checkJavascript(t, `(function() {
		var foo = new Service("foo",
		[new Container("bar"), new Container("baz")]);
		return foo.children();
	})()`, expChildren)
}

func TestConnect(t *testing.T) {
	t.Parallel()

	pre := `var foo = new Service("foo", []);
	var bar = new Service("bar", []);
	deployment.deploy([foo, bar]);`

	checkConnections(t, pre+`foo.connect(new Port(80), bar);`,
		[]Connection{
			{
				From:    "foo",
				To:      "bar",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkConnections(t, pre+`foo.connect(new PortRange(80, 85), bar);`,
		[]Connection{
			{
				From:    "foo",
				To:      "bar",
				MinPort: 80,
				MaxPort: 85,
			},
		})

	checkConnections(t, pre+`foo.connect(80, publicInternet);`,
		[]Connection{
			{
				From:    "foo",
				To:      "public",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkConnections(t, pre+`foo.connect(80, publicInternet);`,
		[]Connection{
			{
				From:    "foo",
				To:      "public",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkConnections(t, pre+`publicInternet.connect(80, foo);`,
		[]Connection{
			{
				From:    "public",
				To:      "foo",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkError(t, pre+`foo.connect(new PortRange(80, 81), publicInternet);`,
		"public internet cannot connect on port ranges")
	checkError(t, pre+`publicInternet.connect(new PortRange(80, 81), foo);`,
		"public internet cannot connect on port ranges")
}

func TestVet(t *testing.T) {
	pre := `var foo = new Service("foo", []);
	deployment.deploy([foo]);`

	// Connect to undeployed label.
	checkError(t, pre+`foo.connect(80, new Service("baz", []));`,
		"foo has a connection to undeployed service: baz")

	checkError(t, pre+`foo.place(new MachineRule(false, {
			provider: "Amazon"
		}));
	foo.place(new LabelRule(true, new Service("baz", [])));`,
		"foo has a placement in terms of an undeployed service: baz")

	checkError(t, `
		var foo = new Service("foo", new Container("image").replicate(2));
		foo.place(new MachineRule(false, {
			floatingIp: "123",
		}));
		foo.connectFromPublic(80);
		deployment.deploy([foo]);
	`, "foo has a floating IP and multiple containers. This is "+
		"not yet supported.")
}

func TestCustomDeploy(t *testing.T) {
	t.Parallel()

	checkLabels(t, `deployment.deploy(
		{
			deploy: function(deployment) {
				deployment.deploy([
				new Service("web_tier", [new Container("nginx")]),
				new Service("web_tier2", [new Container("nginx")])
			]);
			}
		}
	);`,
		map[string]Label{
			"web_tier": {
				Name: "web_tier",
				IDs: []string{
					"76e8c2b7cc361d3b56876afbe99abbd9ead4f497",
				},
				Annotations: []string{},
			},
			"web_tier2": {
				Name: "web_tier2",
				IDs: []string{
					"1c8527df6750f34b0936c855b0fefe23d3209162",
				},
				Annotations: []string{},
			},
		})

	checkError(t, `deployment.deploy({})`,
		`only objects that implement "deploy(deployment)" can be deployed`)
}

func TestRunModule(t *testing.T) {
	checkJavascript(t, `(function() {
		module.exports = function() {}
	})()`, nil)
}

func TestGithubKeys(t *testing.T) {
	HTTPGet = func(url string) (*http.Response, error) {
		resp := http.Response{
			Body: ioutil.NopCloser(bytes.NewBufferString("githubkeys")),
		}
		return &resp, nil
	}

	checkJavascript(t, `(function() {
		return githubKeys("username");
	})()`, []string{"githubkeys"})
}

func TestQuery(t *testing.T) {
	t.Parallel()

	namespaceChecker := queryChecker(func(handle Stitch) interface{} {
		return handle.Namespace
	})
	maxPriceChecker := queryChecker(func(handle Stitch) interface{} {
		return handle.MaxPrice
	})
	adminACLChecker := queryChecker(func(handle Stitch) interface{} {
		return handle.AdminACL
	})

	namespaceChecker(t, `createDeployment({namespace: "myNamespace"});`,
		"myNamespace")
	namespaceChecker(t, ``, "default-namespace")
	maxPriceChecker(t, `createDeployment({maxPrice: 5});`, 5.0)
	maxPriceChecker(t, ``, 0.0)
	adminACLChecker(t, `createDeployment({adminACL: ["local"]});`, []string{"local"})
	adminACLChecker(t, ``, []string{})
}

func TestMarshal(t *testing.T) {
	t.Parallel()

	exp := Stitch{
		Machines: []Machine{
			{
				Role:     "Master",
				Provider: "Amazon",
			},
			{
				Role:     "Worker",
				Provider: "Amazon",
			},
		},
	}

	actual, err := FromJSON(exp.String())
	assert.Nil(t, err)
	assert.Equal(t, exp, actual)
}

func TestHash(t *testing.T) {
	t.Parallel()

	checkJavascript(t, `hash("foo");`, "0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33")
	checkError(t, `hash();`, "RangeError: hash requires an argument")
}

func checkJavascript(t *testing.T, code string, exp interface{}) {
	resultKey := "result"

	vm, err := newVM(ImportGetter{
		Path: ".",
	})
	if err != nil {
		t.Errorf(`Unexpected error: "%s".`, err.Error())
		return
	}

	exec := fmt.Sprintf(`exports.%s = %s;`, resultKey, code)
	moduleVal, err := runSpec(vm, "<test_code>", exec)
	if err != nil {
		t.Errorf(`Unexpected error: "%s".`, err.Error())
		return
	}

	actualVal, err := moduleVal.Object().Get(resultKey)
	if err != nil {
		t.Errorf(`Unexpected error retrieving result from VM: "%s".`,
			err.Error())
		return
	}

	actual, _ := actualVal.Export()
	if !reflect.DeepEqual(actual, exp) {
		t.Errorf("Bad javascript code: Expected %s, got %s.",
			spew.Sdump(exp), spew.Sdump(actual))
	}
}

func checkError(t *testing.T, code string, exp string) {
	_, err := FromJavascript(code, ImportGetter{
		Path: ".",
	})
	if err == nil {
		t.Errorf(`Expected error "%s", but got nothing.`, exp)
		return
	}
	if actual := err.Error(); actual != exp {
		t.Errorf(`Expected error "%s", but got "%s".`, exp, actual)
	}
}

func queryChecker(
	queryFunc func(Stitch) interface{}) func(*testing.T, string, interface{}) {

	return func(t *testing.T, code string, exp interface{}) {
		handle, err := FromJavascript(code, DefaultImportGetter)
		if err != nil {
			t.Errorf(`Unexpected error: "%s".`, err.Error())
			return
		}

		actual := queryFunc(handle)
		if !reflect.DeepEqual(actual, exp) {
			t.Errorf("Bad query: Expected %s, got %s.",
				spew.Sdump(exp), spew.Sdump(actual))
		}
	}
}

var checkMachines = queryChecker(func(s Stitch) interface{} {
	return s.Machines
})

var checkContainers = queryChecker(func(s Stitch) interface{} {
	// Convert the slice to a map because the ordering is non-deterministic.
	containersMap := make(map[string]Container)
	for _, c := range s.Containers {
		containersMap[c.ID] = c
	}
	return containersMap
})

var checkPlacements = queryChecker(func(s Stitch) interface{} {
	return s.Placements
})

var checkLabels = queryChecker(func(s Stitch) interface{} {
	// Convert the slice to a map because the ordering is non-deterministic.
	labelsMap := make(map[string]Label)
	for _, label := range s.Labels {
		labelsMap[label.Name] = label
	}
	return labelsMap
})

var checkConnections = queryChecker(func(s Stitch) interface{} {
	return s.Connections
})
