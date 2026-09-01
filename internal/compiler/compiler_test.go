package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetaDeclaresFixedMigrationCorpus(t *testing.T) {
	root := filepath.Join("..", "..")
	meta, _, err := LoadMeta(filepath.Join(root, ".gooo", "migration-compiler.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Denominator.Cases != 9 || len(meta.Cases) != 9 {
		t.Fatalf("fixed denominator = %d/%d, want 9/9", meta.Denominator.Cases, len(meta.Cases))
	}
	if _, ok := meta.Mapping("rename-load"); !ok {
		t.Fatal("rename mapping is not declared")
	}
	if _, ok := meta.Mapping("split-process"); !ok {
		t.Fatal("split mapping is not declared")
	}
	if _, ok := meta.Mapping("remove-legacy-cache"); !ok {
		t.Fatal("incompatible removal is not declared")
	}
}

func TestConformanceOutcomesAndUnknownTuple(t *testing.T) {
	root := filepath.Join("..", "..")
	meta, raw, err := LoadMeta(filepath.Join(root, ".gooo", "migration-compiler.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	index, err := RunConformance(meta, raw, root, out)
	if err != nil {
		t.Fatal(err)
	}
	if index.Closed != 3 || index.Unknown != 3 || index.Refuted != 3 {
		t.Fatalf("outcomes = %d/%d/%d, want 3/3/3", index.Closed, index.Unknown, index.Refuted)
	}
	for _, item := range index.Cases {
		if item.Observed != DecisionUnknown {
			continue
		}
		rawReport, readErr := os.ReadFile(filepath.Join(out, "cases", item.CaseID, "case-report.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(rawReport) == 0 {
			t.Fatal("unknown case report is empty")
		}
	}
}

func TestOutputMustBeOutsideInputRepository(t *testing.T) {
	if err := EnsureOutputOutsideRoot("/tmp/repository", "/tmp/repository/generated"); err == nil {
		t.Fatal("repository-owned output was accepted")
	}
}
