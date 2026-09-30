//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func verifyCostSource(tb testing.TB, name string) ([]byte, int, *gts.Language) {
	tb.Helper()
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		tb.Fatal("missing grammar")
	}
	if name == "powershell" {
		var source bytes.Buffer
		for i := 0; source.Len() < 15*1024; i++ {
			fmt.Fprintf(&source, "$value%06d = 100\nWrite-Output $value%06d\n", i, i)
		}
		return source.Bytes(), source.Len(), entry.Language()
	}
	size := 1024 * 1024
	if name == "go" {
		size = 137 * 1024
	}
	source, marker, err := benchfixtures.GeneratedSource(name, size)
	if err != nil {
		tb.Fatal(err)
	}
	at := bytes.Index(source, []byte(marker))
	if at < 0 {
		tb.Fatal("missing generated edit marker")
	}
	return source, at, entry.Language()
}

func verifyCostCEdit(edit gts.InputEdit) sitter.InputEdit {
	return sitter.InputEdit{
		StartByte: uint(edit.StartByte), OldEndByte: uint(edit.OldEndByte), NewEndByte: uint(edit.NewEndByte),
		StartPosition:  sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)},
		OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)},
		NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)},
	}
}

// Run each grammar in a separate Docker process. The typing session compares
// every intermediate tree, including partial commands, with fresh Go and C.
func TestIncrementalVerifyCost(t *testing.T) {
	for _, name := range []string{"powershell", "c_sharp", "go"} {
		t.Run(name, func(t *testing.T) {
			source, at, lang := verifyCostSource(t, name)
			p := gts.NewParser(lang)
			if name != "c_sharp" {
				p.SetAdmissionCandidateRoute(true)
			}
			cl, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			tree, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { tree.Release() }()
			initial := bytes.Clone(source)
			steps := 4
			typed := "Write-Output x\n"
			if name == "powershell" {
				steps = len(typed)
			}
			for step := 0; step < steps; step++ {
				nextSource := bytes.Clone(source)
				oldEnd, newEnd := at+1, at+1
				if name == "powershell" {
					at = len(source)
					oldEnd, newEnd = at, at+1
					nextSource = append(nextSource, typed[step])
				} else if step%2 == 0 {
					nextSource[at] = 'y'
				} else {
					nextSource[at] = initial[at]
				}
				edit := canonicalGoInputEdit(source, nextSource, at, oldEnd, newEnd)
				verifyCostBeginWorkCount()
				started := time.Now()
				tree.Edit(edit)
				editNanos := time.Since(started).Nanoseconds()
				next, profile, err := p.ParseIncrementalProfiled(nextSource, tree)
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
				cDigest, err := COracleDeepDigest(ct)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 || got.SHA256 != cDigest {
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s", step, got.SHA256, want.SHA256, cDigest)
				}
				if next.RootNode().EndByte() != uint32(len(nextSource)) ||
					(next.RootNode().IsError() && !next.RootNode().HasError()) {
					t.Fatal("incremental tree violates coverage/error invariant")
				}
				t.Logf("COUNTERS language=%s source=%x bytes=%d step=%d tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d edit_ns=%d reuse_ns=%d reparse_ns=%d fallback=%s", name, sha256.Sum256(nextSource), len(nextSource), step, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, editNanos, profile.ReuseCursorNanos, profile.ReparseNanos, profile.ReuseUnsupportedReason)
				fresh.Release()
				ct.Close()
				tree.Release()
				tree, source = next, nextSource
			}
			if allocs := testing.AllocsPerRun(5, func() {
				next, err := p.ParseIncremental(source, tree)
				if err != nil {
					t.Fatal(err)
				}
				next.Release()
			}); allocs != 0 {
				t.Fatalf("no-edit reparse allocations=%g", allocs)
			}
		})
	}
}

func BenchmarkIncrementalVerifyCost(b *testing.B) {
	for _, name := range []string{"powershell", "c_sharp", "go"} {
		b.Run(name, func(b *testing.B) {
			source, at, lang := verifyCostSource(b, name)
			edited := bytes.Clone(source)
			oldEnd, newEnd := at+1, at+1
			if name == "powershell" {
				edited = append(edited, 'x')
				oldEnd, newEnd = at, at+1
			} else {
				edited[at] = 'y'
			}
			forward := canonicalGoInputEdit(source, edited, at, oldEnd, newEnd)
			reverse := canonicalGoInputEdit(edited, source, at, newEnd, oldEnd)
			engines := []string{"Go", "C"}
			for _, index := range verifyCostBenchOrder(b.Name(), len(engines)) {
				engine := engines[index]
				b.Run(engine, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(source)))
					if engine == "Go" {
						p := gts.NewParser(lang)
						if name != "c_sharp" {
							p.SetAdmissionCandidateRoute(true)
						}
						tree, err := p.Parse(source)
						if err != nil {
							b.Fatal(err)
						}
						defer func() { tree.Release() }()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							to, edit := edited, forward
							if i%2 != 0 {
								to, edit = source, reverse
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
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							to, edit := edited, verifyCostCEdit(forward)
							if i%2 != 0 {
								to, edit = source, verifyCostCEdit(reverse)
							}
							tree.Edit(&edit)
							next := p.Parse(to, tree)
							if next == nil {
								b.Fatal("C incremental parse returned no tree")
							}
							tree.Close()
							tree = next
						}
					}
				})
			}
		})
	}
}
