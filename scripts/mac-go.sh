#!/bin/sh
set -eu
# Optional tools/cache stay outside the source checkout.
tools=${PAIRSHELL_BUILD_TOOLS:-"$HOME/Library/Caches/PairShell/BuildTools"}
if [ -d "$tools/gomod" ]; then
  export GOMODCACHE="$tools/gomod"
  export GOPROXY="${GOPROXY:-off}"
fi
if command -v go >/dev/null 2>&1; then
  exec go "$@"
fi
exec "$tools/go/bin/go" "$@"
