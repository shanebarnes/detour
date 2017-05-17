#!/bin/bash

set -e

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GOPATH="$script_dir"
export GOBIN="${GOPATH}/bin"
#go env

cd "$GOPATH"
mkdir -p "$GOBIN"

printf "Downloading and installing packages and dependencies...\n"
go get ./...

printf "Compiling packages and dependencies...\n"
go build -ldflags -s

exit $?
