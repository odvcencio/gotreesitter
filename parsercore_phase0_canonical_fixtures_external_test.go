//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func init() {
	gotreesitter.SetYAMLSelectedProjectionLanguageForTest(grammars.YamlLanguage())
	gotreesitter.SetDiagnosticParserCoreCanonicalFixtureLoaderForTest(func(id string) (gotreesitter.DiagnosticParserCoreCanonicalFixtureForTest, error) {
		for _, fixture := range benchfixtures.GoFullParseFixtures() {
			if fixture.ID != id {
				continue
			}
			source, err := fixture.Load()
			if err != nil {
				return gotreesitter.DiagnosticParserCoreCanonicalFixtureForTest{}, err
			}
			return gotreesitter.DiagnosticParserCoreCanonicalFixtureForTest{
				ID:                fixture.ID,
				Source:            source,
				SourceSHA256:      fixture.SHA256,
				DeepTreeSHA256:    fixture.DeepTreeSHA256,
				UncompressedBytes: fixture.UncompressedBytes,
			}, nil
		}
		return gotreesitter.DiagnosticParserCoreCanonicalFixtureForTest{}, fmt.Errorf("canonical Go fixture %q not found", id)
	})
	gotreesitter.SetDiagnosticParserCoreCanonicalTreeDigestForTest(func(tree *gotreesitter.Tree, lang *gotreesitter.Language) (string, error) {
		if tree == nil {
			return "", fmt.Errorf("canonical deep-tree digest requires a tree")
		}
		inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
		if err != nil {
			return "", err
		}
		return inspection.SHA256, nil
	})
	gotreesitter.SetDiagnosticParserCoreGoHighlightQueryForTest(func() (string, error) {
		entry := grammars.DetectLanguage("main.go")
		if entry == nil {
			return "", fmt.Errorf("Go grammar registry entry not found")
		}
		return entry.HighlightQuery, nil
	})
}

// TestUnaryProgramArtifactEquivalence accepts a grammar name so fleet checks
// can run one grammar per process; ordinary test runs check Go by default.
func TestUnaryProgramArtifactEquivalence(t *testing.T) {
	name := os.Getenv("GTS_UNARY_LANG")
	if name == "" {
		name = "go"
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("unknown grammar %q", name)
	}
	report, err := gotreesitter.CheckUnaryProgramForTest(entry.Language())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("unary equivalence %s: %v", name, report)
}

func BenchmarkReviewColdSelectedPolicy(b *testing.B) {
	name := os.Getenv("GTS_UNARY_LANG")
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		b.Fatalf("unknown grammar %q", name)
	}
	p := gotreesitter.NewParser(entry.Language())
	if err := gotreesitter.ReviewBuildColdPolicyForTest(p); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := gotreesitter.ReviewBuildColdPolicyForTest(p); err != nil {
			b.Fatal(err)
		}
	}
}
