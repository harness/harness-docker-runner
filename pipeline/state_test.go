package pipeline

import (
	"testing"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/logstream/remote"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogStreamClient_PlumbsLogResilient(t *testing.T) {
	for _, logResilient := range []bool{true, false} {
		s := NewState()
		s.Set(nil, nil, api.LogConfig{URL: "http://log-service.test", AccountID: "acct"},
			tiCfg.Cfg{}, "network", logResilient)

		client, ok := s.GetLogStreamClient().(*remote.HTTPClient)
		require.True(t, ok, "expected a remote HTTP client when log config URL is set")
		assert.Equal(t, logResilient, client.LogResilient)
	}
}

func TestGetLogStreamClient_FilestoreWhenNoURL(t *testing.T) {
	s := NewState()
	s.Set(nil, nil, api.LogConfig{}, tiCfg.Cfg{}, "network", true)

	_, ok := s.GetLogStreamClient().(*remote.HTTPClient)
	assert.False(t, ok, "expected the filestore client when no log config URL is set")
}

func TestGetLogStreamClient_CachesClient(t *testing.T) {
	s := NewState()
	s.Set(nil, nil, api.LogConfig{URL: "http://log-service.test"}, tiCfg.Cfg{}, "network", true)

	first := s.GetLogStreamClient()
	second := s.GetLogStreamClient()
	assert.Same(t, first, second)
}
