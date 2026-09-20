//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// recoverEOFPublishSweepCases is review finding B2/M4's moved-input list,
// reproduced independently: a 19-input sweep of short malformed sources
// against all 182 default=true grammars in testdata/c_recovery_gate_fleet.json
// found the same five grammars the review lists moving from a one-child
// grammar root to a childless ERROR root once tryPublishCRecoverEOFRoot
// publishes the C-recovery lineage's bare recover_eof root
// (parser_result_root_build.go). The exact input set differs slightly from
// the review's (36 inputs move here against the review's 30), because the
// two sweeps picked different candidate delimiters, but both sweeps name
// the same five grammars.
var recoverEOFPublishSweepCases = map[string][]string{
	"corn":       {"#", "(", ")", ",", ":", ";", "<", ">", "@", "`", "|"},
	"dtd":        {"#", ".", ":", "@", "]", "`", "{", "}"},
	"jsdoc":      {"\"", "#", "'", "(", ")", ",", ".", "0x", ":", ";", "<", ">", "]", "`", "|"},
	"powershell": {"\""},
	"vhdl":       {"`"},
}

func recoverEOFPublishSweepGoLanguage(name string) *gotreesitter.Language {
	switch name {
	case "corn":
		return grammars.CornLanguage()
	case "dtd":
		return grammars.DtdLanguage()
	case "jsdoc":
		return grammars.JsdocLanguage()
	case "powershell":
		return grammars.PowershellLanguage()
	case "vhdl":
		return grammars.VhdlLanguage()
	default:
		return nil
	}
}

// TestParityRecoverEOFPublishSweep is review finding B2/M4's C-oracle
// receipt. For every (grammar, input) pair in recoverEOFPublishSweepCases,
// it parses the input on the pinned tree-sitter C oracle and on the Go
// port, and asserts C also produces a childless ERROR root: the exact shape
// tryPublishCRecoverEOFRoot now publishes. Run one language per container
// (see cgo_harness/docker/run_parity_in_docker.sh --label <lang> -- ... -run
// '^TestParityRecoverEOFPublishSweep$/^<lang>$').
func TestParityRecoverEOFPublishSweep(t *testing.T) {
	for grammar, inputs := range recoverEOFPublishSweepCases {
		t.Run(grammar, func(t *testing.T) {
			cLanguage, err := COracleLanguage(grammar)
			if err != nil {
				t.Fatalf("COracleLanguage(%s): %v", grammar, err)
			}
			goLang := recoverEOFPublishSweepGoLanguage(grammar)
			if goLang == nil {
				t.Fatalf("no Go language mapped for %q", grammar)
			}

			for _, input := range inputs {
				t.Run(inputSubtestName(input), func(t *testing.T) {
					src := []byte(input)

					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLanguage); err != nil {
						t.Fatalf("SetLanguage: %v", err)
					}
					cTree := cParser.Parse(src, nil)
					defer cTree.Close()
					cRoot := cTree.RootNode()

					goParser := gotreesitter.NewParser(goLang)
					goTree, err := goParser.Parse(src)
					if err != nil {
						t.Fatalf("Go Parse: %v", err)
					}
					defer goTree.Release()
					goRoot := goTree.RootNode()

					if goRoot.Type(goLang) != "ERROR" || goRoot.ChildCount() != 0 {
						t.Fatalf("Go root = type %q children %d, want the published childless ERROR root; sexp=%s",
							goRoot.Type(goLang), goRoot.ChildCount(), goRoot.SExpr(goLang))
					}

					if cRoot.Kind() != "ERROR" || cRoot.ChildCount() != 0 {
						t.Errorf(
							"C oracle disagrees for %s %q: C kind=%s hasError=%v children=%d sexp=%s; Go publishes a bare ERROR root here",
							grammar, input, cRoot.Kind(), cRoot.HasError(), cRoot.ChildCount(), cRoot.ToSexp(),
						)
						return
					}
					if !cRoot.HasError() {
						t.Errorf("C oracle ERROR root for %s %q reports HasError()=false", grammar, input)
					}
				})
			}
		})
	}
}

// inputSubtestName maps a short malformed input to a stable, readable
// subtest name (t.Run sanitizes slashes and spaces, which several inputs
// here are made of).
func inputSubtestName(input string) string {
	names := map[string]string{
		"{": "brace_open", "}": "brace_close",
		"(": "paren_open", ")": "paren_close",
		"[": "bracket_open", "]": "bracket_close",
		"<": "angle_open", ">": "angle_close",
		"\"": "double_quote", "'": "single_quote", "`": "backtick",
		",": "comma", ";": "semicolon", ":": "colon", ".": "dot",
		"0x": "hex_prefix", "@": "at", "#": "hash", "|": "pipe",
	}
	if name, ok := names[input]; ok {
		return name
	}
	return input
}
