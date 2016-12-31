package region

import (
	"fmt"

	"github.com/NetSys/quilt/db"
)

const (
	// AmazonDefault is the default region for AWS instances.
	AmazonDefault = "us-west-1"

	// GoogleDefault is the default zone for GCE instances.
	GoogleDefault = "us-east1-b"
)

// SetDefault populates `m.Region` for the provided db.Machine if one isn't
// specified. This is intended to allow users to omit the cloud provider region when
// they don't particularly care where a system is placed.
func SetDefault(m db.Machine) db.Machine {
	if m.Region == "" {
		m.Region = Default(m.Provider)
	}

	return m
}

// Default returns the default region for the given provider. It is in a variable so it
// can be mocked out by the unit tests.
var Default = func(p db.Provider) string {
	switch p {
	case db.Amazon:
		return AmazonDefault
	case db.Google:
		return GoogleDefault
	case db.Vagrant:
	default:
		panic(fmt.Sprintf("Unknown Cloud Provider: %s", p))
	}

	return ""
}
