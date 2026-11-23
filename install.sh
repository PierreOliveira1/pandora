#!/bin/sh
set -eu

REPO="PierreOliveira1/pandora"
VERSION="${PANDORA_VERSION:-}"
INSTALL_DIR="${PANDORA_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "error: $*" >&2
	exit 1
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
FreeBSD) os=freebsd ;;
*) fail "unsupported OS: $(uname -s). Download a build from https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported architecture: $(uname -m). Download a build from https://github.com/$REPO/releases" ;;
esac

if [ -z "$VERSION" ]; then
	latest_url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
		fail "could not find the latest pandora release"
	case "$latest_url" in
	*/tag/v*) VERSION=${latest_url##*/tag/v} ;;
	*) fail "could not find the latest pandora version" ;;
	esac
fi
VERSION=${VERSION#v}

archive="pandora_${VERSION}_${os}_${arch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/v$VERSION"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL -o "$tmpdir/$archive" "$base_url/$archive" ||
	fail "could not download $archive from $base_url; check that v$VERSION has a $os/$arch build"
curl -fsSL -o "$tmpdir/checksums.txt" "$base_url/checksums.txt" ||
	fail "could not download checksums.txt from $base_url"

expected=$(awk -v file="$archive" '$2 == file { print $1 }' "$tmpdir/checksums.txt")
[ -n "$expected" ] || fail "checksums.txt has no entry for $archive"

if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmpdir/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$tmpdir/$archive" | awk '{ print $1 }')
else
	fail "sha256sum or shasum is required to verify the download"
fi

[ "$actual" = "$expected" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmpdir/$archive" -C "$tmpdir"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmpdir/pandora" "$INSTALL_DIR/pandora"

echo "pandora $VERSION installed to $INSTALL_DIR"
case ":$PATH:" in
*":$INSTALL_DIR:"*) ;;
*)
	echo "Add it to your PATH:"
	echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
	;;
esac
echo "Next: pandora add ... (see https://github.com/$REPO#quick-start)"
