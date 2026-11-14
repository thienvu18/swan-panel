#!/bin/sh
# Build native-architecture static Linux binary with the official CGO SQLite driver.
# Requires Go, musl-gcc, file and readelf on the *target architecture*.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

[ "$(go env GOOS)" = "linux" ] || {
  echo "Linux build required (use Linux AMD64/ARM64 runner)" >&2
  exit 1
}
command -v musl-gcc >/dev/null 2>&1 || {
  echo "Install musl-tools: apt-get install musl-tools" >&2
  exit 1
}
command -v readelf >/dev/null 2>&1 || {
  echo "Install binutils to validate static linking" >&2
  exit 1
}
command -v file >/dev/null 2>&1 || {
  echo "Install file to validate the output binary" >&2
  exit 1
}

ARCH=$(go env GOARCH)
case "$ARCH" in
  amd64|arm64) ;;
  *) echo "Unsupported release architecture: $ARCH" >&2; exit 1 ;;
esac

VERSION=${VERSION:-dev}
OUTPUT=${OUTPUT:-dist/swan-panel-linux-$ARCH}
mkdir -p "$(dirname -- "$OUTPUT")"

export CGO_ENABLED=1
export CC=musl-gcc
export GOOS=linux
# Must be run on a native runner to avoid silent cross-compilation with host C compiler.
case "$(uname -m):$ARCH" in
  x86_64:amd64|aarch64:arm64|arm64:arm64) ;;
  *) echo "Refusing to build with mismatched native CC for GOARCH=$ARCH" >&2; exit 1 ;;
esac

echo "Building $OUTPUT with CGO=$CGO_ENABLED, CC=$CC, version=$VERSION"
go build \
  -trimpath \
  -tags "netgo,osusergo,sqlite_omit_load_extension" \
  -ldflags "-s -w -linkmode external -extldflags '-static' -X main.version=$VERSION" \
  -o "$OUTPUT" \
  ./cmd/server

file "$OUTPUT"
if ! file "$OUTPUT" | grep -q 'statically linked'; then
  echo "Release artifact is not statically linked" >&2
  exit 1
fi
if readelf -l "$OUTPUT" | grep -q 'INTERP'; then
  echo "Release artifact has a dynamic ELF interpreter" >&2
  exit 1
fi

# The binary can execute its metadata check without root, SQLite or strongSwan.
"$OUTPUT" --version | grep -Fx "SwanPanel $VERSION"
