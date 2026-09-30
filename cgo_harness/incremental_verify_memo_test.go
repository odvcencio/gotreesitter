//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func verifyMemoSource(size int) ([]byte, int) {
	var source bytes.Buffer
	source.WriteString("package p\n")
	for i := 0; source.Len() < size; i++ {
		fmt.Fprintf(&source, "func f%d() { _ = 1 }\n", i)
	}
	return source.Bytes(), bytes.Index(source.Bytes(), []byte("_ = 1")) + 4
}

func TestIncrementalVerifyMemoLockedC(t *testing.T) {
	verifyMemoLockedC(t, 19*1024)
}

func verifyMemoLockedC(t *testing.T, size int) {
	t.Helper()
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact_%t", compact), func(t *testing.T) {
			lang := grammars.GoLanguage()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			cl, err := COracleLanguage("go")
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			source, at := verifyMemoSource(size)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			for step := 0; step < 6; step++ {
				nextSource := bytes.Clone(source)
				nextSource[at] = '2'
				if step%2 != 0 {
					nextSource[at] = '1'
				}
				edit := canonicalGoInputEdit(source, nextSource, at, at+1, at+1)
				verifyCostBeginWorkCount()
				old.Edit(edit)
				next, profile, err := p.ParseIncrementalProfiled(nextSource, old)
				verifyCostEndWorkCount(t)
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := p.Parse(nextSource)
				if err != nil {
					t.Fatal(err)
				}
				ct := cp.Parse(nextSource, nil)
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
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s", step, got.SHA256, want.SHA256, oracle)
				}
				if next.RootNode().EndByte() != uint32(len(nextSource)) ||
					(next.RootNode().IsError() && !next.RootNode().HasError()) {
					t.Fatal("numeric edit violates root coverage/error invariant")
				}
				if profile.TokenInvariantDependencyChecks != 1 || profile.NewNodesAllocated != 0 || profile.ReusedBytes != uint64(len(nextSource)) || profile.ReparseNanos != 0 {
					t.Fatalf("numeric edit lost authenticated reuse: %+v", profile)
				}
				t.Logf("step=%d bytes=%d nodes=%d reused=%d dependencies=%d", step, len(nextSource), profile.NewNodesAllocated, profile.ReusedBytes, profile.TokenInvariantDependencyChecks)
				fresh.Release()
				ct.Close()
				old.Release()
				old, source = next, nextSource
			}
			if allocs := testing.AllocsPerRun(5, func() {
				next, err := p.ParseIncremental(source, old)
				if err != nil {
					t.Fatal(err)
				}
				next.Release()
			}); allocs != 0 {
				t.Fatalf("no-edit allocations=%g", allocs)
			}
		})
	}
}

func BenchmarkIncrementalVerifyMemo(b *testing.B) {
	benchmarkIncrementalVerifyMemo(b, 19*1024)
}

func benchmarkIncrementalVerifyMemo(b *testing.B, size int) {
	b.Helper()
	engines := []string{"Go", "C"}
	for _, index := range verifyCostBenchOrder(b.Name(), len(engines)) {
		engine := engines[index]
		b.Run(engine, func(b *testing.B) {
			source, at := verifyMemoSource(size)
			edited := bytes.Clone(source)
			edited[at] = '2'
			forward := canonicalGoInputEdit(source, edited, at, at+1, at+1)
			reverse := canonicalGoInputEdit(edited, source, at, at+1, at+1)
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			if engine == "Go" {
				p := gts.NewParser(grammars.GoLanguage())
				old, err := p.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				defer func() { old.Release() }()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					nextSource, edit := edited, forward
					if i%2 != 0 {
						nextSource, edit = source, reverse
					}
					old.Edit(edit)
					next, err := p.ParseIncremental(nextSource, old)
					if err != nil {
						b.Fatal(err)
					}
					old.Release()
					old = next
				}
			} else {
				cl, err := COracleLanguage("go")
				if err != nil {
					b.Fatal(err)
				}
				p := sitter.NewParser()
				defer p.Close()
				if err := p.SetLanguage(cl); err != nil {
					b.Fatal(err)
				}
				old := p.Parse(source, nil)
				defer func() { old.Close() }()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					nextSource, edit := edited, forward
					if i%2 != 0 {
						nextSource, edit = source, reverse
					}
					cEdit := verifyCostCEdit(edit)
					old.Edit(&cEdit)
					next := p.Parse(nextSource, old)
					if next == nil {
						b.Fatal("C parse returned no tree")
					}
					old.Close()
					old = next
				}
			}
		})
	}
}
