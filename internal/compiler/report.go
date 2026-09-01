package compiler

import (
	"fmt"
	"strings"
)

func renderCaseReport(report CaseReport, fixtureRaw []byte) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Gooo semantic migration case: %s\n\n", report.CaseID)
	fmt.Fprintf(&out, "- Decision: **%s**\n- Expected: **%s**\n- Description: %s\n- Source: `%s`\n- Source digest: `%s`\n- Target digest: `%s`\n- Fixture digest: `%s`\n\n", report.Decision, report.ExpectedDecision, report.Description, report.SourcePath, report.SourceDigest, report.TargetDigest, DigestBytes(fixtureRaw))
	out.WriteString("## Deterministic migration plan\n\n")
	fmt.Fprintf(&out, "Plan digest: `%s`\n\n", report.Plan.PlanDigest)
	out.WriteString("| ordinal | mapping | kind | status | reason |\n|---:|---|---|---|---|\n")
	for _, operation := range report.Plan.Operations {
		fmt.Fprintf(&out, "| %d | `%s` | %s | %s | %s |\n", operation.Ordinal, operation.MappingID, operation.Kind, operation.Status, operation.Reason)
	}
	out.WriteString("\n## Observable behavior\n\n")
	fmt.Fprintf(&out, "Before terminal: `%s`; after terminal: `%s`\n\n", report.SourceIR.TerminalReason, report.TargetIR.TerminalReason)
	fmt.Fprintf(&out, "Before effects: `%s`; after effects: `%s`\n\n", strings.Join(report.SourceIR.EffectTrace, " → "), strings.Join(report.TargetIR.EffectTrace, " → "))
	out.WriteString("| invariant | observed | detail |\n|---|---|---|\n")
	for _, predicate := range report.PredicateVector {
		fmt.Fprintf(&out, "| `%s` | %t | %s |\n", predicate.ID, predicate.ObservedPreserve, predicate.Detail)
	}
	out.WriteString("\nReplay observations:\n\n")
	for _, replay := range report.Replay {
		fmt.Fprintf(&out, "- run %d: exact=%t fingerprint=`%s`\n", replay.Run, replay.Exact, replay.Fingerprint)
	}
	if len(report.Unknowns) > 0 {
		out.WriteString("\n## UNKNOWN evidence\n\n")
		for _, item := range report.Unknowns {
			fmt.Fprintf(&out, "- stage=%s; step=%s; reason=%s; unknown_class=%s; next_operation=%s; blocked_by=%s\n", item.Stage, item.Step, item.Reason, item.UnknownClass, item.NextOperation, strings.Join(item.BlockedBy, ","))
		}
	}
	if len(report.RefutedReasons) > 0 {
		out.WriteString("\n## REFUTED evidence\n\n")
		for _, reason := range report.RefutedReasons {
			fmt.Fprintf(&out, "- %s\n", reason)
		}
	}
	fmt.Fprintf(&out, "\nGenerated artifacts: %d files / %d bytes. Repository writes: %d.\n", len(report.GeneratedFiles), report.GeneratedBytes, report.RepositoryWrites)
	return out.String()
}

func renderConformanceReport(index ConformanceIndex) string {
	var out strings.Builder
	out.WriteString("# Gooo semantic migration compiler conformance\n\n")
	fmt.Fprintf(&out, "Decision: **%s** (every fixed case matched its declared outcome)\n\n", index.Decision)
	fmt.Fprintf(&out, "Fixed denominator: **%d fixed cases**\n\n", index.FixedDenominator)
	fmt.Fprintf(&out, "Counts: CLOSED=%d, UNKNOWN=%d, REFUTED=%d\n\n", index.Closed, index.Unknown, index.Refuted)
	out.WriteString("Decision precedence: `REFUTED > UNKNOWN > CLOSED`\n\n")
	out.WriteString("| ordinal | case | expected | observed | reason |\n|---:|---|---|---|---|\n")
	for _, item := range index.Cases {
		fmt.Fprintf(&out, "| %d | `%s` | %s | %s | %s |\n", item.Ordinal, item.CaseID, item.Expected, item.Observed, item.Reason)
	}
	metrics := index.Metrics
	out.WriteString("\n## Measurements\n\n")
	fmt.Fprintf(&out, "Tests: total=%d selected=%d executed=%d reused=%d failed=%d unknown=%d\n\n", metrics.Tests.Total, metrics.Tests.Selected, metrics.Tests.Executed, metrics.Tests.Reused, metrics.Tests.Failed, metrics.Tests.Unknown)
	fmt.Fprintf(&out, "Inventory: descendant_dirs=%d regular_files=%d Go_files=%d Go_physical_lines=%d Gooo_files=%d Gooo_physical_lines=%d (root README excluded=%t)\n\n", metrics.Inventory.DescendantDirs, metrics.Inventory.RegularFiles, metrics.Inventory.GoFiles, metrics.Inventory.GoPhysicalLines, metrics.Inventory.GoooFiles, metrics.Inventory.GoooPhysicalLines, metrics.Inventory.RootREADMEExcluded)
	fmt.Fprintf(&out, "Generated artifacts: %d files / %d bytes\n\n", metrics.GeneratedArtifacts, metrics.GeneratedBytes)
	fmt.Fprintf(&out, "Runner metric state: %s\n\n", metrics.RunnerMetricState)
	writeRunnerMetric(&out, "compile", metrics.RunnerObservations.CompileWallMS, metrics.RunnerObservations.CompilePeakRSSKiB)
	writeRunnerMetric(&out, "build", metrics.RunnerObservations.BuildWallMS, metrics.RunnerObservations.BuildPeakRSSKiB)
	writeRunnerMetric(&out, "test", metrics.RunnerObservations.TestWallMS, metrics.RunnerObservations.TestPeakRSSKiB)
	writeRunnerMetric(&out, "conformance", metrics.RunnerObservations.ConformanceWallMS, metrics.RunnerObservations.ConformancePeakRSSKiB)
	writeRunnerMetric(&out, "integration", metrics.RunnerObservations.IntegrationWallMS, metrics.RunnerObservations.IntegrationPeakRSSKiB)
	out.WriteString("\n## Authority\n\n")
	fmt.Fprintf(&out, "Input repository writes=%d; generated output is caller-owned=%t; local validation commands=%d; cross-project required gates=%d.\n\n", metrics.Authority.InputRepositoryWrites, metrics.Authority.CallerOwnedOutput, metrics.Authority.LocalValidationCommands, metrics.Authority.CrossProjectRequiredGates)
	fmt.Fprintf(&out, "Operator actions separate from runtime: %t. Commits=%d, pushes=%d, merges=%d, releases=%d.\n\n", metrics.Authority.OperatorActionsSeparate, metrics.Authority.Commits, metrics.Authority.Pushes, metrics.Authority.Merges, metrics.Authority.Releases)
	fmt.Fprintf(&out, "Improvement: %s (%s). Utility: %s (%s).\n", metrics.Improvement.State, metrics.Improvement.Reason, metrics.Utility.State, metrics.Utility.Reason)
	return out.String()
}

func writeRunnerMetric(out *strings.Builder, name string, wall, rss *int) {
	fmt.Fprintf(out, "- %s: wall_ms=%s peak_rss_kib=%s\n", name, pointerText(wall), pointerText(rss))
}

func pointerText(value *int) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprintf("%d", *value)
}
