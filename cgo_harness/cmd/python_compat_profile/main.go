// Per-pass profiler for python compat tier.
// Usage: go run ./cmd/python_compat_profile <pyfile>
package main

import (
	"fmt"
	"os"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: python_compat_profile <pyfile>")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}

	os.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	gotreesitter.ResetParseEnvConfigCacheForTests()

	lang := grammars.PythonLanguage()
	p := gotreesitter.NewParser(lang)

	// Warm
	if _, err := p.Parse(src); err != nil {
		fmt.Fprintf(os.Stderr, "warm: %v\n", err)
		os.Exit(1)
	}

	const iters = 20
	start := time.Now()
	var lastRT gotreesitter.ParseRuntime
	for i := 0; i < iters; i++ {
		tree, err := p.Parse(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse: %v\n", err)
			os.Exit(1)
		}
		lastRT = tree.ParseRuntime()
	}
	elapsed := time.Since(start)
	avg := elapsed / iters

	fmt.Printf("File: %s (%d bytes)  iters=%d  avg wall=%v\n\n", os.Args[1], len(src), iters, avg)

	fmt.Printf("=== Phase totals (last iter) ===\n")
	fmt.Printf("  ParserLoopNanos       = %v\n", time.Duration(lastRT.ParserLoopNanos))
	fmt.Printf("  ResultTreeBuildNanos  = %v\n", time.Duration(lastRT.ResultTreeBuildNanos))
	fmt.Printf("  ResultFinalizeRoot    = %v\n", time.Duration(lastRT.ResultFinalizeRootNanos))
	fmt.Printf("  ResultCompatibility   = %v\n", time.Duration(lastRT.ResultCompatibilityNanos))
	fmt.Printf("  Normalization         = %v  (visited=%d rewrites=%d)\n",
		time.Duration(lastRT.NormalizationNanos),
		lastRT.NormalizationNodesVisited,
		lastRT.NormalizationNodesRewritten)
	fmt.Printf("  GLRMergeNanos         = %v\n", time.Duration(lastRT.GLRMergeNanos))
	fmt.Printf("  ActionDispatchNanos   = %v\n", time.Duration(lastRT.ActionDispatchNanos))

	fmt.Printf("\n=== Per-pass normalization breakdown ===\n")
	passes := lastRT.NormalizationPasses
	if passes == nil {
		fmt.Println("(no per-pass data)")
		return
	}
	fmt.Printf("%-50s  %12s  %8s  %8s  %10s\n", "pass", "nanos", "checked", "ran", "rewrites")
	var totalNanos int64
	for _, p := range *passes {
		fmt.Printf("%-50s  %12v  %8d  %8d  %10d\n",
			p.Name,
			time.Duration(p.Nanos),
			p.Checked,
			p.Run,
			p.NodesRewritten)
		totalNanos += p.Nanos
	}
	fmt.Printf("%-50s  %12v\n", "TOTAL (sum of per-pass)", time.Duration(totalNanos))
}
