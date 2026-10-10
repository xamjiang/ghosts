#!/usr/bin/env bash
# Build a Debian package from a prebuilt Linux binary.
#
#   packaging/deb.sh VERSION ARCH BINARY OUTPUT.deb
#
# ARCH is a Debian architecture (amd64, arm64). The binary is static (no cgo
# on Linux), so the package has no dependencies. Needs dpkg-deb.
set -euo pipefail

version="$1" arch="$2" binary="$3" output="$4"
root="$(cd "$(dirname "$0")/.." && pwd)"
pkg="$(mktemp -d)"
trap 'rm -rf "$pkg"' EXIT
# mktemp makes the directory 0700, and the package root maps onto "/".
chmod 755 "$pkg"

# Debian sorts "~" before anything, so 0.2.0~rc1 comes before 0.2.0.
version="${version//-/\~}"

install -Dm755 "$binary" "$pkg/usr/bin/ghosts"
install -Dm644 "$root/LICENSE" "$pkg/usr/share/doc/ghosts/copyright"
install -Dm644 "$root/README.md" "$pkg/usr/share/doc/ghosts/README.md"

mkdir -p "$pkg/DEBIAN"
cat > "$pkg/DEBIAN/control" <<EOF
Package: ghosts
Version: ${version}
Architecture: ${arch}
Maintainer: Sam <sam@xamjiang.com>
Installed-Size: $(du -sk "$pkg/usr" | cut -f1)
Section: utils
Priority: optional
Homepage: https://github.com/xamjiang/ghosts
Description: Find and stop forgotten dev servers and Docker containers
 Ghosts lists the ports your own processes listen on and the Docker
 containers that are running, shows what each one costs in CPU and
 memory, and stops them from a terminal UI.
EOF

dpkg-deb --root-owner-group --build "$pkg" "$output"
