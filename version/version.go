package version

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc"
)

const master = "master"

var (
	// Version is the Quilt version number.
	Version = master

	// errVersionStr denotes the error that is reported when a minion detects it is
	// running a different version of a quilt than the daemon that sent a request to
	// it.
	errVersionStr = "minion quilt version incompatible with requesting daemon's"

	// versionFormatStr is appended to ErrVersionStr. They are separate to facilitate
	// the daemon being able to easily check if a given error is a version error.
	versionFormatStr = "\nminion version: %s\ndaemon version: %s"
)

// Compatible returns true if the provided version label denotes a compatible version
// of Quilt with respect to the current instance.
func Compatible(other string) bool {
	otherMajor, otherMinor := parse(other)
	major, minor := parse(Version)
	return major == otherMajor && minor == otherMinor
}

// NewError returns an error indicating that there exists incompatible versions of Quilt
// on the minion and the daemon. This function should only ever be used on a minion.
func NewError(other string) error {
	return fmt.Errorf(errVersionStr+versionFormatStr, Version, other)
}

// IsVersionError checks if the given error indicates incompatible versions on Quilt on a
// minion and the daemon.
func IsVersionError(err error) bool {
	// The error will likely be inspected on the daemon, so we strip the gRPC error
	// data before comparing.
	return strings.HasPrefix(grpc.ErrorDesc(err), errVersionStr)
}

// parse parses a semantic version label into its major and minor version values. The
// master branch is defined to have the values -1 and -1 for each of these,
// respectively. Since valid version numbers must have all non-negative numbers, master
// is guaranteed to be only compatible with itself. The revision number is ignored, but
// still checked for validity.
// A valid version label is either the string 'master' or a string matching the regex
// `\d+\.\d+\.\d+` for all non-negative integers. If a version label is invalid, parse
// will panic, as that is an indication of programming error.
func parse(version string) (int, int) {
	if version == master {
		return -1, -1
	}

	subversions := strings.Split(version, ".")
	badVersionErr := fmt.Sprintf("invalid version number: %s", version)
	if len(subversions) != 3 {
		panic(badVersionErr)
	}

	major, err := strconv.Atoi(subversions[0])
	if err != nil || major < 0 {
		panic(badVersionErr)
	}

	minor, err := strconv.Atoi(subversions[1])
	if err != nil || minor < 0 {
		panic(badVersionErr)
	}

	// Just for error checking.
	revision, err := strconv.Atoi(subversions[2])
	if err != nil || revision < 0 {
		panic(badVersionErr)
	}

	return major, minor
}
