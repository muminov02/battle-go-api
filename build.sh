#!/bin/bash
# Build release binaries into ./bin
set -e
cd "$(dirname "$0")"

echo "Building battle-api + battle-worker (release)…"
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/api    ./cmd/api
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/worker ./cmd/worker
echo "Done:"
ls -lh bin/
