//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPythonA3CompactCertificationFullCorpusSweep is the A3
// certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) full-corpus verification receipt
// for Python. Python already shipped CompactConvergedReductionSplitDropsCertified;
// this sweep gates the added CompactPrimaryAcceptanceDerivationCertified
// grant (grammars/runtime_profiles.go) on zero unadjudicated compact-vs-C
// divergence across the real corpus plus the tied-election and known-gap
// tuple/f-string witnesses (python_scheduler_action_local_parity_test.go).
func TestPythonA3CompactCertificationFullCorpusSweep(t *testing.T) {
	lang := grammars.PythonLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("python did not receive the A3 primary-acceptance-derivation certification")
	}
	if !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("python lost its pre-existing converged-split-drop certification")
	}

	sources := a3LoadRealCorpusDir(t, filepath.Join("corpus_real", "python"))
	sources = append(sources, a3CertificationSweepSource{
		Name:   "smoke_sample",
		Source: []byte(grammars.ParseSmokeSample("python")),
	})
	sources = append(sources, pythonA3AdversarialSources()...)

	result := runA3CertificationSweep(t, "python", "python", lang, sources)
	a3ReportSweep(t, result)
}

// pythonA3AdversarialSources gathers Python's tied-election witness (the
// bare-tuple assignment right-hand side) plus its known-gap and neutral
// control sources
// (python_scheduler_action_local_parity_test.go), all already vetted as
// conflict-heavy shapes for this grammar's pattern_list/expression_list
// election.
func pythonA3AdversarialSources() []a3CertificationSweepSource {
	return []a3CertificationSweepSource{
		{Name: "assignment_bare_tuple_real_corpus_witness", Source: []byte("x, y, z = 1, 2, 3\nxyz = x, y, z\n")},
		{Name: "assignment_bare_pair", Source: []byte("a = 1\nb = 2\npair = a, b\n")},
		{Name: "assignment_bare_single_trailing_comma", Source: []byte("a = 1\nsingle = a,\n")},
		{Name: "fstring_interpolation_bare_tuple", Source: []byte("x = 1\ny = 2\nz = f\"{x, y}\"\n")},
		{Name: "fstring_interpolation_splat", Source: []byte("xs = [1, 2]\nz = f\"{*xs,}\"\n")},
		{Name: "print_chevron", Source: []byte("print >>sys.stderr, \"x\"\n")},
		{Name: "collapsed_pass_own_line", Source: []byte("def f():\n    pass\n")},
		{Name: "collapsed_pass_inline_block", Source: []byte("if True: pass\n")},
		{Name: "collapsed_continue_inline_block", Source: []byte("while True:\n    if True: continue\n")},
		{Name: "collapsed_break_inline_block", Source: []byte("while True:\n    if True: break\n")},
		{Name: "inline_return_block", Source: []byte("def f():\n    if True: return 1\n")},
		{Name: "inline_raise_block", Source: []byte("def f():\n    if True: raise ValueError(\"x\")\n")},
		{Name: "inline_yield_block", Source: []byte("def f():\n    if True: yield 1\n")},
		{Name: "inline_tuple_block", Source: []byte("def f():\n    if True: a, b = 1, 2\n")},
		{Name: "wildcard_import", Source: []byte("from os import *\n")},
		{Name: "match_case_as_pattern", Source: []byte("match p:\n    case (a, b) as pair:\n        pass\n")},
		{Name: "match_case_wildcard", Source: []byte("match p:\n    case _:\n        pass\n")},
		{Name: "for_target_tuple_negative_control", Source: []byte("pairs = [(1, 2)]\nfor a, b in pairs:\n    pass\n")},
		{Name: "chained_assignment_lhs_negative_control", Source: []byte("a, b = c, d = 1, 2\n")},
		{Name: "del_tuple_negative_control", Source: []byte("a = 1\nb = 2\ndel a, b\n")},
		{Name: "walrus_in_comprehension", Source: []byte("data = [1, 2, 3]\nresult = [y for x in data if (y := x * 2) > 2]\n")},
		{Name: "decorated_async_def", Source: []byte("import functools\n\n@functools.wraps\nasync def f(x, *, y=1, **kw):\n    return x, y\n")},
		{Name: "star_expr_unpack", Source: []byte("first, *rest = [1, 2, 3]\n")},
		{Name: "lambda_tuple_return", Source: []byte("f = lambda x, y: (x, y)\n")},
		{Name: "nested_fstring_conversion", Source: []byte("name = \"world\"\ns = f\"{name!r:>{10}}\"\n")},
	}
}

// a3LoadRealCorpusDir reads every regular file directly under dir (relative
// to the cgo_harness package directory) into an a3CertificationSweepSource
// slice, sorted by name for deterministic ordering. It skips (does not fail)
// when dir does not exist, so a language with no corpus_real directory
// simply contributes zero real-corpus sources.
func a3LoadRealCorpusDir(t *testing.T, dir string) []a3CertificationSweepSource {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read corpus dir %s: %v", dir, err)
	}
	var out []a3CertificationSweepSource
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read corpus file %s: %v", path, err)
		}
		out = append(out, a3CertificationSweepSource{Name: entry.Name(), Source: data})
	}
	return out
}
