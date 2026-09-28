#!/usr/bin/env bash
# Builds a release archive into dist/ for one platform.
#   scripts/package.sh <goos> <goarch> <version>
# macOS and Linux builds need CGO and must run on the target OS (macOS can
# build both architectures); Windows builds without CGO from anywhere.
set -euo pipefail

goos=$1
goarch=$2
version=$3
name=virtual-zpl-printer
app="Virtual ZPL Printer"
base="$name-$version-$goos-$goarch"
ldflags="-s -w -X main.version=$version"
tags=desktop,production

rm -rf "build/$base" && mkdir -p "build/$base" dist
cp README.md LICENSE "build/$base/"

case "$goos" in
darwin)
  bundle="build/$base/$app.app"
  mkdir -p "$bundle/Contents/MacOS"
  CGO_ENABLED=1 GOOS=darwin GOARCH="$goarch" \
    go build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$bundle/Contents/MacOS/$name" .
  cat > "$bundle/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$app</string>
  <key>CFBundleDisplayName</key><string>$app</string>
  <key>CFBundleExecutable</key><string>$name</string>
  <key>CFBundleIdentifier</key><string>com.github.jochen42.virtual-zpl-printer</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleVersion</key><string>${version#v}</string>
  <key>CFBundleShortVersionString</key><string>${version#v}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
  # Unsigned apps are rejected on Apple Silicon; an ad-hoc signature is enough to run.
  codesign --force --deep --sign - "$bundle"
  ditto -c -k --norsrc --keepParent "$bundle" "dist/$base.zip"
  ;;
windows)
  CGO_ENABLED=0 GOOS=windows GOARCH="$goarch" \
    go build -trimpath -tags "$tags" -ldflags "$ldflags -H windowsgui" -o "build/$base/$name.exe" .
  (cd build && zip -qr "../dist/$base.zip" "$base")
  ;;
linux)
  CGO_ENABLED=1 GOOS=linux GOARCH="$goarch" \
    go build -trimpath -tags "$tags,webkit2_41" -ldflags "$ldflags" -o "build/$base/$name" .
  tar -C build -czf "dist/$base.tar.gz" "$base"
  ;;
*)
  echo "unsupported GOOS $goos" >&2
  exit 1
  ;;
esac

ls dist/"$base".*
