package cluster

import (
	"github.com/NetSys/quilt/cluster/machine"
)

// ChooseSize returns an acceptable machine size for the given provider that fits the
// provided ram, cpu, and price constraints.
var ChooseSize = machine.ChooseSize
