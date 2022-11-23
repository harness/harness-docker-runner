// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package main

import (
	"github.com/harness/harness-docker-runner/cli"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cli.Command()
}
