#!/bin/sh
# Require test names and source filenames to describe behavior, not project phases.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# Reject test names prefixed with project-priority or arbitrary stage identifiers.
phase_named_tests="$(find internal -type f -name '*_test.go' \
    -exec grep -EnH 'func[[:space:]]+Test(P[0-9]+|Step[A-Z])[[:alnum:]_]*\(' {} + || true)"
if [ -n "$phase_named_tests" ]; then
  printf >&2 'Error: test names must describe behavior, not phases:\n%s\n' "$phase_named_tests"
  exit 1
fi

# Reject phase-coded filenames in source, tests, and documentation.
if find internal ui/src ui/tests docs -type f \
    \( -iname 'p[0-9]*_test.go' -o -iname 'p[0-9]*.ts' -o -iname 'p[0-9]*.mjs' \
       -o -iname '*step[a-z]_test.go' -o -iname 'p[0-9]*.md' \) \
    | grep .; then
  echo 'Error: rename phase-based files to descriptive names.' >&2
  exit 1
fi

printf 'PASS: descriptive regression-test naming\n'
