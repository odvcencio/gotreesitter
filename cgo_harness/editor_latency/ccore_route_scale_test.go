//go:build cgo && treesitter_c_parity

package editorlatency

// A fresh parse by the core and a reparse after a one-byte edit against the present engine's fresh parse and incremental
// edit, for json, python, c and typescript at 32 KiB, 137 KiB and 1 MiB. The two
// arms alternate inside one process; the median of CCORE_RUNS runs is printed
// per operation. Memory: CCORE_SCALE_MEMORY=1 runs 1,000 reparses at 1 MiB and
// prints the heap and the process peak after 100, 500 and 1,000.
//
//	CCORE_SCALE=1 CCORE_SCALE_LANG=<json|python|c|typescript> go test -run '^TestCcoreRouteScale$'

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
)

func ccoreScaleSource(lang string, size int) []byte {
	var b strings.Builder
	switch lang {
	case "json":
		b.WriteString("[\n")
		for i := 0; b.Len() < size; i++ {
			if i > 0 {
				b.WriteString(",\n")
			}
			fmt.Fprintf(&b, "  {\"k\": %d, \"v\": [1, 2, \"x\"]}", i)
		}
		b.WriteString("\n]\n")
	case "python":
		for i := 0; b.Len() < size; i++ {
			fmt.Fprintf(&b, "def f%d(a):\n    return a + %d\n\n", i, i)
		}
	case "c":
		for i := 0; b.Len() < size; i++ {
			fmt.Fprintf(&b, "int f%d(int a) { return a + %d; }\n", i, i)
		}
	case "typescript":
		for i := 0; b.Len() < size; i++ {
			fmt.Fprintf(&b, "function f%d(a: number): number { return a + %d; }\n", i, i)
		}
	}
	return []byte(b.String())
}

// ccoreScaleEdit replaces one digit near the middle with another digit.
func ccoreScaleEdit(src []byte) (at int, old, nw byte) {
	at = len(src) / 2
	for src[at] < '0' || src[at] > '9' {
		at++
	}
	old = src[at]
	return at, old, '0' + (old-'0'+1)%10
}

func ccoreScalePoint(src []byte, at int) gts.Point {
	var p gts.Point
	for _, c := range src[:at] {
		if c == '\n' {
			p.Row++
			p.Column = 0
		} else {
			p.Column++
		}
	}
	return p
}

