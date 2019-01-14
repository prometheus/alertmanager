#!/usr/bin/env bash
#
# Generate all protobuf bindings.
# Run from repository root.
#
# Initial script taken from etcd under the Apache 2.0 license
# File: https://github.com/coreos/etcd/blob/78a5eb79b510eb497deddd1a76f5153bc4b202d2/scripts/genproto.sh

set -e
set -u

if ! [[ "$0" =~ "scripts/genproto.sh" ]]; then
    echo "must be run from repository root"
    exit 255
fi

if ! [[ $(protoc --version) =~ "3.5.1" ]]; then
    echo "could not find protoc 3.5.1, is it installed + in PATH?"
    exit 255
fi

# Exact version of plugins to build.
PROTOC_GEN_GOGOFAST_SHA="971cbfd2e72b513a28c74af7462aee0800248d69"
PROTOC_GEN_GRPC_ECOSYSTEM_SHA="e4b8a938efae14de11fd97311e873e989896348c"

echo "installing plugins"
pushd ${GOPATH}
  go install "golang.org/x/tools/cmd/goimports"
  go get -d -u "github.com/gogo/protobuf/protoc-gen-gogo"
  go get -d -u "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway"
pushd ${GOPATH}/src/github.com/gogo/protobuf
  git reset --hard "${PROTOC_GEN_GOGOFAST_SHA}"
  go install "github.com/gogo/protobuf/protoc-gen-gogofast"
pushd ${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway
  git reset --hard "${PROTOC_GEN_GRPC_ECOSYSTEM_SHA}"
  go install "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway"

pushd -0 && dirs -c

GOGOPROTO_ROOT="${GOPATH}/src/github.com/gogo/protobuf"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"
GRPC_GATEWAY_ROOT="${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway"

DIRS="nflog/nflogpb silence/silencepb cluster/clusterpb"

for dir in ${DIRS}; do
    pushd ${dir}
        protoc --gogofast_out=plugins=grpc:. -I=. \
            -I="${GOGOPROTO_PATH}" \
            -I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
            *.proto

        sed -i.bak -E 's/import _ \"gogoproto\"//g' *.pb.go
        sed -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
        rm -f *.bak
        goimports -w *.pb.go
    popd
done
