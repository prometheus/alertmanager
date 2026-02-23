#!/usr/bin/env bash
# Generate api
set -euo pipefail
shopt -s failglob

if ! [[ "$0" = "scripts/swagger.sh" ]]; then
  echo "must be run from repository root"
  exit 255
fi

FIRST_GOPATH=$(go env GOPATH | cut -d: -f1)

pushd "internal/tools" >/dev/null
go install "github.com/go-swagger/go-swagger/cmd/swagger"
popd >/dev/null

echo "generating files"
  rm -r api/v2/{client,models,restapi} ||:
  SWAGGER="${FIRST_GOPATH}/bin/swagger"
	$SWAGGER generate server -f api/v2/openapi.yaml --copyright-file=COPYRIGHT.txt --exclude-main -A alertmanager --target api/v2/
	$SWAGGER generate client -f api/v2/openapi.yaml --copyright-file=COPYRIGHT.txt --skip-models --target api/v2

