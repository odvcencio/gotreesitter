package gotreesitter_test

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// findAllNodesOfType does a depth-first traversal collecting every node
// (named or not) with the given Type() in source order. It's the multi-match
// counterpart to findNamedChild (parser_go_test.go), needed here because
// several of these regression fixtures deliberately contain more than one
// call_expression and the tests must pin each one independently.
func findAllNodesOfType(lang *gotreesitter.Language, node *gotreesitter.Node, typeName string) []*gotreesitter.Node {
	var out []*gotreesitter.Node
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == typeName {
			out = append(out, n)
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(node)
	return out
}

// assertGoCandidateMatchesProduction routes src through the compact candidate
// engine and requires the result to equal the production tree byte for byte.
// The Phase-3 compact route fails closed on the generic-instantiation conflict,
// so this holds whether the candidate routes or declines to production; the
// helper only proves the route never diverges from production for this fixture.
func assertGoCandidateMatchesProduction(t *testing.T, lang *gotreesitter.Language, src string, prodTree *gotreesitter.Tree) {
	t.Helper()
	prodSExpr := prodTree.RootNode().SExpr(lang)
	gotreesitter.ResetAdmissionCandidateCountersForTest()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	candTree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("candidate parse failed: %v", err)
	}
	defer candTree.Release()
	if got := candTree.RootNode().SExpr(lang); got != prodSExpr {
		routed, fallback := gotreesitter.AdmissionCandidateCounters()
		t.Fatalf("candidate route diverged from production (routed=%d fallback=%d):\n  production: %s\n  candidate:  %s",
			routed, fallback, prodSExpr, got)
	}
}

