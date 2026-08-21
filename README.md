# Harness-Docker-Runner

## Running locally

* Create an `.env` file. It can be empty.
* Generate tls credentials: `go run main.go certs`
* Start the server: `go run main.go server`
* Check the health status of the server: `go run main.go client`

## Building binaries

Cross-compiling needs no extra setup — the binaries land in the `release` directory.

**Linux**
```
GOOS=linux GOARCH=amd64 go build -o release/harness-docker-runner-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o release/harness-docker-runner-linux-arm64 .
```

**Windows**
```
GOOS=windows GOARCH=amd64 go build -o release/harness-docker-runner-windows-amd64.exe .
```

To stamp a version into the binary (otherwise `--version` reports empty):
```
go build -ldflags "-X github.com/harness/harness-docker-runner/version.Version=1.2.3" -o release/harness-docker-runner-linux-amd64 .
```

## Running QA Automation

VMs live in the `ci-play` GCP project:

| VM | Purpose |
| --- | --- |
| [`self-hosted-qa-vm`](https://console.cloud.google.com/compute/instancesDetail/zones/us-central1-f/instances/self-hosted-qa-vm?authuser=1&project=ci-play) (`us-central1-f`) | Linux — runs the amd64 binary as a background process, and as a docker run |
| [`ci-window-qa`](https://console.cloud.google.com/compute/instancesDetail/zones/us-central1-c/instances/ci-window-qa?authuser=1&project=ci-play) (`us-central1-c`) | Windows server VM |
| [`ci-window-automation-linux`](https://console.cloud.google.com/compute/instancesDetail/zones/us-central1-a/instances/ci-window-automation-linux?authuser=1&project=ci-play) (`us-central1-a`) | Linux VM used for the Windows runs |

### Deploying a local build to the Linux QA VM

Copy the binary up. Use an absolute destination path — `$HOME` on this VM does not
match the directory you actually land in, so `~` will appear to swallow the file:

```
gcloud compute scp release/harness-docker-runner-linux-amd64 \
  "self-hosted-qa-vm":/home/$USER/harness-docker-runner-linux-amd64 \
  --zone "us-central1-f" --project "ci-play"
```

The first copy can take a couple of minutes while gcloud propagates SSH keys to the
project metadata. Then SSH in:

```
gcloud compute ssh "self-hosted-qa-vm" --zone "us-central1-f" --project "ci-play"
```

The runner is served out of `/root`, so install the new binary there:

```
sudo cp /home/$USER/harness-docker-runner-linux-amd64 /root/harness-docker-runner-linux-amd64
```

### Managing the background runner

The runner is started with `nohup`, so it is orphaned to `ppid=1` and is not managed
by systemd — nothing restarts it for you after a kill.

```
# list the running runners
ps -eo pid,ppid,user,etime,stat,cmd | grep -i harness-docker-runner | grep -v grep

# find which one is actually serving the API (port 3000)
sudo ss -ltnp | grep 3000
```

More than one process can survive here: only the one holding port 3000 is live, and
any other is a stale leftover from an earlier start. Kill by pid, then restart:

```
sudo kill <pid>          # add -9 if it does not exit

cd /root
sudo nohup ./harness-docker-runner-linux-amd64 server > runner.log 2>&1 &
tail -f /root/runner.log
```

Verify it is up:

```
curl http://localhost:3000/healthz
```

### Instruction for Running it as Windows Service

If you want to install Harness-Docker-Runner as a service in Windows, please follow the bellow instructions.

#### Instruction for creating msi from Exe file
* choco install go-msi (https://github.com/mh-cbon/go-msi)
* create the binary for harness-docker-runner-windows-amd64.exe
```
  go build -o harness-docker-runner-windows-amd64.exe
```
* run the below command from root directory of Harness-Docker-Runner to create msi
```
  go-msi make --msi harness-docker-runner-svc.msi --version your_version
```

#### Installation
* Download the msi (harness-docker-runner-svc.msi) from latest github release
* Double click on the msi to start the installation process
* Accept the liecence, click next and finish the installation.
* You can test your service availibility by running below command in your cmd terminal
```
  curl http://localhost:3000/healthz
```
* Logs for the runner can be found in path : C:\Windows\system32\harness-docker-runner-timestamp.log

#### Uninstallation
* Double click on the msi to start the installation process
* You will see three options Change, Repair and Remove
* Click on Remove and finish.

#### Additional Instructions
* Service will automatically started even if you re-started your VM
* Please do not attemp to start/stop/delete the service manually, it may cause issue in uninstallation.
* In case you see any issues and not able to verfiy the runner, Uninstall and install again using the msi

#### Error and Resolutions
* For the below both the error, we need to give a full permission to our msi file to get it working. Since this is an extenal msi which is not trusted by windows security, it blocks it initially which can be handle by manually assigning the required permissions from Properties => Security tab.
* If you are facing Error code 2502 or 2503 during installation, please follow the below instruction or link: https://help.krisp.ai/hc/en-us/articles/8083286001820-Error-during-installation-2502-and-2503#h_01HNCW0XCN8AJCWK84K8MQVY7Y
* For error "This installation package could not be opened" then please follow this link: https://answers.microsoft.com/en-us/windows/forum/all/this-installation-package-could-not-be/d6d913e9-aac7-429a-ac0d-c39ad3a7c5eb
## Release procedure

Run the changelog generator.

```BASH
docker run -it --rm -v "$(pwd)":/usr/local/src/your-app githubchangeloggenerator/github-changelog-generator -u harness -p lite-engine -t <secret github token>
```

You can generate a token by logging into your GitHub account and going to Settings -> Personal access tokens.

Next we tag the PR's with the fixes or enhancements labels. If the PR does not fulfil the requirements, do not add a label.

**Before moving on make sure to update the version file `version/version.go`.**

Run the changelog generator again with the future version according to semver.

```BASH
docker run -it --rm -v "$(pwd)":/usr/local/src/your-app githubchangeloggenerator/github-changelog-generator -u harness -p lite-engine -t <secret token> --future-release v0.2.0
```

Create your pull request for the release. Get it merged then tag the release.
