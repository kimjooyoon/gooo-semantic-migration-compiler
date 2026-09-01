package compiler

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type MigrationResult struct {
	Target         Program
	Operations     []MigrationOperation
	OriginMap      []OriginPair
	Unknowns       []UnknownRecord
	RefutedReasons []string
}

func CompileMigration(meta MetaContract, source Program, caseDecl CaseDecl) (MigrationResult, error) {
	if err := source.Validate(); err != nil {
		return MigrationResult{}, err
	}
	if source.Dialect != meta.Dialects[0] {
		return MigrationResult{}, fmt.Errorf("source dialect %s is not the declared old dialect", source.Dialect)
	}
	if _, ok := meta.Contract(meta.Dialects[1]); !ok {
		return MigrationResult{}, errors.New("declared new semantic contract is missing")
	}
	target := cloneProgram(source)
	target.Dialect = meta.Dialects[1]
	origins := map[string][]string{}
	for _, node := range source.Nodes {
		origins[node.ID] = []string{node.ID}
	}
	result := MigrationResult{Target: target, Operations: []MigrationOperation{}, OriginMap: []OriginPair{}, Unknowns: []UnknownRecord{}, RefutedReasons: []string{}}
	apply := func(mappingID string) {
		mapping, ok := meta.Mapping(mappingID)
		if !ok {
			result.Operations = append(result.Operations, MigrationOperation{Ordinal: len(result.Operations) + 1, MappingID: mappingID, Status: "UNKNOWN", Reason: "mapping declaration is missing"})
			result.Unknowns = append(result.Unknowns, UnknownRecord{"COMPILE_MIGRATION_PLAN", "RESOLVE_MAPPING", "mapping declaration is missing", "MAPPING_MISSING", "declare_complete_mapping", []string{mappingID}})
			return
		}
		operation := MigrationOperation{Ordinal: len(result.Operations) + 1, MappingID: mapping.ID, Kind: mapping.Kind, From: mapping.From, To: splitListPreserveOrder(mapping.To), Status: "DECLARED"}
		if !mapping.Complete || !mapping.ScopeFixed {
			operation.Status = "UNKNOWN"
			operation.Reason = mapping.Reason
			if operation.Reason == "" {
				operation.Reason = "mapping completeness or scope is not fixed"
			}
			unknownClass := "MAPPING_INCOMPLETE"
			if !mapping.ScopeFixed {
				unknownClass = "SCOPE_UNFIXED"
			}
			result.Operations = append(result.Operations, operation)
			result.Unknowns = append(result.Unknowns, UnknownRecord{"COMPILE_MIGRATION_PLAN", "RESOLVE_MAPPING", operation.Reason, unknownClass, "supply_complete_mapping", []string{mapping.ID}})
			return
		}
		if !mapping.Compatible {
			operation.Status = "REFUTED"
			operation.Reason = mapping.Reason
			if operation.Reason == "" {
				operation.Reason = "mapping is declared incompatible"
			}
			result.Operations = append(result.Operations, operation)
			result.RefutedReasons = append(result.RefutedReasons, strings.ToUpper(mapping.Kind)+"_INFORMATION_LOSS:"+operation.Reason)
		} else {
			operation.Status = "APPLIED"
			result.Operations = append(result.Operations, operation)
		}
		if err := applyMapping(&result.Target, mapping, origins); err != nil {
			result.Operations[len(result.Operations)-1].Status = "REFUTED"
			result.Operations[len(result.Operations)-1].Reason = err.Error()
			result.RefutedReasons = append(result.RefutedReasons, "MAPPING_APPLICATION_FAILURE:"+err.Error())
			return
		}
		if !mapping.InvariantSafe {
			reason := mapping.Reason
			if reason == "" {
				reason = "mapping is declared unsafe for preservation invariants"
			}
			result.RefutedReasons = append(result.RefutedReasons, "INVARIANT_DECLARED_UNSAFE:"+reason)
		}
	}

	switch caseDecl.Plan {
	case "rename-load":
		apply("rename-load")
	case "split-process":
		apply("split-process")
	case "rename-split":
		apply("rename-load")
		apply("split-process")
	case "missing-mapping":
		apply("missing-declaration")
	case "ambiguous-split":
		apply("ambiguous-split")
	case "unbounded-merge":
		apply("unbounded-merge")
	case "remove-legacy-cache":
		apply("remove-legacy-cache")
	case "break-effect":
		apply("break-effect")
	case "counterexample-terminal":
		apply("counterexample-terminal")
		if caseDecl.TargetTerminal != "" {
			result.Target.TerminalReason = caseDecl.TargetTerminal
		}
	default:
		result.Unknowns = append(result.Unknowns, UnknownRecord{"COMPILE_MIGRATION_PLAN", "SELECT_PLAN", "case plan is not declared by the meta source", "PLAN_MISSING", "declare_case_plan", []string{caseDecl.Plan}})
	}
	result.Target = result.Target.canonical()
	result.OriginMap = originPairs(origins)
	return result, nil
}

