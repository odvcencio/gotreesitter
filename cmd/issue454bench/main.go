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
//	go run ./cmd/issue454bench <lang> <sizeKB> <mode> [reps]
//
// The Make report fixture also accepts broken, broken-mid, broken-last, and
// half-typed modes. sizeKB accepts decimals such as 3.5.
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
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func gen(lang string, n int) ([]byte, string) {
	source, marker, err := benchfixtures.GeneratedSource(lang, n)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return source, marker
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
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: bench <lang> <sizeKB> <full|replace|insert|delete|query|highlight> [reps]")
		return 2
	}
	lang := args[0]
	kbFloat, _ := strconv.ParseFloat(args[1], 64)
	kb := kbFloat
	sizeBytes := int(kbFloat * 1024)
	mode := args[2]
	reps := 5
	if len(args) > 3 {
		reps, _ = strconv.Atoi(args[3])
	}
	grammarName := strings.TrimSuffix(lang, "-comments")
	switch lang {
	case "scala_report":
		grammarName = "scala"
	case "make-report":
		grammarName = "make"
	case "dart-report", "dart-report-class", "dart-report-single":
		grammarName = "dart"
	case "sh":
		grammarName = "bash"
	case "ps1":
		grammarName = "powershell"
	case "md":
		grammarName = "markdown"
	case "http-comments":
		grammarName = "http"
	}
	entry := grammars.DetectLanguageByName(grammarName)
	if entry == nil || entry.Language() == nil {
		fmt.Fprintf(os.Stderr, "language %q unavailable\n", lang)
		return 2
	}
	language := entry.Language()
	src, marker := gen(lang, sizeBytes)
	site := bytes.Index(src, []byte(marker))
	if site < 0 {
		fmt.Fprintf(os.Stderr, "marker %q not found\n", marker)
		return 2
	}
	parser := ts.NewParser(language)
	if path := os.Getenv("ISSUE454_CPUPROFILE"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	if mode == "highlight" {
		h, err := ts.NewHighlighter(language, entry.HighlightQuery)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		var ds []time.Duration
		var ranges []ts.HighlightRange
		for i := 0; i < reps; i++ {
			start := time.Now()
			ranges = h.Highlight(src)
			ds = append(ds, time.Since(start))
		}
		fmt.Printf("RESULT lang=%s size=%gKB mode=highlight bytes=%d ranges=%d highlight_ms=%.3f\n", lang, kb, len(src), len(ranges), float64(median(ds))/1e6)
		return 0
	}
	if mode == "query" {
		querySource := entry.HighlightQuery
		if lang == "nushell-comments" && mode == "query" {
			querySource = "((comment)+ @comment.documentation @spell . (decl_def))"
		}
		q, err := ts.NewQuery(querySource, language)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		tree, err := parser.Parse(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer tree.Release()
		var ds []time.Duration
		matches := 0
		for i := 0; i < reps; i++ {
			start := time.Now()
			matches = len(q.Execute(tree))
			ds = append(ds, time.Since(start))
		}
		fmt.Printf("RESULT lang=%s size=%gKB mode=%s bytes=%d matches=%d query_ms=%.3f\n", lang, kb, mode, len(src), matches, float64(median(ds))/1e6)
		return 0
	}
	caseMode := mode
	if lang == "make-report" && strings.HasPrefix(mode, "broken") {
		positions := []int{site}
		for pos := site + 1; pos < len(src); {
			at := bytes.Index(src[pos:], []byte("\ntarget"))
			if at < 0 {
				break
			}
			pos += at + 1
			positions = append(positions, pos)
			pos += len("target")
		}
		position := positions[0]
		switch mode {
		case "broken-mid":
			position = positions[len(positions)/2]
		case "broken-last":
			position = positions[len(positions)-1]
		}
		src = append(append(append([]byte{}, src[:position+6]...), '('), src[position+6:]...)
		mode = "full"
	} else if lang == "make-report" && mode == "half-typed" {
		line := []byte("@echo target0")
		at := bytes.Index(src, line)
		if at < 0 {
			fmt.Fprintln(os.Stderr, "missing Make recipe")
			return 2
		}
		start := at + len("@echo ")
		src = append(append(append([]byte{}, src[:start]...), []byte("$(sh")...), src[start+len("target0"):]...)
		mode = "full"
	}
	if mode == "full" {
		var ds []time.Duration
		nodes := 0
		hasErr := false
		var stop ts.ParseStopReason
		var rootEnd uint32
		var runtimeSummary string
		for i := 0; i < reps; i++ {
			t0 := time.Now()
			tree, err := parser.Parse(src)
			ds = append(ds, time.Since(t0))
			if err != nil {
				tree.Release()
				fmt.Printf("RESULT lang=%s size=%gKB mode=%s err=%v\n", lang, kb, caseMode, err)
				return 1
			}
			nodes = countNodes(tree.RootNode())
			hasErr = tree.RootNode().HasError()
			stop = tree.ParseRuntime().StopReason
			rootEnd = tree.RootNode().EndByte()
			runtimeSummary = tree.ParseRuntime().Summary()
			tree.Release()
		}
		fmt.Printf("RESULT lang=%s size=%gKB mode=%s bytes=%d full_ms=%.3f min_ms=%.3f nodes=%d has_error=%v stop=%s root_end=%d\n",
			lang, kb, caseMode, len(src), float64(median(ds))/1e6, float64(ds[0])/1e6, nodes, hasErr, stop, rootEnd)
		if os.Getenv("ISSUE454_FULLPROFILE") != "" {
			fmt.Printf("  runtime=%s\n", runtimeSummary)
		}
		return 0
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
		return 2
	}

	t0 := time.Now()
	old, err := parser.Parse(src)
	fullD := time.Since(t0)
	defer old.Release()
	if err != nil {
		fmt.Printf("RESULT lang=%s size=%gKB mode=%s err=%v\n", lang, kb, mode, err)
		return 1
	}
	editStart := time.Now()
	old.Edit(edit)
	editD := time.Since(editStart)
	t0 = time.Now()
	inc, prof, err := parser.ParseIncrementalProfiled(edited, old)
	incD := time.Since(t0)
	defer inc.Release()
	if err != nil {
		fmt.Printf("RESULT lang=%s size=%gKB mode=%s inc_err=%v inc_ms=%.3f\n", lang, kb, mode, err, float64(incD)/1e6)
		return 1
	}
	t0 = time.Now()
	fresh, err := parser.Parse(edited)
	freshD := time.Since(t0)
	defer fresh.Release()
	if err != nil {
		fmt.Printf("RESULT lang=%s size=%gKB mode=%s fresh_err=%v\n", lang, kb, mode, err)
		return 1
	}
	in, fn, sexprMatch, comparisonErr := compareTrees(inc.RootNode(), fresh.RootNode(), language)
	fmt.Printf("RESULT lang=%s size=%gKB mode=%s site=%d full_ms=%.3f edit_ms=%.3f inc_ms=%.3f reuse_cursor_ms=%.3f reparse_ms=%.3f fresh_ms=%.3f ratio_inc_full=%.2f inc_nodes=%d fresh_nodes=%d match=%v sexpr_match=%v inc_err=%v fresh_err=%v reused_subtrees=%d reused_bytes=%d reuse_pct=%.1f new_nodes=%d tokens=%d unsupported=%v reason=%q rej_rootnonleaf=%d rej_scanner=%d stop=%v fresh_stop=%v inc_root_end=%d fresh_root_end=%d\n",
		lang, kb, mode, site, float64(fullD)/1e6, float64(editD)/1e6, float64(incD)/1e6, float64(prof.ReuseCursorNanos)/1e6, float64(prof.ReparseNanos)/1e6, float64(freshD)/1e6, float64(incD)/float64(fullD), in, fn, in == fn, sexprMatch, inc.RootNode().HasError(), fresh.RootNode().HasError(),
		prof.ReusedSubtrees, prof.ReusedBytes, 100*float64(prof.ReusedBytes)/float64(len(edited)), prof.NewNodesAllocated, prof.TokensConsumed, prof.ReuseUnsupported, prof.ReuseUnsupportedReason, prof.ReuseRejectRootNonLeafChanged, prof.ReuseRejectScannerUnquiescent, prof.StopReason, fresh.ParseRuntime().StopReason, inc.RootNode().EndByte(), fresh.RootNode().EndByte())
	if os.Getenv("ISSUE454_FULLPROFILE") != "" {
		rt := inc.ParseRuntime()
		fmt.Printf("  profile=%+v\n  runtime=%s\n  compactIncremental: reuseRoute=%v fullRecoveryRoute=%v reusedSubtrees=%d reusedBytes=%d fallbackReason=%q\n",
			prof, rt.Summary(), rt.CompactIncrementalReuseRoute, rt.CompactIncrementalFullRecoveryRoute, rt.CompactIncrementalReusedSubtrees, rt.CompactIncrementalReusedBytes, rt.CompactIncrementalFallbackReason)
	}
	if comparisonErr != nil {
		fmt.Fprintln(os.Stderr, comparisonErr)
		return 1
	}
	return 0
}

// compareTrees checks the tree properties reported by this command.
// It does not certify spans, fields, or parser-state parity.
func compareTrees(inc, fresh *ts.Node, language *ts.Language) (in, fn int, sexprMatch bool, err error) {
	if inc == nil || fresh == nil {
		return 0, 0, false, fmt.Errorf("comparison requires two tree roots")
	}
	in, fn = countNodes(inc), countNodes(fresh)
	sexprMatch = inc.SExpr(language) == fresh.SExpr(language)
	if in != fn || !sexprMatch || inc.HasError() != fresh.HasError() {
		err = fmt.Errorf("incremental tree differs from fresh tree")
	}
	return
}
