//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIncrementalCReuseLedgerReadDependencies(t *testing.T) {
	// The rows affected by certified read invalidation, plus the nearby
	// below-tolerance rows, keep this regression scoped to the affected fixtures.
	fixtures := map[string]bool{}
	for _, name := range []string{"awk", "c", "cmake", "css", "eex", "elisp", "elixir", "fidl", "forth", "gn", "go", "graphql", "html", "jq", "meson", "racket", "ron", "sparql", "ssh_config", "twig", "v"} {
		fixtures[name] = true
	}
	data, err := os.ReadFile("../internal/benchfixtures/real_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language string `json:"language"`
			Role     string `json:"role"`
			Path     string `json:"committed_path"`
			SHA256   string `json:"sha256"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, item := range manifest.Entries {
		if item.Role != "sample" || item.Path == "" || !fixtures[item.Language] {
			continue
		}
		t.Run(item.Language, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("../internal/benchfixtures", item.Path))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != item.SHA256 {
				t.Fatalf("sample digest=%s, want %s", got, item.SHA256)
			}
			step := benchfixtures.EditingSession(source)[0]
			lang := grammars.DetectLanguageByName(item.Language).Language()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			cl, err := COracleLanguage(item.Language)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			cOld := cp.Parse(source, nil)
			defer cOld.Close()
			old.Edit(step.Edit)
			cEdit := realCorpusCInputEdit(step.Edit)
			cOld.Edit(&cEdit)
			cDirty, matchedDirty := 0, 0
			var walk func(*sitter.Node)
			walk = func(n *sitter.Node) {
				if n.HasChanges() && n.EndByte() < uint(step.Edit.StartByte) {
					cDirty++
					goNode := old.RootNode().DescendantForByteRange(uint32(n.StartByte()), uint32(n.EndByte()))
					if goNode != nil && goNode.HasChanges() {
						matchedDirty++
					}
				}
				for i := uint(0); i < n.ChildCount(); i++ {
					walk(n.Child(i))
				}
			}
			walk(cOld.RootNode())
			next, profile, err := p.ParseIncrementalProfiled(step.Source, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			fresh, err := p.Parse(step.Source)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != want.SHA256 {
				t.Fatalf("ledger edit incremental=%s fresh=%s", got.SHA256, want.SHA256)
			}
			ct := cp.Parse(step.Source, nil)
			defer ct.Close()
			oracle, err := COracleDeepDigest(ct)
			if err != nil {
				t.Fatal(err)
			}
			// C, JQ, and Meson have existing fresh-Go/C differences on
			// these malformed samples; every other fixture is exact.
			if item.Language != "c" && item.Language != "jq" && item.Language != "meson" && got.SHA256 != oracle {
				t.Fatalf("ledger edit Go=%s locked C=%s", got.SHA256, oracle)
			}
			if item.Language == "meson" {
				// Existing extra Go wrappers keep the full digests different.
				// Every locked C node must still be represented: the former
				// incremental tree dropped five variableunit reductions.
				goRecords := map[string]int{}
				gts.Walk(next.RootNode(), func(n *gts.Node, _ int) gts.WalkAction {
					key := fmt.Sprintf("%s:%d:%d:%t:%d", n.Type(lang), n.StartByte(), n.EndByte(), n.IsNamed(), n.ChildCount())
					goRecords[key]++
					return gts.WalkContinue
				})
				var visit func(*sitter.Node)
				visit = func(n *sitter.Node) {
					key := fmt.Sprintf("%s:%d:%d:%t:%d", n.Kind(), n.StartByte(), n.EndByte(), n.IsNamed(), n.ChildCount())
					if goRecords[key] == 0 {
						t.Fatalf("incremental tree dropped locked C node %s", key)
					}
					goRecords[key]--
					for i := uint(0); i < n.ChildCount(); i++ {
						visit(n.Child(i))
					}
				}
				visit(ct.RootNode())
			}
			t.Logf("READ_DEPENDENCIES language=%s source=%s edit=%d C_dirty_before_edit=%d Go_matches=%d tokens=%d nodes=%d reused_bytes=%d incremental=%s fresh=%s C=%s Go_error=%t C_error=%t", item.Language, item.SHA256, step.Edit.StartByte, cDirty, matchedDirty, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedBytes, got.SHA256, want.SHA256, oracle, next.RootNode().HasError(), ct.RootNode().HasError())
		})
	}
}

func TestIncrementalCReuseEOFInvalidatesDartLibrary(t *testing.T) {
	source := []byte("library;")
	edited := []byte("library;\n")
	lang := grammars.DartLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	cl, err := COracleLanguage("dart")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	cOld := cp.Parse(source, nil)
	defer cOld.Close()
	edit := canonicalGoInputEdit(source, edited, len(source), len(source), len(edited))
	old.Edit(edit)
	cEdit := realCorpusCInputEdit(edit)
	cOld.Edit(&cEdit)
	if !cOld.RootNode().Child(0).HasChanges() {
		t.Fatal("locked C did not invalidate its EOF-dependent library subtree")
	}
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	ct := cp.Parse(edited, cOld)
	defer ct.Close()
	assertLockedCTreeExact(t, "EOF-dependent Dart library", next, lang, ct)
	var cleanLeafBytes, cleanLeaves uint64
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n.ChildCount() == 0 {
			if !n.HasChanges() {
				cleanLeafBytes += uint64(n.EndByte() - n.StartByte())
				cleanLeaves++
			}
			return
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(cOld.RootNode().Child(0))
	if profile.NewNodesAllocated == 0 || profile.ReusedBytes > cleanLeafBytes || profile.ReusedSubtrees > cleanLeaves {
		t.Fatalf("EOF-dependent library was not rebuilt from C-clean leaves: %+v; clean leaves=%d bytes=%d", profile, cleanLeaves, cleanLeafBytes)
	}
	t.Logf("Dart EOF reused subtrees=%d bytes=%d C-clean leaves=%d bytes=%d", profile.ReusedSubtrees, profile.ReusedBytes, cleanLeaves, cleanLeafBytes)
}

func TestIncrementalCReuseEOFInvalidatesHTTPSections(t *testing.T) {
	source := []byte("### a\n# c\nGET /\n### b")
	edited := append(append([]byte(nil), source...), '\n')
	lang := grammars.HttpLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	cl, err := COracleLanguage("http")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	cOld := cp.Parse(source, nil)
	defer cOld.Close()
	edit := canonicalGoInputEdit(source, edited, len(source), len(source), len(edited))
	old.Edit(edit)
	cEdit := realCorpusCInputEdit(edit)
	cOld.Edit(&cEdit)
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n.EndByte() < uint(len(source)) && !n.HasChanges() {
			goNode := old.RootNode().DescendantForByteRange(uint32(n.StartByte()), uint32(n.EndByte()))
			if goNode == nil || goNode.StartByte() != uint32(n.StartByte()) || goNode.EndByte() != uint32(n.EndByte()) || goNode.HasChanges() {
				t.Fatalf("C-clean prefix %s [%d,%d] lost its unchanged Go span", n.Kind(), n.StartByte(), n.EndByte())
			}
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(cOld.RootNode())
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	ct := cp.Parse(edited, cOld)
	defer ct.Close()
	assertLockedCTreeExact(t, "EOF-dependent HTTP sections", next, lang, ct)
	if profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("C-clean HTTP prefix lost fresh-verified reuse: %+v", profile)
	}
	t.Logf("HTTP EOF reused subtrees=%d bytes=%d", profile.ReusedSubtrees, profile.ReusedBytes)
}

func TestIncrementalCReuseReceiptEOFMatchesFresh(t *testing.T) {
	for _, langName := range []string{"solidity", "wgsl"} {
		t.Run(langName, func(t *testing.T) {
			var sources [][]byte
			if langName == "solidity" {
				for _, path := range []string{"small__IERC3156.sol", "medium__Initializable.sol", "large__Packing.sol"} {
					data, err := os.ReadFile(filepath.Join("../testdata/dispatcher_census_a0/solidity", path))
					if err != nil {
						t.Fatal(err)
					}
					sources = append(sources, data)
				}
				sources = append(sources, []byte("contract C { function f(address a) public view returns (address) { return a.owner; } }\n"), []byte("contract C { function f(uint256 x) public pure returns (uint256) { return uint256(x); } }\n"))
			} else {
				for _, path := range []string{"medium__radiosity.wgsl", "small__fragmentTextureQuad.wgsl"} {
					data, err := os.ReadFile(filepath.Join("../testdata/dispatcher_census_a0/wgsl", path))
					if err != nil {
						t.Fatal(err)
					}
					sources = append(sources, data)
				}
				sources = append(sources, []byte("fn main() {}\n"))
			}
			for index, source := range sources {
				t.Run(fmt.Sprint(index), func(t *testing.T) {
					lang := grammars.DetectLanguageByName(langName).Language()
					p := gts.NewParser(lang)
					p.SetAdmissionCandidateRoute(false)
					base := bytes.TrimSuffix(source, []byte{'\n'})
					old, err := p.Parse(base)
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					old.Edit(canonicalGoInputEdit(base, source, len(base), len(base), len(source)))
					next, profile, err := p.ParseIncrementalProfiled(source, old)
					if err != nil {
						t.Fatal(err)
					}
					defer next.Release()
					fresh, err := p.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
					if err != nil {
						t.Fatal(err)
					}
					want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
					if err != nil {
						t.Fatal(err)
					}
					if got.SHA256 != want.SHA256 {
						t.Fatalf("EOF append incremental=%s fresh=%s", got.SHA256, want.SHA256)
					}
					t.Logf("EOF_RECEIPT language=%s index=%d digest=%s fresh=%s subtrees=%d reused_bytes=%d tokens=%d nodes=%d", langName, index, got.SHA256, want.SHA256, profile.ReusedSubtrees, profile.ReusedBytes, profile.TokensConsumed, profile.NewNodesAllocated)
				})
			}
		})
	}
}

func cReuseFixture(tb testing.TB, name string) ([]byte, []byte, gts.InputEdit, *gts.Language) {
	tb.Helper()
	return cReuseFixtureAtSize(tb, name, 137*1024)
}

func cReuseFixtureAtSize(tb testing.TB, name string, size int) ([]byte, []byte, gts.InputEdit, *gts.Language) {
	tb.Helper()
	source, marker, err := benchfixtures.GeneratedSource(name, size)
	if err != nil {
		tb.Fatal(err)
	}
	at := bytes.Index(source, []byte(marker))
	if at < 0 {
		tb.Fatal("missing edit marker")
	}
	edited := bytes.Clone(source)
	edited[at] = 'y'
	edit := canonicalGoInputEdit(source, edited, at, at+1, at+1)
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		tb.Fatal("missing language")
	}
	return source, edited, edit, entry.Language()
}

func TestIncrementalCReuse137K(t *testing.T) {
	for _, name := range []string{"go", "java"} {
		t.Run(name, func(t *testing.T) {
			source, edited, edit, lang := cReuseFixture(t, name)
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			cLang, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			for step := 0; step < 4; step++ {
				from, to := source, edited
				if step%2 != 0 {
					from, to = edited, source
				}
				cReuseBeginWorkCount()
				old.Edit(edit)
				next, profile, err := p.ParseIncrementalProfiled(to, old)
				if err != nil {
					t.Fatal(err)
				}
				cReuseEndWorkCount(t)
				fresh, err := p.Parse(to)
				if err != nil {
					t.Fatal(err)
				}
				ct := cp.Parse(to, nil)
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				cDigest, err := COracleDeepDigest(ct)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 || got.SHA256 != cDigest {
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s", step, got.SHA256, want.SHA256, cDigest)
				}
				t.Logf("COUNTERS language=%s bytes=%d source=%x step=%d tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d mismatch=%d fallback=%s", name, len(to), sha256.Sum256(from), step, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, profile.ReuseObservedPreGotoStateMismatch, profile.ReuseUnsupportedReason)
				fresh.Release()
				ct.Close()
				old.Release()
				old = next
			}
		})
	}
}

// The fleet session also retains the malformed Julia boundary witness as a
// D8 regression. Its fresh Go recovery tree already differs from C; this
// clean SQL witness authenticates the boundary fix against both runtimes.
func TestIncrementalCReuseKeywordBoundary(t *testing.T) {
	for _, tc := range []struct{ name, source, boundary string }{
		{"sql", "SELECT id, name FROM users WHERE id = 1;\n", " users"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			at := bytes.LastIndex(source, []byte(tc.boundary))
			edited := bytes.Clone(source)
			edited[at] = 'x'
			lang := grammars.DetectLanguageByName(tc.name).Language()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			old.Edit(canonicalGoInputEdit(source, edited, at, at+1, at+1))
			next, err := p.ParseIncremental(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			fresh, err := p.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			cl, err := COracleLanguage(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(edited, nil)
			defer ct.Close()
			got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			oracle, err := COracleDeepDigest(ct)
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != want.SHA256 || got.SHA256 != oracle {
				t.Fatalf("incremental=%s fresh=%s C=%s Go-tree=%s C-tree=%s", got.SHA256, want.SHA256, oracle, next.RootNode().SExpr(lang), ct.RootNode().ToSexp())
			}
		})
	}
}

func BenchmarkIncrementalCReuse137K(b *testing.B) {
	for _, name := range []string{"go", "java"} {
		b.Run(name, func(b *testing.B) { benchmarkIncrementalCReuse(b, name, 137*1024) })
	}
}

func BenchmarkIncrementalCReuseSizes(b *testing.B) {
	for _, name := range []string{"go", "java"} {
		b.Run(name, func(b *testing.B) {
			for _, size := range []int{32, 1024} {
				b.Run(fmt.Sprintf("%dKiB", size), func(b *testing.B) { benchmarkIncrementalCReuse(b, name, size*1024) })
			}
		})
	}
}

func benchmarkIncrementalCReuse(b *testing.B, name string, size int) {
	source, edited, edit, lang := cReuseFixtureAtSize(b, name, size)
	for _, engine := range []string{"Go", "C"} {
		b.Run(engine, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			if engine == "Go" {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(false)
				tree, err := p.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				defer func() { tree.Release() }()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					to := edited
					if i%2 != 0 {
						to = source
					}
					tree.Edit(edit)
					next, err := p.ParseIncremental(to, tree)
					if err != nil {
						b.Fatal(err)
					}
					tree.Release()
					tree = next
				}
			} else {
				cl, err := COracleLanguage(name)
				if err != nil {
					b.Fatal(err)
				}
				p := sitter.NewParser()
				defer p.Close()
				if err := p.SetLanguage(cl); err != nil {
					b.Fatal(err)
				}
				tree := p.Parse(source, nil)
				defer func() { tree.Close() }()
				ce := sitter.InputEdit{StartByte: uint(edit.StartByte), OldEndByte: uint(edit.OldEndByte), NewEndByte: uint(edit.NewEndByte), StartPosition: sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)}}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					to := edited
					if i%2 != 0 {
						to = source
					}
					tree.Edit(&ce)
					next := p.Parse(to, tree)
					if next == nil {
						b.Fatal(fmt.Sprintf("C parse failed at %d", i))
					}
					tree.Close()
					tree = next
				}
			}
		})
	}
}
