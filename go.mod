module github.com/rai-project/raid

go 1.12

require (
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Unknwon/com v0.0.0-20151008135407-28b053d5a292
	github.com/airbrake/gobrake v3.7.4+incompatible // indirect
	github.com/caio/go-tdigest v2.3.0+incompatible // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e
	github.com/cpuguy83/go-md2man v1.0.10 // indirect
	github.com/evalphobia/logrus_fluent v0.4.0 // indirect
	github.com/evalphobia/logrus_kinesis v0.2.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20201116121440-e84ac1befdf8 // indirect
	github.com/fatih/color v1.7.0
	github.com/go-openapi/spec v0.20.3 // indirect
	github.com/googleapis/gnostic v0.5.4 // indirect
	github.com/knq/pemutil v0.0.0-20181215144041-fb6fad722528 // indirect
	github.com/knq/sdhook v0.0.0-20190114230312-41b9ccbff0b5 // indirect
	github.com/leesper/go_rng v0.0.0-20190531154944-a612b043e353 // indirect
	github.com/lithammer/dedent v1.1.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/openzipkin/zipkin-go v0.1.3 // indirect
	github.com/pkg/errors v0.9.1
	github.com/rai-project/cmd v0.0.0-20181119122707-69f8e596de1c
	github.com/rai-project/config v0.0.0-20190322074539-d39524e3455d
	github.com/rai-project/googlecloud v0.0.0-20181119123731-8938dc83da61 // indirect
	github.com/rai-project/logger v0.0.0-20181119115247-3edfaed4af1c
	github.com/rai-project/server v0.0.0-20190322080104-08a695459727
	github.com/sebest/logrusly v0.0.0-20180315190218-3235eccb8edc // indirect
	github.com/segmentio/go-loggly v0.5.0 // indirect
	github.com/sirupsen/logrus v1.4.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/viper v1.7.0
	github.com/vrecan/death v0.0.0-20180327224622-8f7e3eef97d0
	github.com/wercker/journalhook v0.0.0-20180428041537-5d0a5ae867b3 // indirect
	go.uber.org/goleak v1.1.10 // indirect
	golang.org/x/build v0.0.0-20190314133821-5284462c4bec // indirect
	gonum.org/v1/gonum v0.8.2 // indirect
	gopkg.in/gemnasium/logrus-airbrake-hook.v3 v3.0.2 // indirect
	gopkg.in/gemnasium/logrus-graylog-hook.v2 v2.0.7 // indirect
	gopkg.in/go-playground/assert.v1 v1.2.1 // indirect
	k8s.io/cli-runtime v0.20.4 // indirect
	k8s.io/client-go v11.0.0+incompatible // indirect
	k8s.io/kube-openapi v0.0.0-20210305164622-f622666832c1 // indirect
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10 // indirect
)

replace github.com/rai-project/server => ../server

replace k8s.io/client-go => k8s.io/client-go v0.20.0
