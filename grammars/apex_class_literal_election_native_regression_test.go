//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Apex's class_literal (seq(_unannotated_type, ".", ci("class"))) and
// field_access (seq(choice(primary_expression, super),
// _property_navigation, choice(identifier, this))) both reach a live GLR
// fork on `Type.class`: the grammar declares no dynamic precedence between
// the _unannotated_type and primary_expression readings of the leading
// name, so both the class-literal and field-access derivations complete as
// fully valid, differently-shaped parses of the same bytes. The locked C
// oracle always keeps field_access. This is fixed natively at two points
// for the classic engine (production, compact's fallback target, and
// incremental all share it), neither apex-specific:
//
//   - relexTokenForStackLexState (parser.go) now also runs, unconditionally,
//     right before a multi-stack no-action kill: the shared lexer's
//     keyword-versus-identifier choice used to starve the field_access
//     fork of the plain `identifier` token its state needs for the
//     trailing `class` text, because a sibling fork's state only needed
//     the `class` keyword. The per-stack DFA re-probe now gives a
//     starved fork its own tokenization before killing it, exactly as the
//     faithful C-recovery port already does for its own gated languages.
//   - stackCompareForResultSelectionWithRawShape (parser_result.go) now
//     skips the raw-shape/symbol-id fallback tie-break for languages
//     certified CompactPrimaryAcceptanceDerivationCertified (apex among
//     them): that fallback broke the resulting tie on raw grammar symbol
//     id, an artifact of compiler assignment order unrelated to which
//     production C actually prefers. Skipping it lets the existing
//     branchOrder tie-break -- which already encodes "never took a
//     GLR-forked conflict branch," the same signal the compact route's own
//     certified primary-derivation election already trusts -- decide
//     instead.
//
// The GSS-forest experimental fast path (ParseForestExperimental) is a
// separate engine with its own dispatch loop and does not share either fix
// site; it is not in apex's automatic-dispatch set
// (glr_forest.go builtinForestDefaults), so no ordinary Parse call reaches
// it for this language, but an explicit ParseForestExperimental call still
// needs the compat arm below. dispatch.apex therefore stays live, narrowed
// to that one remaining route -- see TestApexClassLiteralForestStillNeedsResultCompatibility.
var (
	apexClassLiteralAliasFixture = []byte(
		"public class C {\n" +
			"  void m() {\n" +
			"    Object t = RecordPage.class;\n" +
			"  }\n" +
			"}",
	)
	apexQualifiedClassLiteralAliasFixture = []byte(
		"public class C {\n" +
			"  void m() {\n" +
			"    Object t = Outer.Inner.class;\n" +
			"  }\n" +
			"}",
	)
	apexPlainFieldAccessFixture = []byte(
		"public class C {\n" +
			"  void m() {\n" +
			"    Object t = RecordPage.someField;\n" +
			"  }\n" +
			"}",
	)
)

func TestApexClassLiteralElectionNeedsNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := ApexLanguage()

	for _, fixture := range [][]byte{apexClassLiteralAliasFixture, apexQualifiedClassLiteralAliasFixture} {
		tree, err := gotreesitter.NewParser(language).ParseNoResultCompatibilityBenchmarkOnly(fixture)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tree.Release)
		assertNoNormalizationPasses(t, tree)
		assertApexClassLiteralIsFieldAccess(t, "compatibility-free", tree.RootNode(), language)

		production, err := gotreesitter.NewParser(language).Parse(fixture)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(production.Release)
		if runtime := production.ParseRuntime(); runtime.NormalizationNodesRewritten != 0 {
			t.Fatalf("production normalization rewrote %d nodes", runtime.NormalizationNodesRewritten)
		}
		assertApexClassLiteralIsFieldAccess(t, "production", production.RootNode(), language)
	}
}

