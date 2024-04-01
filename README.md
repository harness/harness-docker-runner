# Harness-Docker-Runner

How to use:

* Create an .env file. It can be empty.
* To build linux and windows `GOOS=windows go build -o lite-engine.exe; go build`
* Generate tls credentials: go run main.go certs
* Start server: go run main.go server
* Client call to check health status of server: go run main.go client.

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
