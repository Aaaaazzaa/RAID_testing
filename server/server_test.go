package server

import (
	"os"
	"testing"
	"time"

	"github.com/rai-project/config"
	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	svr, err := New()
	if !assert.NoError(t, err) {
		return
	}
	assert.NotNil(t, svr)

	err = svr.Connect()
	defer svr.Disconnect()

	if !assert.NoError(t, err) {
		return
	}
	time.Sleep(time.Minute)

}

func TestMain(m *testing.M) {
	config.Init(
		config.VerboseMode(true),
		config.DebugMode(true),
		config.ColorMode(true),
	)
	os.Exit(m.Run())
}
