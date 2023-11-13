#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

cd asset
cp embed.go.tmpl embed.go
echo -n "//go:embed" >> embed.go

GZIP_OPTS="-c"

function compress_asset() {
    file="$1"
    strip_prefix="$2"
    add_prefix="$3"
    target_path="$add_prefix${file#"$strip_prefix"}.gz"
    mkdir -p "${target_path%/*}" || true
    gzip "$GZIP_OPTS" "$file" > "$target_path"
    echo -n " $target_path" >> embed.go
}

find . -type f -name '*.gz' -delete
# compress_asset  "../ui/app/script.js" "../ui/app/"
compress_asset  "../ui/app/index.html" "../ui/app/" "static/"
compress_asset  "../ui/app/favicon.ico" "../ui/app/" "static/"
while IFS= read -rd $'\0' file; do
    compress_asset "$file" "../ui/app/" "static/"
done < <(find ../ui/app/lib -type f -print0)
compress_asset  "../template/default.tmpl" "../template/" "templates/"
compress_asset  "../template/email.tmpl" "../template/" "templates/"
echo "" >> embed.go
echo var EmbedFS embed.FS >> embed.go
