// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package pipeline

import (
	"sync"

	"github.com/harness/lite-engine/api"
	"github.com/harness/lite-engine/engine/spec"
	"github.com/harness/lite-engine/logstream"
	"github.com/harness/lite-engine/logstream/filestore"
	"github.com/harness/lite-engine/logstream/remote"
)

var (
	state *State
	once  sync.Once
)

const (
	SharedVolPath = "/tmp/engine"
	SharedVolName = "_engine"
)

// State stores the pipeline state.
type State struct {
	mu        sync.Mutex
	volumes   []*spec.Volume
	logConfig api.LogConfig
	tiConfig  api.TIConfig
	secrets   []string
	logClient logstream.Client
	network   string
}

func NewState() *State {
	return &State{
		volumes:   make([]*spec.Volume, 0),
		logConfig: api.LogConfig{},
		tiConfig:  api.TIConfig{},
		secrets:   make([]string, 0),
		logClient: nil,
	}
}

func (s *State) Set(volumes []*spec.Volume, secrets []string, logConfig api.LogConfig, tiConfig api.TIConfig, network string) { // nolint:gocritic
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volumes = volumes
	s.secrets = secrets
	s.logConfig = logConfig
	s.tiConfig = tiConfig
	s.network = network
}

func (s *State) GetSecrets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.secrets
}

func (s *State) GetVolumes() []*spec.Volume {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.volumes
}

func (s *State) GetLogStreamClient() logstream.Client {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.logClient == nil {
		if s.logConfig.URL != "" {
			s.logClient = remote.NewHTTPClient(s.logConfig.URL, s.logConfig.AccountID,
				s.logConfig.Token, s.logConfig.IndirectUpload, false)
		} else {
			s.logClient = filestore.New(SharedVolPath)
		}
	}
	return s.logClient
}

func (s *State) GetTIConfig() *api.TIConfig {
	s.mu.Lock()
	defer s.mu.Unlock()

	return &s.tiConfig
}

func (s *State) GetNetwork() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.network
}

func GetState() *State {
	once.Do(func() {
		state = &State{
			mu:        sync.Mutex{},
			logConfig: api.LogConfig{},
			tiConfig:  api.TIConfig{},
			secrets:   make([]string, 0),
			volumes:   make([]*spec.Volume, 0),
			logClient: nil,
		}
	})
	return state
}
