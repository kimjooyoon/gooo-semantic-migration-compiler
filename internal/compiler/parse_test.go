package compiler

import (
	"strings"
	"testing"
)

func TestParseMetaRejectsDuplicateHeaders(t *testing.T) {
	_, err := ParseMeta([]byte("gooo semantic_migration_compiler v1\n" +
		"gooo semantic_migration_compiler v1\n"))
	if err == nil || !strings.Contains(err.Error(), "duplicate meta header") {
		t.Fatalf("ParseMeta error=%v want duplicate meta header", err)
	}
}

func TestParseProgramRejectsDuplicateHeaders(t *testing.T) {
	_, err := ParseProgram([]byte("gooo program v1\n" +
		"gooo program v1\n"))
	if err == nil || !strings.Contains(err.Error(), "duplicate program header") {
		t.Fatalf("ParseProgram error=%v want duplicate program header", err)
	}
}
