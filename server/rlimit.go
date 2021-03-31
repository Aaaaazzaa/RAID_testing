package server

import (
	"github.com/minio/minio/pkg/sys"
	"github.com/rai-project/config"
)

func setMaxFileRLimit() error {
	max := func(a, b uint64) uint64 {
		if a > b {
			return a
		}
		return b
	}

	softLimit := Config.RLimitFileSoft
	hardLimit := Config.RLimitFileHard

	sysSoftLimit, sysHardLimit, err := sys.GetMaxOpenFileLimit()
	if err == nil {
		softLimit = max(softLimit, sysSoftLimit)
		hardLimit = max(hardLimit, sysHardLimit)
	}

	return sys.SetMaxOpenFileLimit(softLimit, hardLimit)
}

func init() {
	config.AfterInit(func() {
		Config.Wait()
		if err := setMaxFileRLimit(); err != nil {
			log.WithError(err).Error("cannot set maximum file limit")
		}
	})
}
