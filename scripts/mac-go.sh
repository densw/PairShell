#!/bin/sh
set -eu
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# Use the project-local toolchain and pinned, synchronized dependency cache.
export GOMODCACHE="$repo/.tools/gomod"
export GOPROXY="${GOPROXY:-off}"
exec "$repo/.tools/go/bin/go" "$@"
