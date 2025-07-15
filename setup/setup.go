// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package setup

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/harness/harness-docker-runner/config"
	"github.com/sirupsen/logrus"
)

type InstanceInfo struct {
	osType   string
	archType string
}

func GetInstanceInfo() InstanceInfo {
	osType := runtime.GOOS
	archType := runtime.GOARCH
	return InstanceInfo{osType: osType, archType: archType}
}

func PrepareSystem(config *config.Config) {
	instanceInfo := GetInstanceInfo()
	if !GitInstalled(instanceInfo) {
		installGit(instanceInfo)
	}
	if !DockerInstalled(instanceInfo) {
		installDocker(instanceInfo)
	}
	if !(PluginInstalled(instanceInfo)) {
		installPlugin(instanceInfo, config.Server.PluginBinaryURI)
	}
	if !(EnvmanInstalled(instanceInfo)) {
		installEnvman(instanceInfo, config.Server.EnvmanBinaryURI)
	}
}

const windowsString = "windows"
const osxString = "darwin"

func GitInstalled(instanceInfo InstanceInfo) (installed bool) {
	logrus.Infoln("checking git is installed")
	switch instanceInfo.osType {
	case windowsString:
		logrus.Infoln("windows: we should check git installation here")
	default:
		_, err := os.Stat("/usr/bin/git")
		if os.IsNotExist(err) {
			logrus.Warnln("git is not installed")
		}
	}
	return true
}

func PluginInstalled(instanceInfo InstanceInfo) (installed bool) {
	logrus.Infoln("checking plugin is installed")
	plugin := "plugin"
	switch instanceInfo.osType {
	case windowsString:
		plugin = "plugin.exe"
	}

	path, err := exec.LookPath(plugin)
	if err != nil {
		logrus.Infoln("plugin binary not found in PATH")
		return false
	}
	cmd := exec.Command(path, "healthz")
	if err := cmd.Run(); err != nil {
		logrus.Infof("Error running plugin healthz: %v\n", err)
		return false
	}
	return true
}

func EnvmanInstalled(instanceInfo InstanceInfo) (installed bool) {
	logrus.Infoln("checking envman is installed")
	envman := "envman"
	switch instanceInfo.osType {
	case windowsString:
		//envman doesn't exist for windows
		return
	}

	path, err := exec.LookPath(envman)
	if err != nil {
		logrus.Infoln("envman binary not found in PATH")
		return false
	}
	cmd := exec.Command(path, "version")
	if err := cmd.Run(); err != nil {
		logrus.Infof("Error running envman version: %v\n", err)
		return false
	}
	return true
}

func DockerInstalled(instanceInfo InstanceInfo) (installed bool) {
	logrus.Infoln("checking docker is installed")
	switch instanceInfo.osType {
	case windowsString:
		logrus.Infoln("windows: we should check docker installation here")
	case osxString:
		cmd := exec.Command("/usr/local/bin/docker", "ps")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return false
		}
	default:
		cmd := exec.Command("docker", "ps")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return false
		}
	}
	logrus.Infoln("docker is installed")
	return true
}

func GetLiteEngineLog(instanceInfo InstanceInfo) string {
	switch instanceInfo.osType {
	case "linux":
		content, err := os.ReadFile("/var/log/lite-engine.log")
		if err != nil {
			return "no log file at /var/log/lite-engine.log"
		}
		return string(content)
	default:
		return "no log file"
	}
}

func ensureChocolatey() {
	const windowsInstallChoco = "Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1')) " //nolint:lll
	cmd := exec.Command("choco", "-h")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		cmd := exec.Command(windowsInstallChoco)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		chocoErr := cmd.Run()
		if chocoErr != nil {
			logrus.Errorf("failed to install chocolatey: %s", chocoErr)
		}
	}
}

func installPlugin(instanceInfo InstanceInfo, pluginBinaryURI string) {
	url := fmt.Sprintf("%s/plugin-%s-%s", pluginBinaryURI, instanceInfo.osType, instanceInfo.archType)
	dir := "/usr/local/bin"
	binary := "plugin"
	switch instanceInfo.osType {
	case windowsString:
		binary = "plugin.exe"
		dir = "C:\\Windows"
		url += ".exe"
	}
	err := downloadFile(url, dir, binary)
	if err != nil {
		logrus.
			WithField("error", err).
			Error("plugin download failed")
		logrus.Infoln("Some of the steps like action/bitrise might not work without plugin binary.")
		logrus.Infof("You can manually download, name it as plugin and put into your PATH from %s\n", pluginBinaryURI)
	}
}

func installEnvman(instanceInfo InstanceInfo, envmanBinaryURI string) {
	url := envmanBinaryURI
	dir := "/usr/local/bin"
	binary := "envman"
	switch instanceInfo.osType {
	case osxString:
		if instanceInfo.archType == "arm64" {
			url += "/envman-Darwin-arm64"
		} else {
			url += "/envman-Darwin-x86_64"
		}
	case windowsString:
		//envman doesn't exist for windows
		return
	default:
		if instanceInfo.archType == "arm64" {
			url += "/envman-Linux-arm64"
		} else {
			url += "/envman-Linux-x86_64"
		}
	}
	err := downloadFile(url, dir, binary)
	if err != nil {
		logrus.
			WithField("error", err).
			Error("envman download failed")
		logrus.Infoln("Some of the steps like action/bitrise might not work without envman binary.")
		logrus.Infof("You can manually download, name it as envman and put into your PATH from %s\n", url)
	}
}

func downloadFile(url string, targetDir string, binary string) error {
	targetFile := filepath.Join(targetDir, binary)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("Failed to download: %v\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Download failed: %s\n", resp.Status)
	}
	out, err := os.Create(targetFile)
	if err != nil {
		return fmt.Errorf("Failed to create file: %v\n", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to save file: %v\n", err)
	}

	err = os.Chmod(targetFile, 0755)
	if err != nil {
		return fmt.Errorf("Failed to make file executable: %v\n", err)
	}

	fmt.Printf("Binary downloaded and installed to %s\n", targetFile)
	return nil
}

func installGit(instanceInfo InstanceInfo) {
	logrus.Infoln("installing git")
	switch instanceInfo.osType {
	case windowsString:
		ensureChocolatey()
		cmd := exec.Command("choco", "install", "git.install", "-y")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		gitErr := cmd.Run()
		if gitErr != nil {
			logrus.Errorf("failed to install choco: %s", gitErr)
		}
	default:
		cmd := exec.Command("apt-get", "install", "git")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			logrus.Errorf("failed to install git: %s", err)
		}
	}
}

func installDocker(instanceInfo InstanceInfo) {
	logrus.Infoln("installing docker")
	switch instanceInfo.osType {
	case windowsString:
		ensureChocolatey()
		cmd := exec.Command("choco", "install", "docker", "-y")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		gitErr := cmd.Run()
		if gitErr != nil {
			logrus.Errorf("failed to install docker: %s", gitErr)
			return
		}
	default:
		cmd := exec.Command("curl", "-fsSL", "https://get.docker.com", "-o", "get-docker.sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		getScriptErr := cmd.Run()
		if getScriptErr != nil {
			logrus.
				WithField("error", getScriptErr).
				Error("get docker install script failed")
			return
		}

		cmd = exec.Command("sudo", "sh", "get-docker.sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		dockerInstallErr := cmd.Run()
		if dockerInstallErr != nil {
			logrus.
				WithField("error", dockerInstallErr).
				Error("get docker install script failed")
			return
		}
	}
	logrus.Infoln("docker installed")
}
