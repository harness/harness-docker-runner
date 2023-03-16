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
	"strings"
	"time"

	"github.com/dchest/uniuri"
	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/config"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/engine/docker"
	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/harness/harness-docker-runner/executor"
	"github.com/harness/harness-docker-runner/livelog"
	"github.com/harness/harness-docker-runner/logger"
	"github.com/harness/harness-docker-runner/pipeline"
	prruntime "github.com/harness/harness-docker-runner/pipeline/runtime"
	"github.com/harness/harness-docker-runner/ti"
	tiCfg "github.com/harness/lite-engine/ti/config"

	"github.com/sirupsen/logrus"
)

// random generator function
var random = func() string {
	return uniuri.NewLen(20)
}

// HandleSetup returns an http.HandlerFunc that does the initial setup
// for executing the step
func HandleSetup(config *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := time.Now()

		var s api.SetupRequest
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			WriteBadRequest(w, err)
			return
		}
		id := s.ID

		updateVolumes(s)

		// Add ti volume where all the TI related data (CG, Agent logs, config) will be stored
		// Add this dir to TIConfig for uploading the data
		tiVolume := getTiVolume(s.ID)
		s.Volumes = append(s.Volumes, tiVolume)
		tiConfig := getTiCfg(s.TIConfig, tiVolume.HostPath.Path)

		setProxyEnvs(s.Envs)
		engine, err := engine.NewEnv(docker.Opts{})
		if err != nil {
			logger.FromRequest(r).WithError(err).Errorln("could not instantiate engine for the execution")
			WriteError(w, err)
			return
		}
		stepExecutor := prruntime.NewStepExecutor(engine)
		state := pipeline.NewState()
		state.Set(s.Volumes, s.Secrets, s.LogConfig, tiConfig, s.SetupRequestConfig.Network.ID)

		log := logrus.New()
		var logr *logrus.Entry
		if s.LogConfig.URL == "" {
			log.Out = os.Stdout
		} else {
			client := state.GetLogStreamClient()
			wc := livelog.New(client, s.LogKey, id, nil)
			defer func() {
				if err := wc.Close(); err != nil {
					logrus.WithError(err).Debugln("failed to close log stream")
				}
			}()

			log.Out = wc
			log.SetLevel(logrus.TraceLevel)
		}
		logr = log.WithField("id", s.ID).
			WithField("correlation_id", s.CorrelationID)

		logr.Traceln("starting setup execution")
		logger.FromRequest(r).Traceln("starting the setup process")

		if s.MountDockerSocket == nil || *s.MountDockerSocket { // required to support m1 where docker isn't installed.
			s.Volumes = append(s.Volumes, getDockerSockVolume())
		}

		// fmt.Printf("setup request config: %+v\n", s.SetupRequestConfig)
		s.Volumes = append(s.Volumes, getSharedVolume())
		s.Volumes = append(s.Volumes, getGlobalVolumes(config)...)

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

		// Add the state of this execution to the executor
		stageData := &executor.StageData{
			Engine:       engine,
			StepExecutor: stepExecutor,
			State:        state,
		}

		ex := executor.GetExecutor()
		if err := ex.Add(id, stageData); err != nil {
			logger.FromRequest(r).WithError(err).Errorln("could not store stage data")
			WriteError(w, err)
			return
		}

		if err := engine.Setup(r.Context(), cfg); err != nil {
			logger.FromRequest(r).WithError(err).
				WithField("latency", time.Since(st)).
				WithField("time", time.Now().Format(time.RFC3339)).
				Infoln("api: failed stage setup")
			WriteError(w, err)
			ex.Remove(id)
			return
		}

		WriteJSON(w, api.SetupResponse{IPAddress: "127.0.0.1"}, http.StatusOK)
		logger.FromRequest(r).
			WithField("latency", time.Since(st)).
			WithField("time", time.Now().Format(time.RFC3339)).
			Infoln("api: successfully completed the stage setup")

		logger.FromRequest(r).Traceln("completed the setup process")
		logr.Traceln("completed the setup process")
	}
}

// updates the volume paths to make them compatible with the Docker runner.
// It hashes the clone path based on the runtime identifier.
func updateVolumes(r api.SetupRequest) {
	for _, v := range r.Volumes {
		if v.HostPath != nil {
			// Update the clone path to be created and removed once the build is completed
			// Hash the path with a unique identifier to avoid clashes.
			if v.HostPath.ID == "harness" {
				v.HostPath.Create = true
				v.HostPath.Remove = true
				v.HostPath.Path = v.HostPath.Path + "-" + sanitize(r.ID)
			}
		}
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

// getTiVolume returns a volume (directory) which is used to store TI related data
func getTiVolume(setupID string) *spec.Volume {
	tiDir := fmt.Sprintf("%s-%s", ti.VolumePath, sanitize(setupID))
	return &spec.Volume{
		HostPath: &spec.VolumeHostPath{
			Name:   ti.VolumeName,
			Path:   tiDir,
			Create: true,
			Remove: true,
		},
	}
}

func sanitize(r string) string {
	return strings.ReplaceAll(r, "[-_]", "")
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

func getGlobalVolumes(config *config.Config) []*spec.Volume {
	var volumes []*spec.Volume
	runnerVolumes := config.Runner.Volumes
	for _, runnerVolume := range runnerVolumes {
		volume, err := parseVolume(runnerVolume)
		if err != nil {
			panic(err)
		}
		volumes = append(volumes, volume)
	}
	return volumes
}

func parseVolume(runnerVolume string) (volume *spec.Volume, err error) {

	z := strings.SplitN(runnerVolume, ":", 2)
	if len(z) != 2 {
		return volume, fmt.Errorf("volume %s is not in the format src:dest", runnerVolume)
	}
	return &spec.Volume{
		HostPath: &spec.VolumeHostPath{
			Name: z[0],
			Path: z[0],
			ID:   random(),
		},
	}, nil
}

func setProxyEnvs(environment map[string]string) {
	proxyEnvs := []string{"http_proxy", "https_proxy", "no_proxy", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
	for _, v := range proxyEnvs {
		os.Setenv(v, environment[v])
	}
}

func getTiCfg(t api.TIConfig, dataDir string) tiCfg.Cfg {
	cfg := tiCfg.New(t.URL, t.Token, t.AccountID, t.OrgID, t.ProjectID, t.PipelineID, t.BuildID, t.StageID, t.Repo,
		t.Sha, t.CommitLink, t.SourceBranch, t.TargetBranch, t.CommitBranch, dataDir, false)
	return cfg
}
