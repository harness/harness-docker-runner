// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

// Package server provides an HTTPS server with support for TLS
// and graceful shutdown.
package server

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/harness/harness-docker-runner/version"

	"github.com/docker/go-connections/tlsconfig"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// A Server defines parameters for running an HTTPS/TLS server.
type Server struct {
	Addr           string // TCP address to listen on
	Handler        http.Handler
	CAFile         string // CA certificate file
	CertFile       string // Server certificate PEM file
	KeyFile        string // Server key PEM file
	ClientCertFile string // Trusted client certificate PEM file for client authentication
	Insecure       bool   // run without TLS
}

// Start initializes a server to respond to HTTPS/TLS network requests.
func (s *Server) Start(ctx context.Context) error {
	// Uncomment the following line for local run
	// s.Insecure = true

	logrus.Infof("Runner version: %s", version.Version)

	var tlsConfig *tls.Config
	logrus.Infof("Runner version: %s", version.Version)
	if s.Insecure {
		tlsConfig = nil
		logrus.Warnln("RUNNING IN INSECURE MODE")
	} else {
		tlsOptions := tlsconfig.Options{
			CAFile:             s.CAFile,
			CertFile:           s.CertFile,
			KeyFile:            s.KeyFile,
			ExclusiveRootPools: true,
		}
		tlsOptions.ClientAuth = tls.RequireAndVerifyClientCert
		var err error
		tlsConfig, err = tlsconfig.Server(tlsOptions)
		if err != nil {
			return err
		}
		tlsConfig.MinVersion = tls.VersionTLS13
	}

	srv := &http.Server{
		Addr:      s.Addr,
		Handler:   s.Handler,
		TLSConfig: tlsConfig,
	}

	var g errgroup.Group
	g.Go(func() error {
		// Uncomment the following line for local run
		// s.Insecure = true

		if s.Insecure {
			return srv.ListenAndServe()
		}
		return srv.ListenAndServeTLS(s.CertFile, s.KeyFile)
	})
	g.Go(func() error {
		<-ctx.Done()
		srv.Shutdown(ctx) // nolint: errcheck
		return nil
	})
	return g.Wait()
}
