// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package main

import (
	"runtime"

	"github.com/harness/harness-docker-runner/cli"
	_ "github.com/joho/godotenv/autoload"
	"github.com/kardianos/service"
)

func main() {
	if runtime.GOOS == "windows" {
		svcConfig := &service.Config{
			Name:        "harness-docker-runner-svc",
			DisplayName: "harness-docker-runner-svc",
			Description: "This is a service runing for harness-docker-runner",
		}

		runAsService(svcConfig, func() {
			cli.Command()
		})
	} else {
		cli.Command()
	}
}

func runAsService(svcConfig *service.Config, run func()) error {
	s, err := service.New(&program{exec: run}, svcConfig)
	if err != nil {
		return err
	}
	return s.Run()
}

type program struct {
	exec func()
}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	go p.exec()
	return nil
}
func (p *program) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	return nil
}
