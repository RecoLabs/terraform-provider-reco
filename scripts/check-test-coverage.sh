#!/usr/bin/env bash
set -euo pipefail

missing=0
for f in internal/resources/*.go internal/datasources/*.go; do
	[[ $f == *_test.go || $f == */schema_constants.go ]] && continue
	base="${f%.go}"
	if [[ ! -f "${base}_test.go" ]]; then
		echo "Missing test file: ${base}_test.go"
		missing=1
	fi
done

if [[ $missing -ne 0 ]]; then
	echo "ERROR: Some resource/datasource files are missing test files." >&2
	exit 1
fi
echo "Test coverage check passed."
