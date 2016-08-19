//go:generate ../scripts/generate-bindings bindings.js

package stitch

import (
	"fmt"

	"github.com/robertkrimen/otto"

	// Automatically import the Javascript underscore utility-belt library into
	// the Stitch VM.
	_ "github.com/robertkrimen/otto/underscore"

	"github.com/NetSys/quilt/util"
)

// A Stitch is an abstract representation of the policy language.
type Stitch struct {
	code string
	ctx  *evalCtx
}

// A Placement constraint guides where containers may be scheduled, either relative to
// the labels of other containers, or the machine the container will run on.
type Placement struct {
	TargetLabel string

	Exclusive bool

	// Label Constraint
	OtherLabel string

	// Machine Constraints
	Provider string
	Size     string
	Region   string
}

// A Container may be instantiated in the stitch and queried by users.
type Container struct {
	ID      int
	Image   string
	Command []string
	Env     map[string]string
}

// A Label represents a logical group of containers.
type Label struct {
	Name        string
	IDs         []int
	Annotations []string
}

// A Connection allows containers implementing the From label to speak to containers
// implementing the To label in ports in the range [MinPort, MaxPort]
type Connection struct {
	From    string
	To      string
	MinPort int
	MaxPort int
}

// A ConnectionSlice allows for slices of Collections to be used in joins
type ConnectionSlice []Connection

// A Machine specifies the type of VM that should be booted.
type Machine struct {
	Provider string
	Role     string
	Size     string
	CPU      Range
	RAM      Range
	DiskSize int
	Region   string
	SSHKeys  []string
}

// A Range defines a range of acceptable values for a Machine attribute
type Range struct {
	Min float64
	Max float64
}

// PublicInternetLabel is a magic label that allows connections to or from the public
// network.
const PublicInternetLabel = "public"

// Accepts returns true if `x` is within the range specified by `stitchr` (include),
// or if no max is specified and `x` is larger than `stitchr.min`.
func (stitchr Range) Accepts(x float64) bool {
	return stitchr.Min <= x && (stitchr.Max == 0 || x <= stitchr.Max)
}

type evalCtx struct {
	containers  map[int]Container
	labels      map[string]Label
	connections map[Connection]struct{}
	placements  map[Placement]struct{}
	machines    []Machine
	invariants  []invariant

	adminACL  []string
	maxPrice  float64
	namespace string
}

func run(filename string, spec string, getter ImportGetter) (*otto.Otto, error) {
	vm := otto.New()
	if err := vm.Set("githubKeys", githubKeysImpl); err != nil {
		return vm, err
	}
	if err := vm.Set("require", getter.requireImpl); err != nil {
		return vm, err
	}

	script, err := vm.Compile("<javascript_bindings>", javascriptBindings)
	if err != nil {
		return vm, err
	}
	if _, err := vm.Run(script); err != nil {
		return vm, err
	}

	script, err = vm.Compile(filename, spec)
	if err != nil {
		return vm, err
	}
	if _, err := vm.Run(script); err != nil {
		return vm, err
	}

	return vm, nil
}

// Compile transforms the Stitch at the given filepath into an executable string.
func Compile(filepath string, getter ImportGetter) (string, error) {
	specStr, err := util.ReadFile(filepath)
	if err != nil {
		return "", err
	}

	vm, err := run(filepath, specStr, getter)
	if err != nil {
		return "", err
	}

	imports, err := getImports(vm)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("importSources = %s;", imports) + specStr, nil
}

// FromFile gets a Stitch handle from a file on disk.
func FromFile(filename string, getter ImportGetter) (Stitch, error) {
	compiled, err := Compile(filename, getter)
	if err != nil {
		return Stitch{}, err
	}
	return New(compiled, getter)
}

// New parses and executes a stitch (in text form), and returns an abstract Dsl handle.
func New(specStr string, getter ImportGetter) (Stitch, error) {
	vm, err := run("<raw_string>", specStr, getter)
	if err != nil {
		return Stitch{}, err
	}

	ctx, err := parseContext(vm)
	if err != nil {
		return Stitch{}, err
	}
	ctx.createPortRules()

	spec := Stitch{
		code: specStr,
		ctx:  &ctx,
	}

	if len(ctx.invariants) == 0 {
		return spec, nil
	}

	graph, err := InitializeGraph(spec)
	if err != nil {
		return Stitch{}, err
	}

	if err := checkInvariants(graph, ctx.invariants); err != nil {
		return Stitch{}, err
	}

	return spec, nil
}

func (ctx *evalCtx) createPortRules() {
	ports := make(map[int][]string)
	for c := range ctx.connections {
		if c.From != PublicInternetLabel && c.To != PublicInternetLabel {
			continue
		}

		target := c.From
		if c.From == PublicInternetLabel {
			target = c.To
		}

		min := c.MinPort
		ports[min] = append(ports[min], target)
	}

	for _, labels := range ports {
		for _, tgt := range labels {
			for _, other := range labels {
				ctx.placements[Placement{
					Exclusive:   true,
					TargetLabel: tgt,
					OtherLabel:  other,
				}] = struct{}{}
			}
		}
	}
}

// QueryLabels retrieves all labels declared in the Stitch.
func (stitch Stitch) QueryLabels() []Label {
	var res []Label
	for _, l := range stitch.ctx.labels {
		res = append(res, l)
	}

	return res
}

// QueryContainers retrieves all containers declared in stitch.
func (stitch Stitch) QueryContainers() []Container {
	var containers []Container
	for _, c := range stitch.ctx.containers {
		containers = append(containers, c)
	}
	return containers
}

// QueryMachines returns all machines declared in the stitch.
func (stitch Stitch) QueryMachines() []Machine {
	return stitch.ctx.machines
}

// QueryConnections returns the connections declared in the stitch.
func (stitch Stitch) QueryConnections() []Connection {
	var connections []Connection
	for c := range stitch.ctx.connections {
		connections = append(connections, c)
	}
	return connections
}

// QueryPlacements returns the placements declared in the stitch.
func (stitch Stitch) QueryPlacements() []Placement {
	var placements []Placement
	for p := range stitch.ctx.placements {
		placements = append(placements, p)
	}
	return placements
}

// QueryMaxPrice returns the max allowable machine price declared in the stitch.
func (stitch Stitch) QueryMaxPrice() float64 {
	return stitch.ctx.maxPrice
}

// QueryNamespace returns the namespace declared in the stitch.
func (stitch Stitch) QueryNamespace() string {
	return stitch.ctx.namespace
}

// QueryAdminACL returns the admin ACLs declared in the stitch.
func (stitch Stitch) QueryAdminACL() []string {
	return stitch.ctx.adminACL
}

// String returns the stitch in its code form.
func (stitch Stitch) String() string {
	return stitch.code
}

// Get returns the value contained at the given index
func (cs ConnectionSlice) Get(ii int) interface{} {
	return cs[ii]
}

// Len returns the number of items in the slice
func (cs ConnectionSlice) Len() int {
	return len(cs)
}

func stitchError(vm *otto.Otto, err error) {
	panic(vm.MakeCustomError("StitchError", err.Error()))
}

// Otto uses `panic` with `*otto.Error`s to signify Javascript runtime errors.
// This function asserts that the given error is safe to panic.
func assertOttoError(err error) error {
	if _, ok := err.(*otto.Error); err == nil || ok {
		return err
	}
	panic("Only otto errors can be returned. Got: " + err.Error())
}
