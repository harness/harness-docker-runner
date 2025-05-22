// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/harness/harness-docker-runner/config"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/logger"
	"github.com/harness/harness-docker-runner/pipeline/runtime"
)

// Handler returns an http.Handler that exposes the service resources.
func Handler(config *config.Config, engine *engine.Engine, stepExecutor *runtime.StepExecutor) http.Handler {
	r := chi.NewRouter()
	r.Use(logger.Middleware)
	r.Use(middleware.Recoverer)

	// Setup stage endpoint
	r.Mount("/setup", func() http.Handler {
		sr := chi.NewRouter()
		sr.Post("/", HandleSetup(config))
		return sr
	}())

	// Destroy stage endpoint
	r.Mount("/destroy", func() http.Handler {
		sr := chi.NewRouter()
		sr.Post("/", HandleDestroy())
		return sr
	}())

	// Start step endpoint
	r.Mount("/step", func() http.Handler {
		sr := chi.NewRouter()
		sr.Post("/", HandleStartStep(config))
		return sr
	}())

	// Poll step endpoint
	r.Mount("/poll_step", func() http.Handler {
		sr := chi.NewRouter()
		sr.Post("/", HandlePollStep(stepExecutor))
		return sr
	}())

	// Health check
	r.Mount("/healthz", func() http.Handler {
		sr := chi.NewRouter()
		sr.Get("/", HandleHealth(config))
		return sr
	}())

	return r
}
