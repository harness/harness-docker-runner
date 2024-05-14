// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/executor"
	"github.com/harness/harness-docker-runner/logger"
)

const (
	destroyTimeout = 10 * time.Minute
)

// HandleDestroy returns an http.HandlerFunc that destroy the stage resources
func HandleDestroy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		go func(r *http.Request, s api.DestroyRequest) {
			b := createBackoff(destroyTimeout)
			cnt := 0
			var lastErr error

			for {
				duration := b.NextBackOff()
				<-time.After(duration)

				if err := handleDestroyInternal(r, s); err != nil {
					if duration == backoff.Stop {
						logger.FromRequest(r).WithField("id", s.ID).WithError(err).Errorln("could not cleanup resources")
						return
					}
					if lastErr == nil || lastErr.Error() != err.Error() {
						logger.FromRequest(r).WithField("id", s.ID).WithError(err).Errorln("could not cleanup resources, retry count ", cnt)
						lastErr = err
					}
					cnt++
					continue
				}
				return
			}
		}(r, s)
		WriteJSON(w, api.DestroyResponse{}, http.StatusOK)
	}
}

func handleDestroyInternal(r *http.Request, s api.DestroyRequest) error {
	st := time.Now()
	ex := executor.GetExecutor()
	d, err := ex.Get(s.ID)
	if err != nil {
		return fmt.Errorf("stage mapping does not exist")
	}
	if d != nil {
		logger.FromRequest(r).WithField("id", s.ID).Traceln("starting the destroy process")
		if err := d.Engine.Destroy(r.Context()); err != nil {
			return err
		} else {
			ex.Remove(s.ID)
			logger.FromRequest(r).
				WithField("latency", time.Since(st)).
				WithField("time", time.Now().Format(time.RFC3339)).
				Infoln("api: successfully destroyed the stage resources")
		}
	}
	return nil
}

func createBackoff(maxElapsedTime time.Duration) *backoff.ExponentialBackOff {
	exp := backoff.NewExponentialBackOff()
	exp.MaxElapsedTime = maxElapsedTime
	return exp
}
