//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestIssue454IncrementalFreshCOracle checks each fresh source against the locked C parser.
func TestIssue454IncrementalFreshCOracle(t *testing.T) {
	var js, diff, less, toml strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&js, "function fn%d(a, b) {\n\tvar x%d = a + b;\n\treturn x%d;\n}\n\n", i, i, i)
	}
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&diff, "diff --git a/f%d.txt b/f%d.txt\n--- a/f%d.txt\n+++ b/f%d.txt\n@@ -1,2 +1,2 @@\n-old\n+new\n", i, i, i, i)
		fmt.Fprintf(&less, ".rule%d {\n  padding: 10px;\n  color: red;\n}\n", i)
	}
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&toml, "[section%d]\nx0 = %d\nname = \"f%d\"\nenabled = true\n\n", i, i, i)
	}
	fixtures := []struct {
		name   string
		lang   *gts.Language
		source string
		marker string
		insert string
	}{
		{"javascript", grammars.JavascriptLanguage(), js.String(), "x0", "\""},
		{"diff", grammars.DiffLanguage(), diff.String(), "--- a/f76.txt", "\""},
		{"less", grammars.LessLanguage(), less.String(), "padding:", "/"},
		{"toml", grammars.TomlLanguage(), toml.String(), "x0 = 0", "\""},
	}
	for _, tc := range fixtures {
		if selected := os.Getenv("GTS_ISSUE454_LANG"); selected != "" && selected != tc.name {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			cLang, err := ParityCLanguage(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			offset := strings.Index(tc.source, tc.marker)
			if offset < 0 {
				t.Fatalf("fixture marker %q is missing", tc.marker)
			}
			if tc.name == "diff" {
				offset += 2
			} else if tc.name == "less" {
				offset += len(tc.marker)
			}
			sources := []string{tc.source, tc.source[:offset] + tc.insert + tc.source[offset:]}
			if tc.name == "javascript" {
				sources = append(sources, tc.source)
			} else if tc.name == "diff" {
				sources[0], sources[1] = sources[1], sources[0]
			} else if tc.name == "toml" {
				sources = issue454TomlOracleSources(tc.source)
			}
			for step, source := range sources {
				goTree, err := gts.NewParser(tc.lang).Parse([]byte(source))
				if err != nil {
					t.Fatal(err)
				}
				cTree := compactT3ParseC(t, cLang, []byte(source))
				if diff := FirstDivergenceDumpV1(goTree.RootNode(), tc.lang, cTree.RootNode()); diff != nil {
					t.Errorf("fresh Go/C mismatch at step=%d: %+v", step, diff)
				} else if err := firstLockedCTreeFlagDivergence(goTree.RootNode(), tc.lang, cTree.RootNode(), "/"); err != nil {
					t.Errorf("fresh Go/C flags at step=%d: %v", step, err)
				} else {
					inspection, err := benchfixtures.InspectGoTree(goTree.RootNode(), tc.lang)
					if err != nil {
						t.Fatal(err)
					}
					digest, err := COracleDeepDigest(cTree)
					if err != nil {
						t.Fatal(err)
					}
					if inspection.SHA256 != digest {
						t.Errorf("fresh Go/C digest at step=%d: Go=%s C=%s", step, inspection.SHA256, digest)
					}
				}
				cTree.Close()
				goTree.Release()
			}
		})
	}
}

// The reporter's attached TOML script is unavailable in this workspace.
// These sources match the reconstructed 72-step regression test.
func issue454TomlOracleSources(source string) []string {
	sources := []string{source}
	appendStep := func(next string) {
		source = next
		sources = append(sources, source)
	}
	baseValue := "0"
	var lastBlock string
	for cycle := 0; cycle < 12; cycle++ {
		if cycle%2 == 0 {
			lastBlock = fmt.Sprintf("[session%d]\nvalue = %d\n", cycle, cycle)
			appendStep(source + lastBlock)
		} else {
			appendStep(strings.Replace(source, lastBlock, "", 1))
		}
		clean := source
		marker := "x0 = " + baseValue
		at := strings.Index(source, marker)
		switch cycle % 3 {
		case 0:
			appendStep(source[:at] + "\"" + source[at:])
		case 1:
			appendStep(source[:at] + "}" + source[at:])
		case 2:
			equals := at + len("x0 ")
			appendStep(source[:equals] + source[equals+1:])
		}
		appendStep(clean)
		nextValue := "1"
		if baseValue == "1" {
			nextValue = "0"
		}
		appendStep(strings.Replace(source, marker, "x0 = "+nextValue, 1))
		baseValue = nextValue
		appendStep(source + fmt.Sprintf("half%d = ", cycle))
		appendStep(source + "\"done\"\n")
	}
	return sources
}

func TestIssue454JavaWholeDocumentRecoveryCOracle(t *testing.T) {
	var source strings.Builder
	source.WriteString("class Session {\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&source, "static int fn%d(int x) { return x + %d; }\n", i, i)
	}
	source.WriteString("}\n")
	clean := source.String()
	const offset = 632
	if len(clean) <= offset {
		t.Fatal("Java fixture is shorter than the reconstructed edit")
	}
	broken := []byte(clean[:offset] + "\"" + clean[offset:])
	goLang := grammars.JavaLanguage()
	goTree, err := gts.NewParser(goLang).Parse(broken)
	if err != nil {
		t.Fatal(err)
	}
	defer goTree.Release()
	cLang, err := ParityCLanguage("java")
	if err != nil {
		t.Fatal(err)
	}
	cTree := compactT3ParseC(t, cLang, broken)
	defer cTree.Close()
	if diff := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode()); diff != nil {
		t.Fatalf("fresh Java recovery differs from C: %+v", diff)
	}
	if err := firstLockedCTreeFlagDivergence(goTree.RootNode(), goLang, cTree.RootNode(), "/"); err != nil {
		t.Fatalf("fresh Java flags differ from C: %v", err)
	}
}
