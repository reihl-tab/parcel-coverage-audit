#!/bin/sh
# Install the shipped coverage-audit man pages (issue #45).
# Usage: tools/install_man_pages.sh [PREFIX]   (default: /usr/local)
# Installs to $DESTDIR$PREFIX/share/man/man1/ (DESTDIR for staged packaging).
# Only the shipped commands (the auditor and its support binary) have
# pages; internal CI/dev scripts under scripts/ deliberately get none.
set -eu
PREFIX=${1:-/usr/local}
SRC="$(cd "$(dirname "$0")/.." && pwd)/man/man1"
DEST="${DESTDIR:-}$PREFIX/share/man/man1"
mkdir -p "$DEST"
for page in "$SRC"/coverage-audit*.1; do
    install -m 644 "$page" "$DEST/"
    echo "installed $DEST/$(basename "$page")"
done
