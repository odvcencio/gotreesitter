//go:build cgo && treesitter_c_parity

package cgoharness

// internal/ccore against pinned C: trace equality with C's logger,
// visible-tree digests, determinism, and timing arms (core, legacy, C).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/benchorder"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type ccoreInput struct {
	ID  string
	Src []byte
}

var ccoreGoSmall = []string{
	"package p\n\nfunc f() { _ = Foo[int](a) }\n",
	"package p\n\nfunc f() { _ = Foo[int](Foo[int](a)) }\n",
	"package p\n\nfunc f() {\n\tx := a[b](c)\n\ty := m[k]\n\t_, _ = x, y\n}\n",
	"package p\n\ntype T[P any] struct{ f P }\n\nfunc g[Q any](x Q) Q { return x }\n\nvar v = g[T[int]](T[int]{})\n",
	"package p\n\nfunc f(s []int) {\n\tfor i := range s {\n\t\tif s[i] > 0 {\n\t\t\tcontinue\n\t\t}\n\t}\n\tswitch x := s[0]; {\n\tcase x < 1:\n\t}\n}\n",
}

// ccoreSamples names the committed sample of each language that the equality
// tests use, so that the input of a required check is listed in this file.
var ccoreSamples = map[string]string{
	"c":          "internal/benchfixtures/testdata/real/c",
	"go":         "internal/benchfixtures/testdata/real/go",
	"json":       "internal/benchfixtures/testdata/real/json",
	"python":     "internal/benchfixtures/testdata/real/python",
	"typescript": "internal/benchfixtures/testdata/real/typescript",
}

func ccoreManifestSample(t testing.TB, lang string) []byte {
	path, ok := ccoreSamples[lang]
	if !ok {
		return ccoreManifestSampleOther(t, lang)
	}
	src, err := os.ReadFile(filepath.Join("..", path))
	if err != nil {
		t.Fatalf("listed sample %s: %v", path, err)
	}
	return src
}

