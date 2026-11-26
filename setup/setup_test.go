// Copyright 2026 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package setup

import (
	"path/filepath"
	"reflect"
	"testing"
)

type downloadCall struct {
	url       string
	targetDir string
	binary    string
}

func TestInstallHarnessCLI(t *testing.T) {
	originalDownloadCLI := downloadCLI
	t.Cleanup(func() { downloadCLI = originalDownloadCLI })

	for _, osType := range []string{"linux", "darwin"} {
		t.Run(osType, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			var calls []downloadCall
			downloadCLI = func(url, targetDir, binary string) error {
				calls = append(calls, downloadCall{url: url, targetDir: targetDir, binary: binary})
				return nil
			}

			const baseURI = "https://storage.googleapis.com/harness-ti/harness-cli-unified/v1.0.1"
			installHarnessCLI(InstanceInfo{osType: osType, archType: "arm64"}, baseURI)

			want := []downloadCall{
				{
					url:       baseURI + "/harness-" + osType + "-arm64",
					targetDir: "/usr/local/bin",
					binary:    "harness",
				},
			}
			if osType == "darwin" {
				want = append(want, downloadCall{
					url:       baseURI + "/harness-linux-arm64",
					targetDir: filepath.Join(homeDir, "harness", "bin"),
					binary:    "harness-linux",
				})
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("downloads = %#v, want %#v", calls, want)
			}
		})
	}
}

func TestCLIDownloadURLWindows(t *testing.T) {
	const baseURI = "https://storage.googleapis.com/harness-ti/harness-cli-unified/v1.0.1"
	want := baseURI + "/harness-windows-amd64.exe"
	if got := cliDownloadURL(baseURI, "harness", "windows", "amd64"); got != want {
		t.Fatalf("cliDownloadURL() = %q, want %q", got, want)
	}
}
