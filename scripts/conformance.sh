#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: conformance.sh PATH_TO_COMPILER OUTPUT_DIR" >&2
  exit 64
fi

bin=$(realpath "$1")
out=$(realpath -m "$2")
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
"$bin" conformance --root "$root" --meta .gooo/migration-compiler.gooo --output-dir "$out" >/dev/null
jq -e '.fixed_denominator == 9 and .closed == 3 and .unknown == 3 and .refuted == 3 and ([.cases[] | select(.pass)] | length) == 9' "$out/conformance-index.json" >/dev/null
echo "semantic migration conformance: PASS (closed=3 unknown=3 refuted=3)"
