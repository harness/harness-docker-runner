// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package handler

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/go-chi/chi/middleware"
	"github.com/harness/harness-docker-runner/config"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/logger"
	"github.com/harness/harness-docker-runner/pipeline/runtime"
)

// Handler returns an http.Handler that exposes the service resources.
func Handler(config *config.Config, engine *engine.Engine, stepExecutor *runtime.StepExecutor) http.Handler {
	r := chi.NewRouter()
	// Set up CORS middleware options
    c := cors.New(cors.Options{
        AllowedOrigins:   []string{"*"}, // Allow all origins
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, // Allow specific HTTP methods
        AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "ngrok-skip-browser-warning"},
        ExposedHeaders:   []string{"Link"},
        AllowCredentials: false,
        MaxAge:           300, // Max age for the preflight request cache
    })
	r.Use(c.Handler)
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
		sr.Get("/", HandleHealth())
		return sr
	}())

	r.Mount("/debug", func() http.Handler {
		sr := chi.NewRouter()
		sr.Post("/", HandleDebug())
		// sr.Options("/", func(w http.ResponseWriter, r *http.Request) {
		// 	w.WriteHeader(http.StatusOK)
		// })
		return sr
	}())

	return r
}