// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/harness/lite-engine/executor"

	"github.com/harness/lite-engine/api"
	"github.com/harness/lite-engine/engine"
	"github.com/harness/lite-engine/engine/spec"
	"github.com/harness/lite-engine/logger"
	pruntime "github.com/harness/lite-engine/pipeline/runtime"
)

// HandleExecuteStep returns an http.HandlerFunc that executes a step
func HandleStartStep() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := time.Now()

		var s api.StartStepRequest
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			WriteBadRequest(w, err)
			return
		}

		if s.MountDockerSocket == nil || *s.MountDockerSocket { // required to support m1 where docker isn't installed.
			s.Volumes = append(s.Volumes, getDockerSockVolumeMount())
		}
		if len(s.StartStepRequestConfig.OutputVars) > 0 {
			s.Files = []*spec.File{
				{
					Path:  fmt.Sprintf("/tmp/engine/%s.out", s.ID),
					IsDir: false,
					Mode:  0777,
				},
			}
		}
		ex := executor.GetExecutor()
		stageData, err := ex.Get(s.StageRuntimeID)
		if err != nil {
			logger.FromRequest(r).Errorln(err.Error())
			WriteError(w, err)
			return
		}

		stageData.State.AppendSecrets(s.Secrets)

		s.StartStepRequestConfig.Network = stageData.State.GetNetwork()
		hv, err := getHarnessVolume(stageData.State.GetVolumes())
		if err != nil {
			WriteError(w, err)
			return
		}

		fmt.Printf("STEP STAGE_ID: %s\n NETWORK_ID: %s\n WORKDIR: %s", s.StageRuntimeID, stageData.State.GetNetwork(), hv.HostPath.Path)
		s.StartStepRequestConfig.WorkingDir = hv.HostPath.Path
		for _, v := range s.StartStepRequestConfig.Volumes {
			if v.Name == "harness" {
				v.Name = hv.HostPath.Name
				v.Path = hv.HostPath.Path
			}
		}

		ctx := r.Context()
		if err := stageData.StepExecutor.StartStep(ctx, &s, stageData.State.GetSecrets(), stageData.State.GetLogStreamClient()); err != nil {
			WriteError(w, err)
		}

		pollResp, err := stageData.StepExecutor.PollStep(ctx, &api.PollStepRequest{ID: s.ID})
		if err != nil {
			WriteError(w, err)
			return
		}

		if pollResp.Error != "" {
			WriteJSON(w, pollResp, http.StatusBadRequest)
		} else {
			WriteJSON(w, pollResp, http.StatusOK)
		}

		logger.FromRequest(r).
			WithField("latency", time.Since(st)).
			WithField("time", time.Now().Format(time.RFC3339)).
			Infoln("api: successfully started the step")
	}
}

func getHarnessVolume(volumes []*spec.Volume) (*spec.Volume, error) {
	for _, v := range volumes {
		if v.HostPath != nil {
			if v.HostPath.Name == "harness" {
				return v, nil
			}
		}
	}
	return nil, errors.New("could not parse the harness volume from volume paths")
}

func HandlePollStep(e *pruntime.StepExecutor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := time.Now()

		var s api.PollStepRequest
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			WriteBadRequest(w, err)
			return
		}

		if response, err := e.PollStep(r.Context(), &s); err != nil {
			WriteError(w, err)
		} else {
			WriteJSON(w, response, http.StatusOK)
		}

		logger.FromRequest(r).
			WithField("latency", time.Since(st)).
			WithField("time", time.Now().Format(time.RFC3339)).
			Infoln("api: successfully polled the step response")
	}
}

func getDockerSockVolumeMount() *spec.VolumeMount {
	path := engine.DockerSockUnixPath
	if runtime.GOOS == "windows" {
		path = engine.DockerSockWinPath
	}
	return &spec.VolumeMount{
		Name: engine.DockerSockVolName,
		Path: path,
	}
}
