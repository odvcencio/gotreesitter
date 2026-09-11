package gotreesitter_test

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// newMakeCorpusEntry pins the leading argument shape and root error status.
// TestGoNewMakeLockedC in cgo_harness/go_newmake_locked_c_test.go compares complete trees with C.
type newMakeCorpusEntry struct {
	form          string
	wantLeadSExpr string
	wantError     bool
	note          string
}

// Preserve all 30 forms, including the remaining malformed recovery divergence.
var goNewMakeDiffCorpus = []newMakeCorpusEntry{
	// Qualified, pointer, and parenthesized types.
	{"new(pkg.Type)", "(qualified_type (package_identifier) (type_identifier))", false, "fixed-residual"},
	{"new(*T)", "(pointer_type (type_identifier))", false, "fixed-residual"},
	{"new(*pkg.Type)", "(pointer_type (qualified_type (package_identifier) (type_identifier)))", false, "fixed-residual"},
	{"new(**T)", "(pointer_type (pointer_type (type_identifier)))", false, "fixed-residual"},
	{"new(a.b)", "(qualified_type (package_identifier) (type_identifier))", false, "fixed-residual"},
	{"new((T))", "(parenthesized_type (type_identifier))", false, "fixed-residual"},
	{"new((pkg.Type))", "(parenthesized_type (qualified_type (package_identifier) (type_identifier)))", false, "fixed-residual"},
	{"new((*T))", "(parenthesized_type (pointer_type (type_identifier)))", false, "fixed-residual"},
	{"make(pkg.Type, 0)", "(qualified_type (package_identifier) (type_identifier))", false, "fixed-residual"},
	{"make(*T, 0)", "(pointer_type (type_identifier))", false, "fixed-residual"},
	// Bare type arguments.
	{"new(T)", "(type_identifier)", false, "base"},
	{"new(dirInfo)", "(type_identifier)", false, "base"},
	{"make(T)", "(type_identifier)", false, "base"},
	{"new(a, b)", "(type_identifier)", false, "base"}, // invalid-Go arity, still a type in slot 0
	{"make(a, b, c)", "(type_identifier)", false, "base"},
	// Composite type controls.
	{"new([]T)", "(slice_type (type_identifier))", false, "composite"},
	{"new(*[]T)", "(pointer_type (slice_type (type_identifier)))", false, "composite"},
	{"new([]*T)", "(slice_type (pointer_type (type_identifier)))", false, "composite"},
	{"new(map[K]V)", "(map_type (type_identifier) (type_identifier))", false, "composite"},
	{"new(*map[K]V)", "(pointer_type (map_type (type_identifier) (type_identifier)))", false, "composite"},
	{"new(chan T)", "(channel_type (type_identifier))", false, "composite"},
	{"new(*chan T)", "(pointer_type (channel_type (type_identifier)))", false, "composite"},
	{"new(struct{})", "(struct_type (field_declaration_list))", false, "composite"},
	{"new(interface{})", "(interface_type)", false, "composite"},
	{"new(func())", "(function_type (parameter_list))", false, "composite"},
	{"new([3]T)", "(array_type (int_literal) (type_identifier))", false, "composite"},
	{"new(pkg.Type[int])", "(generic_type (qualified_type (package_identifier) (type_identifier)) (type_arguments (type_elem (type_identifier))))", false, "composite"},
	{"make([]pkg.Type, 0)", "(slice_type (qualified_type (package_identifier) (type_identifier)))", false, "composite"},
	// Malformed forms retain the locked C recovery expectations.
	{"new(a.b.C)", "(ERROR (qualified_type (package_identifier) (type_identifier)))", true, "malformed"},
	{"new(&T)", "(ERROR)", true, "malformed"},
}

// TestParseGoNewMakeTypeArgumentDiffCorpus checks the locked C argument expectations.
func TestParseGoNewMakeTypeArgumentDiffCorpus(t *testing.T) {
	for _, e := range goNewMakeDiffCorpus {
		e := e
		t.Run(e.form, func(t *testing.T) {
			src := "package p\n\nfunc f() {\n\t_ = " + e.form + "\n}\n"
			tree, lang := parseGo(t, src)
			root := tree.RootNode()
			if root.HasError() != e.wantError {
				t.Fatalf("%s: error=%v, want %v", e.form, root.HasError(), e.wantError)
			}
			if !e.wantError {
				assertNoErrorOrMissing(t, lang, root, e.form)
			}

			call := findNamedChild(lang, root, "call_expression")
			if call == nil {
				t.Fatalf("%s: no call_expression; root=%s", e.form, root.SExpr(lang))
			}
			args := call.ChildByFieldName("arguments", lang)
			if args == nil {
				t.Fatalf("%s: no arguments field; root=%s", e.form, root.SExpr(lang))
			}
			lead := args.NamedChild(0)
			if lead == nil {
				t.Fatalf("%s: argument_list has no leading argument; root=%s", e.form, root.SExpr(lang))
			}
			if got := lead.SExpr(lang); got != e.wantLeadSExpr {
				t.Fatalf("%s (%s): leading argument shape mismatch\n got: %s\nwant: %s", e.form, e.note, got, e.wantLeadSExpr)
			}
		})
	}
}

