package pprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"
)

// To profile:
//   - The profiler produces a .prof file, we'll call it 'minion.prof'
//   - SCP minion.prof to your computer
//   - Run `go tool pprof -pdf path/to/minion path/to/minion.prof > minion.pdf`
//   - minion.pdf contains the results of the profile run

type profiler interface {
	Start() error
	Stop() error
	TimedRun(time.Duration) error
}

// The Prof handle represents a profiling job.
type Prof struct {
	fname string
	fd    *os.File
}

// New returns a profiler. Only one profiler may run at a time.
func New(name string) Prof {
	return Prof{
		fname: fmt.Sprintf("/%s.prof", name),
	}
}

// Start a new run of the profiler.
func (pro *Prof) Start() error {
	fd, err := os.Create(pro.fname + ".tmp")
	if err != nil {
		return fmt.Errorf("failed to create tmp file: %s", err)
	}
	pro.fd = fd
	pprof.StartCPUProfile(pro.fd)
	return nil
}

// Stop profiling and output results to a file. Also run dump a memory profile.
func (pro *Prof) Stop() error {
	pprof.StopCPUProfile()
	pro.fd.Close()

	profileDir := "Profile-" + time.Now().Format("Jan_02_2006-15.04.05")
	if err := os.Mkdir(profileDir, 0777); err != nil {
		return err
	}

	cpuProfilePath := filepath.Join(profileDir, pro.fname)
	if err := os.Rename(pro.fname+".tmp", cpuProfilePath); err != nil {
		return fmt.Errorf("failed to rename tmp file: %s", err)
	}

	memProfilePath := filepath.Join(profileDir, pro.fname+".mem")
	memFile, err := os.Create(memProfilePath)
	if err != nil {
		return fmt.Errorf("failed to create mem file: %s", err)
	}

	runtime.GC()
	if err := pprof.WriteHeapProfile(memFile); err != nil {
		return fmt.Errorf("failed to write heap profile: %s", err)
	}
	return nil
}

// TimedRun is a convenience function that starts and then stops after a given duration.
func (pro *Prof) TimedRun(duration time.Duration) error {
	timer := time.NewTimer(duration)
	if err := pro.Start(); err != nil {
		return err
	}
	<-timer.C
	if err := pro.Stop(); err != nil {
		return err
	}
	return nil
}
