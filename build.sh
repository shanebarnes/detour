#!/bin/bash

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

# E.g., linux
if [ ! -z "${1}" ]; then
    export GOOS="${1}"
fi

# E.g., amd64
if [ ! -z "${2}" ]; then
    export GOARCH="${2}"
fi

printf "Compiling packages and dependencies...\n"
go build -v -ldflags -s

exit $?
