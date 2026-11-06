#!/usr/bin/env bash
# Single entry point for every check: build, unit tests, end-to-end tests
# (T1–T8 and a 3-seed sweep of every config), determinism of every config.
# Run from anywhere:   bash tests/run_all.sh [extra go test args]
# A longer sweep:      SWEEP_SEEDS=20 bash tests/run_all.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOWORK=off   # the grader has no go.work: test exactly what is submitted

echo "==> [1/4] build + vet (src)"
(cd "$ROOT/src" && go build ./... && go vet ./...)

echo "==> [2/4] unit tests"
(cd "$ROOT/tests" && go vet ./... && go test ./units/... -count=1 "$@")

echo "==> [3/4] end-to-end tests"
(cd "$ROOT/tests" && go test ./e2e/... -count=1 "$@")

echo "==> [4/4] determinism check (CLI, every config twice)"
for cfg in "$ROOT"/config/*.json; do
  bash "$ROOT/tests/check_determinism.sh" "$cfg"
done
rm -f "$ROOT"/logs/*_det[12].log   # the two copies only matter while comparing

echo "ALL CHECKS PASSED"
