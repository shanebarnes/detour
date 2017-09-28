#!/bin/bash

command -v glide > /dev/null 2>&1
cmd_glide=$?

set -e
set -o errtrace

function err_handler() {
    local frame=0
    while caller $frame; do
        ((frame++));
    done
    echo "$*"
    exit 1
}

trap 'err_handler' SIGINT ERR

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GOPATH="$script_dir"
export GOBIN="${GOPATH}/bin"
#go env

cd "$GOPATH"
mkdir -p "$GOBIN"
cd "${GOPATH}/src/github.com/shanebarnes/detour"

printf "Downloading and installing packages and dependencies...\n"

if [ $cmd_glide -eq 0 ]; then
    glide -y glide.yaml install
else
    go get ./...
fi

printf "Compiling packages and dependencies...\n"
go build -v -ldflags -s

exit $?
