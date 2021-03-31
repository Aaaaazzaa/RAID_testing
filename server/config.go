package server

import (
	"runtime"
	"time"

	"github.com/k0kubun/pp"
	"github.com/rai-project/config"
	nvidiasmi "github.com/rai-project/nvidia-smi"
	"github.com/rai-project/vipertags"
)

type serverConfig struct {
	ClientAppName                       string        `json:"client_app_name" config:"client.app_name" default:"rai"`
	ClientUploadBucketName              string        `json:"upload_bucket" config:"client.upload_bucket" default:"files.rai-project.com"`
	ClientUploadDestinationDirectory    string        `json:"upload_destination_directory" config:"client.upload_destination_directory" default:"userdata"`
	ClientJobQueueName                  string        `json:"job_queue_name" config:"client.job_queue_name"`
	ClientJobTimeLimit                  time.Duration `json:"client_job_time_limit" config:"client.job_time_limit"`
	NumberOfWorkers                     int           `json:"number_of_workers" config:"server.number_of_workers"`
	DisableRAIDockerNamespaceProtection bool          `json:"disable_rai_docker_namespace_protection" config:"server.disable_rai_docker_namespace_protection" default:"FALSE"`
	RLimitFileSoft                      uint64        `json:"rlimit_file_soft" config:"server.rlimit_file_soft"`
	RLimitFileHard                      uint64        `json:"rlimit_file_hard" config:"server.rlimit_file_hard"`
	done                                chan struct{} `json:"-" config:"-"`
}

var (
	Config = &serverConfig{
		done: make(chan struct{}),
	}

	DefaultRLimitFileSoft uint64 = 500000
	DefaultRLimitFileHard uint64 = 500000
)

func (serverConfig) ConfigName() string {
	return "Server"
}

func (a *serverConfig) SetDefaults() {
	vipertags.SetDefaults(a)
}

func (a *serverConfig) Read() {
	defer close(a.done)
	vipertags.Fill(a)
	if a.ClientJobQueueName == "" || a.ClientJobQueueName == "default" {
		a.ClientJobQueueName = config.App.Name + "_" + runtime.GOARCH
	}
	if a.NumberOfWorkers == 0 {
		a.NumberOfWorkers = runtime.NumCPU()
		if nvidiasmi.HasGPU {
			a.NumberOfWorkers = nvidiasmi.HyperQSize
		}
	}
	if a.RLimitFileSoft == 0 {
		a.RLimitFileSoft = DefaultRLimitFileSoft
	}
	if a.RLimitFileHard == 0 {
		a.RLimitFileHard = DefaultRLimitFileHard
	}
}

func (c serverConfig) Wait() {
	<-c.done
}

func (c serverConfig) String() string {
	return pp.Sprintln(c)
}

func (c serverConfig) Debug() {
	log.Debug("Server Config = ", c)
}

func init() {
	config.Register(Config)
}
