#!/usr/bin/env bash

set -euo pipefail

output="${MINICLAW_OUTPUT:-miniclaw}"
goos="${GOOS:-$(go env GOOS)}"
goarch="${GOARCH:-$(go env GOARCH)}"

printf '[build] building %s for %s/%s\n' "$output" "$goos" "$goarch"
CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags='-s -w' -o "$output" ./cmd/miniclaw
printf '[build] build complete: %s\n' "$output"
