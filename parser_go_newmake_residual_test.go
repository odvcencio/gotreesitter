package gotreesitter_test

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// newMakeCorpusEntry is one fixture in the durable Go new()/make() type-argument
// DIFF corpus. wantLeadSExpr is the S-expression (named nodes only) of the
// leading argument that normalizeGoNewMakeTypeArgument must produce. matchesC
// records whether that shape byte-matches the pinned tree-sitter-go C oracle;
// every entry with matchesC=true was verified node-for-node against the C
// reference in cgo_harness/zz_go_newmake_residual_parity_test.go
// (TestGoNewMakeResidualParity).
type newMakeCorpusEntry struct {
	form          string // the expression spliced into `_ = <form>`
	wantLeadSExpr string
	matchesC      bool
	note          string
}

// goNewMakeDiffCorpus is the committed successor to the ephemeral 30-fixture
// DIFF corpus used during PR #384 review (that corpus was never checked in).
// It fixes the shape of every new()/make() leading argument this normalization
// governs, so any future regression in the retag (or in a broader engine
// change) trips here. Categories:
//
//   - fixed-residual: qualified_type / pointer_type / parenthesized_type forms
//     this change adds (`new(pkg.Type)`, `new(*T)`, ...). Divergent before.
//   - base: bare-identifier forms fixed by PR #384 (`new(T)`).
//   - composite: unambiguous composite types already correct pre-#384; kept as
//     regression controls (the retag must never disturb them).
//   - invalid-error: invalid Go (`new(a.b.C)`, `new(&T)`) where the C oracle
//     drives strict type-context ERROR recovery. The shipped blob is built from
//     upstream's grammar text and recovers the same way; the tree must carry an
//     error (cgo_harness TestGoNewMakeResidualParity compares it with C). The
//     earlier blob kept a clean expression parse here.
var goNewMakeDiffCorpus = []newMakeCorpusEntry{
	// fixed-residual (this change)
	{"new(pkg.Type)", "(qualified_type (package_identifier) (type_identifier))", true, "fixed-residual"},
	{"new(*T)", "(pointer_type (type_identifier))", true, "fixed-residual"},
	{"new(*pkg.Type)", "(pointer_type (qualified_type (package_identifier) (type_identifier)))", true, "fixed-residual"},
	{"new(**T)", "(pointer_type (pointer_type (type_identifier)))", true, "fixed-residual"},
	{"new(a.b)", "(qualified_type (package_identifier) (type_identifier))", true, "fixed-residual"},
	{"new((T))", "(parenthesized_type (type_identifier))", true, "fixed-residual"},
	{"new((pkg.Type))", "(parenthesized_type (qualified_type (package_identifier) (type_identifier)))", true, "fixed-residual"},
	{"new((*T))", "(parenthesized_type (pointer_type (type_identifier)))", true, "fixed-residual"},
	{"make(pkg.Type, 0)", "(qualified_type (package_identifier) (type_identifier))", true, "fixed-residual"},
	{"make(*T, 0)", "(pointer_type (type_identifier))", true, "fixed-residual"},
	// base (PR #384)
	{"new(T)", "(type_identifier)", true, "base"},
	{"new(dirInfo)", "(type_identifier)", true, "base"},
	{"make(T)", "(type_identifier)", true, "base"},
	{"new(a, b)", "(type_identifier)", true, "base"}, // invalid-Go arity, still a type in slot 0
	{"make(a, b, c)", "(type_identifier)", true, "base"},
	// composite (regression controls)
	{"new([]T)", "(slice_type (type_identifier))", true, "composite"},
	{"new(*[]T)", "(pointer_type (slice_type (type_identifier)))", true, "composite"},
	{"new([]*T)", "(slice_type (pointer_type (type_identifier)))", true, "composite"},
	{"new(map[K]V)", "(map_type (type_identifier) (type_identifier))", true, "composite"},
	{"new(*map[K]V)", "(pointer_type (map_type (type_identifier) (type_identifier)))", true, "composite"},
	{"new(chan T)", "(channel_type (type_identifier))", true, "composite"},
	{"new(*chan T)", "(pointer_type (channel_type (type_identifier)))", true, "composite"},
	{"new(struct{})", "(struct_type (field_declaration_list))", true, "composite"},
	{"new(interface{})", "(interface_type)", true, "composite"},
	{"new(func())", "(function_type (parameter_list))", true, "composite"},
	{"new([3]T)", "(array_type (int_literal) (type_identifier))", true, "composite"},
	{"new(pkg.Type[int])", "(generic_type (qualified_type (package_identifier) (type_identifier)) (type_arguments (type_elem (type_identifier))))", true, "composite"},
	{"make([]pkg.Type, 0)", "(slice_type (qualified_type (package_identifier) (type_identifier)))", true, "composite"},
	// invalid-error (the tree has an error, as in the C oracle)
	{"new(a.b.C)", "", true, "invalid-error"},
	{"new(&T)", "", true, "invalid-error"},
}

// TestParseGoNewMakeTypeArgumentDiffCorpus is the durable scorecard for the Go
// new()/make() leading-argument retag. It pins the exact node shape of every
// corpus form and reports the count of forms that byte-match the C oracle.
func TestParseGoNewMakeTypeArgumentDiffCorpus(t *testing.T) {
	matchC := 0
	for _, e := range goNewMakeDiffCorpus {
		e := e
		t.Run(e.form, func(t *testing.T) {
			src := "package p\n\nfunc f() {\n\t_ = " + e.form + "\n}\n"
			tree, lang := parseGo(t, src)
			root := tree.RootNode()
			if e.note == "invalid-error" {
				if !root.HasError() {
					t.Fatalf("%s: invalid Go parsed without an error; root=%s", e.form, root.SExpr(lang))
				}
				return
			}
			assertNoErrorOrMissing(t, lang, root, e.form)

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
		if e.matchesC {
			matchC++
		}
	}
	t.Logf("Go new/make DIFF corpus: %d/%d forms byte-match the C oracle (%d documented invalid-Go divergences)",
		matchC, len(goNewMakeDiffCorpus), len(goNewMakeDiffCorpus)-matchC)
	if matchC != len(goNewMakeDiffCorpus) {
		t.Fatalf("%d forms are recorded as differing from the C oracle; the shipped Go tables equal it", len(goNewMakeDiffCorpus)-matchC)
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
