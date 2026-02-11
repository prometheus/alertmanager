#!/usr/bin/env bash
# Generate all protobuf bindings.
set -euo pipefail
shopt -s failglob

if ! [[ "$0" = "scripts/genproto.sh" ]]; then
  echo "must be run from repository root"
  exit 255
fi

pushd "internal/tools" >/dev/null
INSTALL_PKGS="github.com/bufbuild/buf/cmd/buf google.golang.org/protobuf/cmd/protoc-gen-go"
for pkg in ${INSTALL_PKGS}; do
  go install "$pkg"
done
popd >/dev/null

echo "generating files"
buf dep update
buf generate