func applyMapping(program *Program, mapping MappingDecl, origins map[string][]string) error {
	from := splitListPreserveOrder(mapping.From)
	to := splitListPreserveOrder(mapping.To)
	switch mapping.Kind {
	case "rename":
		if len(from) != 1 || len(to) != 1 {
			return errors.New("rename mapping must have one source and one target")
		}
		if err := renameNode(program, from[0], to[0], mapping.TargetEffects); err != nil {
			return err
		}
		for sourceID, targets := range origins {
			for i, targetID := range targets {
				if targetID == from[0] {
					origins[sourceID][i] = to[0]
				}
			}
		}
	case "split":
		if len(from) != 1 || len(to) < 2 {
			return errors.New("split mapping must have one source and at least two targets")
		}
		if err := splitNode(program, from[0], to); err != nil {
			return err
		}
		for sourceID, targets := range origins {
			for i, targetID := range targets {
				if targetID == from[0] {
					origins[sourceID] = append(append([]string{}, targets[:i]...), append(append([]string{}, to...), targets[i+1:]...)...)
					break
				}
			}
		}
	case "merge":
		if len(from) < 2 || len(to) != 1 {
			return errors.New("merge mapping must have at least two sources and one target")
		}
		if err := mergeNodes(program, from, to[0]); err != nil {
			return err
		}
		for sourceID, targets := range origins {
			for i, targetID := range targets {
				for _, sourceTarget := range from {
					if targetID == sourceTarget {
						origins[sourceID][i] = to[0]
					}
				}
			}
		}
	case "remove":
		if len(from) != 1 {
			return errors.New("remove mapping must have one source")
		}
		if err := removeNode(program, from[0]); err != nil {
			return err
		}
		for sourceID, targets := range origins {
			filtered := []string{}
			for _, targetID := range targets {
				if targetID != from[0] {
					filtered = append(filtered, targetID)
				}
			}
			origins[sourceID] = filtered
		}
	case "terminal":
		return nil
	default:
		return fmt.Errorf("unsupported declared mapping kind %s", mapping.Kind)
	}
	return nil
}

func renameNode(program *Program, from, to, targetEffects string) error {
	index := -1
	for i, node := range program.Nodes {
		if node.ID == from {
			index = i
		}
		if node.ID == to {
			return fmt.Errorf("rename target %s already exists", to)
		}
	}
	if index < 0 {
		return fmt.Errorf("rename source %s does not exist", from)
	}
	program.Nodes[index].ID = to
	program.Nodes[index].Symbol = to
	if targetEffects != "" && targetEffects != "-" {
		program.Nodes[index].Effects = splitListPreserveOrder(targetEffects)
	}
	if program.Entry == from {
		program.Entry = to
	}
	for i := range program.Edges {
		if program.Edges[i].From == from {
			program.Edges[i].From = to
		}
		if program.Edges[i].To == from {
			program.Edges[i].To = to
		}
	}
	return nil
}