// TestParseGoNewMakeResidualCompositeLiteralGuards is the composite-literal
// regression suite the #384 review flagged as the exact hole a broader
// engine-level fix would blow open (Pattern{}, &Pattern{}, []T{{...}},
// map[K]V{...}, the `if x {}` LBRACE disambiguation, and nested composites).
// The narrow retag only relabels new()/make() argument nodes, so composite
// literals are structurally untouched; this is the tripwire that proves it and
// guards against any future engine-level replacement.
func TestParseGoNewMakeResidualCompositeLiteralGuards(t *testing.T) {
	prelude := "package p\n\n" +
		"type T struct{ X int }\n" +
		"type K int\n" +
		"type V int\n" +
		"type Pattern struct{ X int }\n" +
		"type Inner struct{ V int }\n" +
		"type Outer struct{ Inner Inner }\n\n"

	for _, tc := range []struct {
		name     string
		body     string
		wantType string // Type() of the guarded statement node
		wantSub  string // a substring that must appear in that statement's SExpr
	}{
		{"bare-composite", "_ = Pattern{}", "assignment_statement", "(composite_literal (type_identifier) (literal_value))"},
		{"keyed-composite", "_ = Pattern{X: 1}", "assignment_statement", "(keyed_element (literal_element (identifier)) (literal_element (int_literal)))"},
		{"address-of-composite", "_ = &Pattern{}", "assignment_statement", "(unary_expression (composite_literal (type_identifier) (literal_value)))"},
		{"slice-nested-brace", "_ = []T{{X: 1}}", "assignment_statement", "(composite_literal (slice_type (type_identifier))"},
		{"map-literal", "_ = map[K]V{k: v}", "assignment_statement", "(composite_literal (map_type (type_identifier) (type_identifier))"},
		{"if-bare-condition", "if x {\n\t}", "if_statement", "(if_statement (identifier) (block))"},
		{"slice-of-pattern", "_ = []Pattern{{X: 1}, {X: 2}}", "assignment_statement", "(composite_literal (slice_type (type_identifier))"},
		{"if-parenthesized-composite", "if (Pattern{}).X == 1 {\n\t}", "if_statement", "(parenthesized_expression (composite_literal (type_identifier) (literal_value)))"},
		{"nested-composite", "_ = Outer{Inner: Inner{V: 1}}", "assignment_statement", "(literal_element (composite_literal (type_identifier)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := prelude + "func f() {\n\tvar x bool\n\t_ = x\n\t" + tc.body + "\n}\n"
			tree, lang := parseGo(t, src)
			root := tree.RootNode()
			assertNoErrorOrMissing(t, lang, root, tc.name)

			// The guarded statement is the last one in the function body.
			sl := findNamedChild(lang, root, "statement_list")
			if sl == nil {
				t.Fatalf("%s: no statement_list; root=%s", tc.name, root.SExpr(lang))
			}
			var stmt *gotreesitter.Node
			for i := sl.NamedChildCount() - 1; i >= 0; i-- {
				if c := sl.NamedChild(i); c != nil && c.Type(lang) == tc.wantType {
					stmt = c
					break
				}
			}
			if stmt == nil {
				t.Fatalf("%s: no %s statement found; root=%s", tc.name, tc.wantType, root.SExpr(lang))
			}
			if sx := stmt.SExpr(lang); !containsSub(sx, tc.wantSub) {
				t.Fatalf("%s: statement SExpr missing %q\n got: %s", tc.name, tc.wantSub, sx)
			}
			// The retag must never introduce a qualified_type or pointer_type
			// outside a new()/make() call. None of these composite fixtures
			// contains such a call, so neither type node may appear.
			if hasNodeOfType(lang, stmt, "qualified_type") || hasNodeOfType(lang, stmt, "pointer_type") {
				t.Fatalf("%s: composite fixture unexpectedly carries a retagged type node:\n%s", tc.name, stmt.SExpr(lang))
			}
		})
	}
}

func containsSub(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}

