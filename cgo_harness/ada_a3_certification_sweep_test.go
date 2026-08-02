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

	result := runA3CertificationSweep(t, "ada", "ada", lang, sources)
	a3ReportSweep(t, result)
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
	}
}