// assertNoErrorOrMissing walks the whole tree and fails the test if any
// ERROR or MISSING node is found anywhere, printing the S-expression for
// diagnosis. Modeled on the existing GoInterpretedStringFalseErrors /
// GoWhitespaceAdjacentImmediateStringToken / GoIfElseSameLineKeywordPromotion
// walk-assertion pattern in parser_go_test.go.
func assertNoErrorOrMissing(t *testing.T, lang *gotreesitter.Language, root *gotreesitter.Node, label string) {
	t.Helper()
	if root.HasError() {
		t.Fatalf("%s: root.HasError()==true:\n%s", label, root.SExpr(lang))
	}
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		if n.IsError() || n.IsMissing() {
			t.Fatalf("%s: found ERROR/MISSING node:\n%s", label, root.SExpr(lang))
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
}

// TestParseGoNewMakeVariadicMultiArgTypePositionOnly pins the multi-argument
// shape of the "new"/"make" builtin call: tree-sitter-go's grammar routes
// literal new()/make() calls through a dedicated special_argument_list
// production ([$._type, ($._expression)*] — see grammargen/go_grammar.go's
// special_argument_list definition and the root-cause comment on
// normalizeGoNewMakeTypeArgument in parser_result_go.go), so the retag must
// fire on ONLY the leading/first argument, leaving every subsequent argument
// as a plain expression (identifier) untouched — regardless of how many
// total arguments follow. `new(a, b)` is invalid Go (new takes one argument)
// but is syntactically well-formed per the grammar (this is a semantic check
// the Go compiler makes, not something tree-sitter's context-free grammar
// enforces), so it must parse cleanly with no ERROR/MISSING nodes and no
// panic — never silently corrupt or over-retag beyond the first argument.
//
// Verified directly against a real, locally-built tree-sitter-go C oracle
// (github.com/tree-sitter/tree-sitter-go @ 2346a3a, via `tree-sitter parse`):
//
//	new(a, b)      -> argument_list(type_identifier, identifier)
//	make(a, b, c)  -> argument_list(type_identifier, identifier, identifier)
func TestParseGoNewMakeVariadicMultiArgTypePositionOnly(t *testing.T) {
	for _, tc := range []struct {
		name      string
		src       string
		wantTypes []string // NamedChild types of the argument_list, in order
	}{
		{
			name:      "new-two-args-invalid-go-but-parses",
			src:       "package p\n\nfunc f() {\n\tvar a, b int\n\t_ = new(a, b)\n}\n",
			wantTypes: []string{"type_identifier", "identifier"},
		},
		{
			name:      "make-three-args",
			src:       "package p\n\nfunc f() {\n\tvar a, b, c int\n\t_ = make(a, b, c)\n}\n",
			wantTypes: []string{"type_identifier", "identifier", "identifier"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			assertNoErrorOrMissing(t, lang, root, tc.name)

			call := findNamedChild(lang, root, "call_expression")
			if call == nil {
				t.Fatalf("no call_expression found; root=%s", root.SExpr(lang))
			}
			args := call.ChildByFieldName("arguments", lang)
			if args == nil {
				t.Fatalf("call_expression has no arguments field; root=%s", root.SExpr(lang))
			}
			if args.Type(lang) != "argument_list" {
				t.Fatalf("arguments field Type() = %q, want %q", args.Type(lang), "argument_list")
			}
			if got := args.NamedChildCount(); got != len(tc.wantTypes) {
				t.Fatalf("argument_list NamedChildCount() = %d, want %d; root=%s", got, len(tc.wantTypes), root.SExpr(lang))
			}
			for i, want := range tc.wantTypes {
				child := args.NamedChild(i)
				if child == nil {
					t.Fatalf("argument_list NamedChild(%d) is nil; root=%s", i, root.SExpr(lang))
				}
				if got := child.Type(lang); got != want {
					t.Fatalf("argument_list NamedChild(%d).Type() = %q, want %q; root=%s", i, got, want, root.SExpr(lang))
				}
			}
		})
	}
}

// TestParseGoNewMakeCompositeTypeFirstArgumentUntouched pins that make() calls
// whose first argument is already a composite type (slice_type, map_type,
// channel_type — never ambiguous with _expression in the grammar, since none
// of those productions can also start an expression) are completely
// unaffected by normalizeGoNewMakeTypeArgument: the retag only ever fires
// when the first named argument's symbol is bare `identifier`
// (parser_result_go.go's `first.symbol != identifierSym` guard), so these
// must keep their real composite-type node shape and the fix must never
// touch a later plain-expression argument (e.g. the length/capacity
// argument) either.
//
// Verified against the real tree-sitter-go C oracle:
//
//	make([]int, 0)        -> argument_list(slice_type(type_identifier), int_literal)
//	make(map[string]int)  -> argument_list(map_type(type_identifier, type_identifier))
//	make(chan int, 5)     -> argument_list(channel_type(type_identifier), int_literal)
func TestParseGoNewMakeCompositeTypeFirstArgumentUntouched(t *testing.T) {
	for _, tc := range []struct {
		name      string
		src       string
		wantTypes []string
	}{
		{
			name:      "make-slice",
			src:       "package p\n\nfunc f() {\n\ts := make([]int, 0)\n\t_ = s\n}\n",
			wantTypes: []string{"slice_type", "int_literal"},
		},
		{
			name:      "make-map",
			src:       "package p\n\nfunc f() {\n\tm := make(map[string]int)\n\t_ = m\n}\n",
			wantTypes: []string{"map_type"},
		},
		{
			name:      "make-chan",
			src:       "package p\n\nfunc f() {\n\tc := make(chan int, 5)\n\t_ = c\n}\n",
			wantTypes: []string{"channel_type", "int_literal"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			assertNoErrorOrMissing(t, lang, root, tc.name)

			call := findNamedChild(lang, root, "call_expression")
			if call == nil {
				t.Fatalf("no call_expression found; root=%s", root.SExpr(lang))
			}
			args := call.ChildByFieldName("arguments", lang)
			if args == nil {
				t.Fatalf("call_expression has no arguments field; root=%s", root.SExpr(lang))
			}
			if got := args.NamedChildCount(); got != len(tc.wantTypes) {
				t.Fatalf("argument_list NamedChildCount() = %d, want %d; root=%s", got, len(tc.wantTypes), root.SExpr(lang))
			}
			for i, want := range tc.wantTypes {
				child := args.NamedChild(i)
				if child == nil {
					t.Fatalf("argument_list NamedChild(%d) is nil; root=%s", i, root.SExpr(lang))
				}
				if got := child.Type(lang); got != want {
					t.Fatalf("argument_list NamedChild(%d).Type() = %q, want %q; root=%s", i, got, want, root.SExpr(lang))
				}
			}
		})
	}
}

// TestParseGoNewMakeRetagOnlyFiresForLiteralBuiltinCalls pins that
// normalizeGoNewMakeTypeArgument only ever fires when the call's function
// field is the literal identifier "new" or "make" (parser_result_go.go's
// `name != "new" && name != "make"` guard) — never for append/len/cap, and
// never for a user-defined function whose name happens to share a substring
// (or even be a completely unrelated call). In particular `foo(bar)` must
// keep `bar` as a plain `identifier`, never `type_identifier`.
//
// Verified against the real tree-sitter-go C oracle: none of these produce a
// type_identifier for their argument (append/len/cap aren't part of the
// grammar's special_argument_list alias at all; only the literal tokens
// "new" and "make" are).
func TestParseGoNewMakeRetagOnlyFiresForLiteralBuiltinCalls(t *testing.T) {
	src := "package p\n\n" +
		"func foo(bar int) int { return bar }\n\n" +
		"func f() {\n" +
		"\tvar xs []int\n" +
		"\tvar y int\n" +
		"\tvar x int\n" +
		"\t_ = append(xs, y)\n" +
		"\t_ = len(x)\n" +
		"\t_ = cap(xs)\n" +
		"\tbar := 1\n" +
		"\t_ = foo(bar)\n" +
		"}\n"
	tree, lang := parseGo(t, src)
	root := tree.RootNode()
	assertNoErrorOrMissing(t, lang, root, "retag-scoped-to-new-make")

	calls := findAllNodesOfType(lang, root, "call_expression")
	source := tree.Source()
	byFuncName := map[string]*gotreesitter.Node{}
	for _, call := range calls {
		fn := call.ChildByFieldName("function", lang)
		if fn == nil {
			continue
		}
		byFuncName[fn.Text(source)] = call
	}

	for _, tc := range []struct {
		fname   string
		argType string
	}{
		{"append", "identifier"}, // first arg is xs, a plain identifier expression
		{"len", "identifier"},
		{"cap", "identifier"},
		{"foo", "identifier"}, // MUST stay identifier, never type_identifier
	} {
		call, ok := byFuncName[tc.fname]
		if !ok {
			t.Fatalf("no call_expression found with function %q; root=%s", tc.fname, root.SExpr(lang))
		}
		fn := call.ChildByFieldName("function", lang)
		if fn == nil || fn.Type(lang) != "identifier" {
			t.Fatalf("call to %q: function field missing or not identifier; root=%s", tc.fname, root.SExpr(lang))
		}
		args := call.ChildByFieldName("arguments", lang)
		if args == nil || args.NamedChildCount() == 0 {
			t.Fatalf("call to %q: no arguments; root=%s", tc.fname, root.SExpr(lang))
		}
		if got := args.NamedChild(0).Type(lang); got != tc.argType {
			t.Fatalf("call to %q: first argument Type() = %q, want %q; root=%s", tc.fname, got, tc.argType, root.SExpr(lang))
		}
	}
}

// TestParseGoNewMakeShadowedCallStillMatchesCOracleByDesign documents and
// pins an intentional, verified-against-C-oracle quirk: because
// tree-sitter's grammar is purely syntactic (no scope/semantic awareness),
// a call to a *locally shadowed* variable that happens to be named "new" or
// "make" is STILL routed through the literal call_expression/
// special_argument_list production and STILL gets its bare-identifier sole
// argument retagged to type_identifier — exactly like a real call to the
// builtin. This is not a bug in normalizeGoNewMakeTypeArgument; it is
// required for C parity. Verified directly against a real, locally-built
// tree-sitter-go C oracle (`tree-sitter parse` against
// github.com/tree-sitter/tree-sitter-go @ 2346a3a):
//
//	new := func(x int) int { return x }
//	dirInfo := 5
//	y := new(dirInfo)
//	-> call_expression(function: identifier, arguments: argument_list(type_identifier))
//
// See also TestParseGoNewMakeUserShadowNotMisclassified, which pins the
// complementary case (a user function whose *name* happens to be a
// former/oracle-relevant identifier, "dirInfo", not "new"/"make" itself —
// that one must NOT be routed through the builtin production at all).
func TestParseGoNewMakeShadowedCallStillMatchesCOracleByDesign(t *testing.T) {
	src := "package p\n\nfunc f() {\n" +
		"\tnew := func(x int) int { return x }\n" +
		"\tdirInfo := 5\n" +
		"\ty := new(dirInfo)\n" +
		"\t_ = y\n" +
		"}\n"
	tree, lang := parseGo(t, src)
	root := tree.RootNode()
	assertNoErrorOrMissing(t, lang, root, "shadowed-new-call-matches-oracle")

	calls := findAllNodesOfType(lang, root, "call_expression")
	var shadowedCall *gotreesitter.Node
	source := tree.Source()
	for _, call := range calls {
		fn := call.ChildByFieldName("function", lang)
		if fn != nil && fn.Text(source) == "new" {
			shadowedCall = call
			break
		}
	}
	if shadowedCall == nil {
		t.Fatalf("no call_expression with function text %q found; root=%s", "new", root.SExpr(lang))
	}
	args := shadowedCall.ChildByFieldName("arguments", lang)
	if args == nil || args.NamedChildCount() != 1 {
		t.Fatalf("shadowed new(dirInfo) call: unexpected arguments shape; root=%s", root.SExpr(lang))
	}
	if got := args.NamedChild(0).Type(lang); got != "type_identifier" {
		t.Fatalf("shadowed new(dirInfo) sole argument Type() = %q, want %q (must match C oracle); root=%s", got, "type_identifier", root.SExpr(lang))
	}
}

// TestParseGoNewMakeGenericInstantiationUntouched confirms
// normalizeGoNewMakeTypeArgument has zero interaction with generic type
// instantiation syntax: `Foo[int]{}` is a composite_literal wrapping a
// generic_type, and `Foo[int](x)` is a type_conversion_expression wrapping a
// generic_type — NEITHER produces a call_expression node at all (the retag
// walk only ever matches n.symbol == callSym), so type_arguments/
// composite_literal handling is structurally guaranteed to be untouched.
// Verified against the real tree-sitter-go C oracle: identical node shapes,
// no call_expression present for either form.
func TestParseGoNewMakeGenericInstantiationUntouched(t *testing.T) {
	src := "package p\n\n" +
		"type Foo[T any] struct {\n\tV T\n}\n\n" +
		"func f() {\n" +
		"\ta := Foo[int]{}\n" +
		"\tb := Foo[int](a)\n" +
		"\t_ = a\n" +
		"\t_ = b\n" +
		"}\n"
	// Assert the production-engine Go tree shape against the tree-sitter-go C
	// oracle. The Phase-3 admission flip surfaced a compact candidate-route
	// divergence on `Foo[int](a)` (call_expression versus the correct
	// type_conversion_expression); the compact route now fails closed on that
	// conflict class (TestAdmissionCandidateGoTypeConversionFailsClosed). Because
	// the fix is fail-closed, this fixture is re-routed through the candidate:
	// the route must reproduce the production tree byte for byte, whether it
	// routes or declines to production.
	tree, lang := parseGoProduction(t, src)
	root := tree.RootNode()
	assertNoErrorOrMissing(t, lang, root, "generic-instantiation-untouched")
	assertGoCandidateMatchesProduction(t, lang, src, tree)

	if calls := findAllNodesOfType(lang, root, "call_expression"); len(calls) != 0 {
		t.Fatalf("expected zero call_expression nodes for generic instantiation syntax, got %d; root=%s", len(calls), root.SExpr(lang))
	}

	composite := findNamedChild(lang, root, "composite_literal")
	if composite == nil {
		t.Fatalf("no composite_literal found for Foo[int]{}; root=%s", root.SExpr(lang))
	}
	genType := composite.ChildByFieldName("type", lang)
	if genType == nil || genType.Type(lang) != "generic_type" {
		t.Fatalf("composite_literal type field = %v, want generic_type; root=%s", genType, root.SExpr(lang))
	}
	if got := genType.ChildByFieldName("type", lang); got == nil || got.Type(lang) != "type_identifier" {
		t.Fatalf("generic_type type field not type_identifier; root=%s", root.SExpr(lang))
	}
	if got := composite.ChildByFieldName("body", lang); got == nil || got.Type(lang) != "literal_value" {
		t.Fatalf("composite_literal body field not literal_value; root=%s", root.SExpr(lang))
	}

	conv := findNamedChild(lang, root, "type_conversion_expression")
	if conv == nil {
		t.Fatalf("no type_conversion_expression found for Foo[int](a); root=%s", root.SExpr(lang))
	}
	convType := conv.ChildByFieldName("type", lang)
	if convType == nil || convType.Type(lang) != "generic_type" {
		t.Fatalf("type_conversion_expression type field = %v, want generic_type; root=%s", convType, root.SExpr(lang))
	}
}

// TestParseGoNewMakeFixDoesNotRegressCompositeLiterals is the composite-
// literal regression guard called for by the review: this is the exact
// syntactic shape (ordinary struct composite literals: bare, with a keyed
// field, and address-of) that a broader "engine-level" fix to the
// grammargen LALR/GLR conflict resolution (the rejected alternative
// approach) could plausibly regress, because it would touch shared
// reduce/reduce resolution machinery used well beyond the narrow new/make
// call shape. The narrow post-parse retag in normalizeGoNewMakeTypeArgument
// only ever mutates the sole/leading argument of a literal new()/make()
// call_expression — composite_literal has an entirely different symbol and
// is never visited by that walk — so this must stay green both today and
// as a tripwire for any future attempt to replace the narrow retag with a
// broader engine fix.
func TestParseGoNewMakeFixDoesNotRegressCompositeLiterals(t *testing.T) {
	src := "package p\n\n" +
		"type Pattern struct {\n\tX int\n}\n\n" +
		"func f() {\n" +
		"\t_ = Pattern{}\n" +
		"\t_ = Pattern{X: 1}\n" +
		"\t_ = &Pattern{}\n" +
		"}\n"
	tree, lang := parseGo(t, src)
	root := tree.RootNode()
	assertNoErrorOrMissing(t, lang, root, "composite-literal-regression-guard")

	composites := findAllNodesOfType(lang, root, "composite_literal")
	if len(composites) != 3 {
		t.Fatalf("expected 3 composite_literal nodes (bare, keyed, address-of), got %d; root=%s", len(composites), root.SExpr(lang))
	}
	for i, c := range composites {
		typeField := c.ChildByFieldName("type", lang)
		if typeField == nil || typeField.Type(lang) != "type_identifier" {
			t.Fatalf("composite_literal[%d] type field = %v, want type_identifier; root=%s", i, typeField, root.SExpr(lang))
		}
		if typeField.Text(tree.Source()) != "Pattern" {
			t.Fatalf("composite_literal[%d] type field text = %q, want %q", i, typeField.Text(tree.Source()), "Pattern")
		}
		body := c.ChildByFieldName("body", lang)
		if body == nil || body.Type(lang) != "literal_value" {
			t.Fatalf("composite_literal[%d] body field = %v, want literal_value; root=%s", i, body, root.SExpr(lang))
		}
	}

	// The second composite literal (Pattern{X: 1}) must additionally carry
	// a keyed_element with field key "X" -- confirms the retag's field-name
	// lookups (ChildByFieldName("arguments")/("function")) elsewhere in the
	// same tree-build pipeline haven't disturbed unrelated field wiring.
	keyed := findNamedChild(lang, composites[1].ChildByFieldName("body", lang), "keyed_element")
	if keyed == nil {
		t.Fatalf("expected keyed_element inside Pattern{X: 1}; root=%s", root.SExpr(lang))
	}
}

// TestParseGoNewMakeSoleArgumentIsTypeIdentifierIncremental is the
// incremental counterpart to TestParseGoNewMakeSoleArgumentIsTypeIdentifier
// (parser_go_test.go): normalizeGoNewMakeTypeArgument runs inside
// normalizeGoReturnedTreeCompatibilityWithCensus, which every Go result-tree build
// path invokes (parser_result_root_build.go / parser_result_compat.go),
// fresh AND incremental alike. This drives an actual incremental re-parse
// (via ParseIncrementalWithTokenSource, matching the existing
// TestParseGoIncrementalWithTokenSourceReusesSubtrees pattern) so the
// retag is proven to apply correctly to freshly-reparsed subtrees produced
// by the incremental path too, not just a from-scratch parse. It also
// cross-checks byte-for-byte against a from-scratch parse of the
// post-edit source.
func TestParseGoNewMakeSoleArgumentIsTypeIdentifierIncremental(t *testing.T) {
	lang := grammars.GoLanguage()
	// Drives GoTokenSource incremental reuse; ts2go-specific. Skip when the
	// current Go blob is grammargen-compiled (mirrors the guard used by
	// TestParseGoIncrementalWithTokenSourceReusesSubtrees /
	// TestParseGoIncrementalRangeClauseReturnEdit above).
	if _, ok := lang.SymbolByName("source_file_token1"); !ok {
		t.Skip("GoTokenSource is ts2go-specific; current Go blob is grammargen-compiled")
	}

	for _, tc := range []struct {
		name       string
		before     string
		after      string
		editMarker string // substring immediately preceding the edited byte
	}{
		{
			name:       "new-user-type",
			before:     "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\td := new(dirInfoX)\n\t_ = d\n}\n",
			after:      "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\td := new(dirInfo)\n\t_ = d\n}\n",
			editMarker: "new(dirInfo",
		},
		{
			name:       "new-builtin-primitive",
			before:     "package p\n\nfunc f() {\n\td := new(intx)\n\t_ = d\n}\n",
			after:      "package p\n\nfunc f() {\n\td := new(int)\n\t_ = d\n}\n",
			editMarker: "new(int",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			before := []byte(tc.before)
			after := []byte(tc.after)

			editAt := bytes.Index(before, []byte(tc.editMarker)) + len(tc.editMarker)
			if editAt < len(tc.editMarker) {
				t.Fatalf("could not find edit marker %q in before source", tc.editMarker)
			}
			start := pointAtOffset(before, editAt)
			end := pointAtOffset(before, editAt+1)
			edit := gotreesitter.InputEdit{
				StartByte:   uint32(editAt),
				OldEndByte:  uint32(editAt + 1),
				NewEndByte:  uint32(editAt),
				StartPoint:  start,
				OldEndPoint: end,
				NewEndPoint: start,
			}

			ts := mustGoTokenSource(t, before, lang)
			oldTree, err := parser.ParseWithTokenSource(before, ts)
			if err != nil {
				t.Fatalf("initial ParseWithTokenSource failed: %v", err)
			}
			if oldTree.RootNode() == nil {
				t.Fatal("initial parse returned nil root")
			}

			oldTree.Edit(edit)
			ts.Reset(after)
			newTree, err := parser.ParseIncrementalWithTokenSource(after, oldTree, ts)
			if err != nil {
				t.Fatalf("ParseIncrementalWithTokenSource failed: %v", err)
			}
			if newTree.RootNode() == nil {
				t.Fatal("incremental parse returned nil root")
			}
			incRoot := newTree.RootNode()
			assertNoErrorOrMissing(t, lang, incRoot, tc.name+"/incremental")

			incCall := findNamedChild(lang, incRoot, "call_expression")
			if incCall == nil {
				t.Fatalf("incremental: no call_expression found; root=%s", incRoot.SExpr(lang))
			}
			incArgs := incCall.ChildByFieldName("arguments", lang)
			if incArgs == nil || incArgs.NamedChildCount() == 0 {
				t.Fatalf("incremental: call_expression has no arguments; root=%s", incRoot.SExpr(lang))
			}
			if got := incArgs.NamedChild(0).Type(lang); got != "type_identifier" {
				t.Fatalf("incremental: sole new/make argument Type() = %q, want %q; root=%s", got, "type_identifier", incRoot.SExpr(lang))
			}

			// Cross-check against a from-scratch parse of the post-edit
			// source: both must agree not just on the retag but on the
			// whole tree text.
			freshTS := mustGoTokenSource(t, after, lang)
			freshTree, err := parser.ParseWithTokenSource(after, freshTS)
			if err != nil {
				t.Fatalf("fresh ParseWithTokenSource failed: %v", err)
			}
			freshRoot := freshTree.RootNode()
			assertNoErrorOrMissing(t, lang, freshRoot, tc.name+"/fresh")

			freshCall := findNamedChild(lang, freshRoot, "call_expression")
			if freshCall == nil {
				t.Fatalf("fresh: no call_expression found; root=%s", freshRoot.SExpr(lang))
			}
			freshArgs := freshCall.ChildByFieldName("arguments", lang)
			if freshArgs == nil || freshArgs.NamedChildCount() == 0 {
				t.Fatalf("fresh: call_expression has no arguments; root=%s", freshRoot.SExpr(lang))
			}
			if got := freshArgs.NamedChild(0).Type(lang); got != "type_identifier" {
				t.Fatalf("fresh: sole new/make argument Type() = %q, want %q; root=%s", got, "type_identifier", freshRoot.SExpr(lang))
			}

			if got, want := incRoot.Text(after), freshRoot.Text(after); got != want {
				t.Fatalf("incremental root text mismatch with fresh parse:\nincremental=%q\nfresh=%q", got, want)
			}
		})
	}
}

