//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAdaA3CompactCertificationFullCorpusSweep is the A3
// certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) full-corpus verification receipt
// for Ada. Ada has no corpus_real directory (it is not one of the top-50
// languages cgo_harness/corpus_real's manifest profiles), so this sweep uses
// the smoke sample, the three existing tied-election witnesses
// (ada_election_local_parity_test.go, including both tied aggregate
// elections the finding names), and 18 constructed adversarial sources
// exercising Ada's conflict-heavy shapes (aggregates, attributes, generics,
// tasks, protected types, access types, exception handlers).
func TestAdaA3CompactCertificationFullCorpusSweep(t *testing.T) {
	lang := grammars.AdaLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("ada did not receive the A3 primary-acceptance-derivation certification")
	}
	if !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("ada did not receive the A3 converged-split-drop certification")
	}

	sources := []a3CertificationSweepSource{
		{Name: "smoke_sample", Source: []byte(grammars.ParseSmokeSample("ada"))},
	}
	sources = append(sources, adaA3TiedElectionWitnesses()...)
	sources = append(sources, adaA3AdversarialSources()...)
	// Ada has no cgo_harness/corpus_real directory on any host (it is not
	// one of the top-50 languages the real-corpus manifest profiles), so
	// real=0 here is a documented, permanent property of this sweep, not a
	// silent degradation -- unlike python/perl (a3LoadRealCorpusDir), there
	// is no missing-fixture condition to detect for this language.
	t.Logf("ada A3 full-corpus sweep source denominator: real=0 (no corpus_real/ada on any host) constructed=%d total=%d", len(sources), len(sources))

	result := runA3CertificationSweep(t, "ada", "ada", lang, sources, adaA3KnownDivergences)
	a3ReportSweep(t, result)
}

// adaA3KnownDivergences are already-triaged, pre-existing production-route
// defects the tightened sweep criterion surfaced: the compact route only
// reproduces what production already produces (verified directly, with the
// compact route disabled) on each of these witnesses.
//
// Six of the seven are family M (materialization: an inherited field-map
// entry names a child Go's flattened-hidden-span heuristic should not; C's
// node.c filters inherited entries and never does). Ada's grammar declares
// "name"/"subtype_mark" as inherited fields on productions whose actual
// child production (named_array_aggregate, range_g) never carries them
// directly; Go's applyParentFieldToFlattenedHiddenSpan re-attaches the
// inherited field anyway.
//
// attribute_constraint is family D: Ada's grammar declares
// prec.dynamic(1, discriminant_constraint) over index_constraint with an
// explicit grammar comment choosing it, and Go's reduceForkWindowPreference
// still picks index_constraint.
//
// Not tied elections, not this gate's scope; repair lanes are tracked
// separately.
var adaA3KnownDivergences = []a3KnownDivergence{
	{
		Witness:   "attribute_constraint",
		FirstPath: "/compilation/compilation_unit[0]/subprogram_body[0]/handled_sequence_of_statements[3]/assignment_statement[0]/expression[2]/term[0]/allocator[0]/index_constraint[2]",
		GoValue:   "index_constraint", CValue: "discriminant_constraint", Family: "D",
	},
	{
		Witness:   "locked_positional_array_aggregate",
		FirstPath: "/compilation/compilation_unit[0]/package_declaration[0]/full_type_declaration[3]/array_type_definition[3]/range_g[2]",
		GoValue:   "subtype_mark", CValue: "", Family: "M",
	},
	{
		Witness:   "array_others_choice",
		FirstPath: "/compilation/compilation_unit[0]/subprogram_body[0]/handled_sequence_of_statements[3]/assignment_statement[0]/expression[2]/term[0]/named_array_aggregate[0]",
		GoValue:   "name", CValue: "", Family: "M",
	},
	{
		Witness:   "named_array_aggregate",
		FirstPath: "/compilation/compilation_unit[0]/package_declaration[0]/full_type_declaration[3]/array_type_definition[3]/range_g[2]",
		GoValue:   "subtype_mark", CValue: "", Family: "M",
	},
	{
		Witness:   "mixed_positional_named_aggregate",
		FirstPath: "/compilation/compilation_unit[0]/package_declaration[0]/full_type_declaration[3]/array_type_definition[3]/range_g[2]",
		GoValue:   "subtype_mark", CValue: "", Family: "M",
	},
	{
		Witness:   "qualified_expression",
		FirstPath: "/compilation/compilation_unit[0]/package_declaration[0]/full_type_declaration[3]/array_type_definition[3]/range_g[2]",
		GoValue:   "subtype_mark", CValue: "", Family: "M",
	},
	{
		Witness:   "attribute_range_and_first_last",
		FirstPath: "/compilation/compilation_unit[0]/subprogram_body[0]/non_empty_declarative_part[2]/full_type_declaration[0]/array_type_definition[3]/range_g[2]",
		GoValue:   "subtype_mark", CValue: "", Family: "M",
	},
}

