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

	sources := a3LoadRealCorpusDir(t, filepath.Join("corpus_real", "perl"))
	sources = append(sources, a3CertificationSweepSource{
		Name:   "smoke_sample",
		Source: []byte(grammars.ParseSmokeSample("perl")),
	})
	sources = append(sources, perlA3AdversarialSources()...)

	result := runA3CertificationSweep(t, "perl", "perl", lang, sources)
	a3ReportSweep(t, result)
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
	}
}
