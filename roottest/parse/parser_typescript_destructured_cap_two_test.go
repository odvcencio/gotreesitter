package parse_test

import (
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// typeScriptDestructuredArrowReturnTypeCapCompareShapes are the
// destructured-arrow-return-type shapes whose per-shape source-text detector
// (typeScriptSourceHasDestructuredArrowReturnType) was retired in favor of
// the plain typeScriptSteadyStateMergeCap floor (PR #416's follow-up). The
// detector used to force this shape's merge-per-key cap up to six; the plain
// floor is two.
var typeScriptDestructuredArrowReturnTypeCapCompareShapes = []struct {
	name string
	src  string
}{
	{"small", "const remainingPaths = arrayFrom(allFileNames.entries(), ([fileName, { isRedirect, isInNodeModules }]): ModulePath => ({ path: fileName, isRedirect, isInNodeModules }));\n"},
	// largeUnionListDts stands in for the union-type-list .d.ts sources (for
	// example dom.generated.d.ts) that #389 and #416 measured against: a large
	// run of top-level union-type declarations preceding the
	// destructured-arrow-return-type declaration under test.
	{"largeUnionListDts", strings.Repeat("declare type FieldName = 'a' | 'b' | 'c' | 'd' | 'e' | 'f' | 'g' | 'h';\n", 4096) +
		"const remainingPaths = arrayFrom(allFileNames.entries(), ([fileName, { isRedirect, isInNodeModules }]): ModulePath => ({ path: fileName, isRedirect, isInNodeModules }));\n"},
}

// TestTypeScriptDestructuredArrowReturnTypeByteMatchesAcrossCaps verifies that
// retiring the destructured-arrow-return-type detector left the parsed tree
// unchanged: the shipping cap-two steady state produces a byte-identical
// s-expression and identical parse-runtime node-allocation and max-stack
// counts to the six-survivor cap the retired detector used to force. This
// reconfirms, at plain cap-two with the detector fully removed, the #416
// review's byte-identical-allocs finding across caps for this shape family --
// including on a large union-list .d.ts-shaped source, so the retirement does
// not reintroduce the intra-token population explosion #389 fixed.
func TestTypeScriptDestructuredArrowReturnTypeByteMatchesAcrossCaps(t *testing.T) {
	for _, lang := range []string{"typescript", "tsx"} {
		lang := lang
		for _, s := range typeScriptDestructuredArrowReturnTypeCapCompareShapes {
			s := s
			t.Run(lang+"/"+s.name, func(t *testing.T) {
				capTwoTree, capTwoLang := parseTypeScriptAtMergeCap(t, lang, s.src, 0)
				defer capTwoTree.Release()
				capSixTree, _ := parseTypeScriptAtMergeCap(t, lang, s.src, 6)
				defer capSixTree.Release()

				capTwoRoot := capTwoTree.RootNode()
				capSixRoot := capSixTree.RootNode()
				if capTwoRoot.HasError() {
					t.Fatalf("%s/%s: cap-two tree has error: %s", lang, s.name, capTwoRoot.SExpr(capTwoLang))
				}
				if capSixRoot.HasError() {
					t.Fatalf("%s/%s: cap-six tree has error: %s", lang, s.name, capSixRoot.SExpr(capTwoLang))
				}

				capTwoSExpr := capTwoRoot.SExpr(capTwoLang)
				capSixSExpr := capSixRoot.SExpr(capTwoLang)
				if capTwoSExpr != capSixSExpr {
					t.Fatalf("%s/%s: cap-two tree diverges from cap-six tree\ncap-two: %s\ncap-six: %s", lang, s.name, capTwoSExpr, capSixSExpr)
				}

				capTwoRuntime := capTwoTree.ParseRuntime()
				capSixRuntime := capSixTree.ParseRuntime()
				if capTwoRuntime.NodesAllocated != capSixRuntime.NodesAllocated {
					t.Fatalf("%s/%s: NodesAllocated cap-two=%d cap-six=%d, want byte-identical allocs across caps", lang, s.name, capTwoRuntime.NodesAllocated, capSixRuntime.NodesAllocated)
				}
				if capTwoRuntime.MaxStacksSeen != capSixRuntime.MaxStacksSeen {
					t.Fatalf("%s/%s: MaxStacksSeen cap-two=%d cap-six=%d, want identical peak stack population across caps", lang, s.name, capTwoRuntime.MaxStacksSeen, capSixRuntime.MaxStacksSeen)
				}
			})
		}
	}
}

// parseTypeScriptAtMergeCap parses src under the named TypeScript-family
// grammar with GOT_GLR_MAX_MERGE_PER_KEY forced to cap (or left at the
// production default when cap is zero). It resets the package's cached env
// config both before and after the parse so the forced cap never leaks into
// sibling tests.
func parseTypeScriptAtMergeCap(t *testing.T, langName, src string, cap int) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	if cap > 0 {
		t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", strconv.Itoa(cap))
	} else {
		t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	}
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)
	return ironwoodParseNoFatal(t, langName, src)
}
