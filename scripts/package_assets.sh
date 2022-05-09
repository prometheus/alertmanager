#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

version="$(< VERSION)"
mkdir -p .tarballs
tar czf .tarballs/alertmanager-web-ui-${version}.tar.gz ui/app/script.js ui/app/index.html
