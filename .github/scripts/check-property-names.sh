#!/usr/bin/env bash
# Fails when rapid.Check is called in a function whose name does not
# contain "Property". property-nightly.yml selects its long run with
# -run Property, so a rapid test with any other name only ever runs at the
# per-PR count. Run from the repository root.
set -euo pipefail

bad=$(git ls-files -- '*_test.go' | xargs awk '
  /^func / { name = $2; sub(/\(.*/, "", name) }
  /rapid\.Check\(/ && name !~ /Property/ { printf "%s:%d: rapid.Check in %s\n", FILENAME, FNR, name }
')
if [ -n "$bad" ]; then
  echo "$bad"
  echo "Rename these functions to contain \"Property\" (for example TestX_PropertyY) so property-nightly.yml runs them." >&2
  exit 1
fi
