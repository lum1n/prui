#!/bin/sh
# Install the latest goreleaser binary of prui.
#   curl -fsSL https://raw.githubusercontent.com/lum1n/prui/master/install.sh | sh
#   BINDIR=/usr/local/bin sh install.sh
set -eu

REPO="lum1n/prui"
BASE="https://github.com/${REPO}/releases/latest/download"
BINDIR="${BINDIR:-${HOME}/.local/bin}"

need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "prui-install: need $1 on PATH" >&2
		exit 1
	fi
}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*)
	echo "prui-install: unsupported arch: $(uname -m)" >&2
	exit 1
	;;
esac
case "$os" in
linux | darwin) ;;
*)
	echo "prui-install: unsupported OS: $(uname -s) (linux and macOS only)" >&2
	exit 1
	;;
esac

need curl
need tar
need install

sums=$(curl -fsSL "${BASE}/checksums.txt")
asset=$(printf '%s\n' "$sums" | awk -v o="$os" -v a="$arch" '
	$2 ~ ("^prui_.+_" o "_" a "\\.tar\\.gz$") { print $2; exit }
')
if [ -z "$asset" ]; then
	echo "prui-install: no release asset for ${os}/${arch}" >&2
	exit 1
fi
want=$(printf '%s\n' "$sums" | awk -v f="$asset" '$2 == f { print $1; exit }')

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "${BASE}/${asset}" -o "${tmp}/${asset}"

got=""
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "${tmp}/${asset}" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	got=$(shasum -a 256 "${tmp}/${asset}" | awk '{ print $1 }')
else
	echo "prui-install: need sha256sum or shasum" >&2
	exit 1
fi
if [ "$got" != "$want" ]; then
	echo "prui-install: checksum mismatch for ${asset}" >&2
	exit 1
fi

tar -xzf "${tmp}/${asset}" -C "$tmp" prui
mkdir -p "$BINDIR"
install -m 755 "${tmp}/prui" "${BINDIR}/prui"

echo "installed ${BINDIR}/prui"
"${BINDIR}/prui" version
if ! command -v prui >/dev/null 2>&1; then
	echo "prui-install: add ${BINDIR} to PATH" >&2
fi
