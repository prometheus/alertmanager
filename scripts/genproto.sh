#!/usr/bin/env bash
# Generate all protobuf bindings.
set -euo pipefail
shopt -s failglob

if ! [[ "$0" = "scripts/genproto.sh" ]]; then
  echo "must be run from repository root"
  exit 255
fi

echo "generating files"
go tool -modfile=internal/tools/go.mod buf dep update
go tool -modfile=internal/tools/go.mod buf generate
