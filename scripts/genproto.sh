#!/usr/bin/env bash
# Generate all protobuf bindings.
set -euo pipefail
shopt -s failglob

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
if [[ "$(pwd -P)" != "$repo_root" ]]; then
  echo "must be run from repository root"
  exit 255
fi

echo "generating files"
go tool -modfile=internal/tools/go.mod buf dep update
go tool -modfile=internal/tools/go.mod buf generate