// TestParseGoNewMakeSoleArgumentIsTypeIdentifierIncrementalPlain is a
// blob-agnostic sibling of TestParseGoNewMakeSoleArgumentIsTypeIdentifierIncremental:
// it drives the incremental path through the plain Parser.Parse /
// Parser.ParseIncremental API (no GoTokenSource), so unlike the
// ts2go-specific test above it is NOT skipped when the active Go blob is
// grammargen-compiled (the default in this checkout — see
// TestParseGoIncrementalWithTokenSourceReusesSubtrees's skip guard). This is
// what actually exercises "fresh AND incremental" end to end in this repo's
// default build configuration.
func TestParseGoNewMakeSoleArgumentIsTypeIdentifierIncrementalPlain(t *testing.T) {
	lang := grammars.GoLanguage()

	for _, tc := range []struct {
		name       string
		before     string
		after      string
		editMarker string // substring immediately preceding the edited byte
	}{
		{
			name:       "new-user-type",
			before:     "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\td := new(dirInfoX)\n\t_ = d\n}\n",
			after:      "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\td := new(dirInfo)\n\t_ = d\n}\n",
			editMarker: "new(dirInfo",
		},
		{
			name:       "new-builtin-primitive",
			before:     "package p\n\nfunc f() {\n\td := new(intx)\n\t_ = d\n}\n",
			after:      "package p\n\nfunc f() {\n\td := new(int)\n\t_ = d\n}\n",
			editMarker: "new(int",
		},
		{
			name:       "unrelated-edit-far-from-call-still-retagged-on-rewalk",
			before:     "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\tx := 0\n\td := new(dirInfo)\n\t_ = d\n\t_ = x\n}\n",
			after:      "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\tx := 1\n\td := new(dirInfo)\n\t_ = d\n\t_ = x\n}\n",
			editMarker: "x := 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			before := []byte(tc.before)
			after := []byte(tc.after)

			editAt := bytes.Index(before, []byte(tc.editMarker)) + len(tc.editMarker) - 1
			if editAt < 0 {
				t.Fatalf("could not find edit marker %q in before source", tc.editMarker)
			}
			start := pointAtOffset(before, editAt)
			end := pointAtOffset(before, editAt+1)
			edit := gotreesitter.InputEdit{
				StartByte:   uint32(editAt),
				OldEndByte:  uint32(editAt + 1),
				NewEndByte:  uint32(editAt + 1),
				StartPoint:  start,
				OldEndPoint: end,
				NewEndPoint: end,
			}

			oldTree, err := parser.Parse(before)
			if err != nil {
				t.Fatalf("initial Parse failed: %v", err)
			}
			if oldTree.RootNode() == nil {
				t.Fatal("initial parse returned nil root")
			}

			oldTree.Edit(edit)
			newTree, err := parser.ParseIncremental(after, oldTree)
			if err != nil {
				t.Fatalf("ParseIncremental failed: %v", err)
			}
			if newTree.RootNode() == nil {
				t.Fatal("incremental parse returned nil root")
			}
			incRoot := newTree.RootNode()
			assertNoErrorOrMissing(t, lang, incRoot, tc.name+"/incremental")

			incCall := findNamedChild(lang, incRoot, "call_expression")
			if incCall == nil {
				t.Fatalf("incremental: no call_expression found; root=%s", incRoot.SExpr(lang))
			}
			incArgs := incCall.ChildByFieldName("arguments", lang)
			if incArgs == nil || incArgs.NamedChildCount() == 0 {
				t.Fatalf("incremental: call_expression has no arguments; root=%s", incRoot.SExpr(lang))
			}
			if got := incArgs.NamedChild(0).Type(lang); got != "type_identifier" {
				t.Fatalf("incremental: sole new/make argument Type() = %q, want %q; root=%s", got, "type_identifier", incRoot.SExpr(lang))
			}

			// Cross-check against a from-scratch parse of the post-edit
			// source.
			freshParser := gotreesitter.NewParser(lang)
			freshTree, err := freshParser.Parse(after)
			if err != nil {
				t.Fatalf("fresh Parse failed: %v", err)
			}
			freshRoot := freshTree.RootNode()
			assertNoErrorOrMissing(t, lang, freshRoot, tc.name+"/fresh")

			freshCall := findNamedChild(lang, freshRoot, "call_expression")
			if freshCall == nil {
				t.Fatalf("fresh: no call_expression found; root=%s", freshRoot.SExpr(lang))
			}
			freshArgs := freshCall.ChildByFieldName("arguments", lang)
			if freshArgs == nil || freshArgs.NamedChildCount() == 0 {
				t.Fatalf("fresh: call_expression has no arguments; root=%s", freshRoot.SExpr(lang))
			}
			if got := freshArgs.NamedChild(0).Type(lang); got != "type_identifier" {
				t.Fatalf("fresh: sole new/make argument Type() = %q, want %q; root=%s", got, "type_identifier", freshRoot.SExpr(lang))
			}

			if got, want := incRoot.Text(after), freshRoot.Text(after); got != want {
				t.Fatalf("incremental root text mismatch with fresh parse:\nincremental=%q\nfresh=%q", got, want)
			}
		})
	}
}
