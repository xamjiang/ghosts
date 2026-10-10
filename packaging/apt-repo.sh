#!/usr/bin/env bash
# Build a signed APT repository from .deb packages.
#
#   packaging/apt-repo.sh OUT_DIR PACKAGE.deb...
#
# Signs with the default key in the GPG keyring (or $GPG_KEY_ID). Needs
# apt-ftparchive (apt-utils), dpkg-deb and gpg. Layout, as served at
# https://ghosts.xamjiang.com/apt:
#
#   key.gpg, key.asc                          public signing key
#   pool/main/g/ghosts/ghosts_<ver>_<arch>.deb
#   dists/stable/main/binary-<arch>/Packages{,.gz}
#   dists/stable/{Release,Release.gpg,InRelease}
#
# Only the given packages are indexed, so each release lists just its own
# version; older .debs stay in the pool for anyone pinning them.
set -euo pipefail

out="$1"
shift
archs=(amd64 arm64)
gpg_args=(--batch --yes) key=()
if [ -n "${GPG_KEY_ID:-}" ]; then
	gpg_args+=(--local-user "$GPG_KEY_ID")
	key=("$GPG_KEY_ID")
fi

mkdir -p "$out/pool/main/g/ghosts"
for deb in "$@"; do
	version="$(dpkg-deb -f "$deb" Version)"
	arch="$(dpkg-deb -f "$deb" Architecture)"
	cp "$deb" "$out/pool/main/g/ghosts/ghosts_${version}_${arch}.deb"
done

cd "$out"
for arch in "${archs[@]}"; do
	dir="dists/stable/main/binary-${arch}"
	mkdir -p "$dir"
	apt-ftparchive --arch "$arch" packages pool >"$dir/Packages"
	gzip -9 -n -k -f "$dir/Packages"
done

apt-ftparchive \
	-o APT::FTPArchive::Release::Origin=ghosts \
	-o APT::FTPArchive::Release::Label=ghosts \
	-o APT::FTPArchive::Release::Suite=stable \
	-o APT::FTPArchive::Release::Codename=stable \
	-o APT::FTPArchive::Release::Architectures="${archs[*]}" \
	-o APT::FTPArchive::Release::Components=main \
	-o APT::FTPArchive::Release::Description="ghosts: find and stop forgotten dev servers" \
	release dists/stable >Release.tmp
mv Release.tmp dists/stable/Release

gpg "${gpg_args[@]}" --clearsign -o dists/stable/InRelease dists/stable/Release
gpg "${gpg_args[@]}" --armor --detach-sign -o dists/stable/Release.gpg dists/stable/Release
gpg "${gpg_args[@]}" --armor --export "${key[@]}" >key.asc
gpg "${gpg_args[@]}" --export "${key[@]}" >key.gpg
