//go:build !grammar_subset

package grammars

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

type recoveryActionMaterializationCase struct {
	name          string
	language      func() *gotreesitter.Language
	source        string
	wantSExpr     string
	wantMissing   string
	wantCRecovery bool
	assert        func(*testing.T, *gotreesitter.Tree, *gotreesitter.Language, []byte)
}

var recoveryActionMaterializationCases = []recoveryActionMaterializationCase{
	{
		name:        "forth_body",
		language:    ForthLanguage,
		source:      ": foo 1 2\n",
		wantSExpr:   "(source_file (word_definition (start_definition) (word) (number (decimal_number)) (number (decimal_number)) (end_definition)))",
		wantMissing: "end_definition",
	},
	{
		name:      "forth_empty",
		language:  ForthLanguage,
		source:    ": foo\n",
		wantSExpr: "(source_file (ERROR (start_definition) (word)))",
	},
	{
		name:      "forth_terminated",
		language:  ForthLanguage,
		source:    ": foo 1 2 ;\n",
		wantSExpr: "(source_file (word_definition (start_definition) (word) (number (decimal_number)) (number (decimal_number)) (end_definition)))",
	},
	{
		name:      "luau_end",
		language:  LuauLanguage,
		source:    "end",
		wantSExpr: "(chunk (ERROR (identifier)))",
	},
	{
		name:      "luau_declaration_end",
		language:  LuauLanguage,
		source:    "local x = 1\nend\n",
		wantSExpr: "(chunk (variable_declaration (assignment_statement (variable_list (identifier)) (expression_list (number)))) (ERROR (identifier)))",
	},
	{
		name:      "luau_if_end",
		language:  LuauLanguage,
		source:    "if true then\nend\nend\n",
		wantSExpr: "(chunk (if_statement (true)) (ERROR (identifier)))",
	},
	{
		name:          "angular_let_non_null",
		language:      AngularLanguage,
		source:        "<div>@let itemLabel = item.label!;</div>",
		wantCRecovery: true,
		assert:        assertAngularLetNonNullMaterialization,
	},
	{
		name:          "angular_binary_non_null",
		language:      AngularLanguage,
		source:        "<div>@if (item.level! > 1) { ok }</div>",
		wantCRecovery: true,
		assert:        assertAngularBinaryNonNullMaterialization,
	},
	{
		name:          "angular_ampersand_text",
		language:      AngularLanguage,
		source:        "<strong>Opinionated & versatile,</strong>",
		wantCRecovery: true,
		assert:        assertAngularAmpersandMaterialization,
	},
	{
		name:          "bibtex_missing_comma_after_key",
		language:      BibtexLanguage,
		source:        "@article{test\n    title     = {title-from-embedded-bibtex-file},\n    author    = {author-from-embedded-bibtex-file},\n}\n",
		wantCRecovery: true,
		assert:        assertBibtexMissingKeyMaterialization,
	},
	{
		name:          "bibtex_missing_key",
		language:      BibtexLanguage,
		source:        "@techreport{\n  journal = {Wirtschaftsinformatik},\n  author = {Sam and jason},\n}\n",
		wantCRecovery: true,
		assert:        assertBibtexMissingKeyMaterialization,
	},
	{
		name:          "eds_embedded_section",
		language:      EdsLanguage,
		source:        "[A]\nEmpty=\n\n[B]\nX=1\n",
		wantCRecovery: true,
		assert:        assertEDSEmbeddedSectionMaterialization,
	},
	{
		name:          "eds_error_lines",
		language:      EdsLanguage,
		source:        "[1003sub0]\nParameterName=Number of errors\nObjectType=0x7\n;StorageLocation=RAM\nDataType=0x0005\nAccessType=rw\nDefaultValue=\nPDOMapping=0\n",
		wantCRecovery: true,
		assert:        assertEDSErrorLineMaterialization,
	},
	{
		name:          "chatito_trailing_alias_word",
		language:      ChatitoLanguage,
		source:        "~[minute]\n    58\n    59",
		wantCRecovery: true,
		assert:        assertChatitoTrailingAliasMaterialization,
	},
}

