// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/harness/lite-engine/api"
	"github.com/harness/lite-engine/engine"
	"github.com/harness/lite-engine/engine/spec"
	"github.com/harness/lite-engine/executor"
	"github.com/harness/lite-engine/logger"
	"github.com/harness/lite-engine/pipeline"
	leruntime "github.com/harness/lite-engine/pipeline/runtime"
)

// HandleExecuteStep returns an http.HandlerFunc that executes a step
func HandleSetup(engine *engine.Engine) http.HandlerFunc {
	fmt.Println("enter HandleSetup")
	return func(w http.ResponseWriter, r *http.Request) {
		st := time.Now()

		var s api.SetupRequest
		err := json.NewDecoder(r.Body).Decode(&s)
		//TODO:xun
		s.Network.ID = s.ID
		if err != nil {
			WriteBadRequest(w, err)
			return
		}
		id := s.ID
		fmt.Println("Handle SetupRequest: %s", s)

		setProxyEnvs(s.Envs)
		state := pipeline.GetState()
		state.Set(s.Secrets, s.LogConfig, s.TIConfig)

		if s.MountDockerSocket == nil || *s.MountDockerSocket { // required to support m1 where docker isn't installed.
			s.Volumes = append(s.Volumes, getDockerSockVolume())
		}
		s.Volumes = append(s.Volumes, getSharedVolume())
		cfg := &spec.PipelineConfig{
			Envs:    s.Envs,
			Network: s.Network,
			Platform: spec.Platform{
				OS:   runtime.GOOS,
				Arch: runtime.GOARCH,
			},
			Volumes:           s.Volumes,
			Files:             s.Files,
			EnableDockerSetup: s.MountDockerSocket,
		}

		if err := engine.Setup(r.Context(), cfg); err != nil {
			logger.FromRequest(r).
				WithField("latency", time.Since(st)).
				WithField("time", time.Now().Format(time.RFC3339)).
				Infoln("api: failed stage setup")
			WriteError(w, err)
			return
		}

		// Setup executors for all stage steps
		stepExecutors := []*leruntime.StepExecutor{}
		// Add the state of this execution to the executor
		stageData := &executor.StageData{
			Engine:        engine,
			StepExecutors: stepExecutors,
			State:         state,
		}
		ex := executor.GetExecutor()
		ex.Add(id, stageData)

		//TODO:xun
		WriteJSON(w, api.SetupResponse{IPAddress: "127.0.0.1"}, http.StatusOK)
		logger.FromRequest(r).
			WithField("latency", time.Since(st)).
			WithField("time", time.Now().Format(time.RFC3339)).
			Infoln("api: successfully completed the stage setup")
	}
}

func getSharedVolume() *spec.Volume {
	return &spec.Volume{
		HostPath: &spec.VolumeHostPath{
			Name: pipeline.SharedVolName,
			Path: pipeline.SharedVolPath,
			ID:   "engine",
		},
	}
}

func getDockerSockVolume() *spec.Volume {
	path := engine.DockerSockUnixPath
	if runtime.GOOS == "windows" {
		path = engine.DockerSockWinPath
	}
	return &spec.Volume{
		HostPath: &spec.VolumeHostPath{
			Name: engine.DockerSockVolName,
			Path: path,
			ID:   "docker",
		},
	}
}

func setProxyEnvs(environment map[string]string) {
	proxyEnvs := []string{"http_proxy", "https_proxy", "no_proxy", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
	for _, v := range proxyEnvs {
		os.Setenv(v, environment[v])
	}
}
