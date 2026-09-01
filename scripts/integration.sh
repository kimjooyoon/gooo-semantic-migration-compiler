#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: integration.sh PATH_TO_COMPILER OUTPUT_DIR" >&2
  exit 64
fi

bin=$(realpath "$1")
out=$(realpath -m "$2")
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
"$bin" migrate --root "$root" --meta .gooo/migration-compiler.gooo --case fixtures/cases/rename-split-closed.json --output-dir "$out" >/dev/null
jq -e '
  .decision == "CLOSED" and
  .source_ir.dialect == "v1" and
  .target_ir.dialect == "v2" and
  .target_ir.effect_trace == ["read","transform","write"] and
  ([.plan.operations[] | select(.status == "APPLIED")] | length) == 2 and
  .repository_writes == 0
' "$out/case-report.json" >/dev/null
echo "semantic migration integration: PASS (source->IR->plan->target->replay->report)"
