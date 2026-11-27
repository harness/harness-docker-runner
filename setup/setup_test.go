// Copyright 2026 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package setup

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/harness/harness-docker-runner/config" //nolint:depguard // Required to exercise PrepareSystem.
)

type downloadCall struct {
	url       string
	targetDir string
	binary    string
}

func TestInstallHarnessCLI(t *testing.T) {
	originalDownloadCLI := downloadCLI
	t.Cleanup(func() { downloadCLI = originalDownloadCLI })

	for _, osType := range []string{"linux", osxString} {
		t.Run(osType, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			var calls []downloadCall
			downloadCLI = func(url, targetDir, binary string) error {
				calls = append(calls, downloadCall{url: url, targetDir: targetDir, binary: binary})
				return nil
			}

			const baseURI = "https://storage.googleapis.com/harness-ti/harness-cli-unified/v1.0.2"
			installHarnessCLI(InstanceInfo{osType: osType, archType: "arm64"}, baseURI)

			want := []downloadCall{
				{
					url:       baseURI + "/harness-" + osType + "-arm64",
					targetDir: "/usr/local/bin",
					binary:    "harness",
				},
			}
			if osType == osxString {
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
	const baseURI = "https://storage.googleapis.com/harness-ti/harness-cli-unified/v1.0.2"
	want := baseURI + "/harness-windows-amd64.exe"
	if got := cliDownloadURL(baseURI, "harness", "windows", "amd64"); got != want {
		t.Fatalf("cliDownloadURL() = %q, want %q", got, want)
	}
}

func TestCLIInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses shell scripts as fake CLI binaries")
	}

	t.Run("installed", func(t *testing.T) {
		binDir := t.TempDir()
		writeFakeCLI(t, binDir, "hcli", "0")
		writeFakeCLI(t, binDir, "harness", "0")
		t.Setenv("PATH", binDir)
		info := InstanceInfo{osType: "linux"}
		if !HcliInstalled(info) {
			t.Fatal("HcliInstalled() = false, want true")
		}
		if !HarnessCLIInstalled(info) {
			t.Fatal("HarnessCLIInstalled() = false, want true")
		}
	})

	t.Run("version fails", func(t *testing.T) {
		binDir := t.TempDir()
		writeFakeCLI(t, binDir, "harness", "1")
		t.Setenv("PATH", binDir)
		if cliInstalled(InstanceInfo{osType: "linux"}, "harness") {
			t.Fatal("cliInstalled() = true, want false")
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if cliInstalled(InstanceInfo{osType: "linux"}, "harness") {
			t.Fatal("cliInstalled() = true, want false")
		}
	})

	t.Run("windows executable name", func(t *testing.T) {
		binDir := t.TempDir()
		writeFakeCLI(t, binDir, "harness.exe", "0")
		t.Setenv("PATH", binDir)
		if !cliInstalled(InstanceInfo{osType: "windows"}, "harness") {
			t.Fatal("cliInstalled() = false, want true")
		}
	})
}

func TestInstallHarnessCLIWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test simulates Windows paths on a Unix host")
	}

	originalDownloadCLI := downloadCLI
	t.Cleanup(func() { downloadCLI = originalDownloadCLI })
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDir) })

	var calls []downloadCall
	downloadCLI = func(url, targetDir, binary string) error {
		calls = append(calls, downloadCall{url: url, targetDir: targetDir, binary: binary})
		return nil
	}

	const baseURI = "https://storage.googleapis.com/harness-ti/harness-cli-unified/v1.0.2"
	installHarnessCLI(InstanceInfo{osType: "windows", archType: "amd64"}, baseURI)

	want := []downloadCall{{
		url:       baseURI + "/harness-windows-amd64.exe",
		targetDir: `C:\Program Files\harness-hcli`,
		binary:    "harness.exe",
	}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("downloads = %#v, want %#v", calls, want)
	}
	if !strings.Contains(os.Getenv("PATH"), `C:\Program Files\harness-hcli`) {
		t.Fatalf("PATH does not contain Windows CLI directory: %q", os.Getenv("PATH"))
	}
}

