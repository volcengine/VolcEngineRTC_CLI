#!/bin/sh

set -eu

profile=${1:-coverage.out}
threshold=${2:-70.0}

die() {
  echo "coverage-gate: $*" >&2
  exit 2
}

case "$threshold" in
  ''|*[!0-9.]*) die "threshold must be a percentage from 0 through 100: $threshold" ;;
esac

if ! awk -v value="$threshold" 'BEGIN {
  if (value !~ /^[0-9]+([.][0-9]+)?$/ || value < 0 || value > 100) exit 1
}' </dev/null; then
  die "threshold must be a percentage from 0 through 100: $threshold"
fi

[ -f "$profile" ] || die "coverage profile not found: $profile"

if ! report=$(go tool cover -func="$profile" 2>&1); then
  echo "$report" >&2
  die "cannot summarize coverage profile: $profile"
fi

coverage=$(printf '%s\n' "$report" | awk '$1 == "total:" {
  value = $3
  sub(/%$/, "", value)
  print value
}')

if ! awk -v value="$coverage" 'BEGIN {
  if (value !~ /^[0-9]+([.][0-9]+)?$/ || value < 0 || value > 100) exit 1
}' </dev/null; then
  die "coverage profile has no valid aggregate percentage: $profile"
fi

if awk -v actual="$coverage" -v required="$threshold" 'BEGIN { exit !(actual >= required) }'; then
  status=PASS
  exit_code=0
else
  status=FAIL
  exit_code=1
fi

message="coverage-gate: $status actual=$coverage% required=$threshold%"
if [ "$exit_code" -eq 0 ]; then
  echo "$message"
else
  echo "$message" >&2
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Go unit coverage"
    echo
    echo "| Status | Actual | Required |"
    echo "| --- | ---: | ---: |"
    echo "| $status | $coverage% | $threshold% |"
    echo
    echo "Scope: \`cmd\` and \`internal\` production packages; subprocess E2E coverage is tracked separately."
  } >> "$GITHUB_STEP_SUMMARY"
fi

exit "$exit_code"
