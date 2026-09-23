package compiler

import (
	"strings"
	"testing"
)

func TestParseProgramRejectsDuplicateTerminalDeclarations(t *testing.T) {
	_, err := ParseProgram([]byte("gooo program v1\nprogram id=p dialect=v1 entry=entry\nterminal reason=first\nterminal reason=second\n"))
	if err == nil || !strings.Contains(err.Error(), "duplicate terminal declaration") {
		t.Fatalf("ParseProgram duplicate terminal error = %v", err)
	}
}
