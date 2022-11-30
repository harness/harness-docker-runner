# Harness Docker Runner

## Build and usage
Linux

    # GOARCH=amd64 go build -o harness-docker-runner-linux-amd64
    # chmod +x harness-docker-runner-linux-amd64
    # ./harness-docker-runner-linux-amd64 server
    
macOS

    # GOOS=darwin GOARCH=amd64 go build -o harness-docker-runner-darwin-amd64
    # ./harness-docker-runner-darwin-amd64 server

Windows

    # GOOS=windows go build -o harness-docker-runner.exe
    # harness-docker-runner.exe server


## [](https://github.com/harness/harness-docker-runner#release-procedure)Release procedure (TBD)
