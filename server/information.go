package server

import (
	"runtime"

	"github.com/BenLubar/memoize"
	nvidiasmi "github.com/rai-project/nvidia-smi"
	"github.com/rai-project/utils"
)

func iinfo() map[string]interface{} {
	localIp, _ := utils.GetLocalIp()
	publicIp, _ := utils.GetExternalIp()
	return map[string]interface{}{
		"local_ip":  localIp,
		"public_ip": publicIp,
		"num_gpus":  nvidiasmi.GPUCount,
		"num_cpus":  runtime.NumCPU(),
	}
}

var info = memoize.Memoize(iinfo).(func() map[string]interface{})