func splitNode(program *Program, from string, targets []string) error {
	index := -1
	var original Node
	for i, node := range program.Nodes {
		if node.ID == from {
			index = i
			original = node
		}
	}
	if index < 0 {
		return fmt.Errorf("split source %s does not exist", from)
	}
	for _, target := range targets {
		for _, node := range program.Nodes {
			if node.ID == target && target != from {
				return fmt.Errorf("split target %s already exists", target)
			}
		}
	}
	parts := make([]Node, 0, len(targets))
	for i, target := range targets {
		part := Node{ID: target, Symbol: target, Capabilities: append([]string{}, original.Capabilities...), Effects: []string{}}
		if i == len(targets)-1 {
			part.Effects = append(part.Effects, original.Effects...)
		}
		parts = append(parts, part)
	}
	for i := range program.Edges {
		if program.Edges[i].From == from {
			program.Edges[i].From = targets[len(targets)-1]
		}
		if program.Edges[i].To == from {
			program.Edges[i].To = targets[0]
		}
	}
	if program.Entry == from {
		program.Entry = targets[0]
	}
	program.Nodes = append(append(append([]Node{}, program.Nodes[:index]...), parts...), program.Nodes[index+1:]...)
	for i := 0; i < len(targets)-1; i++ {
		program.Edges = append(program.Edges, Edge{From: targets[i], To: targets[i+1]})
	}
	return nil
}

func mergeNodes(program *Program, sources []string, target string) error {
	if _, exists := nodeByID(program, target); exists {
		return fmt.Errorf("merge target %s already exists", target)
	}
	combined := Node{ID: target, Symbol: target, Capabilities: []string{}, Effects: []string{}}
	for _, source := range sources {
		node, exists := nodeByID(program, source)
		if !exists {
			return fmt.Errorf("merge source %s does not exist", source)
		}
		combined.Capabilities = append(combined.Capabilities, node.Capabilities...)
		combined.Effects = append(combined.Effects, node.Effects...)
	}
	for i := range program.Edges {
		for _, source := range sources {
			if program.Edges[i].From == source {
				program.Edges[i].From = target
			}
			if program.Edges[i].To == source {
				program.Edges[i].To = target
			}
		}
	}
	if contains(sources, program.Entry) {
		program.Entry = target
	}
	remaining := []Node{}
	for _, node := range program.Nodes {
		if !contains(sources, node.ID) {
			remaining = append(remaining, node)
		}
	}
	program.Nodes = append(remaining, combined)
	return nil
}

func removeNode(program *Program, source string) error {
	if _, exists := nodeByID(program, source); !exists {
		return fmt.Errorf("remove source %s does not exist", source)
	}
	predecessors := []string{}
	successors := []string{}
	remainingEdges := []Edge{}
	for _, edge := range program.Edges {
		if edge.To == source {
			predecessors = append(predecessors, edge.From)
			continue
		}
		if edge.From == source {
			successors = append(successors, edge.To)
			continue
		}
		remainingEdges = append(remainingEdges, edge)
	}
	for _, from := range predecessors {
		for _, to := range successors {
			if from != to {
				remainingEdges = append(remainingEdges, Edge{From: from, To: to})
			}
		}
	}
	remainingNodes := []Node{}
	for _, node := range program.Nodes {
		if node.ID != source {
			remainingNodes = append(remainingNodes, node)
		}
	}
	program.Nodes, program.Edges = remainingNodes, remainingEdges
	if program.Entry == source {
		return errors.New("cannot remove the entry node")
	}
	return nil
}

func nodeByID(program *Program, id string) (Node, bool) {
	for _, node := range program.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

func originPairs(origins map[string][]string) []OriginPair {
	keys := make([]string, 0, len(origins))
	for key := range origins {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]OriginPair, 0, len(keys))
	for _, key := range keys {
		result = append(result, OriginPair{From: key, To: sortedUnique(origins[key])})
	}
	return result
}

func cloneProgram(source Program) Program {
	result := source
	result.Nodes = make([]Node, len(source.Nodes))
	for i, node := range source.Nodes {
		result.Nodes[i] = Node{ID: node.ID, Symbol: node.Symbol, Capabilities: append([]string{}, node.Capabilities...), Effects: append([]string{}, node.Effects...)}
	}
	result.Edges = append([]Edge{}, source.Edges...)
	return result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
