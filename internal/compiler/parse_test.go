package compiler

import (
	"strings"
	"testing"
)

func TestParseMetaRejectsDuplicateSingletonDeclaration(t *testing.T) {
	raw := []byte("gooo semantic_migration_compiler v1\nauthority metacode\nauthority caller\n")
	if _, err := ParseMeta(raw); err == nil || !strings.Contains(err.Error(), `duplicate singleton declaration "authority"`) {
		t.Fatalf("ParseMeta() error = %v, want duplicate singleton declaration", err)
	}
}
