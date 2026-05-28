// Diagnostic: parse one file with GLR equiv audit enabled, dump counters.
// Used to understand the inner-call distribution that drives stack-equivalence
// work in the GLR merge phase.
//
// Usage: go run ./cmd/equiv_audit_oneshot <lang> <file>
//   lang: javascript | python | rust | ...
package main

import (
	"fmt"
	"os"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: equiv_audit_oneshot <lang> <file>")
		os.Exit(2)
	}
	langName := os.Args[1]
	path := os.Args[2]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	var lang *gotreesitter.Language
	switch langName {
	case "javascript", "js":
		lang = grammars.JavascriptLanguage()
	case "python", "py":
		lang = grammars.PythonLanguage()
	case "rust", "rs":
		lang = grammars.RustLanguage()
	case "typescript", "ts":
		lang = grammars.TypescriptLanguage()
	case "go":
		lang = grammars.GoLanguage()
	default:
		fmt.Fprintf(os.Stderr, "unsupported lang: %s\n", langName)
		os.Exit(2)
	}

	gotreesitter.EnableGLREquivAudit(true)
	defer gotreesitter.EnableGLREquivAudit(false)

	parser := gotreesitter.NewParser(lang)

	// Warm up so first-parse one-time costs don't pollute timing.
	if _, err := parser.Parse(src); err != nil {
		fmt.Fprintf(os.Stderr, "warm: %v\n", err)
		os.Exit(1)
	}

	start := time.Now()
	tree, err := parser.Parse(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	rt := tree.ParseRuntime()
	lookups := rt.EquivCacheLookups
	hits := rt.EquivCacheHits
	frontier := rt.EquivFrontierCalls
	childScans := rt.EquivFrontierChildScans
	candidate := rt.EquivFrontierCandidateCompares
	keyMisses := rt.EquivCacheKeyMisses
	epochMisses := rt.EquivCacheEpochMisses
	versionMisses := rt.EquivCacheVersionMisses
	exactCalls := rt.EquivExactCalls

	hitPct := 0.0
	if lookups > 0 {
		hitPct = 100.0 * float64(hits) / float64(lookups)
	}
	keyMissPct := 0.0
	if lookups > 0 {
		keyMissPct = 100.0 * float64(keyMisses) / float64(lookups)
	}

	fmt.Printf("Language: %s\n", langName)
	fmt.Printf("File:     %s (%d bytes)\n", path, len(src))
	fmt.Printf("Wall:     %v  (with audit overhead)\n", elapsed)
	fmt.Printf("\n=== Equiv cache (inner, per-(a,b,depth)) ===\n")
	fmt.Printf("  Lookups:        %d\n", lookups)
	fmt.Printf("  Hits:           %d  (%.1f%%)\n", hits, hitPct)
	fmt.Printf("  Key misses:     %d  (%.1f%% — direct-mapped collision)\n", keyMisses, keyMissPct)
	fmt.Printf("  Epoch misses:   %d  (cache cleared between merge calls)\n", epochMisses)
	fmt.Printf("  Version misses: %d  (node mutated since cached)\n", versionMisses)
	fmt.Printf("\n=== Frontier walk ===\n")
	fmt.Printf("  Frontier calls: %d\n", frontier)
	fmt.Printf("  Child scans:    %d  (%.2f per call avg)\n", childScans, ratio(childScans, frontier))
	fmt.Printf("  Candidate recurse: %d  (%.2f per call avg)\n", candidate, ratio(candidate, frontier))
	fmt.Printf("  Exact calls:    %d\n", exactCalls)

	fmt.Printf("\n=== Header-only merge divergence (the C tree-sitter shape) ===\n")
	hdrEq := rt.MergeHeaderEqTotal
	deepTrue := rt.MergeDeepTrue
	deepFalse := rt.MergeDeepFalse
	divergent := rt.MergeHeaderDeepDivergent
	totalCompared := deepTrue + deepFalse
	fmt.Printf("  Total merge candidates compared:  %d\n", totalCompared)
	fmt.Printf("  Deep equiv -> TRUE  (currently merged):  %d  (%.2f%%)\n", deepTrue, ratio100(deepTrue, totalCompared))
	fmt.Printf("  Deep equiv -> FALSE (currently rejected): %d  (%.2f%%)\n", deepFalse, ratio100(deepFalse, totalCompared))
	fmt.Printf("  Header-only would accept:        %d  (%.2f%%)\n", hdrEq, ratio100(hdrEq, totalCompared))
	fmt.Printf("  Divergent (header=YES, deep=NO): %d  (%.4f%% of total)\n", divergent, ratio100(divergent, totalCompared))
	if totalCompared > 0 {
		rate := 100.0 * float64(divergent) / float64(totalCompared)
		switch {
		case rate < 0.01:
			fmt.Printf("  → SAFE: divergence is <0.01%%. Header-only is the right lever.\n")
		case rate < 0.1:
			fmt.Printf("  → MARGINAL: divergence is %.4f%%. Worth investigating but needs guards.\n", rate)
		default:
			fmt.Printf("  → UNSAFE: divergence is %.4f%%. Deep equivalence is doing real filtering.\n", rate)
		}
	}

	fmt.Printf("\n=== Stack-pair audit (outer, per-(stack-A-ptr, stack-B-ptr, depth)) ===\n")
	keyed := rt.StackEquivPairKeyed
	unkeyed := rt.StackEquivPairUnkeyed
	repeats := rt.StackEquivPairRepeats
	repeatTrue := rt.StackEquivPairRepeatTrue
	repeatFalse := rt.StackEquivPairRepeatFalse
	mismatch := rt.StackEquivPairRepeatMismatch
	stores := rt.StackEquivPairStores
	fmt.Printf("  Outer lookups: %d  (keyed)  +  %d  (unkeyed)\n", keyed, unkeyed)
	fmt.Printf("  Outer repeats: %d  (%.1f%% would short-circuit if cached)\n", repeats, ratio100(repeats, keyed))
	fmt.Printf("    → repeat true:  %d\n", repeatTrue)
	fmt.Printf("    → repeat false: %d\n", repeatFalse)
	fmt.Printf("    → mismatch:     %d\n", mismatch)
	fmt.Printf("  Outer stores:  %d  (distinct pairs seen)\n", stores)

	fmt.Printf("\n=== Phase timing (us) ===\n")
	fmt.Printf("  parser_loop:        %v\n", time.Duration(rt.ParserLoopNanos))
	fmt.Printf("  glr_merge:          %v\n", time.Duration(rt.GLRMergeNanos))
	fmt.Printf("  glr_cull:           %v\n", time.Duration(rt.GLRCullNanos))
	fmt.Printf("  action_dispatch:    %v\n", time.Duration(rt.ActionDispatchNanos))
	fmt.Printf("  action_lookup:      %v\n", time.Duration(rt.ActionLookupNanos))
	fmt.Printf("  token_next:         %v\n", time.Duration(rt.TokenNextNanos))

	// Per-state breakdown — when GLR merging fires from many different states,
	// understanding which state is the heaviest helps target the next lever.
	if len(rt.EquivStateStats) > 0 {
		fmt.Printf("\n=== Per-state frontier calls (top 10 by call volume) ===\n")
		states := make([]gotreesitter.ParseEquivStateRuntime, len(rt.EquivStateStats))
		copy(states, rt.EquivStateStats)
		// Sort by frontier calls descending.
		sortByFrontierCalls(states)
		top := 10
		if len(states) < top {
			top = len(states)
		}
		fmt.Printf("  %-10s %12s %12s %12s\n", "state", "frontier", "cache-lookup", "cache-hits")
		for i := 0; i < top; i++ {
			s := states[i]
			fmt.Printf("  %-10d %12d %12d %12d\n", s.State, s.EquivFrontierCalls, s.EquivCacheLookups, s.EquivCacheHits)
		}
		fmt.Printf("  ... (%d total states)\n", len(states))
	}
}

func ratio(num, den uint64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func ratio100(num, den uint64) float64 {
	if den == 0 {
		return 0
	}
	return 100.0 * float64(num) / float64(den)
}

func sortByFrontierCalls(s []gotreesitter.ParseEquivStateRuntime) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].EquivFrontierCalls < s[j].EquivFrontierCalls; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
