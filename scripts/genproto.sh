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
