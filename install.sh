#!/usr/bin/env bash
# Installs the latest todo release.
#
#   curl -fsSL https://raw.githubusercontent.com/boutdecode/todo/main/install.sh | bash
#
# Environment:
#   VERSION      release tag to install (default: latest), e.g. VERSION=v0.1.0
#   INSTALL_DIR  target directory (default: ~/.local/bin)
set -euo pipefail

REPO="${TODO_REPO:-boutdecode/todo}"
BASE="${TODO_RELEASES_URL:-https://github.com/$REPO/releases}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

need curl
need tar

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $(uname -s) (Linux and macOS only)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

version="${VERSION:-}"
if [ -z "$version" ]; then
  # /releases/latest redirects to /releases/tag/<tag>: no API rate limit, no jq.
  url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE/latest")" || die "cannot reach $BASE/latest"
  version="${url##*/}"
fi
[ -n "$version" ] && [ "$version" != "latest" ] || die "no release found at $BASE"

name="todo_${version}_${os}_${arch}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

say "Downloading todo $version ($os/$arch)…"
curl -fsSL -o "$tmp/$name.tar.gz" "$BASE/download/$version/$name.tar.gz" ||
  die "download failed: $BASE/download/$version/$name.tar.gz"

# Verify the checksum when the release provides one.
if curl -fsSL -o "$tmp/checksums.txt" "$BASE/download/$version/checksums.txt" 2>/dev/null; then
  expected="$(awk -v f="$name.tar.gz" '$2 == f {print $1}' "$tmp/checksums.txt")"
  if [ -n "$expected" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      actual="$(sha256sum "$tmp/$name.tar.gz" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      actual="$(shasum -a 256 "$tmp/$name.tar.gz" | awk '{print $1}')"
    else
      actual=""
      say "warning: no sha256 tool found, skipping checksum verification"
    fi
    if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
      die "checksum mismatch for $name.tar.gz"
    fi
  fi
fi

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
[ -f "$tmp/$name/todo" ] || die "unexpected archive layout"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/$name/todo" "$INSTALL_DIR/todo"
say "Installed $INSTALL_DIR/todo"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) say "Note: $INSTALL_DIR is not in your PATH. Add it with:"
     say "  export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
