//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"fmt"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func init() {
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
}