func TestCcoreRouteScale(t *testing.T) {
	if os.Getenv("CCORE_SCALE") == "" {
		t.Skip("set CCORE_SCALE")
	}
	name := os.Getenv("CCORE_SCALE_LANG")
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("no language %q", name)
	}
	lang := entry.Language()
	runs := 7
	if v, err := strconv.Atoi(os.Getenv("CCORE_RUNS")); err == nil && v > 0 {
		runs = v
	}
	restore, err := bindtest.CertifyForTests(1|2|4|8, []string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if os.Getenv("CCORE_SCALE_MEMORY") != "" {
		_, svcMem0, _ := bind.RouteCounters()
		ccoreScaleMemory(t, name, lang)
		if decl, svc1, _ := bind.RouteCounters(); svc1 <= svcMem0 || len(decl) != 0 {
			t.Fatalf("the core served %d requests during the memory run, declines %v (a fallback must not be measured as core work)", svc1-svcMem0, decl)
		}
		return
	}
	_, svc0, _ := bind.RouteCounters() // the core must serve the timed requests: a fallback must not be timed as core work
	for _, size := range []int{32 << 10, 137 << 10, 1 << 20} {
		src := ccoreScaleSource(name, size)
		at, old, nw := ccoreScaleEdit(src)
		edited := append([]byte(nil), src...)
		edited[at] = nw
		edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1),
			StartPoint: ccoreScalePoint(src, at), OldEndPoint: ccoreScalePoint(src, at+1), NewEndPoint: ccoreScalePoint(src, at+1)}
		undo := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1),
			StartPoint: edit.StartPoint, OldEndPoint: edit.OldEndPoint, NewEndPoint: edit.NewEndPoint}
		_ = old
		mk := func(core bool) *gts.Parser {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(core)
			return p
		}
		budget := 250 * time.Millisecond
		times := map[string][]float64{}
		for r := 0; r < runs+1; r++ {
			for _, core := range []bool{false, true} {
				arm := map[bool]string{false: "present", true: "core"}[core]
				p := mk(core)
				// fresh parse
				n, start := 0, time.Now()
				for time.Since(start) < budget || n < 2 {
					tr, err := p.Parse(src)
					if err != nil {
						t.Fatal(err)
					}
					tr.Release()
					n++
				}
				fresh := float64(time.Since(start).Nanoseconds()) / float64(n)
				// reparse after a one-byte edit (and the undo edit back)
				tree, err := p.Parse(src)
				if err != nil {
					t.Fatal(err)
				}
				cur := src
				var total time.Duration
				ops := 0
				start = time.Now()
				for time.Since(start) < budget || ops < 4 {
					for _, step := range []struct {
						e   gts.InputEdit
						src []byte
					}{{edit, edited}, {undo, src}} {
						t0 := time.Now()
						tree.Edit(step.e)
						next, err := p.ParseIncremental(step.src, tree)
						if err != nil {
							t.Fatal(err)
						}
						total += time.Since(t0)
						if next != tree {
							tree.Release()
						}
						tree, cur = next, step.src
						ops++
					}
				}
				_ = cur
				tree.Release()
				if r > 0 {
					times[arm+"_fresh"] = append(times[arm+"_fresh"], fresh)
					times[arm+"_edit"] = append(times[arm+"_edit"], float64(total.Nanoseconds())/float64(ops))
				}
			}
		}
		fmt.Printf("CCSCALE lang=%s bytes=%d present_fresh_ns=%.0f core_fresh_ns=%.0f present_edit_ns=%.0f core_reparse_ns=%.0f runs=%d\n",
			name, len(src), median(times["present_fresh"]), median(times["core_fresh"]), median(times["present_edit"]), median(times["core_edit"]), runs)
	}
	if decl, svc1, _ := bind.RouteCounters(); svc1 <= svc0 || len(decl) != 0 {
		t.Fatalf("the core served %d requests during the timing, declines %v (a fallback must not be timed as core work)", svc1-svc0, decl)
	}
}

func ccoreScaleMemory(t *testing.T, name string, lang *gts.Language) {
	src := ccoreScaleSource(name, 1<<20)
	at, _, nw := ccoreScaleEdit(src)
	edited := append([]byte(nil), src...)
	edited[at] = nw
	pt := ccoreScalePoint(src, at)
	pt1 := ccoreScalePoint(src, at+1)
	edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1), StartPoint: pt, OldEndPoint: pt1, NewEndPoint: pt1}
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	cur := edited
	for i := 1; i <= 1000; i++ {
		tree.Edit(edit)
		next, err := p.ParseIncremental(cur, tree)
		if err != nil {
			t.Fatal(err)
		}
		if next != tree {
			tree.Release()
		}
		tree = next
		if cur[at] == nw {
			cur = src
		} else {
			cur = edited
		}
		if i == 100 || i == 500 || i == 1000 {
			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			fmt.Printf("CCMEM lang=%s reparses=%d heap_inuse_mib=%.1f heap_sys_mib=%.1f vm_hwm_kib=%d\n", name, i, float64(ms.HeapInuse)/(1<<20), float64(ms.HeapSys)/(1<<20), ccoreVmHWM())
		}
	}
	tree.Release()
}

func ccoreVmHWM() int {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, "VmHWM:") {
			f := strings.Fields(l)
			if len(f) >= 2 {
				v, _ := strconv.Atoi(f[1])
				return v
			}
		}
	}
	return 0
}
