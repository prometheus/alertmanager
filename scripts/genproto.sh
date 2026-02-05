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
INSTALL_PKGS="github.com/bufbuild/buf/cmd/buf golang.org/x/tools/cmd/goimports github.com/gogo/protobuf/protoc-gen-gogofast"
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
  sed -i.bak -E 's/import _ \"gogoproto\"//g' *.pb.go
  sed -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
  sed -i.bak -E 's/\t_ \"google\/protobuf\"//g' -- *.pb.go
  rm -f *.bak
  goimports -w *.pb.go
  popd
done
