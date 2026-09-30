//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strconv"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func scannerFocusSource(tb testing.TB, name string, size int) ([]byte, int, *gts.Language) {
	tb.Helper()
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		tb.Fatal("grammar missing")
	}
	if name == "c_sharp" {
		source, marker, err := benchfixtures.GeneratedSource(name, size)
		if err != nil {
			tb.Fatal(err)
		}
		return source, bytes.Index(source, []byte(marker)), entry.Language()
	}
	var source bytes.Buffer
	sites := []int{}
	for i := 0; source.Len() < size; i++ {
		var line string
		switch name {
		case "bash":
			line = fmt.Sprintf("echo value_%06d\n", i)
		case "blade":
			line = fmt.Sprintf("<div>value_%06d</div>\n", i)
		case "properties":
			line = fmt.Sprintf("value_%06d=%d\n", i, i)
		}
		sites = append(sites, source.Len()+bytes.IndexByte([]byte(line), '_')+1)
		source.WriteString(line)
	}
	return source.Bytes(), sites[len(sites)/2], entry.Language()
}

func scannerFocusCEdit(edit gts.InputEdit) sitter.InputEdit {
	return sitter.InputEdit{StartByte: uint(edit.StartByte), OldEndByte: uint(edit.OldEndByte), NewEndByte: uint(edit.NewEndByte), StartPosition: sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)}}
}

func TestScannerFocusLockedC(t *testing.T) {
	for _, name := range []string{"bash", "blade", "properties", "c_sharp"} {
		t.Run(name, func(t *testing.T) {
			for _, size := range []int{4096, 137 << 10, 1 << 20} {
				t.Run(fmt.Sprint(size), func(t *testing.T) {
					source, at, lang := scannerFocusSource(t, name, size)
					cl, err := COracleLanguage(name)
					if err != nil {
						t.Fatal(err)
					}
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cl); err != nil {
						t.Fatal(err)
					}
					for _, route := range []bool{false, true} {
						t.Run(fmt.Sprintf("compact=%t", route), func(t *testing.T) {
							p := gts.NewParser(lang)
							p.SetAdmissionCandidateRoute(route)
							old, err := p.Parse(source)
							if err != nil {
								t.Fatal(err)
							}
							defer func() { old.Release() }()
							current := bytes.Clone(source)
							for step := 0; step < 4; step++ {
								edited := bytes.Clone(current)
								if step%2 == 0 {
									edited[at] = 'y'
								} else {
									edited[at] = source[at]
								}
								edit := canonicalGoInputEdit(current, edited, at, at+1, at+1)
								old.Edit(edit)
								next, profile, err := p.ParseIncrementalProfiled(edited, old)
								if err != nil {
									t.Fatal(err)
								}
								fresh, err := p.Parse(edited)
								if err != nil {
									next.Release()
									t.Fatal(err)
								}
								ct := cp.Parse(edited, nil)
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
								root := next.RootNode()
								if root.EndByte() != uint32(len(edited)) || (root.IsError() && !root.HasError()) {
									t.Fatal("coverage/error invariant")
								}
								t.Logf("COUNTERS source=%x bytes=%d step=%d tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d reuse_ns=%d reparse_ns=%d fallback=%s", sha256.Sum256(edited), len(edited), step, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, profile.ReuseCursorNanos, profile.ReparseNanos, profile.ReuseUnsupportedReason)
								fresh.Release()
								ct.Close()
								old.Release()
								old = next
								current = edited
							}
							if n := testing.AllocsPerRun(5, func() {
								next, err := p.ParseIncremental(current, old)
								if err != nil {
									t.Fatal(err)
								}
								next.Release()
							}); n != 0 {
								t.Fatalf("no-edit allocations=%g", n)
							}
						})
					}
				})
			}
		})
	}
}

func scannerFocusBenchOrder(name string, count int) []int {
	order := make([]int, count)
	for i := range order {
		order[i] = i
	}
	shuffle := flag.Lookup("test.shuffle")
	if shuffle == nil {
		return order
	}
	seed, err := strconv.ParseInt(shuffle.Value.String(), 10, 64)
	if err != nil {
		return order
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	rng := rand.New(rand.NewSource(seed ^ int64(h.Sum64())))
	rng.Shuffle(count, func(i, j int) { order[i], order[j] = order[j], order[i] })
	return order
}

// Each operation includes Tree.Edit, parse, and releasing the previous tree.
// Initial loading and fixture generation are outside the measured region.
func BenchmarkScannerFocusCompleteEdit(b *testing.B) {
	for _, name := range []string{"bash", "blade", "properties", "c_sharp"} {
		b.Run(name, func(b *testing.B) {
			for _, size := range []int{137 << 10, 1 << 20} {
				b.Run(fmt.Sprint(size), func(b *testing.B) {
					source, at, lang := scannerFocusSource(b, name, size)
					edited := bytes.Clone(source)
					edited[at] = 'y'
					forward := canonicalGoInputEdit(source, edited, at, at+1, at+1)
					reverse := canonicalGoInputEdit(edited, source, at, at+1, at+1)
					engines := []string{"Go", "C", "C", "Go"}
					for _, index := range scannerFocusBenchOrder(b.Name(), len(engines)) {
						engine := engines[index]
						b.Run(fmt.Sprintf("%s%d", engine, index), func(b *testing.B) {
							b.ReportAllocs()
							b.SetBytes(int64(len(source)))
							if engine == "Go" {
								p := gts.NewParser(lang)
								p.SetAdmissionCandidateRoute(false)
								old, err := p.Parse(source)
								if err != nil {
									b.Fatal(err)
								}
								defer func() { old.Release() }()
								b.ResetTimer()
								for i := 0; i < b.N; i++ {
									to, edit := edited, forward
									if i%2 != 0 {
										to, edit = source, reverse
									}
									old.Edit(edit)
									next, err := p.ParseIncremental(to, old)
									if err != nil {
										b.Fatal(err)
									}
									old.Release()
									old = next
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
								old := p.Parse(source, nil)
								defer func() { old.Close() }()
								cf, cr := scannerFocusCEdit(forward), scannerFocusCEdit(reverse)
								b.ResetTimer()
								for i := 0; i < b.N; i++ {
									to, edit := edited, cf
									if i%2 != 0 {
										to, edit = source, cr
									}
									old.Edit(&edit)
									next := p.Parse(to, old)
									if next == nil {
										b.Fatal("C parse returned nil")
									}
									old.Close()
									old = next
								}
							}
						})
					}
				})
			}
		})
	}
}

func BenchmarkScannerFocusFull(b *testing.B) {
	for _, name := range []string{"properties", "bash", "blade", "c_sharp"} {
		b.Run(name, func(b *testing.B) {
			source, _, lang := scannerFocusSource(b, name, 137<<10)
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			warm, err := p.Parse(source)
			if err != nil {
				b.Fatal(err)
			}
			warm.Release()
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree, err := p.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				tree.Release()
			}
		})
	}
}
