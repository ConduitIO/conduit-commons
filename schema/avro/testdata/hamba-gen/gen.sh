#!/bin/bash
# Regenerates ../hamba_compat_fixtures.json with the pre-migration codec:
# conduit-commons v0.6.0, which encodes with github.com/hamba/avro/v2 v2.28.0.
# The corpus is the same compatCases() table the compat test decodes, copied
# with only the package clause changed. The build happens in a throwaway module
# so no hamba/avro reference lands in this repository's module files.
#
# The fixtures are a frozen record of what the old library wrote. Only rerun
# this to ADD cases; never regenerate existing entries with the new codec.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
sed 's/^package avro$/package main/' "$here/../../compat_cases_test.go" > "$tmp/cases.go"
cp "$here/main.go" "$tmp/main.go"
cd "$tmp"
cat > go.mod <<'MOD'
module hambagen

go 1.24.13

require github.com/conduitio/conduit-commons v0.6.0
MOD
go mod tidy
go run . > "$here/../hamba_compat_fixtures.json.new"
mv "$here/../hamba_compat_fixtures.json.new" "$here/../hamba_compat_fixtures.json"
