package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type issue454SessionFixture struct {
	name   string
	lang   *gts.Language
	source []byte
}

// The report describes the session, but its attached program is unavailable.
func issue454SessionFixtures() []issue454SessionFixture {
	var java, goSource, objc, proto, zig strings.Builder
	java.WriteString("class Session {\n")
	goSource.WriteString("package session\n\n")
	proto.WriteString("syntax = \"proto3\";\n")
	zig.WriteString("const std = @import(\"std\");\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&java, "static int fn%d(int x) { return x + %d; }\n", i, i)
		fmt.Fprintf(&goSource, "func fn%d(x int) int { return x + %d }\n", i, i)
		fmt.Fprintf(&objc, "int fn%d(int x) { return x + %d; }\n", i, i)
		fmt.Fprintf(&proto, "message Item%d { string name = 1; int32 count = 2; }\n", i)
		fmt.Fprintf(&zig, "pub fn fn%d(x: i32) i32 { return x + %d; }\n", i, i)
	}
	java.WriteString("}\n")
	return []issue454SessionFixture{
		{"toml", grammars.TomlLanguage(), issue454Toml()},
		{"javascript", grammars.JavascriptLanguage(), issue454JS()},
		{"tsx", grammars.TsxLanguage(), issue454JS()},
		{"java", grammars.JavaLanguage(), []byte(java.String())},
		{"typescript", grammars.TypescriptLanguage(), issue454JS()},
		{"diff", grammars.DiffLanguage(), issue454Diff()},
		{"go", grammars.GoLanguage(), []byte(goSource.String())},
		{"css", grammars.CssLanguage(), issue454Less()},
		{"scss", grammars.ScssLanguage(), issue454Less()},
		{"less", grammars.LessLanguage(), issue454Less()},
		{"objc", grammars.ObjcLanguage(), []byte(objc.String())},
		{"proto", grammars.ProtoLanguage(), []byte(proto.String())},
		{"zig", grammars.ZigLanguage(), []byte(zig.String())},
	}
}

// TestIssue454ReconstructedSessionMatrix covers the reported 13 grammars.
func TestIssue454ReconstructedSessionMatrix(t *testing.T) {
	fixtures := issue454SessionFixtures()
	ledger := readInvariantLedger(t)
	if len(fixtures) != 13 {
		t.Fatalf("fixture count = %d, want 13", len(fixtures))
	}
	if len(ledger.Sessions) != len(fixtures) {
		t.Fatalf("session ledger has %d entries, want %d", len(ledger.Sessions), len(fixtures))
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(fixture.lang)
					parser.SetAdmissionCandidateRoute(compact)
					old, err := parser.Parse(fixture.source)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { old.Release() }()
					seed := uint32(4242)
					steps := 0
					mismatches := 0
					for cycle := 0; cycle < 36; cycle++ {
						seed = seed*1664525 + 1013904223
						at := int(seed % uint32(len(fixture.source)))
						character := []byte{'"', '/', '}'}[cycle%3]
						broken := append(append([]byte{}, fixture.source[:at]...), append([]byte{character}, fixture.source[at:]...)...)
						for step, after := range [][]byte{broken, fixture.source} {
							steps++
							before := fixture.source
							if step == 1 {
								before = broken
							}
							next, mismatch := invariantLedgerStep(t, parser, fixture.lang, old, before, after)
							if mismatch {
								mismatches++
							}
							old.Release()
							old = next
						}
					}
					if steps != 72 {
						t.Fatalf("session steps = %d, want 72", steps)
					}
					checkInvariantLedger(t, ledger.Sessions, fixture.name, compact, steps, mismatches)
				})
			}
		})
	}
}
