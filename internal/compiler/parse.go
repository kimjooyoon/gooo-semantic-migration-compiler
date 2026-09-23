package compiler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func LoadMeta(path string) (MetaContract, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return MetaContract{}, nil, err
	}
	meta, err := ParseMeta(raw)
	if err != nil {
		return MetaContract{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return meta, raw, nil
}

func ParseMeta(raw []byte) (MetaContract, error) {
	meta := MetaContract{AuthorityPolicy: map[string]string{}, SourcePolicy: map[string]string{}}
	for lineNumber, rawLine := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(stripComment(rawLine))
		if line == "" {
			continue
		}
		tokens, err := tokenize(line)
		if err != nil {
			return MetaContract{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(tokens) == 0 {
			continue
		}
		if tokens[0] == "gooo" {
			if len(tokens) != 3 || tokens[1] != "semantic_migration_compiler" || tokens[2] != "v1" {
				return MetaContract{}, fmt.Errorf("line %d: invalid meta header", lineNumber+1)
			}
			meta.Schema = MetaSchema
			continue
		}
		if tokens[0] == "authority" {
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid authority", lineNumber+1)
			}
			meta.Authority = tokens[1]
			continue
		}
		if tokens[0] == "precedence" || tokens[0] == "unknown_fields" {
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid %s declaration", lineNumber+1, tokens[0])
			}
			if tokens[0] == "precedence" {
				meta.Precedence = strings.Split(tokens[1], ">")
			} else {
				meta.UnknownFields = strings.Split(tokens[1], ",")
			}
			continue
		}
		values, err := parseKeyValues(tokens[1:])
		if err != nil {
			return MetaContract{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		switch tokens[0] {
		case "dialects":
			meta.Dialects = splitList(values["versions"])
		case "contract":
			meta.Contracts = append(meta.Contracts, ContractDecl{Version: values["version"], ID: values["id"], Schema: values["schema"], Concepts: splitList(values["concepts"])})
		case "mapping":
			meta.Mappings = append(meta.Mappings, MappingDecl{
				Ordinal: parseInt(values, "ordinal"), ID: values["id"], Kind: values["kind"], From: values["from"], To: values["to"],
				Complete: parseBool(values, "complete"), Compatible: parseBool(values, "compatible"), Reversible: parseBool(values, "reversible"),
				ScopeFixed: parseBool(values, "scope_fixed"), InvariantSafe: parseBool(values, "invariant_safe"), TargetEffects: values["target_effects"], Reason: values["reason"],
			})
		case "invariant":
			meta.Invariants = append(meta.Invariants, InvariantDecl{Ordinal: parseInt(values, "ordinal"), ID: values["id"], Predicate: values["predicate"], Preserve: parseBool(values, "preserve")})
		case "activity":
			meta.Activities = append(meta.Activities, ActivityDecl{Ordinal: parseInt(values, "ordinal"), ID: values["id"], Semantic: values["semantic"]})
		case "denominator":
			meta.Denominator = DenominatorDecl{ID: values["id"], Cases: parseInt(values, "cases"), Unit: values["unit"]}
		case "authority_policy":
			meta.AuthorityPolicy = values
		case "source_policy":
			meta.SourcePolicy = values
		case "case":
			meta.Cases = append(meta.Cases, CaseDecl{Ordinal: parseInt(values, "ordinal"), ID: values["id"], Fixture: values["fixture"], Source: values["source"], Plan: values["plan"], Expected: values["expected"], TargetTerminal: emptyDash(values["target_terminal"])})
		default:
			return MetaContract{}, fmt.Errorf("line %d: unsupported declaration %q", lineNumber+1, tokens[0])
		}
	}
	if err := meta.Validate(); err != nil {
		return MetaContract{}, err
	}
	return meta, nil
}

func LoadFixture(path string, meta MetaContract) (CaseFixture, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CaseFixture{}, nil, err
	}
	var fixture CaseFixture
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		return CaseFixture{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	decl, ok := meta.Case(fixture.CaseID)
	if !ok || fixture.Schema != CaseSchema || fixture.CaseID == "" || fixture.Description == "" || fixture.ReplayRuns <= 0 {
		return CaseFixture{}, nil, fmt.Errorf("fixture %s is not a valid fixed case", path)
	}
	if decl.Fixture != strings.TrimPrefix(path, "./") && !strings.HasSuffix(path, decl.Fixture) {
		return CaseFixture{}, nil, fmt.Errorf("fixture %s does not match meta declaration", path)
	}
	return fixture, raw, nil
}

func LoadProgram(path string) (Program, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Program{}, nil, err
	}
	program, err := ParseProgram(raw)
	if err != nil {
		return Program{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return program, raw, nil
}

func ParseProgram(raw []byte) (Program, error) {
	program := Program{Schema: ProgramSchema, Nodes: []Node{}, Edges: []Edge{}}
	headerSeen := false
	programSeen := false
	for lineNumber, rawLine := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(stripComment(rawLine))
		if line == "" {
			continue
		}
		tokens, err := tokenize(line)
		if err != nil {
			return Program{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(tokens) == 0 {
			continue
		}
		if tokens[0] == "gooo" {
			if len(tokens) != 3 || tokens[1] != "program" || tokens[2] != "v1" {
				return Program{}, fmt.Errorf("line %d: invalid program header", lineNumber+1)
			}
			headerSeen = true
			continue
		}
		values, err := parseKeyValues(tokens[1:])
		if err != nil {
			return Program{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		switch tokens[0] {
		case "program":
			if programSeen {
				return Program{}, fmt.Errorf("line %d: duplicate program declaration", lineNumber+1)
			}
			program.ID, program.Dialect, program.Entry = values["id"], values["dialect"], values["entry"]
			programSeen = true
		case "node":
			program.Nodes = append(program.Nodes, Node{ID: values["id"], Symbol: values["symbol"], Capabilities: splitList(values["capabilities"]), Effects: splitListPreserveOrder(values["effects"])})
		case "edge":
			program.Edges = append(program.Edges, Edge{ID: values["id"], From: values["from"], To: values["to"]})
		case "terminal":
			program.TerminalReason = values["reason"]
		default:
			return Program{}, fmt.Errorf("line %d: unsupported program declaration %q", lineNumber+1, tokens[0])
		}
	}
	if !headerSeen || !programSeen {
		return Program{}, errors.New("program header or declaration is missing")
	}
	program = program.canonical()
	if err := program.Validate(); err != nil {
		return Program{}, err
	}
	return program, nil
}

func parseKeyValues(tokens []string) (map[string]string, error) {
	values := map[string]string{}
	for _, token := range tokens {
		key, value, ok := strings.Cut(token, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("invalid key/value token %q", token)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		values[key] = value
	}
	return values, nil
}

func tokenize(line string) ([]string, error) {
	var result []string
	var current strings.Builder
	inQuote := false
	escaped := false
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && inQuote {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if (r == ' ' || r == '\t') && !inQuote {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if escaped || inQuote {
		return nil, errors.New("unterminated quoted value")
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result, nil
}

func stripComment(value string) string {
	inQuote := false
	for index, r := range value {
		if r == '"' {
			inQuote = !inQuote
		}
		if !inQuote && r == '#' {
			return value[:index]
		}
	}
	return value
}

func splitList(value string) []string {
	if value == "" || value == "-" {
		return []string{}
	}
	return sortedUnique(strings.FieldsFunc(value, func(r rune) bool { return r == '|' || r == ',' }))
}

func splitListPreserveOrder(value string) []string {
	if value == "" || value == "-" {
		return []string{}
	}
	seen := map[string]bool{}
	result := []string{}
	for _, item := range strings.FieldsFunc(value, func(r rune) bool { return r == '|' || r == ',' }) {
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func parseInt(values map[string]string, key string) int {
	value, err := strconv.Atoi(values[key])
	if err != nil {
		return 0
	}
	return value
}

func parseBool(values map[string]string, key string) bool { return values[key] == "true" }

func emptyDash(value string) string {
	if value == "-" {
		return ""
	}
	return value
}
