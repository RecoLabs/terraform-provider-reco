#!/usr/bin/env bash
# Runs the unit tests with the race detector and fails if total statement coverage of the
# provider code drops below MIN_COVERAGE (default 85). The fake API in internal/fakereco is test
# support, so it is excluded from the measurement.
set -euo pipefail

min="${MIN_COVERAGE:-85}"
profile="$(mktemp)"
trap 'rm -f "$profile"' EXIT

pkgs="$(go list ./internal/... | grep -v '/fakereco')"
coverpkg="$(echo "$pkgs" | paste -sd, -)"

# shellcheck disable=SC2086 # $pkgs is a newline-separated package list
go test $pkgs -count=1 -race -coverpkg="$coverpkg" -coverprofile="$profile"

total="$(go tool cover -func="$profile" | awk '/^total:/ { gsub("%", "", $3); print $3 }')"
echo "Total statement coverage: ${total}% (minimum ${min}%)"
if ! awk -v t="$total" -v m="$min" 'BEGIN { exit (t + 0 >= m + 0) ? 0 : 1 }'; then
	echo "ERROR: coverage ${total}% is below the required ${min}%." >&2
	exit 1
fi
