#!/usr/bin/env bash

# Copyright The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

export STATIC_DIR=static
PREBUILT_ASSETS_STATIC_DIR=${PREBUILT_ASSETS_STATIC_DIR:-}
if [ -n "$PREBUILT_ASSETS_STATIC_DIR" ]; then
    STATIC_DIR=$(realpath $PREBUILT_ASSETS_STATIC_DIR)
fi

cd ui
cp embed.go.tmpl embed.go

GZIP_OPTS="-fkn"
# gzip option '-k' may not always exist in the latest gzip available on different distros.
if ! gzip -k -h &>/dev/null; then GZIP_OPTS="-fn"; fi

mkdir -p static
find static -type f -name '*.gz' -delete

# Compress files from the prebuilt static directory and replicate the structure in the current static directory
find "${STATIC_DIR}" -type f ! -name '*.gz' -exec bash -c '
    for file; do
        dest="${file#${STATIC_DIR}}"
        mkdir -p "static/$(dirname "$dest")"
        gzip '"$GZIP_OPTS"' "$file" -c > "static/${dest}.gz"
    done
' bash {} +

# Append the paths of gzipped files to embed.go
find static -type f -name '*.gz' -print0 | sort -z | xargs -0 echo //go:embed >> embed.go

echo var EmbedFS embed.FS >> embed.go
