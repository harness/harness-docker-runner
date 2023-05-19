// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/harness/harness-docker-runner/config"
	"github.com/harness/harness-docker-runner/executor"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/harness/harness-docker-runner/util"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/harness/harness-docker-runner/logger"
	pruntime "github.com/harness/harness-docker-runner/pipeline/runtime"
)

// HandleExecuteStep returns an http.HandlerFunc that executes a step
func HandleStartStep(config *config.Config) http.HandlerFunc {
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
		ex := executor.GetExecutor()
		stageData, err := ex.Get(s.StageRuntimeID)
		if err != nil {
			logger.FromRequest(r).Errorln(err.Error())
			WriteError(w, err)
			return
		}
		s.Volumes = append(s.Volumes, getSharedVolumeMount())
		s.Volumes = append(s.Volumes, getGlobalVolumesMount(config)...)

		stageData.State.AppendSecrets(s.Secrets)

		s.StartStepRequestConfig.Network = stageData.State.GetNetwork()
		hv, err := getHarnessVolume(stageData.State.GetVolumes())
		if err != nil {
			WriteError(w, err)
			return
		}

		s.StartStepRequestConfig.WorkingDir = hv.HostPath.Path
		for _, v := range s.StartStepRequestConfig.Volumes {
			if v.Name == "harness" {
				v.Name = hv.HostPath.Name
				v.Path = hv.HostPath.Path
				v.ContainerPath = hv.HostPath.ContainerPath
			}
		}

		updateDelegateCapacity(&s.StartStepRequestConfig)
		updateGitCloneConfig(&s.StartStepRequestConfig, hv)

		// fmt.Printf("start step request config: %+v\n", s.StartStepRequestConfig)

		ctx := r.Context()
		logger.FromRequest(r).WithField("stage_id", s.StageRuntimeID).
			WithField("step_id", s.ID).Traceln("starting step execution")
		if err := stageData.StepExecutor.StartStep(ctx, &s, stageData.State.GetSecrets(), stageData.State.GetLogStreamClient(), stageData.State.GetTIConfig()); err != nil {
			WriteError(w, err)
		}

		logger.FromRequest(r).WithField("stage_id", s.StageRuntimeID).
			WithField("step_id", s.ID).Traceln("starting polling for step response")

		pollResp, err := stageData.StepExecutor.PollStep(ctx, &api.PollStepRequest{ID: s.ID})
		logger.FromRequest(r).WithField("stage_id", s.StageRuntimeID).
			WithField("step_id", s.ID).WithField("poll_response", pollResp).
			Traceln("received polling response")
		if err != nil {
			WriteJSON(w, convert(err), http.StatusOK)
			return
		}

		WriteJSON(w, pollResp, http.StatusOK)

		logger.FromRequest(r).
			WithField("latency", time.Since(st)).
			WithField("time", time.Now().Format(time.RFC3339)).
			Infoln("api: successfully completed step execution")
	}
}

func convert(err error) api.PollStepResponse {
	if err == nil {
		return api.PollStepResponse{}
	}
	return api.PollStepResponse{Error: err.Error()}
}

func getSharedVolumeMount() *spec.VolumeMount {
	return &spec.VolumeMount{
		Name: pipeline.SharedVolName,
		Path: pipeline.SharedVolPath,
	}
}

func getGlobalVolumesMount(config *config.Config) []*spec.VolumeMount {
	var volumes []*spec.VolumeMount
	runnerVolumes := config.Runner.Volumes
	for _, runnerVolume := range runnerVolumes {
		volume, err := parseVolumeMount(runnerVolume)
		if err != nil {
			panic(err)
		}
		volumes = append(volumes, volume)
	}
	return volumes
}

func parseVolumeMount(runnerVolume string) (volume *spec.VolumeMount, err error) {
	z := strings.SplitN(runnerVolume, ";", 2)
	if len(z) != 2 {
		return volume, fmt.Errorf("volume %s is not in the format src:dest", runnerVolume)
	}

	return &spec.VolumeMount{
		Name: z[0],
		Path: z[1],
	}, nil
}

// this returns back the host volume which is being used to clone repositories
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

// TODO: Move this logic to Java so that we pass in the right arguments to the runner
func updateGitCloneConfig(s *api.StartStepRequestConfig, hv *spec.Volume) {
	if strings.Contains(s.Image, "harness/drone-git") {
		// Always use bind mounted path as the working directory
		s.WorkingDir = hv.HostPath.ContainerPath
		if ws, ok := s.Envs["DRONE_WORKSPACE"]; ok {
			// If it's an explicit git clone step, make sure the workspace is namespaced
			if strings.HasPrefix(ws, "/harness") || strings.HasPrefix(ws, "/tmp/harness") {
				last := ws[strings.LastIndex(ws, "/")+1:]
				if last == "" {
					// Retrieve the name from the remote URL. Eg: https://github.com/harness/drone-git should return drone-git
					if url, ok2 := s.Envs["DRONE_REMOTE_URL"]; ok2 {
						last = url[strings.LastIndex(url, "/")+1:]
					}
				}
				ws := filepath.Join(s.WorkingDir, last)
				s.Envs["DRONE_WORKSPACE"] = ws
			} else if !filepath.IsAbs(ws) {
				s.Envs["DRONE_WORKSPACE"] = filepath.Join(s.WorkingDir, ws)
			}
		}
	}
}

func updateDelegateCapacity(s *api.StartStepRequestConfig) {
	if ws, ok := s.Envs["HARNESS_DELEGATE_ID"]; ok {
		util.RegisterDelegateCapacity(ws)
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
