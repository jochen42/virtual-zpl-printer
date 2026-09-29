#!/usr/bin/env bash
# Regenerates the app icons from assets/icon.svg. Needs rsvg-convert, and
# iconutil (macOS) for the .icns. Outputs are committed, so builds don't
# need these tools.
set -euo pipefail
cd "$(dirname "$0")/.."

svg=assets/icon.svg
tmp=$(mktemp -d)
trap 'rm -r "$tmp"' EXIT

# Linux window icon, embedded in the binary.
rsvg-convert -w 256 -h 256 "$svg" -o assets/appicon.png
# Headless browser UI favicon.
cp "$svg" frontend/icon.svg

# macOS app bundle icon.
set_dir="$tmp/icon.iconset"
mkdir "$set_dir"
for size in 16 32 128 256 512; do
  rsvg-convert -w $size -h $size "$svg" -o "$set_dir/icon_${size}x${size}.png"
  rsvg-convert -w $((size * 2)) -h $((size * 2)) "$svg" -o "$set_dir/icon_${size}x${size}@2x.png"
done
iconutil -c icns "$set_dir" -o assets/icon.icns

# Windows: icon group #3, the ID Wails loads for the window icon. go-winres
# resizes the PNG to the usual icon sizes; the .syso files are linked into
# Windows builds of package main.
rsvg-convert -w 256 -h 256 "$svg" -o "$tmp/icon.png"
cat > "$tmp/winres.json" <<JSON
{"RT_GROUP_ICON": {"#3": {"0000": "icon.png"}}}
JSON
go run github.com/tc-hib/go-winres@v0.3.3 make --in "$tmp/winres.json" --arch amd64,arm64 --out rsrc
