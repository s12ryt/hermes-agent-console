#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
(cd web && npm ci && npm run build)
target_os="${GOOS:-linux}"
target_arch="${GOARCH:-amd64}"
suffix=''
if [ "$target_os" = windows ]; then suffix='.exe'; fi
CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags='-s -w' -o "bin/hermes-console-$target_os-$target_arch$suffix" ./cmd/console
