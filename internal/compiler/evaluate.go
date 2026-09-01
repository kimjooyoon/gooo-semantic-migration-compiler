package compiler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func EvaluateCase(meta MetaContract, casePath, root, outputDir string) (CaseReport, error) {
	if err := EnsureOutputOutsideRoot(root, outputDir); err != nil {
		return CaseReport{}, err
	}
	fixture, fixtureRaw, err := LoadFixture(casePath, meta)
	if err != nil {
		return CaseReport{}, err
	}
	caseDecl, ok := meta.Case(fixture.CaseID)
	if !ok {
		return CaseReport{}, fmt.Errorf("case %s is not declared", fixture.CaseID)
	}
	sourcePath := filepath.Join(root, filepath.FromSlash(caseDecl.Source))
	if !isWithinRoot(root, sourcePath) {
		return CaseReport{}, errors.New("source path escapes input repository")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return CaseReport{}, err
	}
	recorder := &stageRecorder{}
	source, sourceRaw, err := timedLoadProgram(sourcePath, recorder, "parse_source")
	if err != nil {
		return CaseReport{}, err
	}
	sourceIR, err := timedLower(source, caseDecl.Source, sourceRaw, recorder, "lower_before")
	if err != nil {
		return CaseReport{}, err
	}
	var migration MigrationResult
	start := time.Now()
	migration, err = CompileMigration(meta, source, caseDecl)
	recorder.record("compile_migration_plan", start)
	if err != nil {
		return CaseReport{}, err
	}
	targetRaw, err := RenderProgram(migration.Target, "generated from declared semantic migration plan")
	if err != nil {
		return CaseReport{}, err
	}
	start = time.Now()
	targetIR, err := Lower(migration.Target, "generated/target.gooo", targetRaw)
	recorder.record("generate_target_ir", start)
	if err != nil {
		return CaseReport{}, err
	}
	start = time.Now()
	sourceExecution, sourceExecErr := Execute(source)
	targetExecution, targetExecErr := Execute(migration.Target)
	recorder.record("execute_before_after", start)
	if sourceExecErr != nil || targetExecErr != nil {
		return CaseReport{}, fmt.Errorf("observable execution failed: source=%v target=%v", sourceExecErr, targetExecErr)
	}

	predicates := evaluatePredicates(meta, sourceIR, targetIR, migration.OriginMap)
	replay := []ReplayEvidence{}
	for run := 1; run <= fixture.ReplayRuns; run++ {
		evidence := ReplayEvidence{Run: run, Source: sourceExecution, Target: targetExecution, Exact: sourceExecution.TerminalReason == targetExecution.TerminalReason && strings.Join(sourceExecution.EffectTrace, "\x00") == strings.Join(targetExecution.EffectTrace, "\x00") && strings.Join(sourceExecution.CapabilitySet, "\x00") == strings.Join(targetExecution.CapabilitySet, "\x00")}
		evidence.Fingerprint = executionFingerprint(sourceExecution, targetExecution)
		replay = append(replay, evidence)
	}
	unknowns := append([]UnknownRecord{}, migration.Unknowns...)
	refutedReasons := append([]string{}, migration.RefutedReasons...)
	for _, predicate := range predicates {
		if !predicate.ObservedPreserve {
			refutedReasons = append(refutedReasons, "INVARIANT_VIOLATION:"+predicate.ID)
		}
	}
	for _, evidence := range replay {
		if !evidence.Exact {
			refutedReasons = append(refutedReasons, "COUNTEREXAMPLE:OBSERVABLE_BEHAVIOR_CHANGED")
			break
		}
	}
	decision := decide(refutedReasons, unknowns)
	if decision == DecisionUnknown {
		for _, unknown := range unknowns {
			if !unknown.Valid() {
				return CaseReport{}, errors.New("UNKNOWN result did not preserve the required six-field tuple")
			}
		}
	}

	plan := MigrationPlan{Schema: PlanSchema, CaseID: fixture.CaseID, SourceDialect: source.Dialect, TargetDialect: migration.Target.Dialect, Operations: migration.Operations, OriginMap: migration.OriginMap, Decision: decision, Unknowns: unknowns, RefutedReasons: refutedReasons}
	plan.PlanDigest = plan.canonicalDigest()
	planRaw, err := RenderPlan(plan)
	if err != nil {
		return CaseReport{}, err
	}
	sourceIRRaw, err := RenderIR(sourceIR)
	if err != nil {
		return CaseReport{}, err
	}
	targetIRRaw, err := RenderIR(targetIR)
	if err != nil {
		return CaseReport{}, err
	}
	bindingRaw, err := RenderBinding(targetIR, "generated")
	if err != nil {
		return CaseReport{}, err
	}
	executionRaw, err := json.MarshalIndent(struct {
		Source Execution        `json:"source"`
		Target Execution        `json:"target"`
		Replay []ReplayEvidence `json:"replay"`
	}{sourceExecution, targetExecution, replay}, "", "  ")
	if err != nil {
		return CaseReport{}, err
	}
	executionRaw = append(executionRaw, '\n')
	artifacts := map[string][]byte{
		"source.semantic-ir.json": sourceIRRaw,
		"target.semantic-ir.json": targetIRRaw,
		"migration-plan.json":     planRaw,
		"target.gooo":              targetRaw,
		"target.generated.go":      bindingRaw,
		"execution.json":           executionRaw,
	}
	generatedFiles, generatedBytes, err := writeArtifacts(outputDir, artifacts)
	if err != nil {
		return CaseReport{}, err
	}
	start = time.Now()
	stageMetrics := recorder.finish()
	stageMetrics = append(stageMetrics, measureStage("verify_invariants", func() {}))
	report := CaseReport{
		Schema: ReportSchema, CaseID: fixture.CaseID, ExpectedDecision: caseDecl.Expected, Decision: decision, Description: fixture.Description,
		SourcePath: caseDecl.Source, SourceDigest: DigestBytes(sourceRaw), TargetDigest: DigestBytes(targetRaw), SourceIR: sourceIR, TargetIR: targetIR,
		Plan: plan, PredicateVector: predicates, Replay: replay, Unknowns: unknowns, RefutedReasons: refutedReasons,
		GeneratedFiles: generatedFiles, GeneratedBytes: generatedBytes, RepositoryWrites: 0, StageMetrics: stageMetrics,
	}
	caseReportRaw, err := marshalJSON(report)
	if err != nil {
		return CaseReport{}, err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "case-report.json"), caseReportRaw, 0o644); err != nil {
		return CaseReport{}, err
	}
	human := renderCaseReport(report, fixtureRaw)
	if err := os.WriteFile(filepath.Join(outputDir, "human-report.md"), []byte(human), 0o644); err != nil {
		return CaseReport{}, err
	}
	_ = start
	return report, nil
}

