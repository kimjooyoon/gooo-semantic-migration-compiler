package compiler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func RunConformance(meta MetaContract, metaRaw []byte, root, outputDir string) (ConformanceIndex, error) {
	if err := EnsureOutputOutsideRoot(root, outputDir); err != nil {
		return ConformanceIndex{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return ConformanceIndex{}, err
	}
	start := time.Now()
	index := ConformanceIndex{Schema: ConformanceSchema, Decision: DecisionClosed, FixedDenominator: meta.Denominator.Cases, Precedence: append([]string{}, meta.Precedence...), Cases: []ConformanceCase{}}
	metrics := Metrics{Schema: MetricsSchema, FixedDenominator: meta.Denominator.Cases, Tests: TestMetrics{Total: meta.Denominator.Cases, Selected: meta.Denominator.Cases}, Authority: Authority{RepositoryWrites: 0, InputRepositoryWrites: 0, LocalValidationCommands: 0, LocalTestExecutions: 0, LocalBuildExecutions: 0, LocalVetExecutions: 0, LocalConformanceExecutions: 0, LocalIntegrationExecutions: 0, CrossProjectRequiredGates: 0, Commits: 0, Pushes: 0, Merges: 0, Releases: 0, CallerOwnedOutput: true, OperatorActionsSeparate: true}, Improvement: unknownImprovement(metaRaw, "runner-not-yet-bound"), Utility: UtilityEvidence{State: DecisionUnknown, Reason: "NO_EXTERNAL_USER_EVIDENCE"}, RunnerMetricState: DecisionUnknown}
	metrics.RunnerMetricUnknown = unknownMetric("runner observations are provided only by the remote CI runner", "annotate_runner_observations", "runner_metric_fields")
	for _, declaration := range meta.Cases {
		caseInputPath := filepath.Join(root, filepath.FromSlash(declaration.Fixture))
		caseOutput := filepath.Join(outputDir, "cases", declaration.ID)
		report, err := EvaluateCase(meta, caseInputPath, root, caseOutput)
		if err != nil {
			return ConformanceIndex{}, fmt.Errorf("case %s: %w", declaration.ID, err)
		}
		pass := report.Decision == declaration.Expected
		index.Cases = append(index.Cases, ConformanceCase{Ordinal: declaration.Ordinal, CaseID: declaration.ID, Expected: declaration.Expected, Observed: report.Decision, Pass: pass, Reason: caseReason(report), Report: filepath.ToSlash(filepath.Join("cases", declaration.ID, "human-report.md"))})
		metrics.Tests.Executed++
		metrics.StageMetrics = append(metrics.StageMetrics, report.StageMetrics...)
		if !pass {
			metrics.Tests.Failed++
		}
		switch report.Decision {
		case DecisionClosed:
			metrics.Closed++
		case DecisionUnknown:
			metrics.Unknown++
			metrics.Tests.Unknown++
		case DecisionRefuted:
			metrics.Refuted++
		default:
			return ConformanceIndex{}, fmt.Errorf("unsupported decision %s", report.Decision)
		}
	}
	if len(index.Cases) != meta.Denominator.Cases || metrics.Tests.Executed != metrics.Tests.Total {
		return ConformanceIndex{}, errors.New("fixed denominator was not fully evaluated")
	}
	inventory, inventoryErr := InventoryForRoot(root)
	if inventoryErr != nil {
		return ConformanceIndex{}, inventoryErr
	}
	metrics.Inventory = inventory
	metrics.StageMetrics = append(metrics.StageMetrics, StageMetric{Stage: "conformance_runtime", WallMS: int(time.Since(start).Milliseconds()), PeakRSSKiB: peakRSSKiB()})
	if metrics.Closed+metrics.Unknown+metrics.Refuted != metrics.FixedDenominator {
		return ConformanceIndex{}, errors.New("decision counts do not cover the fixed denominator")
	}
	index.Closed = metrics.Closed
	index.Unknown = metrics.Unknown
	index.Refuted = metrics.Refuted
	index.Metrics = metrics
	if err := writeJSON(filepath.Join(outputDir, "conformance-index.json"), index); err != nil {
		return ConformanceIndex{}, err
	}
	generatedArtifacts, generatedBytes, countErr := countOutputArtifacts(outputDir)
	if countErr != nil {
		return ConformanceIndex{}, countErr
	}
	metrics.GeneratedArtifacts, metrics.GeneratedBytes = generatedArtifacts, generatedBytes
	index.Metrics = metrics
	if err := writeJSON(filepath.Join(outputDir, "conformance-index.json"), index); err != nil {
		return ConformanceIndex{}, err
	}
	human := renderConformanceReport(index)
	if err := os.WriteFile(filepath.Join(outputDir, "human-report.md"), []byte(human), 0o644); err != nil {
		return ConformanceIndex{}, err
	}
	if err := writeJSON(filepath.Join(outputDir, "metrics.json"), metrics); err != nil {
		return ConformanceIndex{}, err
	}
	return index, nil
}

func AnnotateMetrics(indexPath, observationsPath string) (ConformanceIndex, error) {
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		return ConformanceIndex{}, err
	}
	var index ConformanceIndex
	decoder := json.NewDecoder(bytes.NewReader(indexRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return ConformanceIndex{}, fmt.Errorf("parse conformance index: %w", err)
	}
	observationsRaw, err := os.ReadFile(observationsPath)
	if err != nil {
		return ConformanceIndex{}, err
	}
	var observations RunnerObservations
	decoder = json.NewDecoder(bytes.NewReader(observationsRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&observations); err != nil {
		return ConformanceIndex{}, fmt.Errorf("parse runner observations: %w", err)
	}
	if !validRunnerObservations(observations) {
		return ConformanceIndex{}, errors.New("runner observations must provide all non-negative integer wall_ms and peak_rss_kib fields")
	}
	index.Metrics.RunnerObservations = observations
	index.Metrics.RunnerMetricState = DecisionClosed
	index.Metrics.RunnerMetricUnknown = nil
	index.Metrics.StageMetrics = append(index.Metrics.StageMetrics,
		StageMetric{Stage: "compile", WallMS: value(observations.CompileWallMS), PeakRSSKiB: value(observations.CompilePeakRSSKiB)},
		StageMetric{Stage: "build", WallMS: value(observations.BuildWallMS), PeakRSSKiB: value(observations.BuildPeakRSSKiB)},
		StageMetric{Stage: "test", WallMS: value(observations.TestWallMS), PeakRSSKiB: value(observations.TestPeakRSSKiB)},
		StageMetric{Stage: "conformance", WallMS: value(observations.ConformanceWallMS), PeakRSSKiB: value(observations.ConformancePeakRSSKiB)},
		StageMetric{Stage: "integration", WallMS: value(observations.IntegrationWallMS), PeakRSSKiB: value(observations.IntegrationPeakRSSKiB)},
	)
	if err := ParseMetricsValue(index.Metrics); err != nil {
		return ConformanceIndex{}, err
	}
	if err := writeJSON(indexPath, index); err != nil {
		return ConformanceIndex{}, err
	}
	humanPath := filepath.Join(filepath.Dir(indexPath), "human-report.md")
	if err := os.WriteFile(humanPath, []byte(renderConformanceReport(index)), 0o644); err != nil {
		return ConformanceIndex{}, err
	}
	if err := writeJSON(filepath.Join(filepath.Dir(indexPath), "metrics.json"), index.Metrics); err != nil {
		return ConformanceIndex{}, err
	}
	return index, nil
}

func ParseMetrics(raw []byte) (Metrics, error) {
	var metrics Metrics
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metrics); err != nil {
		return Metrics{}, err
	}
	if err := ParseMetricsValue(metrics); err != nil {
		return Metrics{}, err
	}
	return metrics, nil
}

func ParseMetricsValue(metrics Metrics) error {
	if metrics.Schema != MetricsSchema || metrics.FixedDenominator != 9 || metrics.Closed+metrics.Unknown+metrics.Refuted != metrics.FixedDenominator || metrics.Tests.Total != metrics.FixedDenominator || metrics.Tests.Selected != metrics.FixedDenominator || metrics.Tests.Executed != metrics.FixedDenominator || metrics.Tests.Reused != 0 || metrics.Tests.Failed != 0 || metrics.Tests.Unknown != metrics.Unknown {
		return errors.New("metrics denominator, test counts, or decision counts are inconsistent")
	}
	if metrics.Authority.RepositoryWrites != 0 || metrics.Authority.InputRepositoryWrites != 0 || metrics.Authority.CrossProjectRequiredGates != 0 || metrics.Authority.LocalValidationCommands != 0 || metrics.Authority.LocalTestExecutions != 0 || metrics.Authority.LocalBuildExecutions != 0 || metrics.Authority.LocalVetExecutions != 0 || metrics.Authority.LocalConformanceExecutions != 0 || metrics.Authority.LocalIntegrationExecutions != 0 {
		return errors.New("authority counts are not zero")
	}
	if metrics.GeneratedArtifacts <= 0 || metrics.GeneratedBytes <= 0 || metrics.Inventory.RootREADMEExcluded != true {
		return errors.New("generated artifact or inventory evidence is missing")
	}
	if metrics.RunnerMetricState != DecisionClosed || !validRunnerObservations(metrics.RunnerObservations) {
		return errors.New("runner metrics are not fully annotated")
	}
	return nil
}

func InventoryForRoot(root string) (Inventory, error) {
	var result Inventory
	result.RootREADMEExcluded = true
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			result.DescendantDirs++
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if rel == "README.md" {
			return nil
		}
		result.RegularFiles++
		switch filepath.Ext(path) {
		case ".go":
			result.GoFiles++
			lines, readErr := physicalLines(path)
			if readErr != nil {
				return readErr
			}
			result.GoPhysicalLines += lines
		case ".gooo":
			result.GoooFiles++
			lines, readErr := physicalLines(path)
			if readErr != nil {
				return readErr
			}
			result.GoooPhysicalLines += lines
		}
		return nil
	})
	return result, err
}

