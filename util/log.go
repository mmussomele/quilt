package util

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

// Formatter implements the log formatter for Quilt.
type Formatter struct{}

// Format converts a logrus entry into a string for logging.
func (f Formatter) Format(entry *log.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	level := strings.ToUpper(entry.Level.String())
	fmt.Fprintf(b, "%s [%s] %-40s", level, entry.Time.Format(time.StampMilli),
		entry.Message)

	for k, v := range entry.Data {
		fmt.Fprintf(b, " %s=%+v", k, v)
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

// LoopTimeLogger is a utility struct that allows us to time how long loops take, as
// well as how often they are triggered.
type LoopTimeLogger struct {
	loopname    string
	lastStart   time.Time
	lastEnd     time.Time
	loopRunning bool
}

// NewLoopTimeLogger creates and returns a ready to use LoopTimeLogger
func NewLoopTimeLogger(loopname string) *LoopTimeLogger {
	return &LoopTimeLogger{
		loopname:    loopname,
		lastEnd:     time.Now(),
		lastStart:   time.Time{},
		loopRunning: false,
	}
}

// LogLoopStart logs the start of a loop and how long it has been since the last trigger.
func (ltl *LoopTimeLogger) LogLoopStart() {
	if ltl.loopRunning {
		return
	}

	ltl.loopRunning = true
	ltl.lastStart = time.Now()
	log.Debugf("Starting %s trigger loop. It has been %v "+
		"since the last trigger.", ltl.loopname, ltl.lastStart.Sub(ltl.lastEnd))
}

// LogLoopEnd logs the end of a loop and how long it took to run.
func (ltl *LoopTimeLogger) LogLoopEnd() {
	if !ltl.loopRunning {
		return
	}

	ltl.loopRunning = false
	ltl.lastEnd = time.Now()
	log.Debugf("%s trigger loop ended. It took %v",
		ltl.loopname, ltl.lastEnd.Sub(ltl.lastStart))
}
