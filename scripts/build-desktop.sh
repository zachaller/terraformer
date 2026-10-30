#!/usr/bin/env bash
# Builds the desktop releases into dist/:
#   Terraformer-<version>-mac.zip      universal (Apple silicon and Intel) .app
#   Terraformer-<version>-windows.zip  64-bit .exe
#   Terraformer-<version>-linux.tar.gz 64-bit binary (built for this machine)
#
# Mac and Windows builds are cross-compiled without cgo, so they work from any
# OS. The Linux build needs cgo and Ebitengine's X11/ALSA headers, so it's
# only made on Linux (skip it with NO_LINUX=1).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
version="$(scripts/version.sh)"
dist="$root/dist"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$dist"
ldflags="-s -w"

# Mac: build both architectures, join them, and wrap them in an app bundle.
for arch in arm64 amd64; do
  GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags="$ldflags" \
    -o "$work/terraformer-$arch" ./cmd/desktop
done
app="$work/mac/Terraformer.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp scripts/Terraformer.icns "$app/Contents/Resources/"
python3 scripts/lipo.py "$app/Contents/MacOS/terraformer" "$work/terraformer-arm64" "$work/terraformer-amd64"
chmod +x "$app/Contents/MacOS/terraformer"
sed "s/@VERSION@/$version/g" scripts/Info.plist > "$app/Contents/Info.plist"
(cd "$work/mac" && rm -f "$dist/Terraformer-$version-mac.zip" && zip -qry "$dist/Terraformer-$version-mac.zip" Terraformer.app)

# Windows.
mkdir -p "$work/windows/Terraformer"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$ldflags -H windowsgui" \
  -o "$work/windows/Terraformer/Terraformer.exe" ./cmd/desktop
(cd "$work/windows" && rm -f "$dist/Terraformer-$version-windows.zip" && zip -qr "$dist/Terraformer-$version-windows.zip" Terraformer)

# Linux.
if [[ "$(go env GOOS)" == linux && -z "${NO_LINUX:-}" ]]; then
  mkdir -p "$work/linux/Terraformer"
  GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$ldflags" \
    -o "$work/linux/Terraformer/terraformer" ./cmd/desktop
  tar -czf "$dist/Terraformer-$version-linux.tar.gz" -C "$work/linux" Terraformer
fi

ls -l "$dist"
