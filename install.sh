#!/bin/sh
# Install zen-linear.
#
# Downloads the release binary for this machine. macOS and Linux, arm64 and
# amd64; anything else installs with go install, which the message says.
#
#   curl -fsSL https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.sh | sh
#
# INSTALL_DIR overrides where the binary lands, and defaults to ~/.local/bin.
# VERSION pins a release, as v0.1.0, and defaults to the latest.

set -eu

REPO="praxis-labs-io/zen-linear"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BINARY="zen-linear"

die() {
	echo "$1" >&2
	exit 1
}

for tool in curl tar; do
	command -v "$tool" >/dev/null 2>&1 ||
		die "$tool is not on PATH, and installing zen-linear needs it."
done

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
x86_64 | amd64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
*) arch="$(uname -m)" ;;
esac

case "$os/$arch" in
darwin/arm64 | darwin/amd64 | linux/amd64 | linux/arm64) ;;
*)
	die "No release binary for $os/$arch. Install it with Go instead:
    go install github.com/$REPO/cmd/$BINARY@latest"
	;;
esac

# The latest release's tag, read out of the API rather than asking for jq. The
# call and the parse are separate so a rate limit or a proxy is not reported as
# a repository with no releases in it.
if [ -n "${VERSION:-}" ]; then
	tag="$VERSION"
else
	# No -f, so an HTTP error comes back as a status to read rather than as a
	# bare non-zero exit. A repository with no releases, a rate limit and an
	# unreachable network are three different answers.
	answer="$(curl -sL -w '\n%{http_code}' "https://api.github.com/repos/$REPO/releases/latest")" ||
		die "Could not reach the GitHub API to look up the latest release."

	case "$(printf '%s' "$answer" | tail -n 1)" in
	200) ;;
	404) die "There is no published release to install yet." ;;
	403) die "The GitHub API refused the lookup, most likely a rate limit. Retry, or pass VERSION=vX.Y.Z." ;;
	*) die "The GitHub API answered $(printf '%s' "$answer" | tail -n 1) looking up the latest release." ;;
	esac

	tag="$(printf '%s' "$answer" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1)"
	[ -n "$tag" ] || die "Could not read a tag out of the latest release."
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

url="https://github.com/$REPO/releases/download/$tag/${BINARY}_${tag#v}_${os}_${arch}.tar.gz"
echo "Downloading $tag for $os/$arch"
curl -fsSL "$url" -o "$work/release.tar.gz" || die "Could not download $url"
tar -xzf "$work/release.tar.gz" -C "$work"

mkdir -p "$INSTALL_DIR"
cp "$work/$BINARY" "$INSTALL_DIR/$BINARY"
chmod 0755 "$INSTALL_DIR/$BINARY"

echo "Installed $INSTALL_DIR/$BINARY"

case ":$PATH:" in
*":$INSTALL_DIR:"*) ;;
*)
	echo
	echo "$INSTALL_DIR is not on your PATH. Add it:"
	echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
	;;
esac

# The binary is no use without a Linear token, and the first run is where that
# is noticed. Naming the command here is cheaper than the error message.
echo
echo "Authenticate before the first run:"
echo "    $BINARY auth login"
