package minion

import (
	"os"
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"
)

const authorizedKeysFile = "/home/quilt/.ssh/authorized_keys"

func syncAuthorizedKeys() {
	conn := db.Open(db.MinionTable)

	waitForMinion(conn)
	for range conn.TriggerTick(30).C {
		if err := runOnce(conn); err != nil {
			log.WithError(err).Error("Failed to sync keys")
		}
	}
}

func waitForMinion(conn db.Conn) {
	for {
		if _, err := conn.MinionSelf(); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func runOnce(conn db.Conn) error {
	if _, err := util.AppFs.Stat(authorizedKeysFile); os.IsNotExist(err) {
		util.AppFs.Create(authorizedKeysFile)
	}
	currKeys, err := util.ReadFile(authorizedKeysFile)
	if err != nil {
		return err
	}

	m, err := conn.MinionSelf()
	if err != nil {
		return err
	}

	if m.AuthorizedKeys == currKeys {
		return nil
	}

	return util.WriteFile(authorizedKeysFile, []byte(m.AuthorizedKeys), 0644)
}
