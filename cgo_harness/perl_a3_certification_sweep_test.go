//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerlA3CompactCertificationFullCorpusSweep is the A3
// certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) full-corpus verification receipt
// for Perl. It gates both new grants
// (CompactPrimaryAcceptanceDerivationCertified and
// CompactConvergedReductionSplitDropsCertified, grammars/runtime_profiles.go)
// on zero unadjudicated compact-vs-C divergence across the real corpus plus
// the tied push-list election witness and its known-gap siblings
// (perl_scheduler_action_local_parity_test.go).
func TestPerlA3CompactCertificationFullCorpusSweep(t *testing.T) {
	lang := grammars.PerlLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("perl did not receive the A3 primary-acceptance-derivation certification")
	}
	if !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("perl did not receive the A3 converged-split-drop certification")
	}

	real := a3LoadRealCorpusDir(t, "perl", filepath.Join("corpus_real", "perl"))
	constructed := append([]a3CertificationSweepSource{{
		Name:   "smoke_sample",
		Source: []byte(grammars.ParseSmokeSample("perl")),
	}}, perlA3AdversarialSources()...)
	sources := append(append([]a3CertificationSweepSource{}, real...), constructed...)
	t.Logf("perl A3 full-corpus sweep source denominator: real=%d constructed=%d total=%d", len(real), len(constructed), len(sources))

	result := runA3CertificationSweep(t, "perl", "perl", lang, sources, perlA3KnownDivergences)
	a3ReportSweep(t, result)
}

// perlA3KnownDivergences are already-triaged, pre-existing production-route
// defects the tightened sweep criterion surfaced: the compact route only
// reproduces what production already produces (verified directly, with the
// compact route disabled) on each of these witnesses. All four are family D
// (GLR derivation selection at a declared conflict: Go's
// reduceForkWindowPreference disagrees with C's ts_parser__select_tree on
// which branch of a real grammar-declared conflict to keep). Not tied
// elections, not this gate's scope; repair lanes are tracked separately.
var perlA3KnownDivergences = []a3KnownDivergence{
	{
		Witness:   "medium__statements.pm",
		FirstPath: "/source_file/try_statement[69]/block[1]/expression_statement[1]/ambiguous_function_call_expression[0]",
		GoValue:   "ambiguous_function_call_expression", CValue: "function_call_expression", Family: "D",
	},
	{
		Witness:   "join_assignment",
		FirstPath: "/source_file/expression_statement[0]/list_expression[0]",
		GoValue:   "list_expression", CValue: "assignment_expression", Family: "D",
	},
	{
		Witness:   "return_list",
		FirstPath: "/source_file/subroutine_declaration_statement[0]/block[2]/expression_statement[1]/list_expression[0]",
		GoValue:   "list_expression", CValue: "return_expression", Family: "D",
	},
	{
		Witness:   "local_dynamic_scope",
		FirstPath: "/source_file/subroutine_declaration_statement[2]/block[2]/expression_statement[3]/ambiguous_function_call_expression[0]",
		GoValue:   "ambiguous_function_call_expression", CValue: "function_call_expression", Family: "D",
	},
}

// perlA3AdversarialSources reuses Perl's tied push-list election witness
// (the finding's "perl push-list") and its documented list-operator sibling
// shapes (perl_scheduler_action_local_parity_test.go), all already vetted
// as conflict-heavy ambiguous_function_call_expression shapes.
func perlA3AdversarialSources() []a3CertificationSweepSource {
	return []a3CertificationSweepSource{
		// Real-corpus witness: cgo_harness/corpus_real/perl/medium__unicode_ranges.pl.
		{Name: "push_two_args_real_corpus_witness", Source: []byte("push @found, $_;\n")},
		{Name: "push_three_args", Source: []byte("push @found, $a, $b;\n")},
		{Name: "unshift_two_args", Source: []byte("unshift @found, $_;\n")},
		{Name: "join_assignment", Source: []byte("my $x = join \"\\n\", \"a\", \"b\";\n")},
		{Name: "join_bare", Source: []byte("join \"\\n\", \"a\", \"b\";\n")},
		{Name: "return_list", Source: []byte("sub f { return $a, $b; }\n")},
		{Name: "hash_slice_assignment", Source: []byte("my %h; @h{qw(a b c)} = (1, 2, 3);\n")},
		{Name: "regex_match_conditional", Source: []byte("if ($x =~ /foo/) { print \"matched\\n\"; }\n")},
		{Name: "here_doc", Source: []byte("my $text = <<\"END\";\nhello\nEND\n")},
		{Name: "package_with_use", Source: []byte("package Foo;\nuse strict;\nuse warnings;\n1;\n")},
		{Name: "anonymous_sub_ref", Source: []byte("my $cb = sub { return $_[0] + 1; };\n")},
		{Name: "map_grep_chain", Source: []byte("my @out = grep { $_ % 2 == 0 } map { $_ * 2 } (1, 2, 3);\n")},
		{Name: "ternary_list_context", Source: []byte("my @r = $flag ? (1, 2) : (3, 4);\n")},
		{Name: "local_dynamic_scope", Source: []byte("our $x = 1;\nsub f { local $x = 2; g(); }\n")},
		{Name: "wantarray_dispatch", Source: []byte("sub f { return wantarray ? (1, 2) : 1; }\n")},
		// print_filehandle_list_material_election is a permanent regression
		// fixture: a genuinely material tied election outside this sweep's
		// original corpus. The C oracle picks the derivation whose outer
		// production is ambiguous_function_call_expression; the certified
		// compact route's old positional primary picked the derivation whose
		// outer production is list_expression instead (matching production,
		// which also diverges from C here). Under the materiality gate
		// (selectCompactAcceptanceDerivation, compactAcceptanceElectionIsVacuous)
		// the two tied derivations are not byte-identical, so this now
		// declines instead of accepting the C-divergent tree.
		{Name: "print_filehandle_list_material_election", Source: []byte("print $fh, \"a\", \"b\";\n")},
	}
}
