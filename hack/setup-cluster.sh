#!/bin/bash

set -e
set -o pipefail

log() {
    echo >&2 "$(basename "$0"): $*"
}

readonly node_image="$1"

if [[ -z "$node_image" ]] ; then
    log 'node_image($1) is required.'
    exit 1
fi
if ! command -v kind >/dev/null 2>&1 ; then
    log "Kind is not installed. Please install Kind manually."
    exit 1
fi
if [[ "$CI" = "true" ]] ; then
    if ! kind get clusters | grep -q 'kind' ; then
        log "No Kind cluster is running. Please start a Kind cluster before running the e2e tests."
    fi
else
    log "To avoid errors caused by pre-existing resources in the kind cluster before make test-unit, make test-e2e, we will recreate the cluster."
    kind delete cluster || true
    kind create cluster --image "$1"
fi
