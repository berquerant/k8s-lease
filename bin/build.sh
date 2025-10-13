#!/bin/bash

short_sha() {
    git rev-parse --short HEAD
}

current_tag() {
    git describe --tags --abbrev=0 --exact-match 2> /dev/null
}

readonly version_package="github.com/berquerant/k8s-lease/version"

ldflags() {
    echo "-X ${version_package}.Version=$(current_tag) -X ${version_package}.Revision=$(short_sha)"
}

build() {
    go build -v -trimpath -ldflags "$(ldflags)" "$@"
}

build "$@"
