package minion

import (
	"testing"
	"time"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/join"
	"github.com/quilt/quilt/stitch"
	"github.com/stretchr/testify/assert"
)

const testImage = "alpine"

func TestContainerTxn(t *testing.T) {
	conn := db.New()
	callback, called := callbackChecker()
	conn.RegisterCallback(callback, "", 0, db.ContainerTable)

	spec := ""
	testContainerTxn(t, conn, spec)
	assert.False(t, called())

	spec = `deployment.deploy(
		new Service("a", [new Container("alpine", ["tail"])])
	)`
	testContainerTxn(t, conn, spec)
	assert.True(t, called())

	testContainerTxn(t, conn, spec)
	assert.False(t, called())

	spec = `var b = new Container("alpine", ["tail"]);
	deployment.deploy([
		new Service("b", [b]),
		new Service("a", [b, new Container("alpine", ["tail"])])
	]);`
	testContainerTxn(t, conn, spec)
	assert.True(t, called())

	spec = `var b = new Service("b", [new Container("alpine", ["cat"])]);
	deployment.deploy([
		b,
		new Service("a",
			b.containers.concat([new Container("alpine", ["tail"])])),
	]);`
	testContainerTxn(t, conn, spec)
	assert.True(t, called())

	spec = `var b = new Service("b", [new Container("ubuntu", ["cat"])]);
	deployment.deploy([
		b,
		new Service("a",
			b.containers.concat([new Container("alpine", ["tail"])])),
	]);`
	testContainerTxn(t, conn, spec)
	assert.True(t, called())

	spec = `deployment.deploy(
		new Service("a", [
			new Container("alpine", ["cat"]),
			new Container("alpine", ["cat"])
		])
	);`
	testContainerTxn(t, conn, spec)
	assert.True(t, called())

	spec = `deployment.deploy(
		new Service("a", [new Container("alpine")])
	)`
	testContainerTxn(t, conn, spec)
	assert.True(t, called())

	spec = `var b = new Service("b", [new Container("alpine")]);
	var c = new Service("c", [new Container("alpine")]);
	deployment.deploy([
		b,
		c,
		new Service("a", b.containers.concat(c.containers)),
	])`
	testContainerTxn(t, conn, spec)
	assert.True(t, called())

	testContainerTxn(t, conn, spec)
	assert.False(t, called())
}

func testContainerTxn(t *testing.T, conn db.Conn, spec string) {
	compiled, err := stitch.FromJavascript(spec, stitch.DefaultImportGetter)
	assert.Nil(t, err)

	var containers []db.Container
	conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		updatePolicy(view, compiled.String())
		containers = view.SelectFromContainer(nil)
		return nil
	})

	for _, e := range queryContainers(compiled) {
		found := false
		for i, c := range containers {
			if e.StitchID == c.StitchID {
				containers = append(containers[:i], containers[i+1:]...)
				found = true
				break
			}
		}

		assert.True(t, found)
	}

	assert.Empty(t, containers)
}

func TestConnectionTxn(t *testing.T) {
	conn := db.New()
	callback, called := callbackChecker()
	conn.RegisterCallback(callback, "", 0, db.ConnectionTable)

	pre := `var a = new Service("a", [new Container("alpine")]);
	var b = new Service("b", [new Container("alpine")]);
	var c = new Service("c", [new Container("alpine")]);
	deployment.deploy([a, b, c]);`

	spec := ""
	testConnectionTxn(t, conn, spec)
	assert.False(t, called())

	spec = pre + `a.connect(80, a);`
	testConnectionTxn(t, conn, spec)
	assert.True(t, called())

	testConnectionTxn(t, conn, spec)
	assert.False(t, called())

	spec = pre + `a.connect(90, a);`
	testConnectionTxn(t, conn, spec)
	assert.True(t, called())

	testConnectionTxn(t, conn, spec)
	assert.False(t, called())

	spec = pre + `b.connect(90, a);
	b.connect(90, c);
	b.connect(100, b);
	c.connect(101, a);`
	testConnectionTxn(t, conn, spec)
	assert.True(t, called())

	testConnectionTxn(t, conn, spec)
	assert.False(t, called())

	spec = pre
	testConnectionTxn(t, conn, spec)
	assert.True(t, called())

	testConnectionTxn(t, conn, spec)
	assert.False(t, called())
}

func testConnectionTxn(t *testing.T, conn db.Conn, spec string) {
	compiled, err := stitch.FromJavascript(spec, stitch.DefaultImportGetter)
	assert.Nil(t, err)

	var connections []db.Connection
	conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		updatePolicy(view, compiled.String())
		connections = view.SelectFromConnection(nil)
		return nil
	})

	exp := compiled.Connections
	for _, e := range exp {
		found := false
		for i, c := range connections {
			if e.From == c.From && e.To == c.To && e.MinPort == c.MinPort &&
				e.MaxPort == c.MaxPort {
				connections = append(
					connections[:i], connections[i+1:]...)
				found = true
				break
			}
		}

		assert.True(t, found)
	}

	assert.Empty(t, connections)
}

func callbackChecker() (func(), func() bool) {
	cur := 0
	last := 0
	callback := func() {
		cur++
	}
	check := func() bool {
		time.Sleep(25 * time.Millisecond)
		called := last != cur
		if called {
			last++
		}
		return called
	}

	return callback, check
}

