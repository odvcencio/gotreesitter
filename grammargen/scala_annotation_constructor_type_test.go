package grammargen

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestScalaAnnotationOwnsArgumentsNotAppliedConstructorType pins the
// smallest verified generator/locked-C mismatch recorded in issue #1067.
//
// Source `class A@a()\n` exercises a shift/reduce conflict at lookahead `(`
// between:
//   - reducing `_simple_type -> _type_identifier` (letting `annotation`'s own
//     `arguments` field own the following `(...)`), and
//   - shifting into `applied_constructor_type` (folding the `(...)` into
//     `_simple_type` itself).
//
// Locked tree-sitter C reduces here, producing `type_identifier` and
// `arguments` as direct children of `annotation`. The live generator shifts,
// wrapping them in `applied_constructor_type`.
//
// `preferredVisibleSiblingAlternativeContinuationShift` in grammargen/lr.go
// forces the shift before ordinary precedence resolution runs. That same
// shift is required for `val c: C(42)\n`, which uses the identical
// `_simple_type -> _type_identifier` reduce against the identical shift
// action (confirmed by state 1311 sharing shift.lhsSyms=[parameters,
// _using_parameters_clause, arguments, _given_constructor,
// applied_constructor_type, _given_constructor_repeat32] across every call
// site with this shape in the generated table, annotation's own site
// included). Disabling the heuristic fixes the annotation case but corrupts
// `val_declaration` parsing outright, because the conflict is resolved once
// for a single merged LALR state shared by both contexts.
//
// This test intentionally pins BOTH outcomes so a future fix cannot regress
// `val c: C(42)\n` while repairing the annotation case. See issue #1067 for
// the full investigation, including the state-sharing evidence and the
// rejected global-precedence-veto experiment.
func TestScalaAnnotationOwnsArgumentsNotAppliedConstructorType(t *testing.T) {
	// Pinned to tree-sitter-scala commit 97aead18d97708190a51d4f551ea9b05b60641c9,
	// the commit the locked/embedded Scala blob (grammars/grammar_blobs/scala.bin)
	// was generated from. Populate this path (for example via
	// cgo_harness/docker/run_grammargen_gates.sh, which clones tree-sitter-scala)
	// before running this test; it skips when the fixture is absent, matching
	// the existing TestGapDiagnostic convention in gap_resolve_test.go.
	const jsonPath = "/tmp/grammar_parity/scala/src/grammar.json"
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("no grammar.json at %s: %v", jsonPath, err)
	}
	g, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	blob, err := Generate(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	genLang, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("load generated blob: %v", err)
	}
	refLang := grammars.ScalaLanguage()

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "annotation_owns_arguments",
			src:  "class A@a()\n",
			// Locked C: annotation's own `arguments` field consumes `()`
			// directly; `_simple_type` reduces to a bare `type_identifier`.
			want: "(compilation_unit (class_definition (identifier) (annotation (type_identifier) (arguments))))",
		},
		{
			name: "type_ascription_keeps_applied_constructor_type",
			src:  "val c: C(42)\n",
			// Locked C: no enclosing production owns the trailing `(42)`, so
			// `_simple_type` must shift into `applied_constructor_type` to
			// attach it at all. This must stay exact.
			want: "(compilation_unit (val_declaration (identifier) (applied_constructor_type (type_identifier) (arguments (integer_literal)))))",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			genParser := gotreesitter.NewParser(genLang)
			refParser := gotreesitter.NewParser(refLang)
			genTree, _ := genParser.Parse([]byte(tc.src))
			refTree, _ := refParser.Parse([]byte(tc.src))
			genSexp := genTree.RootNode().SExpr(genLang)
			refSexp := refTree.RootNode().SExpr(refLang)

			if refSexp != tc.want {
				t.Fatalf("locked-C S-expression drifted from the pinned witness for %q:\n  got:  %s\n  want: %s",
					tc.src, refSexp, tc.want)
			}
			if genSexp != tc.want {
				t.Errorf("generated S-expression mismatch for %q:\n  got:  %s\n  want: %s (locked C)",
					tc.src, genSexp, tc.want)
			}
		})
	}
}
