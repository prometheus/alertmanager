#!/usr/bin/env bash
#
# Generate all protobuf bindings.
# Run from repository root.
set -e
set -u

if ! [[ "$0" =~ "scripts/genproto.sh" ]]; then
  echo "must be run from repository root"
  exit 255
fi

pushd "internal/tools"
INSTALL_PKGS="github.com/bufbuild/buf/cmd/buf golang.org/x/tools/cmd/goimports google.golang.org/protobuf/cmd/protoc-gen-go"
for pkg in ${INSTALL_PKGS}; do
    go install "$pkg"
done
popd

DIRS="nflog/nflogpb silence/silencepb cluster/clusterpb"

echo "generating files"
for dir in ${DIRS}; do
  pushd ${dir}
  buf dep update
  buf generate
  goimports -w *.pb.go
  popd
done
