#!/bin/bash

set -e

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GOPATH="$script_dir"
#export GOBIN="${GOPATH}/bin"
#go env

# E.g., linux
if [ ! -z "${1}" ]; then
    export GOOS="${1}"
fi

# E.g., amd64
if [ ! -z "${2}" ]; then
    export GOARCH="${2}"
fi

cd "$GOPATH"
#mkdir -p "$GOBIN"

#printf "Downloading and installing packages and dependencies...\n"
#go get ./...

printf "Compiling packages and dependencies...\n"
go build -ldflags -s

exit $?
