//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAdaConstraintKindElectionCParity records, per constraint shape,
// whether Ada discriminant and index constraint election matches the pinned
// C reference grammar on the raw parse route (compat tail off) and on the
// production route (compat tail on, dispatch.ada.constraint-kind-election in
// parser_result_ada.go included).
//
//   - Named associations ("F => Pkg.Obj'Access") are unambiguous
//     (index_constraint's discrete_range list carries no
//     discriminant_selector_name/'=>' pair, so index_constraint is never a
//     legal reading here): both routes match the C oracle. The subpass now
//     gates its rewrite on the single-positional-association shape, so it no
//     longer engages on a named association at all.
//   - Positional (unnamed) constraint values that are
//     'Access/'Delta/'Digits/'Mod attribute references, outside an
//     allocator (object declarations, subtype declarations, record
//     component declarations), stay load-bearing: the raw route elects
//     discriminant_constraint wrapping a discriminant_association where the
//     C oracle elects index_constraint directly, and the production route's
//     rewrite produces the exact C shape, subtype_mark/prefix/selector_name
//     fields included.
//
// dispatch.ada.constraint-kind-election remains load-bearing for the
// positional attribute-reference shapes; it is a no-op producer for every
// other shape this suite exercises.
func TestAdaConstraintKindElectionCParity(t *testing.T) {
	goLang := grammars.AdaLanguage()
	cLang, err := COracleLanguage("ada")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		source string
		// wantRawMismatch/wantProductionMismatch record whether the raw
		// (compat tail off) and production (compat tail on) routes are
		// currently expected to match the C oracle exactly. A subtest fails
		// loudly the moment reality stops matching the recorded
		// expectation, in either direction.
		wantRawMismatch        bool
		wantProductionMismatch bool
		note                   string
	}{
		{
			name:                   "named_allocator_witness_unaffected",
			source:                 "procedure P is\nbegin\n   A := new T (F => Pkg.Obj'Access);\nend;\n",
			wantRawMismatch:        false,
			wantProductionMismatch: false,
			note:                   "named association is unambiguous (discriminant_association is the only legal shape); the subpass no longer engages on a multi-child (named) association.",
		},
		{
			name:                   "named_object_decl_unaffected",
			source:                 "procedure P is\n   X : Rec (F => Pkg.Obj'Access);\nbegin\n   null;\nend;\n",
			wantRawMismatch:        false,
			wantProductionMismatch: false,
			note:                   "same named-association shape outside an allocator.",
		},
		{
			name:                   "positional_object_decl_access_load_bearing",
			source:                 "procedure P is\n   X : Rec (Pkg.Obj'Access);\nbegin\n   null;\nend;\n",
			wantRawMismatch:        true,
			wantProductionMismatch: false,
			note:                   "LOAD BEARING: raw Go elects discriminant_constraint(discriminant_association(expression(...))); the C oracle elects index_constraint(selected_component, tick, attribute_designator) directly. The subpass's rebuild produces the exact C shape.",
		},
		{
			name:                   "positional_subtype_decl_access_load_bearing",
			source:                 "procedure P is\n   subtype S is Rec (Pkg.Obj'Access);\nbegin\n   null;\nend;\n",
			wantRawMismatch:        true,
			wantProductionMismatch: false,
			note:                   "LOAD BEARING: same election gap in a subtype_declaration.",
		},
		{
			name:                   "positional_record_component_access_load_bearing",
			source:                 "procedure P is\n   type Holder is record\n      Field : Rec (Pkg.Obj'Access);\n   end record;\nbegin\n   null;\nend;\n",
			wantRawMismatch:        true,
			wantProductionMismatch: false,
			note:                   "LOAD BEARING: same election gap in a record component_declaration.",
		},
		{
			name:                   "positional_nested_selector_access_load_bearing",
			source:                 "procedure P is\n   X : Rec (Pkg.Sub.Obj'Access);\nbegin\n   null;\nend;\n",
			wantRawMismatch:        true,
			wantProductionMismatch: false,
			note:                   "LOAD BEARING: election gap persists for a two-level selected_component prefix.",
		},
		{
			name:                   "positional_allocator_access_still_diverges",
			source:                 "procedure P is\nbegin\n   A := new T (Pkg.Obj'Access);\nend;\n",
			wantRawMismatch:        true,
			wantProductionMismatch: true,
			note:                   "Inside an allocator the grammar's own prec.dynamic(1, discriminant_constraint) keeps the C oracle on discriminant_constraint(expression(...)) even for a bare positional attribute reference; the subpass always renames to index_constraint, so it trades one divergence for another instead of matching.",
		},
		{
			name:                   "positional_object_decl_identifier_no_tick_unaffected",
			source:                 "procedure P is\n   X : Rec (Value);\nbegin\n   null;\nend;\n",
			wantRawMismatch:        true,
			wantProductionMismatch: true,
			note:                   "The same raw election gap exists for a bare identifier, but the subpass is gated on a literal tick in the association text, so it never engages here; the divergence is pre-existing and unrelated to this subpass.",
		},
		{
			name:                   "positional_object_decl_size_attribute_still_diverges",
			source:                 "procedure P is\n   X : Rec (Obj'Size);\nbegin\n   null;\nend;\n",
			wantRawMismatch:        true,
			wantProductionMismatch: true,
			note:                   "adaAttributeReferenceChildren hardcodes the rebuilt attribute-designator leaf to the reserved-word 'access' symbol, which only matches the C oracle's own attribute_designator choice for the four reserved-word designators (access/delta/digits/mod). 'Size uses the plain identifier alternative, so the subpass still mismatches C after rewriting.",
		},
		{
			name:                   "multi_association_unaffected",
			source:                 "procedure P is\nbegin\n   A := new T (F => Pkg.Obj'Access, G => 5);\nend;\n",
			wantRawMismatch:        false,
			wantProductionMismatch: false,
			note:                   "A comma-separated discriminant_association list makes discriminant_constraint have more than 3 children, so the subpass's shape gate never matches regardless of tick content.",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			src := []byte(test.source)

			rawParser := gotreesitter.NewParser(goLang)
			rawParser.SetAdmissionCandidateRoute(false)
			rawTree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(src)
			if err != nil {
				t.Fatalf("raw parse error: %v", err)
			}
			defer rawTree.Release()

			prodParser := gotreesitter.NewParser(goLang)
			prodParser.SetAdmissionCandidateRoute(false)
			prodTree, err := prodParser.Parse(src)
			if err != nil {
				t.Fatalf("production parse error: %v", err)
			}
			defer prodTree.Release()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parse returned a nil tree")
			}
			defer cTree.Close()

			var rawErrs, prodErrs []string
			compareNodes(rawTree.RootNode(), goLang, cTree.RootNode(), "root", &rawErrs)
			compareNodes(prodTree.RootNode(), goLang, cTree.RootNode(), "root", &prodErrs)

			gotRawMismatch := len(rawErrs) != 0
			gotProdMismatch := len(prodErrs) != 0

			if gotRawMismatch != test.wantRawMismatch {
				t.Errorf(
					"raw-vs-C mismatch state = %t, want %t (%s)\n  raw_sexpr:  %s\n  c_sexpr:    %s\n  divergences:\n  %s",
					gotRawMismatch, test.wantRawMismatch, test.note,
					rawTree.RootNode().SExpr(goLang), formatCNodeSExpr(cTree.RootNode()),
					strings.Join(rawErrs, "\n  "),
				)
			}
			if gotProdMismatch != test.wantProductionMismatch {
				t.Errorf(
					"production-vs-C mismatch state = %t, want %t (%s)\n  prod_sexpr: %s\n  c_sexpr:    %s\n  divergences:\n  %s",
					gotProdMismatch, test.wantProductionMismatch, test.note,
					prodTree.RootNode().SExpr(goLang), formatCNodeSExpr(cTree.RootNode()),
					strings.Join(prodErrs, "\n  "),
				)
			}
		})
	}
}

// formatCNodeSExpr renders a tree-sitter C node as a compact,
// named-nodes-only S-expression for test-failure diagnostics.
func formatCNodeSExpr(n *sitter.Node) string {
	var b strings.Builder
	writeCNodeSExpr(&b, n)
	return b.String()
}

func writeCNodeSExpr(b *strings.Builder, n *sitter.Node) {
	if n == nil {
		b.WriteString("(nil)")
		return
	}
	if !n.IsNamed() {
		return
	}
	b.WriteString("(")
	b.WriteString(n.Kind())
	count := n.ChildCount()
	for i := uint(0); i < count; i++ {
		child := n.Child(i)
		if child == nil || !child.IsNamed() {
			continue
		}
		b.WriteString(" ")
		writeCNodeSExpr(b, child)
	}
	b.WriteString(")")
}
