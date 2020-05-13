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

if ! [[ $(protoc --version) =~ "3.11.4" ]]; then
    echo "could not find protoc 3.11.4, is it installed + in PATH?"
    exit 255
fi

echo "installing plugins"
go mod download

INSTALL_PKGS="golang.org/x/tools/cmd/goimports github.com/gogo/protobuf/protoc-gen-gogofast"
for pkg in ${INSTALL_PKGS}; do
    go install -mod=vendor "$pkg"
done

GOGOPROTO_ROOT="$(go list -mod=readonly -f '{{ .Dir }}' -m github.com/gogo/protobuf)"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"

DIRS="nflog/nflogpb silence/silencepb cluster/clusterpb"

echo "generating files"
for dir in ${DIRS}; do
    pushd ${dir}
        protoc --gogofast_out=:. -I=. \
            -I="${GOGOPROTO_PATH}" \
            *.proto

        sed -i.bak -E 's/import _ \"gogoproto\"//g' *.pb.go
        sed -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
        sed -i.bak -E 's/\t_ \"google\/protobuf\"//g' -- *.pb.go
        rm -f *.bak
        goimports -w *.pb.go
    popd
done
