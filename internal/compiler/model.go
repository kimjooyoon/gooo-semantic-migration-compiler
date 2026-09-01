package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MetaSchema        = "gooo/semantic-migration-compiler/meta/v1"
	CaseSchema        = "gooo/semantic-migration-compiler/case/v1"
	ProgramSchema     = "gooo/semantic-migration-compiler/program/v1"
	IRSchema          = "gooo/semantic-migration-compiler/semantic-ir/v1"
	PlanSchema        = "gooo/semantic-migration-compiler/migration-plan/v1"
	BindingSchema     = "gooo/semantic-migration-compiler/generated-binding/v1"
	ReportSchema      = "gooo/semantic-migration-compiler/case-report/v1"
	ConformanceSchema = "gooo/semantic-migration-compiler/conformance/v1"
	MetricsSchema     = "gooo/semantic-migration-compiler/metrics/v1"
	ToolchainVersion  = "go1.27.0"
	DecisionClosed    = "CLOSED"
	DecisionUnknown   = "UNKNOWN"
	DecisionRefuted   = "REFUTED"
)

var Precedence = []string{DecisionRefuted, DecisionUnknown, DecisionClosed}

var RequiredUnknownFields = []string{"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by"}

type MetaContract struct {
	Schema          string
	Authority       string
	Dialects        []string
	Contracts       []ContractDecl
	Mappings        []MappingDecl
	Invariants      []InvariantDecl
	Activities      []ActivityDecl
	Denominator     DenominatorDecl
	Precedence      []string
	UnknownFields   []string
	AuthorityPolicy map[string]string
	SourcePolicy    map[string]string
	Cases           []CaseDecl
}

type ContractDecl struct {
	Version  string
	ID       string
	Schema   string
	Concepts []string
}

type MappingDecl struct {
	Ordinal       int
	ID            string
	Kind          string
	From          string
	To            string
	Complete      bool
	Compatible    bool
	Reversible    bool
	ScopeFixed    bool
	InvariantSafe bool
	TargetEffects string
	Reason        string
}

type InvariantDecl struct {
	Ordinal  int
	ID       string
	Predicate string
	Preserve bool
}

type ActivityDecl struct {
	Ordinal  int
	ID       string
	Semantic string
}

type DenominatorDecl struct {
	ID    string
	Cases int
	Unit  string
}

type CaseDecl struct {
	Ordinal       int
	ID            string
	Fixture       string
	Source        string
	Plan          string
	Expected      string
	TargetTerminal string
}

type CaseFixture struct {
	Schema     string `json:"schema"`
	CaseID     string `json:"case_id"`
	Description string `json:"description"`
	ReplayRuns int    `json:"replay_runs"`
}

type Program struct {
	Schema         string `json:"schema"`
	ID             string `json:"id"`
	Dialect        string `json:"dialect"`
	Entry          string `json:"entry"`
	Nodes          []Node `json:"nodes"`
	Edges          []Edge `json:"edges"`
	TerminalReason string `json:"terminal_reason"`
}

type Node struct {
	ID           string   `json:"id"`
	Symbol       string   `json:"symbol"`
	Capabilities []string `json:"capabilities"`
	Effects      []string `json:"effects"`
}

type Edge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type Execution struct {
	TerminalReason string   `json:"terminal_reason"`
	EffectTrace    []string `json:"effect_trace"`
	CapabilitySet  []string `json:"capability_set"`
	Path           []string `json:"path"`
}

type SemanticIR struct {
	Schema         string   `json:"schema"`
	ProgramID      string   `json:"program_id"`
	Dialect        string   `json:"dialect"`
	SourcePath     string   `json:"source_path"`
	SourceDigest   string   `json:"source_digest"`
	SemanticDigest string   `json:"semantic_digest"`
	Nodes          []Node   `json:"nodes"`
	Edges          []Edge   `json:"edges"`
	Entry          string   `json:"entry"`
	TerminalReason string   `json:"terminal_reason"`
	CapabilitySet  []string `json:"capability_set"`
	EffectTrace    []string `json:"effect_trace"`
}

type OriginPair struct {
	From string   `json:"from"`
	To   []string `json:"to"`
}

type MigrationOperation struct {
	Ordinal   int      `json:"ordinal"`
	MappingID string   `json:"mapping_id"`
	Kind      string   `json:"kind"`
	From      string   `json:"from,omitempty"`
	To        []string `json:"to,omitempty"`
	Status    string   `json:"status"`
	Reason    string   `json:"reason,omitempty"`
}

