#!/usr/bin/env bash
# Runs one scenario twice through the CLI and checks that both logs are
# byte-identical and both runs report the same final state hash.
# Usage: bash tests/check_determinism.sh [config.json]   (default: t1_normal)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CFG="$(cd "$(dirname "${1:-$ROOT/config/t1_normal.json}")" && pwd)/$(basename "${1:-t1_normal.json}")"
NAME="$(basename "$CFG" .json)"
BIN="$ROOT/logs/.sim_bin"
LOG1="$ROOT/logs/${NAME}_det1.log"
LOG2="$ROOT/logs/${NAME}_det2.log"

(cd "$ROOT/src" && go build -o "$BIN" ./cmd/sim)
OUT1="$("$BIN" -config "$CFG" -log "$LOG1")"
OUT2="$("$BIN" -config "$CFG" -log "$LOG2")"
rm -f "$BIN"

if ! cmp -s "$LOG1" "$LOG2"; then
  echo "FAIL: logs differ"; diff "$LOG1" "$LOG2" | head -20; exit 1
fi
S1="${OUT1#*final_state=}"; S1="${S1%% *}"
S2="${OUT2#*final_state=}"; S2="${S2%% *}"
if [ "$S1" != "$S2" ]; then
  echo "FAIL: final state differs: $S1 vs $S2"; exit 1
fi
echo "OK: $NAME logs byte-identical ($(wc -l < "$LOG1") lines), final_state=$S1"