func TestInstallCLIErrors(t *testing.T) {
	originalDownloadCLI := downloadCLI
	t.Cleanup(func() { downloadCLI = originalDownloadCLI })

	t.Run("host download", func(t *testing.T) {
		calls := 0
		downloadCLI = func(_, _, _ string) error {
			calls++
			return errors.New("download failed")
		}
		installHarnessCLI(InstanceInfo{osType: "linux", archType: "amd64"}, "https://example.com")
		if calls != 1 {
			t.Fatalf("download calls = %d, want 1", calls)
		}
	})

	t.Run("container download", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		calls := 0
		downloadCLI = func(_, _, _ string) error {
			calls++
			if calls == 2 {
				return errors.New("download failed")
			}
			return nil
		}
		installHarnessCLI(InstanceInfo{osType: osxString, archType: "arm64"}, "https://example.com")
		if calls != 2 {
			t.Fatalf("download calls = %d, want 2", calls)
		}
	})

	t.Run("container directory", func(t *testing.T) {
		homeFile := filepath.Join(t.TempDir(), "home")
		if err := os.WriteFile(homeFile, []byte("not a directory"), 0600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		t.Setenv("HOME", homeFile)
		calls := 0
		downloadCLI = func(_, _, _ string) error {
			calls++
			return nil
		}
		installHarnessCLI(InstanceInfo{osType: osxString, archType: "arm64"}, "https://example.com")
		if calls != 1 {
			t.Fatalf("download calls = %d, want 1", calls)
		}
	})
}

func TestPrepareSystemInstallsHarnessCLI(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("PrepareSystem uses a fixed Docker path on non-Linux hosts")
	}

	binDir := t.TempDir()
	for _, name := range []string{"docker", "plugin", "envman", "hcli"} {
		writeFakeCLI(t, binDir, name, "0")
	}
	t.Setenv("PATH", binDir)

	originalDownloadCLI := downloadCLI
	t.Cleanup(func() { downloadCLI = originalDownloadCLI })
	var calls []downloadCall
	downloadCLI = func(url, targetDir, binary string) error {
		calls = append(calls, downloadCall{url: url, targetDir: targetDir, binary: binary})
		return nil
	}

	const baseURI = "https://storage.googleapis.com/harness-ti/harness-cli-unified/v1.0.2"
	cfg := &config.Config{}
	cfg.Server.HarnessCLIBinaryURI = baseURI
	PrepareSystem(cfg)

	want := []downloadCall{{
		url:       baseURI + "/harness-linux-" + runtime.GOARCH,
		targetDir: "/usr/local/bin",
		binary:    "harness",
	}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("downloads = %#v, want %#v", calls, want)
	}
}

func TestInstallHcli(t *testing.T) {
	originalDownloadCLI := downloadCLI
	t.Cleanup(func() { downloadCLI = originalDownloadCLI })
	var calls []downloadCall
	downloadCLI = func(url, targetDir, binary string) error {
		calls = append(calls, downloadCall{url: url, targetDir: targetDir, binary: binary})
		return nil
	}

	const baseURI = "https://storage.googleapis.com/harness-ti/hcli/v0.13"
	installHcli(InstanceInfo{osType: "linux", archType: "amd64"}, baseURI)
	want := []downloadCall{{
		url:       baseURI + "/hcli-linux-amd64",
		targetDir: "/usr/local/bin",
		binary:    "hcli",
	}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("downloads = %#v, want %#v", calls, want)
	}
}

func TestInstallHarnessCLIHOMEFallback(t *testing.T) {
	originalDownloadCLI := downloadCLI
	t.Cleanup(func() { downloadCLI = originalDownloadCLI })
	t.Setenv("HOME", "")
	var calls []downloadCall
	downloadCLI = func(url, targetDir, binary string) error {
		calls = append(calls, downloadCall{url: url, targetDir: targetDir, binary: binary})
		return nil
	}

	installHarnessCLI(InstanceInfo{osType: osxString, archType: "arm64"}, "https://example.com")
	if len(calls) != 2 {
		t.Fatalf("download calls = %d, want 2", len(calls))
	}
	if got, want := calls[1].targetDir, "/tmp/harness/bin"; got != want {
		t.Fatalf("container target directory = %q, want %q", got, want)
	}
}

func writeFakeCLI(t *testing.T, dir, name, exitCode string) {
	t.Helper()
	content := []byte("#!/bin/sh\nexit " + exitCode + "\n")
	if err := os.WriteFile(filepath.Join(dir, name), content, 0755); err != nil { //nolint:gosec // Test CLI must be executable.
		t.Fatalf("WriteFile() error = %v", err)
	}
}