type MigrationPlan struct {
	Schema         string               `json:"schema"`
	CaseID         string               `json:"case_id"`
	SourceDialect  string               `json:"source_dialect"`
	TargetDialect  string               `json:"target_dialect"`
	Operations     []MigrationOperation `json:"operations"`
	OriginMap      []OriginPair         `json:"origin_map"`
	Decision       string               `json:"decision"`
	Unknowns       []UnknownRecord      `json:"unknowns,omitempty"`
	RefutedReasons []string             `json:"refuted_reasons,omitempty"`
	PlanDigest     string               `json:"plan_digest"`
}

type UnknownRecord struct {
	Stage         string   `json:"stage"`
	Step          string   `json:"step"`
	Reason        string   `json:"reason"`
	UnknownClass  string   `json:"unknown_class"`
	NextOperation string   `json:"next_operation"`
	BlockedBy     []string `json:"blocked_by"`
}

func (u UnknownRecord) Valid() bool {
	return u.Stage != "" && u.Step != "" && u.Reason != "" && u.UnknownClass != "" && u.NextOperation != "" && len(u.BlockedBy) > 0
}

type PredicateEvidence struct {
	ID               string `json:"id"`
	DeclaredPreserve bool   `json:"declared_preserve"`
	ObservedPreserve bool   `json:"observed_preserve"`
	BeforeDigest     string `json:"before_digest"`
	AfterDigest      string `json:"after_digest"`
	Detail           string `json:"detail"`
}

type ReplayEvidence struct {
	Run         int      `json:"run"`
	Source      Execution `json:"source"`
	Target      Execution `json:"target"`
	Exact       bool     `json:"exact"`
	Fingerprint string   `json:"fingerprint"`
}

type StageMetric struct {
	Stage      string `json:"stage"`
	WallMS     int    `json:"wall_ms"`
	PeakRSSKiB int    `json:"peak_rss_kib"`
}

type CaseReport struct {
	Schema             string               `json:"schema"`
	CaseID             string               `json:"case_id"`
	ExpectedDecision   string               `json:"expected_decision"`
	Decision           string               `json:"decision"`
	Description        string               `json:"description"`
	SourcePath         string               `json:"source_path"`
	SourceDigest       string               `json:"source_digest"`
	TargetDigest       string               `json:"target_digest"`
	SourceIR           SemanticIR           `json:"source_ir"`
	TargetIR           SemanticIR           `json:"target_ir"`
	Plan               MigrationPlan       `json:"plan"`
	PredicateVector    []PredicateEvidence  `json:"predicate_vector"`
	Replay             []ReplayEvidence     `json:"replay"`
	Unknowns           []UnknownRecord      `json:"unknowns"`
	RefutedReasons     []string             `json:"refuted_reasons"`
	GeneratedFiles     []string             `json:"generated_files"`
	GeneratedBytes     int                  `json:"generated_bytes"`
	RepositoryWrites   int                  `json:"repository_writes"`
	StageMetrics       []StageMetric        `json:"stage_metrics"`
}

