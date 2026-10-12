#!/usr/bin/env bash
# Build the plugin for the current GOOS/GOARCH and pack it as a plugin store
# asset: dist/codex-timezone_<version>_<goos>_<goarch>.zip with the versioned
# library at the zip root.
#
#   scripts/release/package.sh 0.1.0 [repository-url]
#
# Cross builds follow the usual Go environment, e.g.
#   GOARCH=amd64 CC="clang -arch x86_64" scripts/release/package.sh 0.1.0
set -euo pipefail

VERSION="${1:?usage: package.sh <version> [repository-url]}"
REPOSITORY="${2:-}"
ID=codex-timezone
cd "$(dirname "$0")/../.."
# Relative by default: on Windows runners Go and Python do not understand
# MSYS-style absolute paths such as /d/a/repo.
BIN_DIR="${BIN_DIR:-bin}"
DIST_DIR="${DIST_DIR:-dist}"

GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
case "$GOOS" in
  windows) EXT=dll ;;
  darwin) EXT=dylib ;;
  *) EXT=so ;;
esac
PY="$(command -v python3 || command -v python)"

LDFLAGS="-s -w -X main.pluginVersion=$VERSION"
if [ -n "$REPOSITORY" ]; then
  LDFLAGS="$LDFLAGS -X main.pluginRepository=$REPOSITORY"
fi

LIB="$BIN_DIR/$ID-v$VERSION.$EXT"
ZIP="$DIST_DIR/${ID}_${VERSION}_${GOOS}_${GOARCH}.zip"
mkdir -p "$BIN_DIR" "$DIST_DIR"
CGO_ENABLED=1 go build -buildmode=c-shared -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$LIB" .
rm -f "${LIB%.*}.h" "$ZIP"
"$PY" -m zipfile -c "$ZIP" "$LIB"
echo "$ZIP"
