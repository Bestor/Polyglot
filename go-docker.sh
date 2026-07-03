#!/bin/sh
# Convenience wrapper to run `go` commands against the project using a
# containerized Go toolchain, so no local Go install is required.
exec docker run --rm \
  -v "$(pwd)":/app \
  -w /app \
  -v valanalyzer-gomod:/go/pkg/mod \
  -v valanalyzer-gocache:/root/.cache/go-build \
  golang:1.26-alpine "$@"
