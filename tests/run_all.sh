#!/usr/bin/env bash
# Single entry point for every check: build, unit tests, end-to-end tests,
# determinism. Run from anywhere:   bash tests/run_all.sh [extra go test args]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOWORK=off   # the grader has no go.work: test exactly what is submitted

echo "==> [1/4] build + vet (src)"
(cd "$ROOT/src" && go build ./... && go vet ./...)

echo "==> [2/4] unit tests"
(cd "$ROOT/tests" && go vet ./... && go test ./units/... -count=1 "$@")

echo "==> [3/4] end-to-end tests"
(cd "$ROOT/tests" && go test ./e2e/... -count=1 "$@")

echo "==> [4/4] determinism check (CLI, same config twice)"
bash "$ROOT/tests/check_determinism.sh" "$ROOT/config/t1_normal.json"

echo "ALL CHECKS PASSED"
