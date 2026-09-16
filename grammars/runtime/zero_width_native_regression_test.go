//go:build !grammar_subset

package grammarruntime

import (
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// typstHistoricalZeroWidthCommaSource minimizes the pinned
// docs/components/section.typ corpus file.
const typstHistoricalZeroWidthCommaSource = `#let html-section(
  nav-buttons: none,
  kind: none,
          icon(16, "hamburger-dark", "Open navigation"),
      })
`

const haskellNestedUpdateSource = `module Main where
f x = y
  where
    y = x
`

const typstDeepGroupSource = `#f(g(h(1,)),)`

func TestHaskellUpdateControlTokenIsZeroWidth(t *testing.T) {
	source := []byte("\nmodule Main where\n")
	lexer := newZeroWidthRetirementLexer(source, 0, 0, 0)
	validSymbols := make([]bool, len(hsSymMap))
	validSymbols[hsUPDATE] = true
	scanner := HaskellExternalScanner{}
	payload := scanner.Create()
	defer scanner.Destroy(payload)

	if !scanner.Scan(payload, lexer, validSymbols) {
		t.Fatal("Haskell scanner rejected the UPDATE control token")
	}
	token, ok := zeroWidthRetirementLexerToken(lexer)
	if !ok {
		t.Fatal("Haskell scanner accepted UPDATE without a token")
	}
	if got, want := token.Symbol, hsSymMap[hsUPDATE]; got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if token.StartByte != 0 || token.EndByte != 0 {
		t.Fatalf("token span = %d..%d, want 0..0", token.StartByte, token.EndByte)
	}
	language := HaskellLanguage()
	symbol := int(token.Symbol)
	if symbol >= len(language.SymbolNames) || symbol >= len(language.SymbolMetadata) {
		t.Fatalf("UPDATE symbol index = %d, outside Haskell metadata", symbol)
	}
	if language.SymbolNames[symbol] != "_token1" {
		t.Fatalf("UPDATE symbol name = %q, want _token1", language.SymbolNames[symbol])
	}
	if language.SymbolMetadata[symbol].Visible || language.SymbolMetadata[symbol].Named {
		t.Fatal("UPDATE must remain hidden and unnamed")
	}
}

func TestZeroWidthArtifactsNeedNoResultCompatibility(t *testing.T) {
	for _, test := range []struct {
		name      string
		language  *gotreesitter.Language
		source    []byte
		wantClean bool
	}{
		{
			name:      "haskell_update",
			language:  HaskellLanguage(),
			source:    []byte("\nmodule Main where\n"),
			wantClean: true,
		},
		{
			name:      "haskell_nested_update",
			language:  HaskellLanguage(),
			source:    []byte(haskellNestedUpdateSource),
			wantClean: true,
		},
		{
			name:     "typst_repetition_skip",
			language: TypstLanguage(),
			source:   []byte(typstHistoricalZeroWidthCommaSource),
		},
		{
			name:      "typst_deep_group",
			language:  TypstLanguage(),
			source:    []byte(typstDeepGroupSource),
			wantClean: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(test.language).
				ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			assertNoZeroWidthArtifact(t, "production", tree, test.language)
			if test.wantClean && tree.RootNode().HasError() {
				t.Fatalf("production root has an error: %s", tree.RootNode().SExpr(test.language))
			}
			runtime := tree.ParseRuntime()
			if runtime.NormalizationPassesChecked != 0 ||
				runtime.NormalizationPassesRun != 0 ||
				runtime.NormalizationNodesVisited != 0 ||
				runtime.NormalizationNodesRewritten != 0 {
				t.Fatalf(
					"normalization checked/run/visited/rewritten=%d/%d/%d/%d",
					runtime.NormalizationPassesChecked,
					runtime.NormalizationPassesRun,
					runtime.NormalizationNodesVisited,
					runtime.NormalizationNodesRewritten,
				)
			}
		})
	}
}

func TestZeroWidthArtifactNativeRoutes(t *testing.T) {
	for _, test := range []struct {
		name     string
		language *gotreesitter.Language
		source   []byte
	}{
		{
			name:     "haskell_update",
			language: HaskellLanguage(),
			source:   []byte("\nmodule Main where\n"),
		},
		{
			name:     "typst_repetition_skip",
			language: TypstLanguage(),
			source:   []byte("#(1,)"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			productionParser := gotreesitter.NewParser(test.language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertNoZeroWidthArtifact(t, "production", production, test.language)

			forestParser := gotreesitter.NewParser(test.language)
			forest, ok := forestParser.ParseForestExperimental(test.source)
			if !ok || forest == nil {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				t.Fatalf(
					"forest declined at %d symbol=%d reason=%s",
					offset,
					symbol,
					reason,
				)
			}
			t.Cleanup(forest.Release)
			assertNoZeroWidthArtifact(t, "forest", forest, test.language)

			nextSource := append(append([]byte(nil), test.source...), ' ')
			oldTree, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			var endPoint gotreesitter.Point
			for _, value := range test.source {
				if value == '\n' {
					endPoint.Row++
					endPoint.Column = 0
				} else {
					endPoint.Column++
				}
			}
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(test.source)),
				OldEndByte:  uint32(len(test.source)),
				NewEndByte:  uint32(len(nextSource)),
				StartPoint:  endPoint,
				OldEndPoint: endPoint,
				NewEndPoint: gotreesitter.Point{
					Row:    endPoint.Row,
					Column: endPoint.Column + 1,
				},
			})
			incremental, _, err := productionParser.ParseIncrementalProfiled(nextSource, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			assertNoZeroWidthArtifact(t, "incremental", incremental, test.language)

			fresh, err := gotreesitter.NewParser(test.language).Parse(nextSource)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(fresh.Release)
			if got, want := incremental.RootNode().SExpr(test.language),
				fresh.RootNode().SExpr(test.language); got != want {
				t.Fatalf("incremental tree = %s, want fresh %s", got, want)
			}
		})
	}
}

func assertNoZeroWidthArtifact(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
) {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("%s root = nil", route)
	}
	var inspect func(*gotreesitter.Node)
	inspect = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.Type(language) == "_token1" && node.StartByte() == node.EndByte() {
			t.Fatalf("%s retained Haskell UPDATE: %s", route, root.SExpr(language))
		}
		if node.Type(language) == "group" {
			for index := 0; index+1 < node.ChildCount(); index++ {
				comma := node.Child(index)
				closeParen := node.Child(index + 1)
				if comma.Type(language) == "," &&
					comma.StartByte() == comma.EndByte() &&
					closeParen.Type(language) == ")" {
					t.Fatalf("%s retained a Typst comma artifact: %s", route, root.SExpr(language))
				}
			}
		}
		for index := 0; index < node.ChildCount(); index++ {
			inspect(node.Child(index))
		}
	}
	inspect(root)
}

//go:linkname newZeroWidthRetirementLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newZeroWidthRetirementLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

//go:linkname zeroWidthRetirementLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func zeroWidthRetirementLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)
