#!/usr/bin/env bash
# Generate api
set -euo pipefail
shopt -s failglob

if ! [[ "$0" = "scripts/swagger.sh" ]]; then
  echo "must be run from repository root"
  exit 255
fi

echo "generating files"
  rm -r api/v2/{client,models,restapi} ||:
  go tool -modfile=internal/tools/go.mod swagger generate server -f api/v2/openapi.yaml --copyright-file=COPYRIGHT.txt --exclude-main -A alertmanager --target api/v2/
  go tool -modfile=internal/tools/go.mod swagger generate client -f api/v2/openapi.yaml --copyright-file=COPYRIGHT.txt --skip-models --target api/v2
