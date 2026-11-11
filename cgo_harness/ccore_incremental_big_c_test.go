//go:build cgo && treesitter_c_parity

package cgoharness

// Input (d): the 1 MiB
// generated Go file, three single-byte edits against pinned C, a 1,000-step
// chain of single-token edits against the core's fresh parse (C on every
// 50th), arena growth, and the edit-plus-reparse benchmark.

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func ccoreIsIdentInterior(src []byte, at int) bool {
	letter := func(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' }
	return at > 0 && at+1 < len(src) && src[at] >= 'a' && src[at] <= 'z' && letter(src[at-1]) && letter(src[at+1])
}

// ccoreReplaceEdit replaces one byte; the edit is one byte long on both sides.
func ccoreReplaceEdit(src []byte, at int, b byte) ([]byte, bindtest.Edit) {
	out := append([]byte(nil), src...)
	out[at] = b
	row, col := ccorePointAt(src, at)
	return out, bindtest.Edit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1),
		StartRow: row, StartCol: col, OldEndRow: row, OldEndCol: col + 1, NewEndRow: row, NewEndCol: col + 1}
}

func ccoreBigEdits(t testing.TB) ([]byte, []string, [][]byte, []bindtest.Edit) {
	src, _, err := benchfixtures.GeneratedSource("go", 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(src) != 1048591 || src[524340] != 'x' {
		t.Fatalf("1 MiB fixture: %d bytes, byte 524340 = %q", len(src), src[524340])
	}
	find := func(from int) int {
		for at := from; at < len(src); at++ {
			if ccoreIsIdentInterior(src, at) {
				return at
			}
		}
		t.Fatal("no identifier byte")
		return 0
	}
	names := []string{"mid_524340", "start", "end"}
	positions := []int{524340, find(2000), find(len(src) - 2000)}
	var srcs [][]byte
	var edits []bindtest.Edit
	for i, at := range positions {
		b := byte('y')
		if i > 0 {
			b = 'q'
			if src[at] == 'q' {
				b = 'r'
			}
		}
		s, e := ccoreReplaceEdit(src, at, b)
		srcs = append(srcs, s)
		edits = append(edits, e)
	}
	return src, names, srcs, edits
}

func ccoreCountLines(lines []string, prefix string) int {
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// TestCcoreBigChain is input (d): three edits against C, then the chain. It
// parses a 1 MiB file about a thousand times, so it runs only when CCORE_HEAVY=1
// is set; CI does not run it.
func TestCcoreBigChain(t *testing.T) {
	if os.Getenv("CCORE_HEAVY") == "" {
		t.Skip("heavy: set CCORE_HEAVY=1")
	}
	gl, cl := ccoreLanguages(t, "go")
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var clog []string
	logOn := false
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if logOn && kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	src, names, srcs, edits := ccoreBigEdits(t)
	core := ccoreNew(gl)
	fresh := ccoreNew(gl)
	if stop := core.Parse(src, nil); stop != "" {
		t.Fatalf("fresh parse stop %q", stop)
	}
	base := core.Tree()
	cbase := cp.Parse(src, nil)
	defer cbase.Close()
	for i := range edits {
		ce := cbase.Clone()
		ce.Edit(ccoreSitterEdit(&edits[i]))
		clog = clog[:0]
		logOn = true
		cn := cp.Parse(srcs[i], ce)
		logOn = false
		cInc := ccoreCDigestChanges(cn, false)
		var slog []string
		edited := core.Edit(base, edits[i])
		tree, stop := core.ParseIncremental(srcs[i], edited, func(s string) { slog = append(slog, s) })
		first := -1
		for k := 0; k < len(clog) || k < len(slog); k++ {
			if k >= len(clog) || k >= len(slog) || clog[k] != slog[k] {
				first = k
				break
			}
		}
		fresh.Parse(srcs[i], nil)
		nodes, bytes, largest := core.ReuseStats()
		cf := cp.Parse(srcs[i], nil)
		cFresh := ccoreCDigestChanges(cf, false)
		cf.Close()
		sInc := core.DigestOf(tree, false)
		fmt.Printf("CCBIG edit=%s stop=%q trace_eq=%t first_diff=%d c_lines=%d inc_eq_c=%t inc_eq_fresh=%t c_inc_eq_c_fresh=%t core_reused=%d core_reused_bytes=%d core_largest=%d c_reused=%d core_lexed=%d c_lexed=%d core_reduce=%d c_reduce=%d\n",
			names[i], stop, first < 0, first, len(clog), sInc == cInc, sInc == fresh.Digest(), cInc == cFresh, nodes, bytes, largest,
			ccoreCountLines(clog, "reuse_node"), ccoreCountLines(slog, "lexed_lookahead"), ccoreCountLines(clog, "lexed_lookahead"),
			ccoreCountLines(slog, "reduce sym"), ccoreCountLines(clog, "reduce sym"))
		ccoreExpect(t, stop == "" && first < 0 && sInc == cInc && sInc == fresh.Digest(), "%s: incremental parse differs (stop %q, first_diff %d)", names[i], stop, first)
		if first >= 0 {
			from := first - 2
			if from < 0 {
				from = 0
			}
			for k := from; k < first+3; k++ {
				c, s := "<none>", "<none>"
				if k < len(clog) {
					c = clog[k]
				}
				if k < len(slog) {
					s = slog[k]
				}
				fmt.Printf("CCBIG   %d C: %s\nCCBIG   %d S: %s\n", k, c, k, s)
			}
		}
		core.Release(tree)
		core.Release(edited)
		cn.Close()
		ce.Close()
	}
	// The 1,000-step chain of single-token edits, from the fresh tree.
	used0, reach0 := core.ArenaUse(base)
	fmt.Printf("CCARENA step=fresh used=%d reachable=%d\n", used0, reach0)
	fmt.Printf("CCSTORE step=fresh %s\n", core.StoreInfo())
	rng := rand.New(rand.NewSource(1000))
	cur := append([]byte(nil), src...)
	tree := base
	eqFresh, eqC, cChecks := 0, 0, 0
	chainSteps := 1000
	if v, err := strconv.Atoi(os.Getenv("CCORE_CHAIN_STEPS")); err == nil && v > 0 {
		chainSteps = v
	}
	for step := 1; step <= chainSteps; step++ {
		var at int
		for {
			at = rng.Intn(len(cur))
			if ccoreIsIdentInterior(cur, at) {
				break
			}
		}
		b := byte('a' + rng.Intn(26))
		if b == cur[at] {
			b = 'z'
			if cur[at] == 'z' {
				b = 'y'
			}
		}
		next, e := ccoreReplaceEdit(cur, at, b)
		edited := core.Edit(tree, e)
		var stop string
		prev := tree
		tree, stop = core.ParseIncremental(next, edited, nil)
		fstop := fresh.Parse(next, nil)
		sd := core.DigestOf(tree, false)
		ok := stop == "" && fstop == "" && sd == fresh.Digest()
		if ok {
			eqFresh++
		}
		line := fmt.Sprintf("CCCHAIN step=%d at=%d stop=%q eq_fresh=%t", step, at, stop, ok)
		if step%50 == 0 {
			cf := cp.Parse(next, nil)
			cEq := ccoreCDigestChanges(cf, false) == sd
			cf.Close()
			cChecks++
			if cEq {
				eqC++
			}
			used, reach := core.ArenaUse(tree)
			line += fmt.Sprintf(" eq_c=%t used=%d reachable=%d", cEq, used, reach)
			fmt.Printf("CCSTORE step=%d %s\n", step, core.StoreInfo())
		}
		fmt.Println(line)
		core.Release(edited)
		core.Release(prev)
		cur = next
	}
	used, reach := core.ArenaUse(tree)
	fmt.Printf("CCCHAIN done eq_fresh=%d/%d eq_c=%d/%d used=%d reachable=%d\n", eqFresh, chainSteps, eqC, cChecks, used, reach)
	ccoreExpect(t, eqFresh == chainSteps && eqC == cChecks, "the chain differs: equal to fresh %d/%d, equal to C %d/%d", eqFresh, chainSteps, eqC, cChecks)
}

// BenchmarkCcoreBigEdit times edit and reparse of the three edits
// for the core, legacy and C (C through the binding, input callbacks included).
func BenchmarkCcoreBigEdit(b *testing.B) {
	gl, cl := ccoreLanguages(b, "go")
	src, names, srcs, edits := ccoreBigEdits(b)
	for i := range edits {
		for _, arm := range []string{"core", "legacy", "c"} {
			i, arm := i, arm
			b.Run(names[i]+"/"+arm, func(b *testing.B) {
				var editNs, parseNs, relNs int64
				switch arm {
				case "core":
					core := ccoreNew(gl)
					core.Parse(src, nil)
					base := core.Tree()
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						t0 := time.Now()
						edited := core.Edit(base, edits[i])
						t1 := time.Now()
						nt, _ := core.ParseIncremental(srcs[i], edited, nil)
						editNs += t1.Sub(t0).Nanoseconds()
						parseNs += time.Since(t1).Nanoseconds()
						b.StopTimer()
						r0 := time.Now()
						core.Release(nt)
						core.Release(edited)
						relNs += time.Since(r0).Nanoseconds()
						b.StartTimer()
					}
				case "legacy":
					p := g.NewParser(gl)
					p.SetAdmissionCandidateRoute(false)
					e := g.InputEdit{StartByte: edits[i].StartByte, OldEndByte: edits[i].OldEndByte, NewEndByte: edits[i].NewEndByte,
						StartPoint:  g.Point{Row: edits[i].StartRow, Column: edits[i].StartCol},
						OldEndPoint: g.Point{Row: edits[i].OldEndRow, Column: edits[i].OldEndCol},
						NewEndPoint: g.Point{Row: edits[i].NewEndRow, Column: edits[i].NewEndCol}}
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						b.StopTimer()
						tree, err := p.Parse(src)
						if err != nil {
							b.Fatal(err)
						}
						b.StartTimer()
						t0 := time.Now()
						tree.Edit(e)
						t1 := time.Now()
						nt, err := p.ParseIncremental(srcs[i], tree)
						if err != nil {
							b.Fatal(err)
						}
						editNs += t1.Sub(t0).Nanoseconds()
						parseNs += time.Since(t1).Nanoseconds()
						b.StopTimer()
						nt.Release()
						tree.Release()
						b.StartTimer()
					}
				case "c":
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cl); err != nil {
						b.Fatal(err)
					}
					base := cp.Parse(src, nil)
					defer base.Close()
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						b.StopTimer()
						ce := base.Clone()
						b.StartTimer()
						t0 := time.Now()
						ce.Edit(ccoreSitterEdit(&edits[i]))
						t1 := time.Now()
						nt := cp.Parse(srcs[i], ce)
						editNs += t1.Sub(t0).Nanoseconds()
						parseNs += time.Since(t1).Nanoseconds()
						b.StopTimer()
						nt.Close()
						ce.Close()
						b.StartTimer()
					}
				}
				b.ReportMetric(float64(editNs)/float64(b.N), "edit-ns/op")
				b.ReportMetric(float64(parseNs)/float64(b.N), "reparse-ns/op")
				if relNs > 0 {
					b.ReportMetric(float64(relNs)/float64(b.N), "release-ns/op")
				}
			})
		}
	}
}
