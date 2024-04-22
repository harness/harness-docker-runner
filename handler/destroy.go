// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/harness/harness-docker-runner/executor"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/logger"
)

// HandleDestroy returns an http.HandlerFunc that destroy the stage resources
func HandleDestroy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := time.Now()

		var s api.DestroyRequest
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			WriteBadRequest(w, err)
			return
		}

		if s.ID == "" {
			logger.FromRequest(r).Errorln("id not specified")
			WriteError(w, errors.New("id not specified"))
			return
		}

		ex := executor.GetExecutor()
		d, err := ex.Get(s.ID)

		if err != nil {
			logger.FromRequest(r).WithError(err).WithField("id", s.ID).Errorln("stage mapping does not exist")
			WriteNotFound(w, err)
			return
		}

		if d != nil {
			logger.FromRequest(r).WithField("id", s.ID).Traceln("starting the destroy process")
			if err := d.Engine.Destroy(r.Context()); err != nil {
				WriteError(w, err)
			} else {
				ex.Remove(s.ID)
				logger.FromRequest(r).
					WithField("latency", time.Since(st)).
					WithField("time", time.Now().Format(time.RFC3339)).
					Infoln("api: successfully destroyed the stage resources")
				WriteJSON(w, api.DestroyResponse{}, http.StatusOK)
			}
		}
	}
}