func TestRecoveryActionMaterializationNeedsNoResultCompatibility(t *testing.T) {
	for _, test := range recoveryActionMaterializationCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			language := test.language()

			tree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)

			assertNoNormalizationPasses(t, tree)
			assertRecoveryActionMaterializationTree(t, tree, language, test)

			production, err := gotreesitter.NewParser(language).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertNoNormalizationPasses(t, production)
			assertRecoveryActionMaterializationTree(t, production, language, test)
			want := collapsedTokenTreeDigest(t, production, language)
			assertRecoveryActionMaterializationDigest(t, "compatibility-free", tree, language, want)
		})
	}
}

func TestRecoveryActionMaterializationRoutesStayExactOrFailClosed(t *testing.T) {
	for _, test := range recoveryActionMaterializationCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			language := test.language()

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertNoNormalizationPasses(t, production)
			assertRecoveryActionMaterializationTree(t, production, language, test)
			want := collapsedTokenTreeDigest(t, production, language)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			direct := routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore
			fallback := routedAfter == routedBefore && fallbackAfter == fallbackBefore+1
			if !direct && !fallback {
				t.Fatalf(
					"compact counters routed=%d/%d fallback=%d/%d: %s",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}
			assertNoNormalizationPasses(t, compact)
			assertRecoveryActionMaterializationDigest(t, "compact", compact, language, want)

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(source)
			if ok && forest != nil {
				t.Cleanup(forest.Release)
				assertNoNormalizationPasses(t, forest)
				assertRecoveryActionMaterializationDigest(t, "forest", forest, language, want)
			} else {
				if forest != nil {
					forest.Release()
					t.Fatal("forest returned a tree with a decline")
				}
				_, _, reason, _ := forestParser.ForestDeclineInfo()
				if reason == "" {
					t.Fatal("forest declined without a reason")
				}
				forestFallback, fallbackErr := forestParser.Parse(source)
				if fallbackErr != nil {
					t.Fatal(fallbackErr)
				}
				t.Cleanup(forestFallback.Release)
				assertNoNormalizationPasses(t, forestFallback)
				assertRecoveryActionMaterializationDigest(
					t,
					"forest-fallback",
					forestFallback,
					language,
					want,
				)
			}

			assertRecoveryActionMaterializationIncremental(t, language, source, want)
		})
	}
}

// TestForthIncrementalTerminatorDeletionMaterializesMissingEnd covers the
// edit shape that actually triggers Forth recovery: removing a definition's
// closing terminator. TestRecoveryActionMaterializationRoutesStayExactOrFailClosed
// above only exercises an incremental edit that appends a trailing byte, so
// it never removes a terminator. This case deletes the `;` from a terminated
// definition and confirms the incremental route still materializes a
// zero-width missing end_definition matching a fresh parse of the edited
// source.
func TestForthIncrementalTerminatorDeletionMaterializesMissingEnd(t *testing.T) {
	language := ForthLanguage()
	oldSource := []byte(": foo 1 2 ;\n")
	semicolon := bytes.IndexByte(oldSource, ';')
	if semicolon < 0 {
		t.Fatal("fixture is missing its terminator")
	}
	newSource := append(append([]byte(nil), oldSource[:semicolon]...), oldSource[semicolon+1:]...)

	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)

	deletePoint := retiredDispatchPointAtByte(oldSource, semicolon)
	afterPoint := retiredDispatchPointAtByte(oldSource, semicolon+1)
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(semicolon),
		OldEndByte:  uint32(semicolon + 1),
		NewEndByte:  uint32(semicolon),
		StartPoint:  deletePoint,
		OldEndPoint: afterPoint,
		NewEndPoint: deletePoint,
	})

	incremental, _, err := parser.ParseIncrementalProfiled(newSource, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	assertNoNormalizationPasses(t, incremental)

	fresh, err := gotreesitter.NewParser(language).Parse(newSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Release)
	assertNoNormalizationPasses(t, fresh)
	want := collapsedTokenTreeDigest(t, fresh, language)
	assertRecoveryActionMaterializationDigest(t, "incremental-terminator-deletion", incremental, language, want)

	missing := findRecoveryActionMaterializationNode(incremental.RootNode(), language, "end_definition")
	if missing == nil || !missing.IsMissing() || missing.StartByte() != missing.EndByte() {
		t.Fatalf("incremental end_definition = %v, want zero-width missing node", missing)
	}
}