// adaA3TiedElectionWitnesses reuses the three witnesses already vetted in
// ada_election_local_parity_test.go (TestAdaElectionRawCOracleParity),
// including both tied aggregate elections the finding names ("both ada
// aggregates").
func adaA3TiedElectionWitnesses() []a3CertificationSweepSource {
	return []a3CertificationSweepSource{
		{
			// dispatch.ada.constraint-kind-election witness.
			Name: "attribute_constraint",
			Source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := new T (F => Pkg.Obj'Access);\n" +
				"end;\n"),
		},
		{
			// dispatch.ada.aggregate-kind-election witness (tree-sitter-ada
			// test/corpus/arrays.txt). The finding's first "ada aggregate".
			Name: "locked_positional_array_aggregate",
			Source: []byte("package P is\n" +
				"   type A is array (1 .. 3) of Boolean;\n" +
				"   V : constant A := (1, 2, 3);\n" +
				"end;\n"),
		},
		{
			// dispatch.ada.aggregate-kind-election +
			// dispatch.ada.association-choice-materialization witness. The
			// finding's second "ada aggregate".
			Name: "array_others_choice",
			Source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := (others => 0);\n" +
				"end;\n"),
		},
	}
}

// adaA3AdversarialSources constructs 18 sources exercising Ada's
// conflict-heavy grammar shapes: mixed aggregates, attribute references,
// generic instantiation, tasks, protected types, access types, exception
// handlers, discriminated records, and qualified expressions.
func adaA3AdversarialSources() []a3CertificationSweepSource {
	return []a3CertificationSweepSource{
		{Name: "named_array_aggregate", Source: []byte(
			"package P is\n" +
				"   type A is array (1 .. 3) of Integer;\n" +
				"   V : constant A := (1 => 10, 2 => 20, 3 => 30);\n" +
				"end P;\n")},
		{Name: "mixed_positional_named_aggregate", Source: []byte(
			"package P is\n" +
				"   type A is array (1 .. 3) of Integer;\n" +
				"   V : constant A := (10, 20, others => 0);\n" +
				"end P;\n")},
		{Name: "record_aggregate_named", Source: []byte(
			"package P is\n" +
				"   type R is record\n" +
				"      X : Integer;\n" +
				"      Y : Integer;\n" +
				"   end record;\n" +
				"   V : constant R := (X => 1, Y => 2);\n" +
				"end P;\n")},
		{Name: "qualified_expression", Source: []byte(
			"package P is\n" +
				"   type A is array (1 .. 3) of Integer;\n" +
				"   V : constant A := A'(1, 2, 3);\n" +
				"end P;\n")},
		{Name: "attribute_range_and_first_last", Source: []byte(
			"procedure P is\n" +
				"   type A is array (1 .. 10) of Integer;\n" +
				"   V : A;\n" +
				"begin\n" +
				"   for I in V'Range loop\n" +
				"      V (I) := V'First + V'Last;\n" +
				"   end loop;\n" +
				"end P;\n")},
		{Name: "generic_package_instantiation", Source: []byte(
			"generic\n" +
				"   type T is private;\n" +
				"package Stacks is\n" +
				"   procedure Push (Item : T);\n" +
				"end Stacks;\n" +
				"package Integer_Stacks is new Stacks (T => Integer);\n")},
		{Name: "task_declaration_and_body", Source: []byte(
			"task Worker is\n" +
				"   entry Start;\n" +
				"end Worker;\n" +
				"task body Worker is\n" +
				"begin\n" +
				"   accept Start do\n" +
				"      null;\n" +
				"   end Start;\n" +
				"end Worker;\n")},
		{Name: "protected_type_declaration", Source: []byte(
			"protected type Counter is\n" +
				"   procedure Increment;\n" +
				"   function Value return Integer;\n" +
				"private\n" +
				"   Count : Integer := 0;\n" +
				"end Counter;\n")},
		{Name: "access_to_subprogram", Source: []byte(
			"package P is\n" +
				"   type Callback is access procedure (X : Integer);\n" +
				"end P;\n")},
		{Name: "access_to_object", Source: []byte(
			"package P is\n" +
				"   type Int_Ptr is access Integer;\n" +
				"   V : Int_Ptr := new Integer'(42);\n" +
				"end P;\n")},
		{Name: "exception_handler_multi_when", Source: []byte(
			"procedure P is\n" +
				"begin\n" +
				"   null;\n" +
				"exception\n" +
				"   when Constraint_Error | Program_Error =>\n" +
				"      null;\n" +
				"   when others =>\n" +
				"      null;\n" +
				"end P;\n")},
		{Name: "discriminated_record", Source: []byte(
			"package P is\n" +
				"   type Buffer (Size : Positive) is record\n" +
				"      Data : String (1 .. Size);\n" +
				"   end record;\n" +
				"end P;\n")},
		{Name: "case_statement_multi_choice", Source: []byte(
			"procedure P (X : Integer) is\n" +
				"begin\n" +
				"   case X is\n" +
				"      when 1 | 2 | 3 =>\n" +
				"         null;\n" +
				"      when 4 .. 10 =>\n" +
				"         null;\n" +
				"      when others =>\n" +
				"         null;\n" +
				"   end case;\n" +
				"end P;\n")},
		{Name: "renaming_declaration", Source: []byte(
			"package P is\n" +
				"   X : Integer := 1;\n" +
				"   Y : Integer renames X;\n" +
				"end P;\n")},
		{Name: "subtype_with_range_constraint", Source: []byte(
			"package P is\n" +
				"   subtype Digit is Integer range 0 .. 9;\n" +
				"end P;\n")},
		{Name: "nested_package_declaration", Source: []byte(
			"package Outer is\n" +
				"   package Inner is\n" +
				"      procedure Do_Something;\n" +
				"   end Inner;\n" +
				"end Outer;\n")},
		{Name: "overloaded_operator_function", Source: []byte(
			"package P is\n" +
				"   type Vector is record\n" +
				"      X, Y : Float;\n" +
				"   end record;\n" +
				"   function \"+\" (Left, Right : Vector) return Vector;\n" +
				"end P;\n")},
		{Name: "extended_return_statement", Source: []byte(
			"package P is\n" +
				"   type R is record\n" +
				"      X : Integer;\n" +
				"   end record;\n" +
				"   function Make return R;\n" +
				"end P;\n" +
				"package body P is\n" +
				"   function Make return R is\n" +
				"   begin\n" +
				"      return Result : R do\n" +
				"         Result.X := 1;\n" +
				"      end return;\n" +
				"   end Make;\n" +
				"end P;\n")},
		// bare_aggregate_assignment_material_election is a permanent
		// regression fixture: a genuinely material tied election outside
		// this sweep's original corpus. The C oracle picks the derivation
		// whose outer production is record_aggregate; the certified compact
		// route's old positional primary picked the derivation whose outer
		// production is named_array_aggregate instead (matching production,
		// which also diverges from C here). Under the materiality gate
		// (selectCompactAcceptanceDerivation, compactAcceptanceElectionIsVacuous)
		// the two tied derivations are not byte-identical, so this now
		// declines instead of accepting the C-divergent tree.
		{Name: "bare_aggregate_assignment_material_election", Source: []byte(
			"procedure P is\n" +
				"begin\n" +
				"   R := (F => 1, G => 2);\n" +
				"end;\n")},
	}
}