type ConformanceCase struct {
	Ordinal   int    `json:"ordinal"`
	CaseID    string `json:"case_id"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
	Pass      bool   `json:"pass"`
	Reason    string `json:"reason"`
	Report    string `json:"report"`
}

type Inventory struct {
	DescendantDirs    int `json:"descendant_dirs"`
	RegularFiles      int `json:"regular_files"`
	GoFiles           int `json:"go_files"`
	GoPhysicalLines   int `json:"go_physical_lines"`
	GoooFiles         int `json:"gooo_files"`
	GoooPhysicalLines int `json:"gooo_physical_lines"`
	RootREADMEExcluded bool `json:"root_readme_excluded"`
}

type TestMetrics struct {
	Total    int `json:"total"`
	Selected int `json:"selected"`
	Executed int `json:"executed"`
	Reused   int `json:"reused"`
	Failed   int `json:"failed"`
	Unknown  int `json:"unknown"`
}

type Authority struct {
	RepositoryWrites          int  `json:"repository_writes"`
	InputRepositoryWrites     int  `json:"input_repository_writes"`
	LocalValidationCommands   int  `json:"local_validation_commands"`
	LocalTestExecutions       int  `json:"local_test_executions"`
	LocalBuildExecutions      int  `json:"local_build_executions"`
	LocalVetExecutions        int  `json:"local_vet_executions"`
	LocalConformanceExecutions int `json:"local_conformance_executions"`
	LocalIntegrationExecutions int `json:"local_integration_executions"`
	CrossProjectRequiredGates int  `json:"cross_project_required_gates"`
	Commits                   int  `json:"commits"`
	Pushes                    int  `json:"pushes"`
	Merges                    int  `json:"merges"`
	Releases                  int  `json:"releases"`
	CallerOwnedOutput         bool `json:"caller_owned_output"`
	OperatorActionsSeparate   bool `json:"operator_actions_separate"`
}

type Improvement struct {
	State          string `json:"state"`
	Reason         string `json:"reason"`
	Scenario       string `json:"scenario"`
	SourceDigest   string `json:"source_digest"`
	ContractDigest string `json:"contract_digest"`
	FixtureDigest  string `json:"fixture_digest"`
	Toolchain      string `json:"toolchain"`
	Runner         string `json:"runner"`
	Before         *int   `json:"before"`
	After          *int   `json:"after"`
}

type UtilityEvidence struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type RunnerObservations struct {
	CompileWallMS       *int `json:"compile_wall_ms"`
	CompilePeakRSSKiB   *int `json:"compile_peak_rss_kib"`
	BuildWallMS         *int `json:"build_wall_ms"`
	BuildPeakRSSKiB     *int `json:"build_peak_rss_kib"`
	TestWallMS          *int `json:"test_wall_ms"`
	TestPeakRSSKiB      *int `json:"test_peak_rss_kib"`
	ConformanceWallMS   *int `json:"conformance_wall_ms"`
	ConformancePeakRSSKiB *int `json:"conformance_peak_rss_kib"`
	IntegrationWallMS   *int `json:"integration_wall_ms"`
	IntegrationPeakRSSKiB *int `json:"integration_peak_rss_kib"`
}

type Metrics struct {
	Schema               string            `json:"schema"`
	FixedDenominator    int               `json:"fixed_denominator"`
	Closed              int               `json:"closed"`
	Unknown             int               `json:"unknown"`
	Refuted             int               `json:"refuted"`
	Tests               TestMetrics       `json:"tests"`
	Inventory           Inventory         `json:"inventory"`
	GeneratedArtifacts  int               `json:"generated_artifacts"`
	GeneratedBytes      int               `json:"generated_bytes"`
	StageMetrics        []StageMetric     `json:"stage_metrics"`
	RunnerObservations  RunnerObservations `json:"runner_observations"`
	RunnerMetricState   string            `json:"runner_metric_state"`
	RunnerMetricUnknown *UnknownRecord   `json:"runner_metric_unknown,omitempty"`
	Authority           Authority         `json:"authority"`
	Improvement         Improvement       `json:"improvement"`
	Utility             UtilityEvidence   `json:"utility"`
}

type ConformanceIndex struct {
	Schema            string             `json:"schema"`
	Decision          string             `json:"decision"`
	FixedDenominator  int                `json:"fixed_denominator"`
	Closed            int                `json:"closed"`
	Unknown           int                `json:"unknown"`
	Refuted           int                `json:"refuted"`
	Precedence        []string           `json:"precedence"`
	Cases             []ConformanceCase  `json:"cases"`
	Metrics           Metrics            `json:"metrics"`
}

func DigestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DigestValue(value any) string {
	raw, _ := json.Marshal(value)
	return DigestBytes(raw)
}

func (m MetaContract) Mapping(id string) (MappingDecl, bool) {
	for _, item := range m.Mappings {
		if item.ID == id {
			return item, true
		}
	}
	return MappingDecl{}, false
}

func (m MetaContract) Case(id string) (CaseDecl, bool) {
	for _, item := range m.Cases {
		if item.ID == id {
			return item, true
		}
	}
	return CaseDecl{}, false
}

func (m MetaContract) Contract(version string) (ContractDecl, bool) {
	for _, item := range m.Contracts {
		if item.Version == version {
			return item, true
		}
	}
	return ContractDecl{}, false
}

func (m MetaContract) Validate() error {
	if m.Schema != MetaSchema || m.Authority != "metacode" {
		return errors.New(".gooo meta source is not the authoritative declaration")
	}
	if len(m.Dialects) != 2 || len(m.Contracts) != 2 || len(m.Mappings) < 8 || len(m.Invariants) != 4 || len(m.Activities) != 7 {
		return errors.New("meta declaration is incomplete")
	}
	if m.Denominator.Cases != 9 || m.Denominator.ID == "" || m.Denominator.Unit != "fixed-case" {
		return errors.New("fixed denominator must contain exactly nine cases")
	}
	if strings.Join(m.Precedence, ">") != "REFUTED>UNKNOWN>CLOSED" || strings.Join(m.UnknownFields, ",") != strings.Join(RequiredUnknownFields, ",") {
		return errors.New("decision precedence or UNKNOWN tuple is not fixed")
	}
	for _, key := range []string{"repository_writes", "local_validation_commands", "local_test_executions", "local_build_executions", "local_vet_executions", "local_conformance_executions", "local_integration_executions", "cross_project_required_gates", "automatic_commit", "automatic_push", "automatic_merge", "automatic_release"} {
		if m.AuthorityPolicy[key] != "0" {
			return fmt.Errorf("authority policy %s must be zero", key)
		}
	}
	if m.SourcePolicy["input_repository_writes"] != "0" || m.SourcePolicy["outputs"] != "caller_owned_only" || m.SourcePolicy["overwrite_source"] != "never" {
		return errors.New("source policy permits an unsafe write")
	}
	requiredKinds := map[string]bool{"rename": false, "split": false, "merge": false, "remove": false}
	for index, item := range m.Mappings {
		if item.Ordinal != index+1 || item.ID == "" || item.Kind == "" || item.From == "" || (item.Kind != "terminal" && item.To == "") {
			return fmt.Errorf("invalid mapping declaration at ordinal %d", index+1)
		}
		requiredKinds[item.Kind] = true
	}
	for kind, present := range requiredKinds {
		if !present {
			return fmt.Errorf("required mapping kind %s is not declared", kind)
		}
	}
	for index, item := range m.Cases {
		if item.Ordinal != index+1 || item.ID == "" || item.Fixture == "" || item.Source == "" || item.Plan == "" || !allowedDecision(item.Expected) {
			return fmt.Errorf("invalid case declaration at ordinal %d", index+1)
		}
	}
	return nil
}

func (p Program) canonical() Program {
	result := p
	result.Nodes = append([]Node(nil), p.Nodes...)
	for i := range result.Nodes {
		result.Nodes[i].Capabilities = sortedUnique(result.Nodes[i].Capabilities)
		result.Nodes[i].Effects = append([]string(nil), result.Nodes[i].Effects...)
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].ID < result.Nodes[j].ID })
	result.Edges = append([]Edge(nil), p.Edges...)
	for i := range result.Edges {
		if result.Edges[i].ID == "" {
			result.Edges[i].ID = "edge-" + result.Edges[i].From + "-" + result.Edges[i].To
		}
	}
	sort.Slice(result.Edges, func(i, j int) bool {
		if result.Edges[i].From != result.Edges[j].From {
			return result.Edges[i].From < result.Edges[j].From
		}
		return result.Edges[i].To < result.Edges[j].To
	})
	return result
}

func (p Program) Validate() error {
	if p.Schema != ProgramSchema || p.ID == "" || p.Dialect == "" || p.Entry == "" || p.TerminalReason == "" || len(p.Nodes) == 0 {
		return errors.New("program identity, entry, terminal, or nodes are missing")
	}
	nodes := map[string]bool{}
	for _, node := range p.Nodes {
		if node.ID == "" || node.Symbol == "" || nodes[node.ID] {
			return fmt.Errorf("invalid or duplicate node %q", node.ID)
		}
		nodes[node.ID] = true
	}
	if !nodes[p.Entry] {
		return fmt.Errorf("entry node %s is missing", p.Entry)
	}
	edges := map[string]bool{}
	for _, edge := range p.Edges {
		if edge.From == "" || edge.To == "" || !nodes[edge.From] || !nodes[edge.To] || edge.From == edge.To || edges[edge.From+"\x00"+edge.To] {
			return fmt.Errorf("invalid edge %s -> %s", edge.From, edge.To)
		}
		edges[edge.From+"\x00"+edge.To] = true
	}
	return nil
}

func (ir SemanticIR) canonicalDigest() string {
	copyIR := ir
	copyIR.SemanticDigest = ""
	raw, _ := json.Marshal(copyIR)
	return DigestBytes(raw)
}

func (p MigrationPlan) canonicalDigest() string {
	copyPlan := p
	copyPlan.PlanDigest = ""
	raw, _ := json.Marshal(copyPlan)
	return DigestBytes(raw)
}

func (r ReplayEvidence) observableEqual() bool {
	return r.Source.TerminalReason == r.Target.TerminalReason && strings.Join(r.Source.EffectTrace, "\x00") == strings.Join(r.Target.EffectTrace, "\x00") && strings.Join(r.Source.CapabilitySet, "\x00") == strings.Join(r.Target.CapabilitySet, "\x00")
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && value != "-" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func allowedDecision(value string) bool {
	return value == DecisionClosed || value == DecisionUnknown || value == DecisionRefuted
}