func assertRecoveryActionMaterializationIncremental(
	t *testing.T,
	language *gotreesitter.Language,
	source []byte,
	want string,
) {
	t.Helper()
	if len(source) == 0 {
		t.Fatal("incremental fixture is empty")
	}

	oldSource := source[:len(source)-1]
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	startPoint := retiredDispatchPointAtByte(oldSource, len(oldSource))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(oldSource)),
		OldEndByte:  uint32(len(oldSource)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  startPoint,
		OldEndPoint: startPoint,
		NewEndPoint: retiredDispatchPointAtByte(source, len(source)),
	})
	incremental, _, err := parser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	assertNoNormalizationPasses(t, incremental)
	assertRecoveryActionMaterializationDigest(
		t,
		"incremental",
		incremental,
		language,
		want,
	)
}

func assertRecoveryActionMaterializationTree(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	test recoveryActionMaterializationCase,
) {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("root is nil")
	}
	if test.wantSExpr != "" {
		if got := root.SExpr(language); got != test.wantSExpr {
			t.Fatalf("tree = %s, want %s", got, test.wantSExpr)
		}
	}
	if test.wantCRecovery && !tree.ParseRuntime().CRecoveryEnteredErrorState {
		t.Fatal("tree did not enter C-style recovery")
	}
	if test.assert != nil {
		test.assert(t, tree, language, []byte(test.source))
	}
	assertRecoveryActionMaterializationParentLinks(t, tree, language)
	if test.wantMissing == "" {
		return
	}
	missing := findRecoveryActionMaterializationNode(root, language, test.wantMissing)
	if missing == nil || !missing.IsMissing() || missing.StartByte() != missing.EndByte() {
		t.Fatalf(
			"%s missing node = %v, want zero-width missing node",
			test.wantMissing,
			missing,
		)
	}
}

func findRecoveryActionMaterializationNode(
	node *gotreesitter.Node,
	language *gotreesitter.Language,
	name string,
) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.Type(language) == name {
		return node
	}
	for index := 0; index < node.ChildCount(); index++ {
		if found := findRecoveryActionMaterializationNode(node.Child(index), language, name); found != nil {
			return found
		}
	}
	return nil
}

func assertRecoveryActionMaterializationDigest(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	want string,
) {
	t.Helper()
	assertRecoveryActionMaterializationParentLinks(t, tree, language)
	if got := collapsedTokenTreeDigest(t, tree, language); got != want {
		t.Fatalf("%s digest = %s, want production %s", route, got, want)
	}
}

func assertRecoveryActionMaterializationParentLinks(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
) {
	t.Helper()
	gotreesitter.Walk(tree.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if depth > 0 && node.Parent() == nil {
			t.Fatalf("node %q at depth %d has no parent", node.Type(language), depth)
		}
		return gotreesitter.WalkContinue
	})
}

func assertAngularLetNonNullMaterialization(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	_ []byte,
) {
	t.Helper()
	let := findRecoveryActionMaterializationNode(tree.RootNode(), language, "let_statement")
	if let == nil || let.ChildCount() != 5 {
		t.Fatalf("let statement = %v, want five children", let)
	}
	assertRecoveryErrorNode(t, let.Child(3), language, "unary_operator", 32, 33)
}

func assertAngularBinaryNonNullMaterialization(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	binary := findRecoveryActionMaterializationNode(tree.RootNode(), language, "binary_expression")
	if binary == nil || binary.ChildCount() != 4 {
		t.Fatalf("binary expression = %v, want four children", binary)
	}
	start := bytes.IndexByte(source, '!')
	if start < 0 {
		t.Fatal("source has no non-null assertion")
	}
	assertRecoveryErrorNode(
		t,
		binary.Child(1),
		language,
		"unary_operator",
		uint32(start),
		uint32(start+1),
	)
}

