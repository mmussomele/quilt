package minion

import (
	"fmt"
	"runtime"
	"time"

	"github.com/NetSys/quilt/api"
	apiServer "github.com/NetSys/quilt/api/server"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/docker"
	"github.com/NetSys/quilt/minion/etcd"
	"github.com/NetSys/quilt/minion/network"
	"github.com/NetSys/quilt/minion/pprofile"
	"github.com/NetSys/quilt/minion/scheduler"
	"github.com/NetSys/quilt/minion/supervisor"
	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"
)

// Run blocks executing the minion.
func Run() {
	// XXX Uncomment the following line to run the profiler
	//runProfiler(5 * time.Minute)

	log.Info("Minion Start")
	cpuLimit := runtime.NumCPU() - 1
	if cpuLimit < 1 {
		cpuLimit = 1
	}
	runtime.GOMAXPROCS(cpuLimit) // Reserve a core for the system

	conn := db.New()
	dk := docker.New("unix:///var/run/docker.sock")
	go minionServerRun(conn)
	go supervisor.Run(conn, dk)
	go scheduler.Run(conn, dk)
	go network.Run(conn, dk)
	go etcd.Run(conn)

	go apiServer.Run(conn, fmt.Sprintf("tcp://0.0.0.0:%d", api.DefaultRemotePort))

	loopLog := util.NewLoopTimeLogger("Minion-Update")
	for range conn.Trigger(db.MinionTable).C {
		loopLog.LogLoopStart()
		conn.Transact(func(view db.Database) error {
			minion, err := view.MinionSelf()
			if err != nil {
				return err
			}

			updatePolicy(view, minion.Role, minion.Spec)
			return nil
		})
		loopLog.LogLoopEnd()
	}
}

func runProfiler(duration time.Duration) {
	go func() {
		p := pprofile.New("minion")
		for {
			if err := p.TimedRun(duration); err != nil {
				log.Error(err)
			}
		}
	}()
}