// TestApexClassLiteralElectionRoutesMatch checks the three routes this fix
// covers: production, compact (which falls back to production for this
// witness -- see the comment below -- rather than routing directly), and
// incremental. The forest fast path is exercised separately: see
// TestApexClassLiteralForestStillNeedsResultCompatibility.
func TestApexClassLiteralElectionRoutesMatch(t *testing.T) {
	language := ApexLanguage()

	tests := []struct {
		name    string
		fixture []byte
	}{
		{"bare_class_literal_alias", apexClassLiteralAliasFixture},
		{"qualified_class_literal_alias", apexQualifiedClassLiteralAliasFixture},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseSource := test.fixture
			source := append(append([]byte(nil), baseSource...), '\n')

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse failed: %v", err)
			}
			t.Cleanup(production.Release)
			assertApexClassLiteralIsFieldAccess(t, "production", production.RootNode(), language)

			// Allow the compact route to fall back to production rather than
			// requiring it to route directly: both fixtures' two tied
			// derivations (field_access vs. class_literal) are not
			// byte-identical, so the compact route's own, separate R1
			// materiality gate (compactAcceptanceElectionIsVacuous,
			// parsercore_phase0_driver.go) declines them and falls back to
			// production on every parse -- documented there as a
			// pre-existing, orthogonal cost this fix does not touch. The
			// fallback still serves production's tree, which this fix makes
			// C-exact, so the route's final output is correct either way.
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatalf("compact parse failed: %v", err)
			}
			t.Cleanup(compact.Release)
			assertApexClassLiteralIsFieldAccess(t, "compact", compact.RootNode(), language)

			oldParser := gotreesitter.NewParser(language)
			oldParser.SetAdmissionCandidateRoute(false)
			oldTree, err := oldParser.Parse(baseSource)
			if err != nil {
				t.Fatalf("incremental base parse failed: %v", err)
			}
			t.Cleanup(oldTree.Release)
			endPoint := retiredDispatchPointAtByte(baseSource, len(baseSource))
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(baseSource)),
				OldEndByte:  uint32(len(baseSource)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  endPoint,
				OldEndPoint: endPoint,
				NewEndPoint: gotreesitter.Point{Row: endPoint.Row + 1},
			})
			incremental, err := oldParser.ParseIncremental(source, oldTree)
			if err != nil {
				t.Fatalf("incremental parse failed: %v", err)
			}
			t.Cleanup(incremental.Release)
			assertApexClassLiteralIsFieldAccess(t, "incremental", incremental.RootNode(), language)
		})
	}
}

// TestApexClassLiteralForestStillNeedsResultCompatibility documents and
// guards the one route this fix does not cover. ParseForestExperimental is
// a separate engine (glr_forest.go) with its own dispatch loop and
// accept-time tie-break; it is not apex's automatic-dispatch route (apex is
// not in builtinForestDefaults), so no ordinary Parse call reaches it, but
// an explicit ParseForestExperimental call still needs
// normalizeApexClassLiteralAccess to reach the C-native field_access shape.
// If this test starts failing (the forest route stops rewriting), the
// forest engine has been fixed natively too: update this test and dispatch.apex
// can retire for good.
func TestApexClassLiteralForestStillNeedsResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := ApexLanguage()
	source := append(append([]byte(nil), apexClassLiteralAliasFixture...), '\n')

	forestParser := gotreesitter.NewParser(language)
	forest, ok := forestParser.ParseForestExperimental(source)
	if !ok {
		t.Fatal("forest route declined")
	}
	t.Cleanup(forest.Release)
	if !forest.ParseRuntime().ForestFastPath {
		t.Fatal("forest parse did not report the forest route")
	}
	assertApexClassLiteralIsFieldAccess(t, "forest", forest.RootNode(), language)

	runtime := forest.ParseRuntime()
	rewrote := false
	if runtime.NormalizationPasses != nil {
		for _, pass := range *runtime.NormalizationPasses {
			if pass.Name == "dispatch.apex.class-literal-alias" && pass.NodesRewritten > 0 {
				rewrote = true
			}
		}
	}
	if !rewrote {
		t.Fatal("forest route no longer needs normalizeApexClassLiteralAccess: " +
			"the forest engine now elects field_access natively too -- " +
			"dispatch.apex can retire (delete parser_result_apex.go and its " +
			"dispatcher case) instead of staying live for this route alone")
	}
}

// TestApexPlainFieldAccessUnaffected is a negative control: a bare
// (non-".class") field access was never ambiguous and must keep resolving
// to field_access without exercising the election this fix touches.
func TestApexPlainFieldAccessUnaffected(t *testing.T) {
	language := ApexLanguage()
	tree, err := gotreesitter.NewParser(language).Parse(apexPlainFieldAccessFixture)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	assertApexClassLiteralIsFieldAccess(t, "production", tree.RootNode(), language)
}

// assertApexClassLiteralIsFieldAccess locates the outermost field_access
// node and requires its trailing field to be a plain identifier -- the
// C-native reading for both `Type.class` and a plain `Type.field` access. A
// regression back to the old election would surface a class_literal node
// instead.
func assertApexClassLiteralIsFieldAccess(
	t *testing.T,
	route string,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
) {
	t.Helper()
	if root == nil || root.HasError() {
		t.Fatalf("%s root = %v", route, root)
	}
	if bad := findRecoveryActionMaterializationNode(root, language, "class_literal"); bad != nil {
		t.Fatalf("%s tree still contains a class_literal node: %s", route, root.SExpr(language))
	}
	access := findRecoveryActionMaterializationNode(root, language, "field_access")
	if access == nil || access.ChildCount() != 3 {
		t.Fatalf("%s field_access = %v, want exactly 3 children: %s", route, access, root.SExpr(language))
	}
	field := access.Child(2)
	if field.Type(language) != "identifier" {
		t.Fatalf("%s field_access trailing child = %q, want %q: %s", route, field.Type(language), "identifier", root.SExpr(language))
	}
}
