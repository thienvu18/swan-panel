#!/bin/sh
# Fail CI when an unreviewed direct Go module appears (no extra dependencies).
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

approved=$(mktemp)
current=$(mktemp)
trap 'rm -f "$approved" "$current"' EXIT HUP INT TERM

awk '
  /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
  { print $1 }
' docs/approved-direct-modules.txt | LC_ALL=C sort -u > "$approved"

awk '
  $1 == "require" && $2 == "(" { in_block = 1; next }
  in_block && $1 == ")" { in_block = 0; next }
  in_block && $1 != "" && $2 ~ /^v/ && $0 !~ /\/\/ indirect/ { print $1; next }
  $1 == "require" && $2 != "(" && $3 ~ /^v/ && $0 !~ /\/\/ indirect/ { print $2 }
' go.mod | LC_ALL=C sort -u > "$current"

if ! diff -u "$approved" "$current"; then
  printf >&2 '\nDependency approval mismatch. Update docs/DEPENDENCY_POLICY.md and docs/approved-direct-modules.txt after review.\n'
  exit 1
fi

printf 'Direct Go dependency policy: %s approved modules; no unreviewed modules.\n' "$(wc -l < "$current" | tr -d ' ')"
