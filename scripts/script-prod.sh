#!/bin/bash
# Check if Account ID and Delegate Token is provided as an argument
if [ $# -ne 3 ]; then
    echo "Usage: $0 <ACCOUNT_ID> <DELEGATE_TOKEN> <DELEGATE_IMAGE_VERSION>"
    exit 1
fi
# Get the operating system type
ostype=$(uname)
# Get the machine architecture type
archtype=$(uname -m)

if [ "$archtype" = "x86_64" ]; then
    archtype="amd64"
fi

# Account ID from script argument
ACCOUNT_ID=$1
# Account ID from script argument
DELEGATE_TOKEN=$2
DELEGATE_IMAGE_VERSION=$3
# Check the operating system type and run commands accordingly
if [ "$ostype" = "Darwin" ]; then
    # Commands for macOS
    echo "Running commands for macOS"

    # Run the Docker container with environment variables
    docker run -d --cpus=1 --memory=2g --restart=always\
      -e DELEGATE_NAME=docker-delegate \
      -e NEXT_GEN="true" \
      -e DELEGATE_TYPE="DOCKER" \
      -e ACCOUNT_ID=$ACCOUNT_ID \
      -e DELEGATE_TOKEN=$DELEGATE_TOKEN \
      -e LOG_STREAMING_SERVICE_URL=https://app.harness.io/log-service/ \
      -e DELEGATE_TAGS="macos-$archtype" \
      -e RUNNER_URL=http://host.docker.internal:3000 \
      -e MANAGER_HOST_AND_PORT=https://app.harness.io harness/delegate:$DELEGATE_IMAGE_VERSION

    # Wait for a while before continuing the script execution
    echo "Sleeping for 2 seconds"
    sleep 2

    echo "Removing already existing binary"
    rm -f harness-docker-runner-linux-$archtype

    # Download the harness-docker-runner for macOS
    echo "Downloading the runner binary"
    wget -q https://github.com/harness/harness-docker-runner/releases/latest/download/harness-docker-runner-darwin-$archtype 
    
    # Make the downloaded file executable
    echo "Chmod runner binary"
    sudo chmod +x harness-docker-runner-darwin-$archtype 
    
    # Start the harness-docker-runner server
    echo "running the binary"
    sudo ./harness-docker-runner-darwin-$archtype server &
    
elif [ "$ostype" = "Linux" ]; then
    echo "Running commands for Linux"

    # Run the Docker container with environment variables
    docker run -d --cpus=1 --memory=2g --net=host --restart=always\
      -e DELEGATE_NAME=docker-delegate \
      -e NEXT_GEN="true" \
      -e DELEGATE_TYPE="DOCKER" \
      -e ACCOUNT_ID=$ACCOUNT_ID \
      -e DELEGATE_TOKEN=$DELEGATE_TOKEN \
      -e LOG_STREAMING_SERVICE_URL=https://app.harness.io/log-service/ \
      -e DELEGATE_TAGS="linux-$archtype" \
      -e MANAGER_HOST_AND_PORT=https://app.harness.io harness/delegate:$DELEGATE_IMAGE_VERSION

    # Wait for a while before continuing the script execution
    echo "Sleeping for 2 seconds"
    sleep 2

    echo "Removing already existing binary"
    rm -f harness-docker-runner-linux-$archtype

    # Download the harness-docker-runner for Linux
    wget -q https://github.com/harness/harness-docker-runner/releases/latest/download/harness-docker-runner-linux-$archtype 
    
    # Make the downloaded file executable
    sudo chmod +x harness-docker-runner-linux-$archtype 
    
    # Start the harness-docker-runner server
    sudo ./harness-docker-runner-linux-$archtype server &
else
    echo "Assuming it's a Windows machine"
    echo "Removing already existing binary"
    rm -f harness-docker-runner-windows-$archtype.exe

    # Download the harness-docker-runner for Windows
    curl -L -o harness-docker-runner-windows-$archtype.exe https://github.com/harness/harness-docker-runner/releases/latest/download/harness-docker-runner-windows-$archtype.exe
    
    # Start the harness-docker-runner server
    ./harness-docker-runner-windows-$archtype.exe server &

    sleep 2

    echo "
          Delegate can't run on a windows machine!
          Paste the below command in either a linux or mac machine
          Replace the IP of the windows machine
        "
    command="docker run -d --cpus=1 --memory=2g --restart=always \
  -e DELEGATE_NAME=docker-delegate \
  -e NEXT_GEN="true" \
  -e DELEGATE_TYPE="DOCKER" \
  -e ACCOUNT_ID=$ACCOUNT_ID \
  -e DELEGATE_TOKEN=$DELEGATE_TOKEN \
  -e LOG_STREAMING_SERVICE_URL=https://app.harness.io/log-service/ \
  -e DELEGATE_TAGS="windows-$archtype" \
  -e RUNNER_URL=http://WINDOWS_MACHINE_HOSTNAME_OR_IP:3000 \
  -e MANAGER_HOST_AND_PORT=https://app.harness.io harness/delegate:$DELEGATE_IMAGE_VERSION"
    echo $command
fi
