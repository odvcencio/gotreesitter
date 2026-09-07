// Command issue454bench reproduces the downstream editor measurements from
// issue #454 on synthetic single-language fixtures.
//
// It generates a repetitive source of the requested size, then either times a
// fresh full parse or applies one single-byte edit (replace, insert, or
// delete) at the first near-top identifier and reports the incremental parse
// profile against a fresh parse of the edited bytes.
//
// Usage:
//
//	go run ./cmd/issue454bench <lang> <sizeKB> <full|replace|insert|delete> [reps]
//
// Set ISSUE454_CPUPROFILE=<path> to write a CPU profile of the measured parses.
package main

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"time"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func gen(lang string, n int) ([]byte, string) {
	var b bytes.Buffer
	m := "x0"
	switch lang {
	case "go":
		b.WriteString("package main\n\nimport \"fmt\"\n\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
		}
		m = "x := a + b"
	case "rust":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "fn f%d(a: i32, b: i32) -> i32 {\n    let x0 = a + b;\n    println!(\"f%d {}\", x0);\n    x0\n}\n\n", i, i)
		}
	case "scala":
		b.WriteString("object Main {\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "  def f%d(a: Int, b: Int): Int = {\n    val x0 = a + b\n    println(x0)\n    x0\n  }\n\n", i)
		}
		b.WriteString("}\n")
	case "cmake":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "function(f%d a b)\n  set(x0 ${a})\n  message(STATUS \"f%d ${x0}\")\nendfunction()\n\n", i, i)
		}
	case "toml":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "[section%d]\nx0 = %d\nname = \"f%d\"\nenabled = true\n\n", i, i, i)
		}
	case "ini":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "[section%d]\nx0 = %d\nname = f%d\n\n", i, i, i)
		}
	case "make":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "f%d: x0.o\n\t$(CC) -o f%d x0.o\n\n", i, i)
		}
	case "css", "scss", "less":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, ".f%d {\n  color: red;\n  width: 10px;\n}\n\n", i)
		}
		m = "red"
	case "typescript", "tsx", "javascript":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "function f%d(a: number, b: number): number {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n", i, i)
		}
		if lang == "javascript" {
			b.Reset()
			for i := 0; b.Len() < n; i++ {
				fmt.Fprintf(&b, "function f%d(a, b) {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n", i, i)
			}
		}
	case "json":
		b.WriteString("[\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "  {\"x0\": %d, \"name\": \"f%d\"},\n", i, i)
		}
		s := strings.TrimSuffix(b.String(), ",\n") + "\n]\n"
		b.Reset()
		b.WriteString(s)
	case "haskell":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "f%d :: Int -> Int -> Int\nf%d a b = x0\n  where x0 = a + b\n\n", i, i)
		}
	case "hcl":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "resource \"aws_instance\" \"f%d\" {\n  x0 = %d\n  name = \"f%d\"\n}\n\n", i, i, i)
		}
	case "diff":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "diff --git a/f%d.txt b/f%d.txt\n--- a/f%d.txt\n+++ b/f%d.txt\n@@ -1,2 +1,2 @@\n-x0 old\n+x0 new\n", i, i, i, i)
		}
	case "c":
		b.WriteString("#include <stdio.h>\n\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "int f%d(int a, int b) {\n    int x0 = a + b;\n    printf(\"f%d %%d\\n\", x0);\n    return x0;\n}\n\n", i, i)
		}
	case "python":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "def f%d(a, b):\n    x0 = a + b\n    print(\"f%d\", x0)\n    return x0\n\n\n", i, i)
		}
	default:
		fmt.Fprintf(os.Stderr, "no generator for %s\n", lang)
		os.Exit(2)
	}
	return b.Bytes(), m
}

func point(src []byte, off int) ts.Point {
	row := bytes.Count(src[:off], []byte{'\n'})
	col := off - (bytes.LastIndexByte(src[:off], '\n') + 1)
	return ts.Point{Row: uint32(row), Column: uint32(col)}
}

func countNodes(n *ts.Node) int {
	if n == nil {
		return 0
	}
	c := 1
	for i := 0; i < n.ChildCount(); i++ {
		c += countNodes(n.Child(i))
	}
	return c
}

