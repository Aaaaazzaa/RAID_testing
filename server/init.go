package server

import (
	"os"

	"github.com/sirupsen/logrus"

	"github.com/rai-project/config"
	logger "github.com/rai-project/logger"
	"github.com/rai-project/utils"
)

var (
	log *logrus.Entry
)

func init() {
	config.AfterInit(func() {

		log = logger.New().WithField("pkg", "server")
		if hostname, err := os.Hostname(); err == nil {
			log = log.WithField("hostname", hostname)
		}
		if ip, err := utils.GetExternalIp(); err != nil {
			log = log.WithField("ip", ip)
		}
	})
}