func assertAngularAmpersandMaterialization(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	element := findRecoveryActionMaterializationNode(tree.RootNode(), language, "element")
	if element == nil || element.ChildCount() != 4 {
		t.Fatalf("element = %v, want four children", element)
	}
	errNode := element.Child(2)
	start := bytes.Index(source, []byte("& versatile,"))
	end := bytes.Index(source, []byte("</strong>"))
	if start < 0 || end < 0 {
		t.Fatal("source lacks the recovery span")
	}
	if errNode == nil || errNode.Type(language) != "ERROR" || !errNode.IsExtra() ||
		errNode.StartByte() != uint32(start) || errNode.EndByte() != uint32(end) {
		t.Fatalf("ampersand ERROR = %v, want extra ERROR at %d:%d", errNode, start, end)
	}
	wantTypes := []string{
		"ERROR",
		"regular_expression_flags",
		"ERROR",
		"s",
		"ERROR",
		"regular_expression_flags",
		"ERROR",
		",",
	}
	if errNode.ChildCount() != len(wantTypes) {
		t.Fatalf("ampersand ERROR child count = %d, want %d", errNode.ChildCount(), len(wantTypes))
	}
	for index, want := range wantTypes {
		if got := errNode.Child(index).Type(language); got != want {
			t.Fatalf("ampersand ERROR child %d = %q, want %q", index, got, want)
		}
	}
}

func assertRecoveryErrorNode(
	t *testing.T,
	node *gotreesitter.Node,
	language *gotreesitter.Language,
	childType string,
	start uint32,
	end uint32,
) {
	t.Helper()
	if node == nil || node.Type(language) != "ERROR" || !node.IsExtra() ||
		node.StartByte() != start || node.EndByte() != end || node.ChildCount() != 1 {
		t.Fatalf("recovery ERROR = %v, want extra ERROR at %d:%d", node, start, end)
	}
	if got := node.Child(0).Type(language); got != childType {
		t.Fatalf("recovery ERROR child = %q, want %q", got, childType)
	}
}

func assertBibtexMissingKeyMaterialization(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	_ []byte,
) {
	t.Helper()
	entry := findRecoveryActionMaterializationNode(tree.RootNode(), language, "entry")
	if entry == nil || entry.ChildCount() < 5 {
		t.Fatalf("entry = %v, want recovered entry", entry)
	}
	recovery := entry.Child(1)
	if recovery == nil || recovery.Type(language) != "ERROR" ||
		!recovery.IsExtra() || !recovery.IsNamed() {
		t.Fatalf("entry recovery = %v, want named extra ERROR", recovery)
	}
	if first := recovery.Child(0); first == nil || first.Type(language) != "{" {
		t.Fatalf("recovery first child = %v, want opening brace", first)
	}
	if valueOpen := entry.Child(2); valueOpen == nil || valueOpen.Type(language) != "{" {
		t.Fatalf("entry value opener = %v, want opening brace", valueOpen)
	}
}

func assertEDSEmbeddedSectionMaterialization(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	_ []byte,
) {
	t.Helper()
	root := tree.RootNode()
	if root.ChildCount() != 2 {
		t.Fatalf("EDS root child count = %d, want 2", root.ChildCount())
	}
	first, second := root.Child(0), root.Child(1)
	if first.Type(language) != "section" || first.EndByte() != 10 {
		t.Fatalf("first EDS section = %v, want section ending at 10", first)
	}
	if second.Type(language) != "section" || second.StartByte() != 12 {
		t.Fatalf("second EDS section = %v, want section starting at 12", second)
	}
}

func assertEDSErrorLineMaterialization(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	_ []byte,
) {
	t.Helper()
	errNode := tree.RootNode().Child(2)
	if errNode == nil || errNode.Type(language) != "ERROR" ||
		errNode.StartByte() != 78 || errNode.ChildCount() != 9 {
		t.Fatalf("EDS error line = %v, want nine-child ERROR at byte 78", errNode)
	}
	if got := errNode.Child(6).StartByte(); got != 122 {
		t.Fatalf("EDS empty-value recovery starts at %d, want 122", got)
	}
}

func assertChatitoTrailingAliasMaterialization(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	root := tree.RootNode()
	if root.ChildCount() != 1 || root.HasError() {
		t.Fatalf("Chatito root = %s, want one clean alias", root.SExpr(language))
	}
	alias := root.Child(0)
	body := findRecoveryActionMaterializationNode(alias, language, "alias_body")
	if alias.EndByte() != uint32(len(source)) || body == nil ||
		body.EndByte() != uint32(len(source)) || body.ChildCount() != 4 {
		t.Fatalf("Chatito alias/body = %v/%v, want full-source four-child body", alias, body)
	}
	word := body.Child(3)
	if word == nil || word.Type(language) != "word" || word.Text(source) != "59" {
		t.Fatalf("Chatito trailing word = %v, want word 59", word)
	}
}
