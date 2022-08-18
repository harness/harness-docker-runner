// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"encoding/json"
	"github.com/harness/lite-engine/executor"
	"net/http"
	"time"

	"github.com/harness/lite-engine/api"
	"github.com/harness/lite-engine/engine"
	"github.com/harness/lite-engine/logger"
)

// HandleDestroy returns an http.HandlerFunc that destroy the stage resources
func HandleDestroy(engine *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := time.Now()

		var s api.DestroyRequest
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			WriteBadRequest(w, err)
			return
		}

		ex := executor.GetExecutor()
		stageData, err := ex.Remove(s.ID)
		if err != nil {
			logger.FromRequest(r).Errorln(err.Error())
			WriteError(w, err)
		}

		if err := stageData.Engine.Destroy(r.Context()); err != nil {
			WriteError(w, err)
		} else {
			WriteJSON(w, api.DestroyResponse{}, http.StatusOK)
		}

		logger.FromRequest(r).
			WithField("latency", time.Since(st)).
			WithField("time", time.Now().Format(time.RFC3339)).
			Infoln("api: successfully destroyed the stage resources")
	}
}
