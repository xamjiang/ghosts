#!/usr/bin/env bash
# Install ghosts: https://github.com/xamjiang/ghosts
#
#   curl -fsSL https://ghosts.xamjiang.com/install | bash
#
# Uses the system's package manager when there is one, so updates arrive the
# usual way:
#   Debian, Ubuntu     adds the ghosts APT repository, then apt installs it
#                      (asks for your sudo password; updates with apt upgrade)
#   Homebrew present   brew install xamjiang/tap/ghosts (updates with brew upgrade)
#   anything else      downloads the release binary, verifies its checksum and
#                      puts it in ~/.local/bin (rerun this script to update)
#
# Environment:
#   GHOSTS_INSTALL_DIR  skip package managers; install the binary into this directory
#   GHOSTS_BASE         where the APT repository lives (default https://ghosts.xamjiang.com)

set -euo pipefail

BASE="${GHOSTS_BASE:-https://ghosts.xamjiang.com}"
RELEASES="https://github.com/xamjiang/ghosts/releases/latest/download"

if [ -t 1 ]; then
	bold=$'\e[1m' dim=$'\e[2m' green=$'\e[32m' red=$'\e[31m' reset=$'\e[0m'
else
	bold="" dim="" green="" red="" reset=""
fi
# Temporary files, removed on exit. Global: the trap runs after main returns.
tmp=""
trap '[ -z "$tmp" ] || rm -rf "$tmp"' EXIT

# fetch URL FILE: retries transient failures and gives up on a dead connection.
fetch() {
	local progress=(--silent --show-error)
	[ -n "${3:-}" ] && [ -t 2 ] && progress=(--progress-bar)
	curl --fail --location --retry 3 --connect-timeout 15 "${progress[@]}" "$1" -o "$2"
}

say() { printf '%s\n' "$*"; }
step() { printf '%s›%s %s\n' "$dim" "$reset" "$*"; }
die() {
	printf '%serror:%s %s\n' "$red" "$reset" "$*" >&2
	exit 1
}

# Everything runs from main, called on the last line, so a download cut off
# halfway through can't run half a script.
main() {
	local os arch
	os="$(uname -s)"
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "unsupported CPU: $(uname -m) (ghosts runs on amd64 and arm64)" ;;
	esac
	case "$os" in
	Darwin | Linux) ;;
	*) die "unsupported system: $os (ghosts runs on macOS and Linux)" ;;
	esac

	say "${bold}Installing ghosts${reset}"
	if [ -n "${GHOSTS_INSTALL_DIR:-}" ]; then
		install_binary "$os" "$arch" "$GHOSTS_INSTALL_DIR"
	elif [ "$os" = Linux ] && [ -f /etc/debian_version ] && command -v apt-get >/dev/null; then
		install_apt
	elif command -v brew >/dev/null; then
		install_brew
	else
		install_binary "$os" "$arch" "$HOME/.local/bin"
	fi
}

install_apt() {
	local sudo=""
	if [ "$(id -u)" -ne 0 ]; then
		command -v sudo >/dev/null || die "run as root, or install sudo"
		sudo="sudo"
		say "Adding the ghosts APT repository needs your sudo password."
	fi
	step "Adding the signing key to /etc/apt/keyrings/ghosts.gpg"
	$sudo install -d -m 0755 /etc/apt/keyrings
	tmp="$(mktemp -d)"
	fetch "$BASE/apt/key.gpg" "$tmp/ghosts.gpg" || die "couldn't download the signing key"
	$sudo install -m 0644 "$tmp/ghosts.gpg" /etc/apt/keyrings/ghosts.gpg

	step "Adding the repository to /etc/apt/sources.list.d/ghosts.list"
	echo "deb [signed-by=/etc/apt/keyrings/ghosts.gpg] $BASE/apt stable main" |
		$sudo tee /etc/apt/sources.list.d/ghosts.list >/dev/null

	# Refresh only this repository: faster, and a broken third-party source
	# elsewhere on the system can't fail the install.
	step "Updating the package list"
	$sudo apt-get update -qq \
		-o Dir::Etc::sourcelist=sources.list.d/ghosts.list \
		-o Dir::Etc::sourceparts=- \
		-o APT::Get::List-Cleanup=0
	step "apt-get install ghosts"
	$sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ghosts >/dev/null
	done_msg "Update with: sudo apt update && sudo apt upgrade"
}

install_brew() {
	if brew list --formula ghosts >/dev/null 2>&1; then
		step "brew upgrade ghosts"
		brew upgrade xamjiang/tap/ghosts
	else
		step "brew install xamjiang/tap/ghosts"
		brew install xamjiang/tap/ghosts
	fi
	done_msg "Update with: brew upgrade ghosts"
}

install_binary() {
	local os="$1" arch="$2" dir="$3" platform name want got
	case "$os" in
	Darwin) platform=darwin_universal ;;
	Linux) platform="linux_${arch}" ;;
	esac
	tmp="$(mktemp -d)"

	step "Looking up the latest release"
	fetch "$RELEASES/checksums.txt" "$tmp/checksums.txt" || die "couldn't reach GitHub"
	name="$(awk -v p="_${platform}.tar.gz" 'substr($2, length($2) - length(p) + 1) == p { print $2 }' "$tmp/checksums.txt")"
	[ -n "$name" ] || die "no release for ${platform}"
	want="$(awk -v n="$name" '$2 == n { print $1 }' "$tmp/checksums.txt")"

	step "Downloading $name"
	fetch "$RELEASES/$name" "$tmp/$name" progress || die "download failed"
	if command -v sha256sum >/dev/null; then
		got="$(sha256sum "$tmp/$name" | cut -d' ' -f1)"
	else
		got="$(shasum -a 256 "$tmp/$name" | cut -d' ' -f1)"
	fi
	[ "$got" = "$want" ] || die "checksum mismatch for $name"
	step "Checksum verified"

	tar -xzf "$tmp/$name" -C "$tmp" ghosts
	mkdir -p "$dir"
	# Move into place in one step, so a running ghosts keeps its old binary.
	install -m 0755 "$tmp/ghosts" "$dir/.ghosts.new"
	mv -f "$dir/.ghosts.new" "$dir/ghosts"
	step "Installed to $dir/ghosts"

	done_msg "Update by running this script again" "$dir/ghosts"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) path_hint "$dir" ;;
	esac
}

# path_hint explains how to put dir on PATH for the user's shell.
path_hint() {
	local dir="$1" rc
	# shellcheck disable=SC2088 # shown to the user, not expanded
	case "$(basename "${SHELL:-}")" in
	zsh) rc="~/.zshrc" ;;
	bash) rc="~/.bashrc" ;;
	fish)
		say ""
		say "$dir isn't on your PATH yet. Add it with:"
		say "  fish_add_path $dir"
		return
		;;
	*) rc="your shell's startup file" ;;
	esac
	say ""
	say "$dir isn't on your PATH yet. Add this line to $rc, then open a new terminal:"
	say "  export PATH=\"$dir:\$PATH\""
}

# done_msg reports the installed version; $2 is the binary when it may not be
# on PATH yet.
done_msg() {
	local version
	version="$("${2:-ghosts}" -version 2>/dev/null || true)"
	say ""
	say "${green}✓${reset} ${bold}${version:-ghosts} is installed.${reset} Run ${bold}ghosts${reset} to start."
	say "${dim}$1${reset}"
}

main "$@"
