#!/usr/bin/env bash
# Cross-compile the Lambda binary to bin/bootstrap and zip it for SAM.
set -euo pipefail
# shellcheck source=lib.sh
# shellcheck source-path=SCRIPTDIR
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

cd "$ROOT"
mkdir -p bin
rm -f bin/bootstrap bin/bootstrap.zip
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/bootstrap ./cmd/auth-gate
(cd bin && zip -q bootstrap.zip bootstrap)
echo "built bin/bootstrap and bin/bootstrap.zip (linux/arm64)"
