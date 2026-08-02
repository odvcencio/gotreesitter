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

	real := a3LoadRealCorpusDir(t, "python", filepath.Join("corpus_real", "python"))
	constructed := append([]a3CertificationSweepSource{{
		Name:   "smoke_sample",
		Source: []byte(grammars.ParseSmokeSample("python")),
	}}, pythonA3AdversarialSources()...)
	sources := append(append([]a3CertificationSweepSource{}, real...), constructed...)
	t.Logf("python A3 full-corpus sweep source denominator: real=%d constructed=%d total=%d", len(real), len(constructed), len(sources))

	result := runA3CertificationSweep(t, "python", "python", lang, sources, pythonA3KnownDivergences)
	a3ReportSweep(t, result)
}

// pythonA3KnownDivergences are already-triaged, pre-existing production-route
// defects the tightened sweep criterion surfaced: the compact route only
// reproduces what production already produces (verified directly, with the
// compact route disabled) on each of these witnesses. The two real-corpus
// entries are family M (materialization: an inherited field-map entry names
// a child Go's flattened-hidden-span heuristic should not; C's node.c filters
// inherited entries and never does). large__python3.8_grammar.py's first
// point is family M; it also carries a family S point further in (an
// aliased "not in"/"is not" span that starts one byte early) not enumerated
// as a separate entry here, since only the first divergence gates the match.
// The two f-string entries are family D (a first-class declared
// pattern_list/expression_list ambiguity; Go's reduceForkWindowPreference
// disagrees with C's ts_parser__select_tree on which side to keep). Not
// tied elections, not this gate's scope; repair lanes are tracked
// separately.
var pythonA3KnownDivergences = []a3KnownDivergence{
	{
		Witness:   "large__python3.8_grammar.py",
		FirstPath: "/module/class_definition[25]/block[4]/function_definition[13]/block[6]/import_from_statement[0]/,[4]",
		GoValue:   "name", CValue: "", Family: "M",
	},
	{
		Witness:   "medium__setup.py",
		FirstPath: "/module/import_from_statement[2]/,[4]",
		GoValue:   "name", CValue: "", Family: "M",
	},
	{
		Witness:   "fstring_interpolation_bare_tuple",
		FirstPath: "/module/assignment[2]/string[2]/interpolation[1]/pattern_list[1]",
		GoValue:   "pattern_list", CValue: "expression_list", Family: "D",
	},
	{
		Witness:   "fstring_interpolation_splat",
		FirstPath: "/module/assignment[1]/string[2]/interpolation[1]/pattern_list[1]",
		GoValue:   "pattern_list", CValue: "expression_list", Family: "D",
	},
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
// slice, sorted by name for deterministic ordering.
//
// cgo_harness/corpus_real is a generated, gitignored fixture
// (cgo_harness/cmd/build_real_corpus); it is not always present. The calling
// test's name claims full-corpus scope, so it must not silently downgrade to
// a constructed-only run when the real corpus is missing -- that used to be
// exactly what this function did (returning nil on a missing directory with
// no signal to the caller or the test log). It now skips the calling test
// loudly instead, matching this package's existing convention for a
// claimed-scope test that cannot honor its own claim on the current host
// (parityRequireExhaustive, parity_cgo_test.go). lang names the calling
// sweep in the skip/log messages.
func a3LoadRealCorpusDir(t *testing.T, lang, dir string) []a3CertificationSweepSource {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		t.Skipf(
			"%s A3 full-corpus sweep requires %s (a generated, gitignored real-corpus fixture; "+
				"see cgo_harness/cmd/build_real_corpus) -- absent on this host, so this sweep cannot "+
				"claim full-corpus scope",
			lang, dir,
		)
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
	if len(out) == 0 {
		t.Skipf("%s A3 full-corpus sweep found %s but it contains no files -- this sweep cannot claim full-corpus scope", lang, dir)
	}
	return out
}