func physicalLines(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return 0, nil
	}
	lines := strings.Count(string(raw), "\n")
	if raw[len(raw)-1] != '\n' {
		lines++
	}
	return lines, nil
}

func countOutputArtifacts(root string) (int, int, error) {
	count := 0
	bytesWritten := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && filepath.Base(path) != "metrics.json" {
			count++
			bytesWritten += int(info.Size())
		}
		return nil
	})
	return count, bytesWritten, err
}

func validRunnerObservations(value RunnerObservations) bool {
	values := []*int{value.CompileWallMS, value.CompilePeakRSSKiB, value.BuildWallMS, value.BuildPeakRSSKiB, value.TestWallMS, value.TestPeakRSSKiB, value.ConformanceWallMS, value.ConformancePeakRSSKiB, value.IntegrationWallMS, value.IntegrationPeakRSSKiB}
	for _, item := range values {
		if item == nil || *item < 0 {
			return false
		}
	}
	return true
}

func value(pointer *int) int {
	if pointer == nil {
		return 0
	}
	return *pointer
}

func unknownMetric(reason, nextOperation string, blockedBy string) *UnknownRecord {
	return &UnknownRecord{Stage: "REMOTE_CI_METRICS", Step: "OBSERVE_RUNNER", Reason: reason, UnknownClass: "METRIC_MISSING", NextOperation: nextOperation, BlockedBy: []string{blockedBy}}
}

func unknownImprovement(metaRaw []byte, runner string) Improvement {
	return Improvement{State: DecisionUnknown, Reason: "IMPROVEMENT_UNKNOWN_EXACT_BEFORE_AFTER_PAIR_NOT_PROVIDED", Scenario: "", SourceDigest: DigestBytes(metaRaw), ContractDigest: DigestBytes([]byte("semantic-migration-compiler-v1")), FixtureDigest: "", Toolchain: ToolchainVersion, Runner: runner, Before: nil, After: nil}
}

func caseReason(report CaseReport) string {
	if report.Decision == DecisionClosed {
		return "all declared mappings, invariants, origin coverage, and replay observations are exact"
	}
	if report.Decision == DecisionUnknown && len(report.Unknowns) > 0 {
		return report.Unknowns[0].Reason
	}
	if len(report.RefutedReasons) > 0 {
		return report.RefutedReasons[0]
	}
	return "decision has no explainable evidence"
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func sortedCaseIDs(cases []ConformanceCase) []string {
	ids := make([]string, 0, len(cases))
	for _, item := range cases {
		ids = append(ids, item.CaseID)
	}
	sort.Strings(ids)
	return ids
}
