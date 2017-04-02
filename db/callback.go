package db

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
)

var (
	changeAction    = "changes to %v"
	timerAction     = "timer"
	externalTrigger = "external trigger"
)

// Callback represents a function callback to be executed upon certain trigger criteria
// within the database.
type Callback struct {
	do       func(string)
	interval int
	causes   chan string
}

// RegisterCallback binds the function do() to the given tables and time interval.
// Whenever one of the tables is modified or the provided time (in seconds) elapses,
// do() will be called. If seconds is less than or equal to 0, there is no time based
// Callback.
// A name can be provided to be included in debugging logs.
func (conn Conn) RegisterCallback(do func(), name string, seconds int,
	tt ...TableType) *Callback {

	c := &Callback{interval: seconds, causes: make(chan string, 1)}

	enter := fmt.Sprintf("Entering callback %s (triggered by %%s)", name)
	exit := fmt.Sprintf("Exiting callback %s (elapsed time: %%v)", name)
	c.do = func(cause string) {
		log.Debugf(enter, cause)
		start := time.Now()
		do()
		log.Debugf(exit, time.Now().Sub(start))
	}

	if seconds > 0 {
		conn.callbacks.Lock()
		conn.callbacks.list = append(conn.callbacks.list, c)
		conn.callbacks.Unlock()
	}

	conn.Txn(tt...).Run(func(view Database) error {
		for _, t := range tt {
			tbl := conn.db.accessTable(t)
			tbl.callbacks = append(tbl.callbacks, c)
		}
		return nil
	})

	go c.listen()
	return c
}

// RegisterTrigger binds the given channel to the callback. Whenever an item is sent
// along the channel, it will trigger this callback.
func (c *Callback) RegisterTrigger(t chan struct{}) {
	go func() {
		for range t {
			select {
			case c.causes <- externalTrigger:
			default:
			}
		}
	}()
}

func (c *Callback) listen() {
	for cause := range c.causes {
		c.do(cause)
	}
}

func (conn Conn) runCallbackTimer() {
	i := 1
	for range time.Tick(time.Second) {
		conn.callbacks.Lock()
		for _, c := range conn.callbacks.list {
			if c.interval%i > 0 {
				continue
			}

			select {
			case c.causes <- timerAction:
			default:
			}
		}
		conn.callbacks.Unlock()
		i++
	}
}
