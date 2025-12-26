// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

// Copyright 2019 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

// Package internal contains runner internals.
package paths

import (
	"os"
	"path/filepath"
)

const (
	defaultWorkingDir = "/tmp"
	workingDirEnvVar  = "WORKING_DIR"
)

var workingDir string

func init() {
	workingDir = os.Getenv(workingDirEnvVar)
	if workingDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			workingDir = defaultWorkingDir
		} else {
			workingDir = cwd
		}
	}
	// Ensure the path is absolute
	if !filepath.IsAbs(workingDir) {
		absPath, err := filepath.Abs(workingDir)
		if err == nil {
			workingDir = absPath
		}
	}
}

// JoinPath joins the working directory with the provided path segments
// If the path is already absolute, it returns the path as-is
func JoinPath(elem ...string) string {
	if len(elem) == 0 {
		return workingDir
	}
	// If the first element is an absolute path, use it directly
	if filepath.IsAbs(elem[0]) {
		return filepath.Join(elem...)
	}
	// Otherwise, join with working directory
	return filepath.Join(append([]string{workingDir}, elem...)...)
}

// GetSharedVolPath returns the path for shared volume
func GetSharedVolPath() string {
	return JoinPath("engine")
}

// GetTIVolPath returns the path for TI volume
func GetTIVolPath() string {
	return JoinPath("ti")
}

// ResolveHostPath resolves a host path to be within the working directory
// System paths (like Docker socket) are returned as-is
// Other absolute paths are made relative to the working directory
func ResolveHostPath(path string) string {
	// If it's already an absolute path, strip the leading / and make it relative to working dir
	// This ensures paths like /harness become {WORKING_DIR}/harness
	if filepath.IsAbs(path) {
		// Remove leading separator(s)
		cleanPath := filepath.Clean(path)
		if len(cleanPath) > 0 && (cleanPath[0] == '/' || cleanPath[0] == '\\') {
			cleanPath = cleanPath[1:]
		}
		// Handle Windows drive letters (C:, D:, etc.)
		if len(cleanPath) >= 2 && cleanPath[1] == ':' {
			cleanPath = cleanPath[2:]
			if len(cleanPath) > 0 && (cleanPath[0] == '/' || cleanPath[0] == '\\') {
				cleanPath = cleanPath[1:]
			}
		}
		return JoinPath(cleanPath)
	}
	// If it's relative, join with working directory
	return JoinPath(path)
}