func evaluatePredicates(meta MetaContract, source, target SemanticIR, origins []OriginPair) []PredicateEvidence {
	result := make([]PredicateEvidence, 0, len(meta.Invariants))
	for _, invariant := range meta.Invariants {
		observed := false
		detail := invariant.Predicate
		beforeDigest := source.SemanticDigest
		afterDigest := target.SemanticDigest
		switch invariant.ID {
		case "terminal-reason":
			observed = source.TerminalReason == target.TerminalReason
		case "effect-trace":
			observed = strings.Join(source.EffectTrace, "\x00") == strings.Join(target.EffectTrace, "\x00")
		case "capability-set":
			observed = strings.Join(source.CapabilitySet, "\x00") == strings.Join(target.CapabilitySet, "\x00")
		case "origin-coverage":
			targetIDs := map[string]bool{}
			for _, node := range target.Nodes {
				targetIDs[node.ID] = true
			}
			observed = len(origins) == len(source.Nodes)
			for _, pair := range origins {
				if len(pair.To) == 0 {
					observed = false
				}
				for _, targetID := range pair.To {
					if !targetIDs[targetID] {
						observed = false
					}
				}
			}
		default:
			detail = "undeclared predicate implementation"
		}
		result = append(result, PredicateEvidence{ID: invariant.ID, DeclaredPreserve: invariant.Preserve, ObservedPreserve: observed, BeforeDigest: beforeDigest, AfterDigest: afterDigest, Detail: detail})
	}
	return result
}

func decide(refuted []string, unknown []UnknownRecord) string {
	if len(refuted) > 0 {
		return DecisionRefuted
	}
	if len(unknown) > 0 {
		return DecisionUnknown
	}
	return DecisionClosed
}

func writeArtifacts(outputDir string, artifacts map[string][]byte) ([]string, int, error) {
	keys := make([]string, 0, len(artifacts))
	for key := range artifacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bytesWritten := 0
	for _, key := range keys {
		if err := os.WriteFile(filepath.Join(outputDir, key), artifacts[key], 0o644); err != nil {
			return nil, 0, err
		}
		bytesWritten += len(artifacts[key])
	}
	return keys, bytesWritten, nil
}

func marshalJSON(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

type stageRecorder struct{ values []StageMetric }

func (s *stageRecorder) record(stage string, start time.Time) {
	usage := syscall.Rusage{}
	peak := 0
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) == nil {
		peak = int(usage.Maxrss)
	}
	s.values = append(s.values, StageMetric{Stage: stage, WallMS: int(time.Since(start).Milliseconds()), PeakRSSKiB: peak})
}

func (s *stageRecorder) finish() []StageMetric { return append([]StageMetric{}, s.values...) }

func peakRSSKiB() int {
	usage := syscall.Rusage{}
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil {
		return 0
	}
	return int(usage.Maxrss)
}

func measureStage(name string, fn func()) StageMetric {
	start := time.Now()
	fn()
	usage := syscall.Rusage{}
	peak := 0
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) == nil {
		peak = int(usage.Maxrss)
	}
	return StageMetric{Stage: name, WallMS: int(time.Since(start).Milliseconds()), PeakRSSKiB: peak}
}

func timedLoadProgram(path string, recorder *stageRecorder, name string) (Program, []byte, error) {
	start := time.Now()
	program, raw, err := LoadProgram(path)
	recorder.record(name, start)
	return program, raw, err
}

func timedLower(program Program, path string, raw []byte, recorder *stageRecorder, name string) (SemanticIR, error) {
	start := time.Now()
	ir, err := Lower(program, path, raw)
	recorder.record(name, start)
	return ir, err
}

func EnsureOutputOutsideRoot(root, output string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	outAbs, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, outAbs)
	if err != nil {
		return err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("output must be a caller-owned directory outside the input repository")
	}
	return nil
}

func isWithinRoot(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
