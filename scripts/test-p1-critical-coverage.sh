#!/bin/sh
# Run the complete P1 statement coverage contract with an isolated CA-failure process.
# Both profiles must instrument the same current source tree.
set -eu

root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$root"
tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/swanpanel-p1-coverage.XXXXXXXX")"
cleanup() {
    case "$tmpdir" in
        */swanpanel-p1-coverage.*)
            if [ -d "$tmpdir" ]; then rm -r -- "$tmpdir"; fi
            ;;
    esac
}
trap cleanup EXIT HUP INT TERM

go test -count=1 -coverpkg=./... -coverprofile="$tmpdir/suite.out" ./...
SWANPANEL_TEST_ROOT_CA_UNAVAILABLE=1 go test -count=1 -coverpkg=./... \
    -coverprofile="$tmpdir/ca-failure.out" ./internal/api \
    -run '^TestUnavailableRootCAFailsClosedAcrossControlPlaneOperations$'
# Use set semantics: a statement covered by either suite counts once.
# Deduplicate instrumentation blocks because -coverpkg includes multiple test binaries.
awk '
    FNR == 1 { if (NR == 1) print "mode: set"; next }
    {
        key = $1 " " $2
        if (!(key in seen)) { seen[key] = 1; order[++n] = key }
        if ($3 > 0) covered[key] = 1
    }
    END {
        for (i = 1; i <= n; i++) print order[i], covered[order[i]] + 0
    }
' "$tmpdir/suite.out" "$tmpdir/ca-failure.out" > "$tmpdir/merged.out"

sh scripts/check-p1-critical-coverage.sh "$tmpdir/merged.out"
