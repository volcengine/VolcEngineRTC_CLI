#!/bin/sh

set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
checker="$repo_root/scripts/check-coverage.sh"
fixture_dir=$(mktemp -d)
trap 'rm -rf "$fixture_dir"' EXIT HUP INT TERM

source_file="$fixture_dir/sample.go"
profile="$fixture_dir/coverage.out"
summary="$fixture_dir/summary.md"

sed 's/^+//' > "$source_file" <<'EOF'
+package sample
+
+func covered() int {
+	return 1
+}
+
+func uncovered() int {
+	return 2
+}
EOF

sed "s|SOURCE|$source_file|g" > "$profile" <<'EOF'
mode: atomic
SOURCE:3.20,5.2 7 1
SOURCE:7.22,9.2 3 0
EOF

GITHUB_STEP_SUMMARY="$summary" "$checker" "$profile" 70.0 >/dev/null
grep -Fq '| PASS | 70.0% | 70.0% |' "$summary"

if "$checker" "$profile" 70.1 >/dev/null 2>&1; then
  echo "check-coverage-test: below-threshold profile unexpectedly passed" >&2
  exit 1
fi

if "$checker" "$fixture_dir/missing.out" 70.0 >/dev/null 2>&1; then
  echo "check-coverage-test: missing profile unexpectedly passed" >&2
  exit 1
fi

sed 's/^+//' > "$fixture_dir/malformed.out" <<'EOF'
+not a Go coverage profile
EOF
if "$checker" "$fixture_dir/malformed.out" 70.0 >/dev/null 2>&1; then
  echo "check-coverage-test: malformed profile unexpectedly passed" >&2
  exit 1
fi

for invalid in -1 100.1 nope 1.2.3; do
  if "$checker" "$profile" "$invalid" >/dev/null 2>&1; then
    echo "check-coverage-test: invalid threshold unexpectedly passed: $invalid" >&2
    exit 1
  fi
done

echo "check-coverage-test: passed"
