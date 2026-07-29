package gotreesitter

import "testing"

func TestActiveParseStopCheckReusesBoundMethod(t *testing.T) {
	parser := &Parser{parseBudgetDepth: 1}
	check := parser.activeParseStopCheck()
	if check == nil {
		t.Fatal("active parse stop check is nil")
	}

	parser.parseStoppedReason = ParseStopTimeout
	if got := check(); got != ParseStopTimeout {
		t.Fatalf("cached stop check returned %q, want %q", got, ParseStopTimeout)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if parser.activeParseStopCheck() == nil {
			t.Fatal("cached active parse stop check is nil")
		}
	})
	if allocs != 0 {
		t.Fatalf("cached active parse stop check allocated %.2f objects per call", allocs)
	}
}

func TestNewParserBindsActiveParseStopCheck(t *testing.T) {
	parser := NewParser(nil)
	if parser.activeParseStopCheckFn == nil {
		t.Fatal("new parser did not bind its active parse stop check")
	}
}