func median(d []time.Duration) time.Duration {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[len(d)/2]
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: bench <lang> <sizeKB> <full|replace|insert|delete> [reps]")
		os.Exit(2)
	}
	lang := os.Args[1]
	kb, _ := strconv.Atoi(os.Args[2])
	mode := os.Args[3]
	reps := 5
	if len(os.Args) > 4 {
		reps, _ = strconv.Atoi(os.Args[4])
	}
	entry := grammars.DetectLanguageByName(lang)
	if entry == nil || entry.Language() == nil {
		fmt.Fprintf(os.Stderr, "language %q unavailable\n", lang)
		os.Exit(2)
	}
	language := entry.Language()
	src, marker := gen(lang, kb<<10)
	site := bytes.Index(src, []byte(marker))
	if site < 0 {
		fmt.Fprintf(os.Stderr, "marker %q not found\n", marker)
		os.Exit(2)
	}
	parser := ts.NewParser(language)
	if path := os.Getenv("ISSUE454_CPUPROFILE"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		defer pprof.StopCPUProfile()
	}

	if mode == "full" {
		var ds []time.Duration
		nodes := 0
		hasErr := false
		for i := 0; i < reps; i++ {
			t0 := time.Now()
			tree, err := parser.Parse(src)
			ds = append(ds, time.Since(t0))
			if err != nil {
				fmt.Printf("RESULT lang=%s size=%dKB mode=full err=%v\n", lang, kb, err)
				os.Exit(1)
			}
			nodes = countNodes(tree.RootNode())
			hasErr = tree.RootNode().HasError()
		}
		fmt.Printf("RESULT lang=%s size=%dKB mode=full bytes=%d full_ms=%.3f min_ms=%.3f nodes=%d has_error=%v\n",
			lang, kb, len(src), float64(median(ds))/1e6, float64(ds[0])/1e6, nodes, hasErr)
		return
	}

	var edited []byte
	var edit ts.InputEdit
	sp := point(src, site)
	switch mode {
	case "replace":
		edited = append([]byte{}, src...)
		edited[site]++
		edit = ts.InputEdit{StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site + 1),
			StartPoint: sp, OldEndPoint: ts.Point{Row: sp.Row, Column: sp.Column + 1}, NewEndPoint: ts.Point{Row: sp.Row, Column: sp.Column + 1}}
	case "insert":
		edited = append(append(append([]byte{}, src[:site]...), src[site]), src[site:]...)
		edit = ts.InputEdit{StartByte: uint32(site), OldEndByte: uint32(site), NewEndByte: uint32(site + 1),
			StartPoint: sp, OldEndPoint: sp, NewEndPoint: ts.Point{Row: sp.Row, Column: sp.Column + 1}}
	case "delete":
		edited = append(append([]byte{}, src[:site]...), src[site+1:]...)
		edit = ts.InputEdit{StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
			StartPoint: sp, OldEndPoint: ts.Point{Row: sp.Row, Column: sp.Column + 1}, NewEndPoint: sp}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %s\n", mode)
		os.Exit(2)
	}

	t0 := time.Now()
	old, err := parser.Parse(src)
	fullD := time.Since(t0)
	if err != nil {
		fmt.Printf("RESULT lang=%s size=%dKB mode=%s err=%v\n", lang, kb, mode, err)
		os.Exit(1)
	}
	old.Edit(edit)
	t0 = time.Now()
	inc, prof, err := parser.ParseIncrementalProfiled(edited, old)
	incD := time.Since(t0)
	if err != nil {
		fmt.Printf("RESULT lang=%s size=%dKB mode=%s inc_err=%v inc_ms=%.3f\n", lang, kb, mode, err, float64(incD)/1e6)
		os.Exit(1)
	}
	t0 = time.Now()
	fresh, err := parser.Parse(edited)
	freshD := time.Since(t0)
	if err != nil {
		fmt.Printf("RESULT lang=%s size=%dKB mode=%s fresh_err=%v\n", lang, kb, mode, err)
		os.Exit(1)
	}
	in := countNodes(inc.RootNode())
	fn := countNodes(fresh.RootNode())
	sexprMatch := inc.RootNode().SExpr(language) == fresh.RootNode().SExpr(language)
	fmt.Printf("RESULT lang=%s size=%dKB mode=%s site=%d full_ms=%.3f inc_ms=%.3f fresh_ms=%.3f ratio_inc_full=%.2f inc_nodes=%d fresh_nodes=%d match=%v sexpr_match=%v inc_err=%v fresh_err=%v reused_subtrees=%d reused_bytes=%d reuse_pct=%.1f new_nodes=%d tokens=%d unsupported=%v reason=%q rej_rootnonleaf=%d rej_scanner=%d stop=%v\n",
		lang, kb, mode, site, float64(fullD)/1e6, float64(incD)/1e6, float64(freshD)/1e6, float64(incD)/float64(fullD), in, fn, in == fn, sexprMatch, inc.RootNode().HasError(), fresh.RootNode().HasError(),
		prof.ReusedSubtrees, prof.ReusedBytes, 100*float64(prof.ReusedBytes)/float64(len(edited)), prof.NewNodesAllocated, prof.TokensConsumed, prof.ReuseUnsupported, prof.ReuseUnsupportedReason, prof.ReuseRejectRootNonLeafChanged, prof.ReuseRejectScannerUnquiescent, prof.StopReason)
	if os.Getenv("ISSUE454_FULLPROFILE") != "" {
		rt := inc.ParseRuntime()
		fmt.Printf("  profile=%+v\n  runtime=%s\n  compactIncremental: reuseRoute=%v fullRecoveryRoute=%v reusedSubtrees=%d reusedBytes=%d fallbackReason=%q\n",
			prof, rt.Summary(), rt.CompactIncrementalReuseRoute, rt.CompactIncrementalFullRecoveryRoute, rt.CompactIncrementalReusedSubtrees, rt.CompactIncrementalReusedBytes, rt.CompactIncrementalFallbackReason)
	}
}
