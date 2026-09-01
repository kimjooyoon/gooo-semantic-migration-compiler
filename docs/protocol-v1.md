# Semantic migration protocol v1

## Authority and boundaries

The `.gooo` meta program is the source of truth for ontology and migration semantics. A mapping is not inferred from similar names. The compiler may only execute a mapping that the meta source declares.

The caller supplies an input root and an output directory. The output directory must be outside the input root. The compiler reads the source and writes only caller-owned output artifacts. It has no operation that edits the source repository.

## Compatibility decision

For each fixed case, the compiler produces a deterministic target graph and compares:

1. terminal reason;
2. ordered effect trace;
3. capability set;
4. complete origin coverage;
5. replay fingerprints.

`CLOSED` requires all declared mappings to be complete, compatible, scope-fixed, invariant-safe, and fully origin-covered. A missing, ambiguous, or unbounded mapping is `UNKNOWN` with the six-field blocked tuple. Information loss, invariant failure, and a replay counterexample are `REFUTED`. The precedence is fixed as `REFUTED > UNKNOWN > CLOSED`.

## Semantic IR

The IR is a canonical graph with a source digest, semantic digest, nodes, typed edges, entry, terminal reason, capability set, and ordered observable effects. The generated Go binding embeds the canonical target IR as a caller-owned artifact; it is not a source mutation or an automatic commit.

## Evidence protocol

Each case has a JSON receipt and a human report. The conformance index fixes the denominator and lists the exact case vector. Metrics include stage measurements and the required inventory fields. Remote runner fields are initially null and explicitly UNKNOWN. CI annotates them only after measuring the corresponding command on the GitHub runner and verifies that annotation does not change case decisions or artifact digests.
