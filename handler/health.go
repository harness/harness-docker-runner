// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"github.com/harness/harness-docker-runner/config"
	"net/http"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/setup"
	"github.com/harness/harness-docker-runner/version"
	"github.com/sirupsen/logrus"
)

func HandleHealth(config *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logrus.Infoln("handler: HandleHealth()")
		instanceInfo := setup.GetInstanceInfo()
		dockerOK := setup.DockerInstalled(instanceInfo, config)
		gitOK := setup.GitInstalled(instanceInfo)
		version := version.Version
		response := api.HealthResponse{
			Version:         version,
			DockerInstalled: dockerOK,
			GitInstalled:    gitOK,
			LiteEngineLog:   setup.GetLiteEngineLog(instanceInfo),
			OK:              dockerOK && gitOK,
		}
		WriteJSON(w, response, http.StatusOK)
	}
}