func TestPlacementTxn(t *testing.T) {
	conn := db.New()
	checkPlacement := func(spec string, exp ...db.Placement) {
		compiled, err := stitch.FromJavascript(spec,
			stitch.DefaultImportGetter)
		assert.Nil(t, err)

		placements := map[db.Placement]struct{}{}
		conn.Txn(db.AllTables...).Run(func(view db.Database) error {
			updatePolicy(view, compiled.String())
			res := view.SelectFromPlacement(nil)

			// Set the ID to 0 so that we can use reflect.DeepEqual.
			for _, p := range res {
				p.ID = 0
				placements[p] = struct{}{}
			}

			return nil
		})

		assert.Equal(t, len(exp), len(placements))
		for _, p := range exp {
			_, ok := placements[p]
			assert.True(t, ok)
		}
	}

	pre := `var foo = new Service("foo", [new Container("foo")]);
	var bar = new Service("bar", [new Container("bar")]);
	var baz = new Service("baz", [new Container("bar")]);
	deployment.deploy([foo, bar, baz]);`

	// Create an exclusive placement.
	spec := pre + `bar.place(new LabelRule(true, foo));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "foo",
		},
	)

	// Change the placement from "exclusive" to "on".
	spec = pre + `bar.place(new LabelRule(false, foo));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   false,
			OtherLabel:  "foo",
		},
	)

	// Add another placement constraint.
	spec = pre + `bar.place(new LabelRule(false, foo));
	bar.place(new LabelRule(true, bar));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   false,
			OtherLabel:  "foo",
		},
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "bar",
		},
	)

	// Machine placement
	spec = pre + `foo.place(new MachineRule(false, {size: "m4.large"}));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "foo",
			Exclusive:   false,
			Size:        "m4.large",
		},
	)

	// Port placement
	spec = pre + `publicInternet.connect(80, foo);
	publicInternet.connect(81, foo);`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "foo",
			Exclusive:   true,
			OtherLabel:  "foo",
		},
	)

	spec = pre + `publicInternet.connect(80, foo);
	publicInternet.connect(80, bar);
	(function() {
		publicInternet.connect(81, bar);
		publicInternet.connect(81, baz);
	})()`

	checkPlacement(spec,
		db.Placement{
			TargetLabel: "foo",
			Exclusive:   true,
			OtherLabel:  "foo",
		},

		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "bar",
		},

		db.Placement{
			TargetLabel: "foo",
			Exclusive:   true,
			OtherLabel:  "bar",
		},

		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "foo",
		},

		db.Placement{
			TargetLabel: "baz",
			Exclusive:   true,
			OtherLabel:  "baz",
		},

		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "baz",
		},

		db.Placement{
			TargetLabel: "baz",
			Exclusive:   true,
			OtherLabel:  "bar",
		},
	)
}

func checkImage(t *testing.T, conn db.Conn, spec string, exp ...db.Image) {
	depl, err := stitch.FromJavascript(spec, stitch.DefaultImportGetter)
	assert.NoError(t, err)

	var images []db.Image
	conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		updatePolicy(view, depl.String())
		images = view.SelectFromImage(nil)
		return nil
	})

	key := func(intf interface{}) interface{} {
		im := intf.(db.Image)
		im.ID = 0
		return im
	}
	_, lonelyLeft, lonelyRight := join.HashJoin(
		db.ImageSlice(images), db.ImageSlice(exp), key, key)
	assert.Empty(t, lonelyLeft, "unexpected images")
	assert.Empty(t, lonelyRight, "missing images")
}

func TestImageTxn(t *testing.T) {
	t.Parallel()

	// Regular image that isn't built by Quilt.
	checkImage(t, db.New(),
		`deployment.deploy(
			new Service("foo", [
				new Container("image")
			])
		);`,
	)

	conn := db.New()
	checkImage(t, conn,
		`deployment.deploy([
			new Service("foo", [
				new Container(
					new Image("a", "1")
				)
			]),
			new Service("foo", [
				new Container(
					new Image("a", "1")
				)
			]),
			new Service("foo", [
				new Container(
					new Image("b", "1")
				)
			]),
			new Service("foo", [
				new Container(
					new Image("c")
				)
			]),
		]);`,
		db.Image{
			Name:       "a",
			Dockerfile: "1",
		},
		db.Image{
			Name:       "b",
			Dockerfile: "1",
		},
	)

	// Ensure existing images are preserved.
	conn.Txn(db.ImageTable).Run(func(view db.Database) error {
		img := view.SelectFromImage(func(img db.Image) bool {
			return img.Name == "a"
		})[0]
		img.DockerID = "id"
		view.Commit(img)
		return nil
	})
	checkImage(t, conn,
		`deployment.deploy([
			new Service("foo", [
				new Container(
					new Image("a", "1")
				)
			]),
			new Service("foo", [
				new Container(
					new Image("b", "2")
				)
			]),
		]);`,
		db.Image{
			Name:       "a",
			Dockerfile: "1",
			DockerID:   "id",
		},
		db.Image{
			Name:       "b",
			Dockerfile: "2",
		},
	)
}
