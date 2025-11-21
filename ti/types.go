// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package ti

import "github.com/harness/harness-docker-runner/internal/paths"

const (
	VolumeName = "ti"
	VolumePath = "/tmp/ti"
)

// VolumePath returns the TI volume path
func GetVolumePath() string {
	return paths.GetTIVolPath()
}
