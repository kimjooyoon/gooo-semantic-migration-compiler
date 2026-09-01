# gooo-semantic-migration-compiler

`gooo-semantic-migration-compiler` is a small, proof-carrying migration compiler for Gooo ontology changes. It moves an existing `.gooo` program from a declared v1 semantic contract to v2 while preserving observable behavior when, and only when, the meta declaration proves that preservation.

The authoritative language is [`.gooo/migration-compiler.gooo`](.gooo/migration-compiler.gooo). It declares both semantic contracts, concepts, rename/split/merge/removal mappings, preservation invariants, compiler activities, authority boundaries, the UNKNOWN tuple, and the fixed conformance corpus. Go is limited to parsing those declarations, lowering programs to semantic IR, applying declared mappings, executing the graph, and rendering evidence.

```text
.gooo meta + old .gooo program
  → deterministic migration plan
  → migrated .gooo + semantic IR + generated Go binding
  → before/after observable execution
  → invariant and counterexample verification
  → machine receipt + human report
```

## Fixed semantic denominator

The nine cases are fixed before implementation in both the meta source and [`contracts/denominator-v1.json`](contracts/denominator-v1.json). The decision order is `REFUTED > UNKNOWN > CLOSED`; no score, percentage, weighted sum, or inferred closed state is emitted.

| ordinal | case | expected |
|---:|---|---|
| 1 | rename-closed | CLOSED |
| 2 | split-closed | CLOSED |
| 3 | rename-split-closed | CLOSED |
| 4 | missing-mapping-unknown | UNKNOWN |
| 5 | ambiguous-split-unknown | UNKNOWN |
| 6 | unbounded-merge-unknown | UNKNOWN |
| 7 | incompatible-removal-refuted | REFUTED |
| 8 | invariant-break-refuted | REFUTED |
| 9 | counterexample-refuted | REFUTED |

Every UNKNOWN carries `stage`, `step`, `reason`, `unknown_class`, `next_operation`, and `blocked_by`. Missing mapping evidence fails closed. Information loss, invariant violations, and observable counterexamples remain REFUTED.

## End-to-end use

All generated output is caller-owned and must be outside the input repository. The input `.gooo` file is never overwritten.

```text
go run ./cmd/gooo-semantic-migration-compiler migrate \
  -root . \
  -meta .gooo/migration-compiler.gooo \
  -case fixtures/cases/rename-split-closed.json \
  -output-dir /tmp/gooo-semantic-migration-compiler-case

go run ./cmd/gooo-semantic-migration-compiler conformance \
  -root . \
  -meta .gooo/migration-compiler.gooo \
  -output-dir /tmp/gooo-semantic-migration-compiler-conformance
```

The case output includes `migration-plan.json`, `target.gooo`, `source.semantic-ir.json`, `target.semantic-ir.json`, `target.generated.go`, `execution.json`, `case-report.json`, and `human-report.md`. The conformance output contains all nine case directories, a fixed-case index, metrics, and a human report.

CI is the validation authority. The workflow records independent compile, build, test, conformance, and integration `wall_ms`/`peak_rss_kib` observations, test totals, repository inventory, generated artifact counts/bytes, and local/remote authority counts. A runner metric is `null` plus UNKNOWN until the remote runner annotates it. Utility remains UNKNOWN because this repository contains no external-user evidence. Improvement remains `IMPROVEMENT_UNKNOWN` unless one CI job supplies an exact integer before/after pair for the same scenario, source, contract, fixture, toolchain, and runner.

The runtime has zero repository-write, commit, push, merge, release, cross-project-gate, and local-validation authority. Release promotion is an operator action after the PR and main workflow succeed; the release workflow uses `github.token` and refuses to overwrite an existing release asset.
