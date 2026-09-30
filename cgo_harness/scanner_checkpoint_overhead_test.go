//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Run one scanner grammar per process, including grammars outside the usual
// parity allowlist. Compare against a fresh locked-C parse on both routes.
func TestQ4ScannerFreshParity(t *testing.T) {
	name := os.Getenv("GTS_Q4_LANGUAGE")
	if name == "" {
		t.Skip("set GTS_Q4_LANGUAGE to one scanner grammar")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("unknown grammar %q", name)
	}
	lang := entry.Language()
	if lang.ExternalScanner == nil {
		t.Fatalf("grammar %q has no scanner", name)
	}
	source := normalizedSource(name, grammars.ParseSmokeSample(name))
	if path := os.Getenv("GTS_Q4_FIXTURE"); path != "" {
		var err error
		source, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	oracle := cParser.Parse(source, nil)
	if oracle == nil {
		t.Fatal("locked C returned no tree")
	}
	defer oracle.Close()
	for _, candidate := range []bool{false, true} {
		t.Run(fmt.Sprintf("candidate=%t", candidate), func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(candidate)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			digest, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("language=%s bytes=%d digest=%s runtime=%s", name, len(source), digest.SHA256, tree.ParseRuntime().Summary())
			assertLockedCTreeExactWithErrors(t, "scanner checkpoint", tree, lang, oracle)
		})
	}
}

// Measure this test executable with /usr/bin/time -v, one engine per process.
// Strict fresh parity for the same fixture is a separate prerequisite.
func TestQ4ScannerRSS(t *testing.T) {
	name, path, engine := os.Getenv("GTS_Q4_LANGUAGE"), os.Getenv("GTS_Q4_FIXTURE"), os.Getenv("GTS_Q4_ENGINE")
	if name == "" || path == "" || engine == "" {
		t.Skip("set GTS_Q4_LANGUAGE, GTS_Q4_FIXTURE, and GTS_Q4_ENGINE")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	switch engine {
	case "Go":
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			t.Fatalf("unknown grammar %q", name)
		}
		parser := gotreesitter.NewParser(entry.Language())
		for i := 0; i < 3; i++ {
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			root := tree.RootNode()
			if root == nil || root.HasError() || root.EndByte() != uint32(len(source)) {
				t.Fatalf("incomplete Go parse: %s", tree.ParseRuntime().Summary())
			}
			t.Logf("iteration=%d source_bytes=%d runtime=%s", i, len(source), tree.ParseRuntime().Summary())
			tree.Release()
		}
	case "C":
		lang, err := ParityCLanguage(name)
		if err != nil {
			t.Fatal(err)
		}
		parser := sitter.NewParser()
		defer parser.Close()
		if err := parser.SetLanguage(lang); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			tree := parser.Parse(source, nil)
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("C returned no root")
			}
			root := tree.RootNode()
			if root.HasError() || root.EndByte() != uint(len(source)) {
				t.Fatal("incomplete C parse")
			}
			tree.Close()
		}
	default:
		t.Fatalf("unknown engine %q", engine)
	}
}

// BenchmarkQ4ScannerFull uses the same parity-checked real corpus in each
// Go-C-C-Go cycle. Select one language with GTS_REAL_CORPUS_BENCH_LANGS.
func BenchmarkQ4ScannerFull(b *testing.B) {
	for _, name := range realCorpusBenchmarkLanguages(b) {
		b.Run(name, func(b *testing.B) {
			cases := prepareRealCorpusBenchmarkCases(b, name)
			verifyRealCorpusBenchmarkFreshParity(b, cases)
			for _, phase := range []string{"Go1", "C1", "C2", "Go2"} {
				b.Run(phase, func(b *testing.B) {
					if strings.HasPrefix(phase, "Go") {
						benchmarkRealCorpusGoParseFull(b, cases)
					} else {
						benchmarkRealCorpusCParseFull(b, cases)
					}
				})
			}
		})
	}
}