// ccoreManifestSampleOther reads the sample of a language that no required
// check uses (the campaign runs over many grammars) from the corpus manifest.
func ccoreManifestSampleOther(t testing.TB, lang string) []byte {
	raw, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			CommittedPath string `json:"committed_path"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Entries {
		if e.Language == lang && e.Role == "sample" && e.CommittedPath != "" {
			src, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", e.CommittedPath))
			if err != nil {
				t.Fatal(err)
			}
			return src
		}
	}
	t.Fatalf("no committed %s sample", lang)
	return nil
}

func ccoreJSON1M(sample []byte) []byte {
	var b strings.Builder
	b.WriteString("[\n")
	for b.Len() < 1<<20 {
		if b.Len() > 2 {
			b.WriteString(",\n")
		}
		b.Write([]byte(strings.TrimSpace(string(sample))))
	}
	b.WriteString("\n]\n")
	return []byte(b.String())
}

func ccoreInputs(t testing.TB, lang string, small bool) []ccoreInput {
	if in, ok := ccoreScanInputs(t, lang, small); ok {
		return in
	}
	var out []ccoreInput
	switch lang {
	case "json":
		s := ccoreManifestSample(t, "json")
		out = append(out, ccoreInput{"json/sample", s})
		if !small {
			out = append(out, ccoreInput{"json/1M", ccoreJSON1M(s)})
		}
	case "go-rec-a":
		// Line E of the recovery prototype: 5 certified fixtures, 5 + 5 with an
		// inserted call or generic literal, and the R4 Go fixture, each with its
		// last byte replaced by "+".
		fixtures := []struct{ name, source string }{
			{"without_package", "func f(){}\nfunc g(){}\n"},
			{"package_alias", "package p\nfunc f(){}\nfunc g(){}\n"},
			{"parameters", "package p\nfunc f(x int) int { return x + 1 }\nfunc g(){}\n"},
			{"loop", "package p\nfunc f(){for i:=0;i<2;i++{_ = i}}\nfunc g(){}\n"},
			{"strings", "package p\nfunc f(){_ = \"a+b\"; _ = `raw`}\nfunc g(){}\n"},
		}
		const callStatement = "func TestIsToolchain(t *testing.T) { test1(t, isToolchainTests, \"IsToolchain\", IsToolchain) }\n"
		const genericLiteral = "var isToolchainTests = []testCase1[string, bool]{\n\t{\"go\", true},\n}\n"
		trail := func(s string) string { return s[:len(s)-1] + "+" }
		insertBeforeLast := func(s, insert string) string {
			body := strings.TrimSuffix(s, "\n")
			at := strings.LastIndex(body, "\n") + 1
			return s[:at] + insert + s[at:]
		}
		for _, f := range fixtures {
			out = append(out, ccoreInput{"certified/" + f.name, []byte(trail(f.source))})
		}
		for _, f := range fixtures {
			out = append(out, ccoreInput{"call_before/" + f.name, []byte(trail(insertBeforeLast(f.source, callStatement)))})
		}
		for _, f := range fixtures {
			out = append(out, ccoreInput{"generic_before/" + f.name, []byte(trail(insertBeforeLast(f.source, genericLiteral)))})
		}
		out = append(out, ccoreInput{"r4_fixture/last_byte_plus", []byte(trail(string(ccoreManifestSample(t, "go"))))})
	case "go-rec-b", "json-rec-d":
		// Single-byte edits of the committed sample: for Go, the 72 session edit
		// positions applied alone (insert, delete, replace in turn); for JSON,
		// 60 positions spread over the file, each deleted, inserted and replaced.
		srcLang := "go"
		if lang == "json-rec-d" {
			srcLang = "json"
		}
		clean := ccoreManifestSample(t, srcLang)
		type edit struct {
			n, at int
			kind  string
		}
		var edits []edit
		if srcLang == "go" {
			for i, step := range benchfixtures.EditingSession(clean) {
				edits = append(edits, edit{i + 1, int(step.Edit.StartByte), []string{"insert", "delete", "replace"}[i%3]})
			}
		} else {
			for k := 0; k < 60; k++ {
				at := (k + 1) * len(clean) / 61
				for _, kind := range []string{"delete", "insert", "replace"} {
					edits = append(edits, edit{len(edits) + 1, at, kind})
				}
			}
		}
		for _, e := range edits {
			if e.at >= len(clean) && e.kind != "insert" {
				continue
			}
			var src []byte
			switch e.kind {
			case "insert":
				src = append(append(append([]byte(nil), clean[:e.at]...), 'x'), clean[e.at:]...)
			case "delete":
				src = append(append([]byte(nil), clean[:e.at]...), clean[e.at+1:]...)
			default:
				src = append([]byte(nil), clean...)
				if src[e.at] == 'x' {
					src[e.at] = 'y'
				} else {
					src[e.at] = 'x'
				}
			}
			out = append(out, ccoreInput{fmt.Sprintf("edit/%d_%s_%d", e.n, e.kind, e.at), src})
		}
	case "go-rec-c":
		for i, step := range benchfixtures.EditingSession(ccoreManifestSample(t, "go")) {
			out = append(out, ccoreInput{fmt.Sprintf("session/%d", i+1), step.Source})
		}
	case "go-rec-e":
		for _, in := range ccoreInputs(t, "go-corpus", true)[:300] {
			rng := rand.New(rand.NewSource(int64(len(out)) + 1))
			at := rng.Intn(len(in.Src))
			src := append(append([]byte(nil), in.Src[:at]...), in.Src[at+1:]...)
			out = append(out, ccoreInput{fmt.Sprintf("%s/del%d", in.ID, at), src})
		}
	case "go-corpus":
		// The 300-case Go generic-call corpus (2 callee names x 10 type
		// arguments x 5 argument lists x 3 contexts), 6 witnesses and the
		// nested case.
		names := []string{"F", "p.G"}
		types := []string{"T", "int", "*T", "[]T", "map[string]T", "p.T", "T,U", "func(int) int", "struct{ X int }", "chan T"}
		args := []string{"x", "x,y", "F[T](x)", "p.G[T](x)", ""}
		contexts := []string{"_ = %s", "return %s", "if true { _ = %s }"}
		for ni, name := range names {
			for ti, ty := range types {
				for ai, arg := range args {
					for ci, context := range contexts {
						src := "package p\nfunc f() { " + fmt.Sprintf(context, name+"["+ty+"]("+arg+")") + " }\n"
						out = append(out, ccoreInput{fmt.Sprintf("corpus/n%d_t%d_a%d_c%d", ni, ti, ai, ci), []byte(src)})
					}
				}
			}
		}
		for _, w := range []struct{ name, src string }{
			{"single_identifier", "package p\nfunc f() { _ = F[int](x) }\n"},
			{"single_qualified", "package p\nfunc f() { _ = p.G[T](x) }\n"},
			{"combined", "package p\nfunc f() { _ = F[int](x); _ = p.G[T](x) }\n"},
			{"reverse", "package p\nfunc f() { _ = p.G[T](x); _ = F[int](x) }\n"},
			{"separate_functions", "package p\nfunc f() { _ = F[int](x) }; func g() { _ = p.G[T](x) }\n"},
			{"ordinary_calls", "package p\nfunc f() { _ = F(x); _ = p.G(x) }\n"},
		} {
			out = append(out, ccoreInput{"witness/" + w.name, []byte(w.src)})
		}
		out = append(out, ccoreInput{"nested/Foo_int_Foo_int_a", []byte("package p\nfunc f() { _ = Foo[int](Foo[int](a)) }\n")})
	case "go":
		for i, s := range ccoreGoSmall {
			out = append(out, ccoreInput{fmt.Sprintf("go/small%d", i+1), []byte(s)})
		}
		fixtures, err := benchfixtures.LoadGoFullParseFixtures()
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range fixtures {
			out = append(out, ccoreInput{"go/canonical/" + f.Fixture.ID, f.Source})
		}
		if small {
			return out
		}
		out = append(out, ccoreInput{"go/sample", ccoreManifestSample(t, "go")})
		for _, size := range []struct {
			name string
			n    int
		}{{"32K", 32 * 1024}, {"137K", 137 * 1024}, {"1M", 1024 * 1024}} {
			src, _, err := benchfixtures.GeneratedSource("go", size.n)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, ccoreInput{"go/generated/" + size.name, src})
		}
	}
	return out
}

func ccoreLanguages(t testing.TB, name string) (*g.Language, *sitter.Language) {
	name = ccoreBaseLang(name)
	bindtest.NormalizeNames(os.Getenv("CCORE_NORM_NAMES") != "")
	switch name {
	case "go-corpus", "go-rec-a", "go-rec-b", "go-rec-c", "go-rec-e":
		name = "go"
	case "json-rec-d":
		name = "json"
	}
	id, err := COracleIdentity(name)
	if err != nil {
		t.Fatalf("oracle identity: %v", err)
	}
	fmt.Printf("ORACLE language=%s grammar=%s runtime=%s artifact=%s\n", name, id.GrammarCommit, id.RuntimeCommit, id.GrammarArtifactSHA256)
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	gl := grammars.DetectLanguageByName(name).Language()
	sum, ok := gl.GrammarBlobSHA256()
	fmt.Printf("EXTSCAN language=%s scanner=%t external_lex_states=%d external_symbols=%d\n", name, gl.ExternalScanner != nil, len(gl.ExternalLexStates), len(gl.ExternalSymbols))
	fmt.Printf("GOLANG language=%s blob=%s ok=%t symbols=%d states=%d\n", name, hex.EncodeToString(sum[:]), ok, gl.SymbolCount, gl.StateCount)
	return gl, cl
}

func ccoreCDigest(tree *sitter.Tree) string {
	h := sha256.New()
	cursor := tree.RootNode().Walk()
	defer cursor.Close()
	var buf []byte
	write := func(n *sitter.Node) {
		s, e := n.StartPosition(), n.EndPosition()
		buf = bindtest.DigestRecord(buf[:0], n.Kind(), uint32(n.StartByte()), uint32(n.EndByte()), uint32(s.Row), uint32(s.Column), uint32(e.Row), uint32(e.Column), n.IsNamed(), n.IsExtra(), n.IsMissing(), false, uint32(n.ChildCount()))
		h.Write(buf)
	}
	rootErr := []byte{0}
	if tree.RootNode().HasError() {
		rootErr[0] = 1
	}
	write(cursor.Node())
	for {
		if cursor.GotoFirstChild() {
			write(cursor.Node())
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				h.Write(rootErr)
				return hex.EncodeToString(h.Sum(nil))
			}
		}
		write(cursor.Node())
	}
}

func ccoreLegacyDigest(tree *g.Tree, lang *g.Language) string {
	h := sha256.New()
	var buf []byte
	var visit func(n *g.Node)
	visit = func(n *g.Node) {
		s, e := n.StartPoint(), n.EndPoint()
		buf = bindtest.DigestRecord(buf[:0], n.Type(lang), n.StartByte(), n.EndByte(), s.Row, s.Column, e.Row, e.Column, n.IsNamed(), n.IsExtra(), n.IsMissing(), false, uint32(n.ChildCount()))
		h.Write(buf)
		for i := 0; i < n.ChildCount(); i++ {
			visit(n.Child(i))
		}
	}
	root := tree.RootNode()
	visit(root)
	if root.HasError() {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestCcoreTrace compares C's parse log with the spike's, line by line.
func TestCcoreTrace(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	gl, cl := ccoreLanguages(t, name)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var clog []string
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	spike := ccoreNew(gl)
	checked := 0
	for _, in := range ccoreInputs(t, name, true) {
		checked++
		clog = clog[:0]
		ct := cp.Parse(in.Src, nil)
		cerr := ct.RootNode().HasError()
		ct.Close()
		var slog []string
		stop := spike.Parse(in.Src, func(s string) { slog = append(slog, s) })
		first := -1
		for i := 0; i < len(clog) || i < len(slog); i++ {
			if i >= len(clog) || i >= len(slog) || ccoreNormLine(clog[i]) != ccoreNormLine(slog[i]) {
				first = i
				break
			}
		}
		fmt.Printf("CCTRACE input=%s c_lines=%d spike_lines=%d equal=%t first_diff=%d stop=%q c_error=%t\n", in.ID, len(clog), len(slog), first < 0, first, stop, cerr)
		ccoreExpect(t, first < 0 && len(clog) > 0, "%s: the trace differs from C at line %d, or C logged nothing (%d lines)", in.ID, first, len(clog))
		if first >= 0 {
			for i := first - 3; i < first+4; i++ {
				if i < 0 {
					continue
				}
				c, s := "<none>", "<none>"
				if i < len(clog) {
					c = clog[i]
				}
				if i < len(slog) {
					s = slog[i]
				}
				fmt.Printf("CCTRACE   %d C: %s\nCCTRACE   %d S: %s\n", i, c, i, s)
			}
		}
	}
	ccoreCount("TestCcoreTrace", name, checked)
}

// TestCcoreDigest compares visible-tree digests with pinned C and
// checks determinism.
func TestCcoreDigest(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	gl, cl := ccoreLanguages(t, name)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	spike := ccoreNew(gl)
	legacy := g.NewParser(gl)
	legacy.SetAdmissionCandidateRoute(false)
	doubleTrees, zeroNodes, balanceChecks, balanceRefusals, allocRecs, reachRecs := 0, 0, 0, 0, 0, 0
	checked := 0
	for _, in := range ccoreInputs(t, name, false) {
		checked++
		legacyEq := false
		if os.Getenv("CCORE_LEGACY") != "" {
			if lt, err := legacy.Parse(in.Src); err == nil {
				ct := cp.Parse(in.Src, nil)
				legacyEq = ccoreLegacyDigest(lt, gl) == ccoreCDigest(ct)
				ct.Close()
				lt.Release()
			}
		}
		ct := cp.Parse(in.Src, nil)
		cd := ccoreCDigest(ct)
		cerr := ct.RootNode().HasError()
		cend := ct.RootNode().EndByte()
		ct.Close()
		stop := spike.Parse(in.Src, nil)
		sd := spike.Digest()
		ad := spike.ArenaDigest()
		steps, tokens, maxv, multi, arena := spike.Stats()
		twice, visited := spike.DoubleReach(spike.Tree())
		ccoreExpect(t, twice == 0, "%s: %d of %d heap records are reached twice in the finished tree", in.ID, twice, visited)
		doubleTrees++
		zeroN, balChecks, balRefusals := spike.BalanceStats()
		zeroNodes += zeroN
		balanceChecks += balChecks
		balanceRefusals += balRefusals
		leaves, nodes, copies := spike.RecordSources()
		allocRecs += leaves + nodes + copies
		reachRecs += visited
		spike.ReleaseLast()
		stop2 := spike.Parse(in.Src, nil)
		det := stop2 == stop && spike.Digest() == sd && spike.ArenaDigest() == ad
		spikeEnd := spike.RootEnd() // read before the release: the package reads only live trees
		spike.ReleaseLast()
		fmt.Printf("CCDIGEST input=%s bytes=%d stop=%q equal=%t deterministic=%t c_error=%t c_end=%d spike_end=%d steps=%d tokens=%d max_versions=%d multi_tokens=%d arena_bytes=%d legacy_eq_c=%t\n",
			in.ID, len(in.Src), stop, sd == cd && stop == "", det, cerr, cend, spikeEnd, steps, tokens, maxv, multi, arena, legacyEq)
		ccoreExpect(t, sd == cd && stop == "", "%s: the digest differs from C (stop %q)", in.ID, stop)
		ccoreExpect(t, det, "%s: two parses give different results", in.ID)
	}
	fmt.Printf("CCSTOREINT test=digest lang=%s shared_in_tree=%d double_free=%d bad_retain=%d\n", name, spike.SharedInTree(), spike.DoubleFree(), spike.BadRetain())
	ccoreExpectStore(t, "TestCcoreDigest "+name, spike)
	fmt.Printf("CCDOUBLE test=digest lang=%s trees=%d\n", name, doubleTrees)
	fmt.Printf("CCBALANCE test=digest lang=%s trees=%d zero_width_nodes=%d walks=%d refusals=%d\n", name, doubleTrees, zeroNodes, balanceChecks, balanceRefusals)
	// A fresh parse leaves the records that the finished tree does not reach in
	// the store until the store ends. For the CI input sets (not the eof and
	// session sets, whose inputs are tiny or near-identical) the share over the
	// whole set must stay at or below 10 percent.
	fmt.Printf("CCUNREACHSHARE lang=%s records=%d reachable=%d unreachable_pct=%.3f\n", name, allocRecs, reachRecs, 100*float64(allocRecs-reachRecs)/float64(max(allocRecs, 1)))
	switch name {
	case "json", "python", "c", "typescript":
		ccoreExpect(t, allocRecs > 0 && (allocRecs-reachRecs)*10 <= allocRecs, "%s: %d of %d records of the fresh parses are unreachable, more than 10 percent", name, allocRecs-reachRecs, allocRecs)
	}
	ccoreCount("TestCcoreDigest", name, checked)
}

// BenchmarkCcore runs the arms of design section 7.
func BenchmarkCcore(b *testing.B) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		b.Skip("set CCORE_LANG")
	}
	gl, cl := ccoreLanguages(b, name)
	var inputs []ccoreInput
	for _, in := range ccoreInputs(b, name, false) {
		switch in.ID {
		case "json/sample", "json/1M", "go/generated/32K", "go/generated/137K", "go/generated/1M":
			inputs = append(inputs, in)
		default:
			if ccoreScanBenchID(in.ID) && !strings.HasPrefix(in.ID, "go/") {
				inputs = append(inputs, in)
			}
		}
	}
	arms := strings.Split(os.Getenv("CCORE_ARMS"), ",")
	var cases []benchorder.Case
	for _, in := range inputs {
		for _, arm := range arms {
			in, arm := in, arm
			cases = append(cases, benchorder.Case{Label: strings.ReplaceAll(in.ID, "/", "_") + "_" + arm, Run: func(b *testing.B) {
				b.SetBytes(int64(len(in.Src)))
				switch arm {
				case "spike", "spikewalk":
					sp := ccoreNew(gl)
					if stop := sp.Parse(in.Src, nil); stop != "" {
						b.Skipf("spike stop %q", stop)
					}
					sp.ReleaseLast()
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						sp.Parse(in.Src, nil)
						if arm == "spikewalk" {
							sp.Walk()
						}
						sp.ReleaseLast()
					}
				case "paired": // Go-C-C-Go cycles (ccore_bench_seed_c_test.go)
					sp := ccoreNew(gl)
					if stop := sp.Parse(in.Src, nil); stop != "" {
						b.Skipf("spike stop %q", stop)
					}
					ccoreReleaseLast(sp)
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cl); err != nil {
						b.Fatal(err)
					}
					ccorePairedFresh(b, sp, cp, in.Src)
				case "legacy":
					p := g.NewParser(gl)
					p.SetAdmissionCandidateRoute(false)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tree, err := p.Parse(in.Src)
						if err != nil {
							b.Fatal(err)
						}
						tree.Release()
					}
				case "compact":
					p := g.NewParser(gl)
					p.SetAdmissionCandidateRoute(true)
					r0, f0 := g.AdmissionCandidateCounters()
					if tree, err := p.Parse(in.Src); err == nil {
						tree.Release()
					}
					r1, f1 := g.AdmissionCandidateCounters()
					fmt.Printf("CCCOMPACT input=%s routed=%d fallbacks=%d reason=%q\n", in.ID, r1-r0, f1-f0, g.AdmissionCandidateLastFallbackReason())
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tree, err := p.Parse(in.Src)
						if err != nil {
							b.Fatal(err)
						}
						tree.Release()
					}
				case "c":
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cl); err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tree := cp.Parse(in.Src, nil)
						tree.Close()
					}
				}
			}})
		}
	}
	benchorder.Run(b, cases, ccoreBenchSeed(b))
}

// BenchmarkCcoreSet times one parse of every input of a set per
// operation, for the spike, legacy and C, plus the clean sample of the same
// language (the broken-against-clean comparison).
func BenchmarkCcoreSet(b *testing.B) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		b.Skip("set CCORE_LANG")
	}
	gl, cl := ccoreLanguages(b, name)
	inputs := ccoreInputs(b, name, false)
	cleanLang := "go"
	if strings.HasPrefix(name, "json") {
		cleanLang = "json"
	}
	clean := []ccoreInput{{"clean", ccoreManifestSample(b, cleanLang)}}
	for _, arm := range []string{"spike", "legacy", "c"} {
		for _, set := range []struct {
			id  string
			ins []ccoreInput
		}{{"broken", inputs}, {"clean", clean}} {
			arm, set := arm, set
			b.Run(set.id+"/"+arm, func(b *testing.B) {
				sp := ccoreNew(gl)
				lp := g.NewParser(gl)
				lp.SetAdmissionCandidateRoute(false)
				cp := sitter.NewParser()
				defer cp.Close()
				if err := cp.SetLanguage(cl); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					for _, in := range set.ins {
						switch arm {
						case "spike":
							sp.Parse(in.Src, nil)
							sp.ReleaseLast()
						case "legacy":
							if tree, err := lp.Parse(in.Src); err == nil {
								tree.Release()
							}
						case "c":
							cp.Parse(in.Src, nil).Close()
						}
					}
				}
				b.ReportMetric(float64(len(set.ins)), "parses/op")
			})
		}
	}
}