func hasNodeOfType(lang *gotreesitter.Language, n *gotreesitter.Node, typeName string) bool {
	if n == nil {
		return false
	}
	if n.Type(lang) == typeName {
		return true
	}
	for i := 0; i < n.ChildCount(); i++ {
		if hasNodeOfType(lang, n.Child(i), typeName) {
			return true
		}
	}
	return false
}

// TestParseGoNewMakeQualifiedPointerIncremental proves the qualified_type and
// pointer_type retag applies on the incremental parse path too, and that the
// incremental result is byte-for-byte identical to a from-scratch parse of the
// post-edit source. It mirrors the plain-API pattern of
// TestParseGoNewMakeSoleArgumentIsTypeIdentifierIncrementalPlain (blob-agnostic;
// not skipped for the grammargen blob).
func TestParseGoNewMakeQualifiedPointerIncremental(t *testing.T) {
	lang := grammars.GoLanguage()

	for _, tc := range []struct {
		name       string
		before     string
		after      string
		editMarker string // substring whose LAST byte is edited
		wantLead   string // expected SExpr of the leading new/make argument
	}{
		{
			name:       "qualified-type",
			before:     "package p\n\nfunc f() {\n\td := new(pkg.TypeX)\n\t_ = d\n}\n",
			after:      "package p\n\nfunc f() {\n\td := new(pkg.Type)\n\t_ = d\n}\n",
			editMarker: "new(pkg.Type",
			wantLead:   "(qualified_type (package_identifier) (type_identifier))",
		},
		{
			name:       "pointer-type",
			before:     "package p\n\nfunc f() {\n\td := new(*TX)\n\t_ = d\n}\n",
			after:      "package p\n\nfunc f() {\n\td := new(*T)\n\t_ = d\n}\n",
			editMarker: "new(*T",
			wantLead:   "(pointer_type (type_identifier))",
		},
		{
			name:       "pointer-to-qualified",
			before:     "package p\n\nfunc f() {\n\td := new(*pkg.TypeX)\n\t_ = d\n}\n",
			after:      "package p\n\nfunc f() {\n\td := new(*pkg.Type)\n\t_ = d\n}\n",
			editMarker: "new(*pkg.Type",
			wantLead:   "(pointer_type (qualified_type (package_identifier) (type_identifier)))",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			before := []byte(tc.before)
			after := []byte(tc.after)

			// Delete the trailing "X" that the before/after differ by.
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

			oldTree, err := parser.Parse(before)
			if err != nil {
				t.Fatalf("initial Parse failed: %v", err)
			}
			oldTree.Edit(edit)
			newTree, err := parser.ParseIncremental(after, oldTree)
			if err != nil {
				t.Fatalf("ParseIncremental failed: %v", err)
			}
			incRoot := newTree.RootNode()
			assertNoErrorOrMissing(t, lang, incRoot, tc.name+"/incremental")

			gotInc := leadNewMakeArgSExpr(t, lang, incRoot)
			if gotInc != tc.wantLead {
				t.Fatalf("incremental leading argument shape mismatch\n got: %s\nwant: %s\nroot=%s", gotInc, tc.wantLead, incRoot.SExpr(lang))
			}

			freshParser := gotreesitter.NewParser(lang)
			freshTree, err := freshParser.Parse(after)
			if err != nil {
				t.Fatalf("fresh Parse failed: %v", err)
			}
			freshRoot := freshTree.RootNode()
			assertNoErrorOrMissing(t, lang, freshRoot, tc.name+"/fresh")
			gotFresh := leadNewMakeArgSExpr(t, lang, freshRoot)
			if gotFresh != tc.wantLead {
				t.Fatalf("fresh leading argument shape mismatch\n got: %s\nwant: %s", gotFresh, tc.wantLead)
			}
			if inc, fresh := incRoot.SExpr(lang), freshRoot.SExpr(lang); inc != fresh {
				t.Fatalf("incremental tree diverges from fresh parse:\ninc=%s\nfresh=%s", inc, fresh)
			}
			if inc, fresh := incRoot.Text(after), freshRoot.Text(after); inc != fresh {
				t.Fatalf("incremental root text mismatch with fresh parse:\ninc=%q\nfresh=%q", inc, fresh)
			}
		})
	}
}

func leadNewMakeArgSExpr(t *testing.T, lang *gotreesitter.Language, root *gotreesitter.Node) string {
	t.Helper()
	call := findNamedChild(lang, root, "call_expression")
	if call == nil {
		t.Fatalf("no call_expression; root=%s", root.SExpr(lang))
	}
	args := call.ChildByFieldName("arguments", lang)
	if args == nil || args.NamedChildCount() == 0 {
		t.Fatalf("call_expression has no arguments; root=%s", root.SExpr(lang))
	}
	return args.NamedChild(0).SExpr(lang)
}
