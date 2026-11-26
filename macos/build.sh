#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-mmacosx-version-min=13.0 ${CGO_CFLAGS:-}"
export CGO_LDFLAGS="-mmacosx-version-min=13.0 ${CGO_LDFLAGS:-}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
app="dist/Ghosts.app/Contents"
mkdir -p "$app/MacOS" "$app/Resources"
go build -o "$app/Resources/ghosts" ./cmd/ghosts
install -m 755 macos/Ghosts.command "$app/Resources/Ghosts.command"
go build -buildmode=c-archive -o "$tmp/scanner.a" ./cmd/ghosts-menubar
swiftc -O -target "$(uname -m)-apple-macosx13.0" -module-cache-path "$tmp/modules" \
  -import-objc-header "$tmp/scanner.h" macos/*.swift "$tmp/scanner.a" \
  -framework CoreFoundation -framework Security -framework SystemConfiguration \
  -o "$app/MacOS/Ghosts"
cp macos/Info.plist "$app/"
codesign --force --sign - dist/Ghosts.app
