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

type LoopTimeLogger struct {
	loopname    string
	lastStart   time.Time
	lastEnd     time.Time
	loopRunning bool
}

func NewLoopTimeLogger(loopname string) *LoopTimeLogger {
	return &LoopTimeLogger{
		loopname:    loopname,
		lastEnd:     time.Now(),
		lastStart:   time.Time{},
		loopRunning: false,
	}
}

func (ltl *LoopTimeLogger) LogLoopStart() {
	if ltl.loopRunning {
		return
	}

	ltl.loopRunning = true
	ltl.lastStart = time.Now()
	log.Infof("Starting %s trigger loop. It has been %v "+
		"since the last trigger.", ltl.loopname, ltl.lastStart.Sub(ltl.lastEnd))
}

func (ltl *LoopTimeLogger) LogLoopEnd() {
	if !ltl.loopRunning {
		return
	}

	ltl.loopRunning = false
	ltl.lastEnd = time.Now()
	log.Infof("%s trigger loop ended. It took %v",
		ltl.loopname, ltl.lastEnd.Sub(ltl.lastStart))
}
