package version

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	defer func() {
		r := recover()
		assert.Nil(t, r)
	}()

	// parse will panic if Version is invalid. This checks that we don't accidentally
	// mistype a version number for a release.
	parse(Version)
}

func TestCompatible(t *testing.T) {
	defer func() {
		Version = master
	}()

	type testData struct {
		this       string
		other      string
		compatible bool
	}

	tests := []testData{
		{master, master, true},
		{master, "1.2.3", false},
		{"1.2.3", master, false},
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", true},
		{"1.3.2", "1.2.3", false},
		{"2.5.7", "1.5.7", false},
		{"2.5.7", "3.2.1", false},
		{"3.2.1", "2.5.7", false},
	}

	for _, test := range tests {
		Version = test.this
		assert.Equal(t, test.compatible, Compatible(test.other))
	}
}

func TestParse(t *testing.T) {
	type testData struct {
		version string
		major   int
		minor   int
		err     bool
	}

	tests := []testData{
		{"master", -1, -1, false},
		{"0.0.0", 0, 0, false},
		{"1.2.3", 1, 2, false},
		{"1.0.-4", 0, 0, true},
		{"1.0", 0, 0, true},
		{"1.1.1.1", 0, 0, true},
		{"not master", 0, 0, true},
	}

	var wg sync.WaitGroup
	for _, test := range tests {
		wg.Add(1)
		test := test
		go func() {
			defer func() {
				defer wg.Done()
				r := recover()
				assert.Equal(t, test.err, r != nil)
			}()

			major, minor := parse(test.version)
			assert.Equal(t, test.major, major)
			assert.Equal(t, test.minor, minor)
		}()
	}
	wg.Wait()
}
