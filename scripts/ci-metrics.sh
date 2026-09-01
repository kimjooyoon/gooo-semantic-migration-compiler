#!/usr/bin/env bash
set -euo pipefail

root=$(pwd)
work=${1:?caller-owned CI work directory is required}
mkdir -p "$work"
first="$work/conformance"
integration="$work/integration"
mkdir -p "$first" "$integration"

measure() {
  name=$1
  shift
  timing="$work/$name.time"
  /usr/bin/time -f '%e %M' -o "$timing" "$@"
  read -r seconds rss < "$timing"
  wall=$(awk -v value="$seconds" 'BEGIN { printf "%d", value * 1000 }')
  jq -n --argjson wall "$wall" --argjson rss "$rss" '{wall_ms:$wall,peak_rss_kib:$rss}' > "$work/$name.observation.json"
}

measure compile go test -run '^$' ./...
measure build go build -trimpath -o "$work/gooo-semantic-migration-compiler" ./cmd/gooo-semantic-migration-compiler
measure test go test ./...

before=$(git status --porcelain=v1 -z --untracked-files=all | sha256sum | awk '{print $1}')
measure conformance "$work/gooo-semantic-migration-compiler" conformance --root "$root" --meta .gooo/migration-compiler.gooo --output-dir "$first" > "$work/conformance.stdout"
measure integration "$work/gooo-semantic-migration-compiler" migrate --root "$root" --meta .gooo/migration-compiler.gooo --case fixtures/cases/rename-split-closed.json --output-dir "$integration" > "$work/integration.stdout"
after=$(git status --porcelain=v1 -z --untracked-files=all | sha256sum | awk '{print $1}')
test "$before" = "$after"

jq -n \
  --slurpfile compile "$work/compile.observation.json" \
  --slurpfile build "$work/build.observation.json" \
  --slurpfile test "$work/test.observation.json" \
  --slurpfile conformance "$work/conformance.observation.json" \
  --slurpfile integration "$work/integration.observation.json" \
  '{compile_wall_ms:$compile[0].wall_ms,compile_peak_rss_kib:$compile[0].peak_rss_kib,
    build_wall_ms:$build[0].wall_ms,build_peak_rss_kib:$build[0].peak_rss_kib,
    test_wall_ms:$test[0].wall_ms,test_peak_rss_kib:$test[0].peak_rss_kib,
    conformance_wall_ms:$conformance[0].wall_ms,conformance_peak_rss_kib:$conformance[0].peak_rss_kib,
    integration_wall_ms:$integration[0].wall_ms,integration_peak_rss_kib:$integration[0].peak_rss_kib}' \
  > "$work/runner-observations.json"

"$work/gooo-semantic-migration-compiler" annotate-metrics --index "$first/conformance-index.json" --observations "$work/runner-observations.json" > "$work/annotated-index.stdout"
"$work/gooo-semantic-migration-compiler" validate-metrics --metrics "$first/metrics.json"

jq -e '
  .schema == "gooo/semantic-migration-compiler/conformance/v1" and
  .fixed_denominator == 9 and
  .closed == 3 and .unknown == 3 and .refuted == 3 and
  ([.cases[] | select(.pass == true)] | length) == 9 and
  (.precedence == ["REFUTED","UNKNOWN","CLOSED"]) and
  (.metrics.runner_metric_state == "CLOSED") and
  (.metrics.authority.repository_writes == 0) and
  (.metrics.authority.input_repository_writes == 0) and
  (.metrics.authority.cross_project_required_gates == 0) and
  (.metrics.authority.local_validation_commands == 0) and
  (.metrics.authority.local_test_executions == 0) and
  (.metrics.authority.local_build_executions == 0) and
  (.metrics.authority.local_vet_executions == 0) and
  (.metrics.authority.local_conformance_executions == 0) and
  (.metrics.authority.local_integration_executions == 0) and
  (.metrics.inventory.root_readme_excluded == true) and
  (.metrics.generated_artifacts > 0) and
  (.metrics.generated_bytes > 0) and
  (.metrics.improvement.state == "UNKNOWN") and
  (.metrics.utility.state == "UNKNOWN") and
  ([.cases[] | select(.observed == "UNKNOWN") |
    (.reason | length > 0)] | all)
' "$first/conformance-index.json"

test "$(jq -r '.decision' "$integration/case-report.json")" = "CLOSED"
test "$(jq -r '.target_ir.dialect' "$integration/case-report.json")" = "v2"
test "$(jq -r '.target_ir.effect_trace | join(",")' "$integration/case-report.json")" = "read,transform,write"

cp -R "$integration" "$first/integration"

manifest_tmp="$work/artifact-file-digests.tmp"
find "$first" -type f ! -name artifact-file-digests.json -print0 |
  while IFS= read -r -d '' file; do
    rel=${file#"$first/"}
    digest=$(sha256sum "$file" | awk '{print $1}')
    size=$(wc -c < "$file" | tr -d ' ')
    jq -n --arg path "$rel" --arg digest "sha256:$digest" --argjson size "$size" '{path:$path,size:$size,digest:$digest}'
  done | jq -s 'sort_by(.path)' > "$manifest_tmp"
mv "$manifest_tmp" "$first/artifact-file-digests.json"

artifact_name=${ARTIFACT_NAME:-gooo-semantic-migration-compiler-evidence-${GITHUB_RUN_ID:-local}}
jq -n \
  --arg repository "${GITHUB_REPOSITORY:-local}" \
  --arg workflow "${GITHUB_WORKFLOW:-local}" \
  --arg event "${GITHUB_EVENT_NAME:-local}" \
  --arg job "${GITHUB_JOB:-local}" \
  --arg run_id "${GITHUB_RUN_ID:-local}" \
  --arg sha "${GITHUB_SHA:-local}" \
  --arg pr "${PR_NUMBER:-0}" \
  --arg artifact "$artifact_name" \
  --arg manifest_digest "sha256:$(sha256sum "$first/artifact-file-digests.json" | awk '{print $1}')" \
  '{repository:$repository,workflow:$workflow,event:$event,job:$job,run_id:$run_id,commit_sha:$sha,pull_request:$pr,artifact_name:$artifact,artifact_manifest_digest:$manifest_digest,authority:{runtime_repository_writes:0,runtime_cross_project_required_gates:0,operator_actions_separate:true}}' \
  > "$first/ci-provenance.json"

echo "CI evidence ready at $first"
