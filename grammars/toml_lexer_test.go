//go:build !grammar_subset || grammar_subset_toml

package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewTomlTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewTomlTokenSource([]byte("a = 1\n"), lang); err == nil {
		t.Fatal("expected error for language missing toml token symbols")
	}
}

func TestNewTomlTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewTomlTokenSourceOrEOF([]byte("a = 1\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestTomlTokenSourceSkipToByte(t *testing.T) {
	lang := TomlLanguage()
	src := []byte("a = 1\nb = 2\n")
	target := bytes.Index(src, []byte("b"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewTomlTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewTomlTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
	if tok.Text != "b" {
		t.Fatalf("expected token text %q, got %q", "b", tok.Text)
	}
}

func TestParseTomlWithTokenSourceReturnsTree(t *testing.T) {
	lang := TomlLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("a = 1\n")
	ts, err := NewTomlTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewTomlTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
}

func TestParseTomlRealWorldSampleParsesCleanlyWithRegisteredBackend(t *testing.T) {
	const srcText = `# This is a TOML document.

title = "TOML Example"

[owner]
name = "Tom Preston-Werner"
dob = 1979-05-27T07:32:00-08:00 # First class dates

[database]
server = "192.168.1.1"
ports = [ 8001, 8001, 8002 ]
connection_max = 5000
enabled = true

[servers]

  # Indentation (tabs and/or spaces) is allowed but not required
  [servers.alpha]
  ip = "10.0.0.1"
  dc = "eqdc10"

  [servers.beta]
  ip = "10.0.0.2"
  dc = "eqdc10"

[clients]
data = [ ["gamma", "delta"], [1, 2] ]

# Line breaks are OK when inside arrays
hosts = [
  "alpha",
  "omega"
]
`

	entry := DetectLanguage("fixture.toml")
	if entry == nil {
		t.Fatal("failed to detect toml entry")
	}

	src := []byte(srcText)
	lang := entry.Language()
	report := EvaluateParseSupport(*entry, lang)
	if report.Backend != ParseBackendDFA && report.Backend != ParseBackendDFAPartial {
		t.Fatalf("registered TOML backend = %q, want native lexer backend", report.Backend)
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil root")
	}

	root := tree.RootNode()
	rt := tree.ParseRuntime()
	if root.HasError() || root.EndByte() != uint32(len(src)) || rt.StopReason != gotreesitter.ParseStopAccepted || rt.Truncated {
		t.Fatalf("registered TOML parse failed: hasError=%v end=%d len=%d runtime=%s",
			root.HasError(), root.EndByte(), len(src), rt.Summary())
	}
}

func TestParseTomlOffsetDateTimeMatchesDFA(t *testing.T) {
	src := []byte("dob = 1979-05-27T07:32:00-08:00\n")
	lang := TomlLanguage()

	dfaParser := gotreesitter.NewParser(lang)
	dfaTree, err := dfaParser.Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if dfaTree == nil || dfaTree.RootNode() == nil {
		t.Fatal("Parse returned nil root")
	}

	tsParser := gotreesitter.NewParser(lang)
	ts, err := NewTomlTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewTomlTokenSource failed: %v", err)
	}
	tsTree, err := tsParser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	if tsTree == nil || tsTree.RootNode() == nil {
		t.Fatal("ParseWithTokenSource returned nil root")
	}

	dfaRoot := dfaTree.RootNode()
	tsRoot := tsTree.RootNode()
	if tsRoot.HasError() != dfaRoot.HasError() ||
		tsRoot.EndByte() != dfaRoot.EndByte() ||
		tsRoot.ChildCount() != dfaRoot.ChildCount() {
		t.Fatalf("offset-date-time parse diverged from DFA: dfa(hasError=%v end=%d children=%d sexpr=%s) ts(hasError=%v end=%d children=%d sexpr=%s)",
			dfaRoot.HasError(), dfaRoot.EndByte(), dfaRoot.ChildCount(), sexpr(dfaRoot, lang),
			tsRoot.HasError(), tsRoot.EndByte(), tsRoot.ChildCount(), sexpr(tsRoot, lang))
	}
}
