package gotreesitter

import (
	"bytes"
	"fmt"
	"os"
	"time"
)

const (
	// Retry no-stacks-alive full parses with a wider GLR cap. Large real-world
	// files (for example this repo's parser.go) can legitimately need >8 stacks
	// at peak even when parse tables report narrower local conflict widths.
	fullParseRetryMaxGLRStacks = 48
	// Some ambiguity clusters need more survivors per merge bucket even after
	// the global GLR cap is widened. Only enable this on retries for parses
	// that already proved the default merge budget was insufficient.
	fullParseRetryMaxMergePerKey = 24
	// Java's default full-parse merge cap stays intentionally narrow for large
	// generated bodies, but annotation-heavy declarations can need a wider
	// bounded accepted-error retry to preserve the expression/declaration branch
	// that C selects.
	javaFullParseRetryMaxGLRStacks   = 64
	javaFullParseRetryMaxMergePerKey = 16
	javaTightMergeCapSourceLen       = 256 * 1024
	// D's tuned default starts at cap 2, then fullParseInitialMaxStacks raises
	// the effective parse cap to this conflict floor. Large D accepted-error
	// retries must not widen beyond it, or expressionsem.d re-enters the
	// survivor-population RSS cliff.
	dLargeFileRetryStackCeiling = 3
	dLargeFileRetryMinBytes     = 64 * 1024
	// goAcceptedErrorMergePerKeyRetry widens Go's merge-per-key survivor
	// budget only on the retry rung, when a fresh full parse at the
	// steady-state cap (3, see the "go" case in effectiveParseMergePerKeyCap)
	// accepts with an error. See fullParseRetryMergePerKeyOverride's "go"
	// case for the full rationale: this keeps clean files (the overwhelming
	// majority, including this repo's own parser.go/parser_reduce.go and
	// grammargen/lr.go, and grammargen/normalize.go) on the cheap cap=3 path
	// with no retry at all, while files that need more survivors to keep the
	// correct index_expression branch alive (the ASI-fix regression files)
	// pay a second parse at this cap instead of a permanently widened budget.
	//
	// 16, not 8: a genuinely fresh parse (no prior failed attempt on the same
	// Parser) reaches a clean result for every regression file at cap=8, but
	// the SAME cap value reached via this retry rung (i.e. after a discarded
	// cap=3 attempt on the same Parser) was insufficient for one of them —
	// stdlib's sort/sort_slices_benchmark_test.go stayed HasError=true
	// through an 8-cap retry and only came back clean at 16. The two paths
	// are not computationally equivalent even though they request the same
	// mergePerKeyCap value: something about having already run a cap=3
	// attempt on the same Parser instance (arena/GSS pooling, or some other
	// carried-over state — not resolveParseMaxStacks's retryPass flag, ruled
	// out directly: it is already true on the very first cap=3 attempt too,
	// since go's tuned initial stack budget of 32 always exceeds
	// maxGLRStacks=8 regardless of merge-per-key) biases the GLR merge
	// selection differently than an independent fresh parse at the same cap.
	// This is the same class of engine nondeterminism as the cap-value
	// non-monotonicity noted on effectiveParseMergePerKeyCap's "go" case
	// (grammargen/normalize.go: clean at cap=3, erroring at every fixed cap
	// from 8 through 16 when that cap is the STEADY STATE) — both point at
	// the GLR merge-selection engine being sensitive to more than just the
	// final cap value, which is real RCA seed material for a proper
	// investigation, not something this rung's cap choice can fully paper
	// over. 16 is empirically sufficient for every case found so far.
	goAcceptedErrorMergePerKeyRetry = 16
	// Retry node-limit full parses with a bounded larger node budget instead of
	// globally raising the default cap for every parse.
	fullParseRetryNodeLimitScale = 2
	// If the first widened retry still stops on node_limit, allow one more
	// bounded escalation. This only applies to parses that already proved the
	// initial retry made progress but still ran out of budget.
	fullParseRetrySecondaryNodeLimitScale = 3
	// Keep retry widening bounded to avoid runaway memory growth on very large
	// malformed inputs. Callers can still override via GOT_GLR_MAX_STACKS.
	fullParseRetryMaxSourceBytes = 1 << 20 // 1 MiB
	// fullParseRetryMaxTotalPasses hard-bounds the number of retry passes a
	// single top-level parse operation may run, independent of any wall-clock
	// budget (the retryDeadline in retryFullParse is a no-op when the caller
	// never sets SetTimeoutMicros — the common case). The deepest legitimate
	// ladder is ~5 passes per retryFullParse invocation and at most a few
	// invocations per operation (main round + external-scanner repeat +
	// incremental->full fallback), so 24 leaves >2x headroom while making
	// runaway repetition (c_sharp was observed running 1076 passes on a
	// pass-1 parse failure, 2026-07 cliff campaign) structurally impossible.
	// Retries are best-effort improvements: when the budget is exhausted the
	// incumbent best tree is returned, never a worse result.
	fullParseRetryMaxTotalPasses = 24
)

type resettableTokenSource interface {
	Reset(source []byte)
}

type fullParseRetryRunner func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree

func shouldRetryFullParse(tree *Tree, sourceLen int) bool {
	if tree == nil {
		return false
	}
	if tree.ParseStopReason() != ParseStopNoStacksAlive {
		return false
	}
	if sourceLen <= 0 {
		return false
	}
	return sourceLen <= fullParseRetryMaxSourceBytes
}

func shouldRetryAcceptedErrorParse(tree *Tree, sourceLen int, initialMaxStacks int) bool {
	if tree == nil {
		return false
	}
	if sourceLen <= 0 || sourceLen > fullParseRetryMaxSourceBytes {
		return false
	}
	if !retryTreeHasError(tree) {
		return false
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != ParseStopAccepted || rt.Truncated || rt.TokenSourceEOFEarly {
		return false
	}
	if tree.language != nil && tree.language.Name == "bash" {
		// Bash uses the forest path for clean large-file parses; if production
		// accepts a full-span error-bearing tree after the forest declines, the
		// accepted-error retry has repeatedly proven to be wasted work on real
		// corpus witnesses (for example git's t9300 fast-import script remains
		// an error-bearing, C-divergent full-span tree while paying multiple
		// full reparse passes). Keep structural/no-stack/node-limit retries
		// available, but do not chase an accepted-error Bash tree that already
		// reached EOF.
		return false
	}
	if tree.language != nil && tree.language.Name == "cpp" {
		return false
	}
	if initialMaxStacks <= 0 {
		initialMaxStacks = maxGLRStacks
	}
	return rt.MaxStacksSeen >= initialMaxStacks
}

func shouldRetryStackPressureCleanFullParse(tree *Tree, sourceLen int, _ int) bool {
	if tree == nil {
		return false
	}
	if sourceLen <= 0 || sourceLen > fullParseRetryMaxSourceBytes {
		return false
	}
	root := rawRootOrNil(tree)
	if root == nil || root.HasError() {
		return false
	}
	rt := tree.ParseRuntime()
	if rt.TokenSourceEOFEarly {
		return false
	}
	if rt.StopReason != ParseStopAccepted && rt.StopReason != ParseStopNoStacksAlive {
		return false
	}
	if rt.Truncated && rt.StopReason != ParseStopAccepted {
		return false
	}
	// Only ACTUAL eviction justifies a clean-tree re-parse: the global cap
	// cull dropped live stacks, so the C-correct interpretation may have
	// been discarded. The old second arm (MaxStacksSeen >= cap+overflow)
	// fired on mere transient pre-merge bloom — python's clean parses hit
	// it on essentially every real file (MaxStacksSeen >= 12 always),
	// buying a guaranteed second full pass (universal ~2x clean-parse tax,
	// 2026-07 cliff campaign) for a tree that was never at risk: when the
	// cull never dropped anything, no lineage was lost and the first clean
	// tree is already the parse's honest answer (also closer to C's
	// single-pass semantics).
	return rt.GlobalCullStacksIn > rt.GlobalCullStacksOut
}

func shouldRetryNodeLimitParse(tree *Tree, sourceLen int) bool {
	if tree == nil {
		return false
	}
	if sourceLen <= 0 || sourceLen > fullParseRetryMaxSourceBytes {
		return false
	}
	return tree.ParseStopReason() == ParseStopNodeLimit
}

func shouldRetryIncrementalParseAsFull(tree *Tree, sourceLen int, initialMaxStacks int) bool {
	if tree == nil {
		return false
	}
	return shouldRetryFullParse(tree, sourceLen) ||
		shouldRetryAcceptedErrorParse(tree, sourceLen, initialMaxStacks) ||
		shouldRetryNodeLimitParse(tree, sourceLen)
}

func treeParseClean(tree *Tree) bool {
	if tree == nil {
		return false
	}
	root := rawRootOrNil(tree)
	if root == nil || retryNodeSubtreeHasError(root, 0) {
		return false
	}
	rt := tree.ParseRuntime()
	return rt.StopReason == ParseStopAccepted && !rt.TokenSourceEOFEarly && retryTreeCoversExpectedEOF(tree)
}

func rawRootOrNil(tree *Tree) *Node {
	if tree == nil {
		return nil
	}
	return tree.root
}

func retryTreeEndByte(tree *Tree) uint32 {
	if tree == nil {
		return 0
	}
	if root := rawRootOrNil(tree); root != nil {
		return root.EndByte()
	}
	return tree.ParseRuntime().RootEndByte
}

func retryTreeChildCount(tree *Tree) int {
	if tree == nil {
		return 0
	}
	if root := rawRootOrNil(tree); root != nil {
		return root.ChildCount()
	}
	return 0
}

func retryTreeHasError(tree *Tree) bool {
	if tree == nil {
		return true
	}
	root := rawRootOrNil(tree)
	if root == nil {
		return true
	}
	return retryNodeSubtreeHasError(root, 0)
}

func retryNodeSubtreeHasError(node *Node, depth int) bool {
	if node == nil {
		return false
	}
	if node.IsError() || node.HasError() {
		return true
	}
	if depth >= maxTreeWalkDepth {
		return false
	}
	for i := 0; i < resultChildCount(node); i++ {
		if retryNodeSubtreeHasError(resultChildAt(node, i), depth+1) {
			return true
		}
	}
	return false
}

func retryTreeCoversExpectedEOF(tree *Tree) bool {
	if tree == nil {
		return false
	}
	rt := tree.ParseRuntime()
	if rt.ExpectedEOFByte == 0 {
		return true
	}
	if rt.LastTokenWasEOF && rt.LastTokenEndByte >= rt.ExpectedEOFByte {
		return true
	}
	endByte := retryTreeEndByte(tree)
	return endByte >= rt.ExpectedEOFByte || parserTailAllowsCleanAcceptance(tree.Source(), endByte, rt.ExpectedEOFByte, tree.includedRanges)
}

func retryStopRank(rt ParseRuntime) int {
	switch rt.StopReason {
	case ParseStopAccepted:
		return 4
	case ParseStopTokenSourceEOF:
		return 3
	case ParseStopNoStacksAlive:
		return 2
	case ParseStopNodeLimit:
		return 1
	default:
		return 0
	}
}

func preferRetryTree(p *Parser, candidate, incumbent *Tree) bool {
	if candidate == nil {
		return false
	}
	if incumbent == nil {
		return true
	}
	if treeParseClean(candidate) {
		return !treeParseClean(incumbent)
	}
	if treeParseClean(incumbent) {
		return false
	}
	candEnd := retryTreeEndByte(candidate)
	incEnd := retryTreeEndByte(incumbent)
	if candEnd != incEnd {
		return candEnd > incEnd
	}
	candRT := candidate.ParseRuntime()
	incRT := incumbent.ParseRuntime()
	if candRT.Truncated != incRT.Truncated {
		return !candRT.Truncated
	}
	if candRT.TokenSourceEOFEarly != incRT.TokenSourceEOFEarly {
		return !candRT.TokenSourceEOFEarly
	}
	candErr := retryTreeHasError(candidate)
	incErr := retryTreeHasError(incumbent)
	if candErr != incErr {
		return !candErr
	}
	if p != nil && p.errorCostCompetitionEnabled() {
		// Faithful C recovery port (recovery-cost-competition.md issue 4,
		// moved to gotreesitter-specs (external)):
		// the retry full-parse must not replace a first-pass tree the C
		// error-cost competition already prefers. C selects trees by
		// ts_subtree_error_cost; with the gate on, a retry tree wins only
		// when it is strictly cheaper. The remaining engine heuristics
		// (notably "fewer root children") break exact cost ties only.
		if cc, ic := p.cTreeErrorCost(candidate), p.cTreeErrorCost(incumbent); cc != ic {
			return cc < ic
		}
	}
	candRoot := rawRootOrNil(candidate)
	incRoot := rawRootOrNil(incumbent)
	candRootIsError := candRoot == nil || candRoot.IsError()
	incRootIsError := incRoot == nil || incRoot.IsError()
	if candRootIsError != incRootIsError {
		return !candRootIsError
	}
	candStop := retryStopRank(candRT)
	incStop := retryStopRank(incRT)
	if candStop != incStop {
		return candStop > incStop
	}
	candChildren := retryTreeChildCount(candidate)
	incChildren := retryTreeChildCount(incumbent)
	if candChildren != incChildren {
		return candChildren < incChildren
	}
	return candRT.NodesAllocated < incRT.NodesAllocated
}

func shouldTakeCleanWideRetry(incumbent, candidate *Tree, sourceLen int, initialMaxStacks int) bool {
	if candidate == nil || retryTreeHasError(candidate) {
		return false
	}
	candRT := candidate.ParseRuntime()
	if candRT.TokenSourceEOFEarly {
		return false
	}
	if retryTreeEndByte(candidate) < retryTreeEndByte(incumbent) {
		return false
	}
	switch candRT.StopReason {
	case ParseStopAccepted:
	case ParseStopNoStacksAlive:
		if !retryTreeCoversExpectedEOF(candidate) {
			return false
		}
	default:
		return false
	}
	if !candRT.Truncated {
		return true
	}
	return shouldRetryFullParse(incumbent, sourceLen) ||
		shouldRetryStackPressureCleanFullParse(incumbent, sourceLen, initialMaxStacks)
}

func scaledNodeLimit(limit, scale int) int {
	if limit <= 0 {
		return 0
	}
	if scale <= 1 {
		return limit
	}
	maxInt := int(^uint(0) >> 1)
	if limit > maxInt/scale {
		return maxInt
	}
	return limit * scale
}

func effectiveFullParseInitialMaxStacks(lang *Language, initialMaxStacks int) int {
	if initialMaxStacks <= 0 {
		initialMaxStacks = maxGLRStacks
	}
	if lang == nil {
		return initialMaxStacks
	}
	switch lang.Name {
	case "bash":
		// bash's historical 256 floor (arrived 2026-03-07 in a C#-focused
		// commit, no bash rationale recorded) was removed by the 2026-07
		// cliff campaign. It papered over the real defect — the bash branch
		// of compareStackCullKeys preferred SHALLOWER stacks, so under cull
		// pressure the deeper accept-capable lineage was evicted at ANY cap
		// (2..256 all ended in an EOF mass-pause + whole-file recovery wrap;
		// only 512 survived) — while multiplying every survivor-count cost
		// by 32x. With the cull ordering fixed, the global default parses
		// the bash corpus clean and byte-shape-identical to the pinned C
		// oracle (small__release.sh 46s -> well under 1s; medium__clean-old.sh
		// 17-20s -> ~50ms), and all bash regression tests pass unwidened.
	case "css", "scss":
		// Large stylesheet corpora spend most of their time churning on the
		// same RS conflicts without needing a wide steady-state stack budget.
		// Keep the built-in default tight, but preserve explicit caller/env
		// overrides for diagnostics and experiments.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "hcl":
		// Large HCL configs spend disproportionate time keeping equivalent
		// branches alive during the first pass. A tight default keeps real-world
		// configs on the winning branch sooner without affecting parity, while
		// still allowing explicit overrides and retry widening.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "objc":
		// ObjC's recovery-heavy GNUstep sources can keep enough equivalent
		// Objective-C method/preprocessor branches alive that C-recovery cost
		// competition exhausts the per-parse memory budget before EOF. Cap 2
		// preserves the C-recovery parity lift while keeping large witnesses
		// bounded; explicit overrides remain available for diagnostics.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "elisp":
		// Wide survivor budgets multiply elisp's huge quoted data lists across
		// equivalent stacks until the per-parse arena budget kills the parse
		// mid-file (authors.el and the leuven/manoj theme files truncate at
		// the default cap). Cap 2 parses them all byte-identical to C.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "properties", "turtle":
		// Both grammars churn equivalent survivor stacks catastrophically at
		// the default cap: properties blows the 512MB arena budget on a 6.6KB
		// catalina.properties and turtle hits the iteration limit on a
		// 954-byte manifest.ttl. Cap 2 parses both byte-identical to C.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "git_config":
		// Long quoted values with escape sequences (e.g. diff xfuncname
		// regexes) churn equivalent survivor stacks until a 618-byte config
		// hits the iteration limit mid-file and truncates (root EndByte 582 vs
		// C 618). Cap 2 parses the curated corpus byte-identical to C; cap 3
		// measures the same, cap 8 still truncates.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "forth":
		// Forth's word-soup grammar multiplies equivalent survivor stacks on
		// real gforth sources until parses truncate: at the default cap only
		// 20/40 corpus files matched C (16 truncated, medianRatio 110x). Cap 2
		// lifts the corpus to 34/40 (medianRatio 3.8x); caps 1 and 3 measure
		// identically. The remaining 6 divergences are not stack-budget
		// effects: 4 are the engine-level leading-whitespace root-span
		// divergence (Go roots at byte 0, C roots after leading extras) and 2
		// are child-count/truncation cases.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "crystal":
		// Crystal's huge uniform hash literals (markd's 111KB entities.cr, 2127
		// "k" => "v" entries) multiply equivalent survivor stacks until the
		// merge-equivalence frontier rescan dominates: at the default cap a
		// 20KB slice of that file takes 60s and the full file never finishes
		// (>240s), with 95% of CPU in mergeStacksWithScratch /
		// stackEntryNodesEquivalentFrontierWithScratch (~2x input -> ~25x
		// time). Cap 2 parses the full file in 130ms and the first 40 corpus
		// files all complete (max 401ms); cap 3 already truncates entities.cr.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "groovy":
		// Groovy's pleac11_15.groovy corpus cliff retains redundant survivor
		// stacks until the Go full-parse attempt either times out at 10s or
		// crosses a 1536MiB RSS watchdog before the file checkpoint. Cap 2 keeps
		// the same exact file bounded (about 1.7s in the contended Docker probe);
		// the file remains a C-shape known gap, not a parity-clean ratchet row.
		// Explicit overrides stay available for diagnosis.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "d":
		// D's large dmd expressionsem.d witness can grow past the 1536MiB RSS
		// watchdog before the first full-parse checkpoint at the global default.
		// Starting tight keeps the Go parse bounded on that file (the grammar's
		// conflict floor raises the effective parse cap to 3) while explicit
		// overrides remain available for diagnosis. The exact witness is still a
		// scoped perf-ledger row because the C oracle itself is high-RSS there.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "javascript":
		// Large JavaScript UMD/runtime bundles need enough survivors to keep the
		// outer call-expression branch alive through long function arguments.
		// Cap 2 is fast on small samples but misrecovers large bundles as ERROR;
		// cap 6 preserves the C-compatible tree without jumping to TSX's wider
		// ambiguity profile.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 6
		}
	case "java":
		// Annotated Java classes with nested declarations can accept with a
		// root ERROR before the retry widener fires. A bounded cap of 14 keeps
		// the class-declaration branch alive and clears the 40-file canonical
		// corpus while preserving explicit overrides.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 14
		}
	case "tsx":
		// React-heavy TSX still needs a wider steady-state budget than plain
		// JavaScript; lower caps misparse real generic-call cases even when they
		// finish faster.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 6
		}
	case "dart":
		// Dart's generic-call/relational ambiguity needs at least six survivors
		// on real-world extension bodies; caps of two or four drop the branch C
		// selects. The default cap of eight preserves parity but keeps redundant
		// GLR frontiers alive through the large-source fallback path, so start at
		// the minimum safe width while preserving explicit diagnostic overrides.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 6
		}
	case "typescript":
		// TypeScript benefits from a tighter steady-state survivor budget than
		// JavaScript/TSX on both synthetic full parses and real-corpus files.
		// Keeping the default at 2 avoids large first-pass ambiguity churn while
		// still preserving retry widening for genuinely harder files.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "rust":
		// Rust's large real-corpus impl/match sites converge more reliably with
		// a much narrower initial survivor budget. Wider defaults preserve the
		// wrong branch through complex arm interactions and produce stable
		// wrong-tree failures without improving accepted parses.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "python":
		// Python's indentation-heavy external-scanner path benefits from a much
		// tighter steady-state survivor budget. The default cap of 8 triggers
		// expensive full-parse retries on simple synthetic and corpus-shaped
		// inputs, while 2 keeps the first pass on the winning branch and still
		// preserves retry widening for genuinely ambiguous cases.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "comment":
		// The comment grammar is intentionally broad and line/noise heavy. The
		// default survivor budget preserves too many equivalent text/URI paths
		// on real .txt corpus files and hits the iteration cap mid-file; cap 2
		// keeps the parser on the C-compatible branch and avoids the blowup.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 2
		}
	case "php":
		// PHP's modifier/recovery-heavy top-level sources can need more than the
		// default stack budget to reach the C-compatible branch. Starting at 16
		// avoids the expensive retry cycle on the high-population corpus while
		// preserving the selected recovery tree; 32 changes the hot keywords
		// sample's parse parity.
		if initialMaxStacks < 16 {
			initialMaxStacks = 16
		}
	case "go":
		// Under the ts2go Go blob the initial cap was held at 2 because cap=8
		// caused exponential blowup on large files — and the retry-with-widening
		// cycle handled edge cases. Our grammargen-compiled Go blob (shipped as
		// of #35) has a markedly different GLR conflict profile thanks to LR(1)
		// state splitting, so the blowup no longer applies; cap=2 now triggers
		// the retry cycle on most real-world Go files (parser.go, parser_reduce.go,
		// parser_test.go / query_test.go styles). Raising the default to 32
		// matches the pattern used for Ruby ("avoids an expensive retry-with-
		// widening cycle on every parse, cutting memory usage roughly in half").
		if initialMaxStacks < 32 {
			initialMaxStacks = 32
		}
	case "ruby":
		// Ruby's ambiguous syntax (optional parentheses, flexible method calls,
		// complex string/regex literals) requires wider GLR stacks than the
		// default cap of 8. Real-world Ruby files consistently need ~18 stacks.
		// Setting this to 32 avoids an expensive retry-with-widening cycle on
		// every parse, cutting memory usage roughly in half.
		if initialMaxStacks < 32 {
			initialMaxStacks = 32
		}
	case "markdown":
		// Markdown block parsing benefits from a tight steady-state survivor
		// budget, but link-reference-use followed by a definition needs the
		// ninth live stack to preserve the clean paragraph + definition branch.
		// Cap 5 keeps the cull threshold at 9, retaining that branch without
		// returning to the broader default GLR budget.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 5
		}
	case "markdown_inline":
		// Dense inline-heavy markdown (mixed **bold**/*em*/`code`/tables/
		// footnotes) converges on the winning branch very quickly. Wider
		// steady-state survivor budgets keep equivalent GLR branches alive
		// through the whole parse, and the stack-merge phase dominates CPU
		// (~70% cum in pprof). A tight initial cap of 4 forces early pruning
		// (50x speed-up on the mdpp zero-cgo-parsing.mdpp corpus) and still lets
		// the retry-widen cycle handle genuinely harder inputs.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 4
		}
	}
	return initialMaxStacks
}

func fullParseInitialMaxStacks(lang *Language, conflictWidth int) int {
	initialMaxStacks := effectiveFullParseInitialMaxStacks(lang, parseMaxGLRStacksValue())
	if conflictWidth > initialMaxStacks {
		initialMaxStacks = conflictWidth
	}
	return initialMaxStacks
}

func effectiveParseMergePerKeyCap(lang *Language, mergePerKeyCap int, incremental bool, sourceLen ...int) int {
	if lang == nil {
		return mergePerKeyCap
	}
	if incremental {
		if lang.Name == "dart" && dartIncrementalFallbackCanUseTightMergeCap(sourceLen...) &&
			!parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 4 {
			return 4
		}
		return mergePerKeyCap
	}
	switch lang.Name {
	case "dart":
		// Dart's generic/postfix ambiguity keeps redundant same-key survivors
		// alive across full parses. Three survivors preserve the current
		// parse/highlight parity surface while reducing merge-equivalence churn;
		// explicit env overrides stay available for grammar diagnosis.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 3 {
			return 3
		}
	case "go":
		// Go's full-tree path is false-equivalence heavy around expression/type
		// ambiguity. Three same-key survivors preserve the current parse,
		// highlight, and query gates, while cap=2 prunes a required branch.
		// With faithful cap-one condense, tied same-key readings are
		// preserved through multi-link GSS nodes, so the steady-state
		// full-parse cap can tighten. Explicit diagnostic overrides and
		// incremental reparses stay wide.
		//
		// This steady-state cap does NOT widen for the
		// `_automatic_semicolon` external-scanner ASI fix's fallout
		// (grammars/go_scanner.go; the fix itself restructures the LALR
		// table enough that Go's pre-existing, upstream-intentional
		// dynamic-precedence tie between index_expression and
		// generic_type(composite_literal), both PrecDynamic(1, ...), needs
		// more merge-per-key survivors on some real files than cap=3
		// provides). Two things were tried and reverted before landing on
		// the retry-rung design actually used (fullParseRetryMergePerKeyOverride's
		// "go" case, goAcceptedErrorMergePerKeyRetry): (1) a source-content
		// gate for `identifier[identifier] (!=|==) identifier[identifier]`
		// shapes only (shipped in d6d5e5b7) missed non-bracket-shaped
		// triggers entirely — cursor_test.go, language_forest_optin_test.go,
		// query_kotlin_regression_test.go (this repo) and
		// sort_slices_benchmark_test.go (stdlib) all parsed clean pre-ASI-fix
		// and clean under the C oracle, but flipped to ERROR under that gate.
		// (2) An unconditional steady-state raise to cap=8 (shipped in
		// a03cdff0) fixed those four plus a pre-existing misparse
		// (TestParseGoRangeWithNestedFunctionLiteralBody) but cost 4-6x on
		// large real files that never needed the wider budget (this repo's
		// own parser.go, parser_reduce.go, grammargen/lr.go — all clean at
		// cap=3) AND was itself non-monotonic in the cap value:
		// grammargen/normalize.go parsed clean at cap=3 but produced a false
		// ERROR at every fixed steady-state cap from 8 through 16 tested (a
		// from-scratch full parse at a fixed, elevated cap can select a
		// WORSE merge winner than one that started at cap=3 — a genuine GLR
		// merge-selection engine finding, not specific to Go). That
		// non-monotonicity is why this steady-state cap stays at 3 rather
		// than being raised again: there is no single fixed value that is
		// safe for every file.
		//
		// The retry rung sidesteps both problems for free: it only fires
		// when the cap=3 parse itself reports HasError (ParseStopAccepted +
		// retryTreeHasError), so clean files (the overwhelming majority,
		// including all of parser.go/parser_reduce.go/grammargen/lr.go/
		// grammargen/normalize.go) never retry and never risk the
		// non-monotonic misselection above; only files already broken at
		// cap=3 (the four regression files, residual_adjacent_funcs_call_then_if_ne,
		// and TestParseGoRangeWithNestedFunctionLiteralBody) pay a second
		// parse to get fixed. See goAcceptedErrorMergePerKeyRetry's doc
		// comment for why that second parse uses cap=16, not cap=8: a
		// genuinely fresh parse reaches clean at cap=8 for every one of
		// those files, but sort_slices_benchmark_test.go specifically did
		// not reach clean through an 8-cap *retry* (same target cap, worse
		// result than a fresh parse at that cap) — a second, distinct
		// instance of the same engine-level path-dependence.
		//
		// RCA seed for a real engine investigation (not this fix): the
		// underlying PrecDynamic-tie merge-selection nondeterminism — both
		// the cap-value non-monotonicity (grammargen/normalize.go) and the
		// retry-vs-fresh-parse discrepancy at an identical cap
		// (sort_slices_benchmark_test.go) — is real and not Go-specific; the
		// same PrecDynamic-tie family shows up in tsx/typescript's `a<b>(c)`
		// ambiguity (see the "typescript"/"tsx" case in
		// fullParseRetryMergePerKeyOverride). Minimal 153-byte repro that is
		// clean at cap=3 but errors at every fixed steady-state cap from 8
		// through 16 (this one IS fixed by the retry rung, since the rung
		// never fires for it — it is seed material for the underlying engine
		// behavior, not an open regression):
		//
		//	package p
		//	func f() {
		//		for value, names := range candidatesByValue {
		//			if anonymousSources[value] {
		//				out[names[0]] = true
		//			}
		//		}
		//	}
		if !parseMaxMergePerKeyEnvConfigured() {
			if glrFaithfulCapOneMerge && mergePerKeyCap > 1 {
				return 1
			}
			if mergePerKeyCap > 3 {
				return 3
			}
		}
	case "c":
		// C's declaration/expression recovery can keep many redundant
		// same-key survivors alive on large full parses. One survivor matches
		// the parity corpus while removing most merge-equivalence churn; keep
		// explicit env overrides available for grammar diagnosis.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "cpp":
		// C++ token-source recovery can retain many equivalent declaration-list
		// survivors on accepted-error parses. One same-key survivor keeps the
		// current C++ parse/highlight/query gates clean while removing most of
		// the full-parse merge-equivalence churn; keep explicit env overrides
		// available for diagnosing grammar-specific recovery cases.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "json":
		// JSON recovery has a small conflict surface, but retaining many
		// alternatives per merge key makes equivalence checks dominate full
		// parses without changing the accepted tree in parity coverage.
		if mergePerKeyCap > 1 {
			return 1
		}
	case "kotlin":
		// Kotlin's statement-recovery conflicts overflow the default per-key
		// survivor budget frequently on fresh parses. Parity coverage remains
		// stable with one survivor, while avoiding the redundant alternatives
		// removes most merge-equivalence churn.
		if mergePerKeyCap > 1 {
			return 1
		}
	case "scheme":
		// Scheme's accepted-error corpus path can retain many same-key
		// survivors around dense datum/recovery ambiguity. One survivor keeps
		// the bounded Scheme shape set stable while making the s/5_3.ss wall
		// measurable; explicit env overrides remain available for diagnosis.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "php":
		// PHP's namespace/modifier-heavy corpus keeps many equivalent recovery
		// branches alive around statement/declaration ambiguity. One full-parse
		// survivor preserves the current parse and highlight parity gates while
		// removing most merge-equivalence churn; incremental reparses keep the
		// wider default above.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "sql":
		// SQL recovery can retain thousands of same-key statement-expression
		// alternatives on SELECT-heavy inputs. One full-parse survivor preserves
		// the focused parse/highlight parity gate while removing the redundant
		// GLR churn; explicit env overrides and incremental reparses stay wide.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "r":
		// R's call/argument grammar can keep many same-key alternatives alive
		// even on tiny call-heavy inputs. One full-parse survivor preserves the
		// current parse/highlight parity surface while preventing no-tree GLR
		// churn from growing into multi-GB RSS.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "scala":
		// Scala's expression/template grammar can retain huge same-key survivor
		// sets before result selection on real-world files. Keep one full-parse
		// survivor by default so the language remains bounded and measurable;
		// explicit env overrides stay available for deeper parity diagnosis.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "powershell":
		// PowerShell's command/pipeline grammar can keep redundant same-key
		// recovery survivors alive across script-sized inputs. One full-parse
		// survivor preserves the current parity surface and brings both full
		// and no-tree parse paths back into the C-tier range.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "graphql":
		// GraphQL schema/query sources can retain redundant same-key value and
		// operation-definition alternatives. One full-parse survivor preserves
		// the current parity surface while removing the merge-equivalence churn.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "haskell":
		// Haskell's layout-heavy grammar can retain redundant same-key module
		// and declaration alternatives long enough for large generated sources
		// to blow past practical parse bounds. One full-parse survivor preserves
		// the current real-corpus C parity surface while making large files
		// measurable; incremental reparses and explicit env overrides stay wide.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "make":
		// Makefile line-text ambiguities need both the open-repeat and
		// close-repeat branches to preserve the C-compatible tree; one survivor
		// misrecovers the current corpus. Two same-key survivors keep that
		// branch pair alive while cutting most of the redundant GLR frontier.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 2 {
			return 2
		}
	case "lua":
		// Lua's string/call-heavy recovery can keep redundant alternatives
		// alive even on small files, so the cap stays below the default. It
		// cannot drop to 1: the table-constructor field list (field
		// (sep field)* sep?) needs two same-key survivors at each separator
		// or the trailing-separator branch is pruned and a clean parse
		// degrades into recovery (`t = { a = 1, b = 2, c = 3, }` grew a
		// zero-width MISSING field with cap=1 under the DFA lexer path).
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 2 {
			return 2
		}
	case "ruby":
		// Ruby still needs a wider stack budget for some real-world files, but
		// same-key merge survivors are redundant on the current parity surface.
		// One full-parse survivor removes the result-selection churn while
		// preserving explicit env overrides for grammar diagnosis.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "rust":
		// Rust's impl/match-heavy full parses keep redundant same-key recovery
		// branches alive through large AST-shaped sources. One survivor cuts
		// full-parse GLR work while the Rust recovery path now clones recovered
		// top-level chunks directly into the result arena, avoiding the old
		// offset-root allocation cliff. Incremental reparses and explicit env
		// overrides keep the wider default.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "xml":
		// XML's nested markup grammar can keep equivalent element/text branches
		// alive on document-shaped inputs. One full-parse survivor keeps the
		// current parse/highlight parity clean while reducing merge work.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "toml":
		// TOML has a small conflict surface, but redundant same-key table/value
		// survivors dominate the current real-corpus full parse. One survivor
		// keeps parse/highlight parity clean and brings it under the C baseline.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "nix":
		// Nix real-corpus parses are tiny in token count but spend most full-parse
		// time comparing redundant same-key expression alternatives. One survivor
		// preserves the current parity surface while removing the merge churn;
		// incremental reparses and explicit env overrides keep the wider default.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "ocaml":
		// OCaml real-corpus full parses can retain over a million same-key
		// survivors around expression/operator ambiguity. One survivor preserves
		// strict C parity on the current corpus and removes the merge-equivalence
		// cliff; incremental reparses and explicit env overrides keep the wider
		// default.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "javascript":
		// Plain JS can develop many near-equivalent GLR survivors on large
		// runtime bundles. Keeping more than four alternatives per merge key
		// causes merge-equivalence checks to dominate without improving the
		// accepted tree; retry widening should not undo this language cap.
		if mergePerKeyCap > 4 {
			return 4
		}
	case "starlark":
		// Bazel/Starlark BUILD files and .bzl files accumulate many same-key
		// alternatives around call-heavy top-level forms. One survivor matches
		// the current parse/highlight/query gates and removes the merge phase
		// as the dominant full-parse cost on Aspect-shaped workloads.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	case "elixir":
		// Elixir's terminator/repetition conflicts can keep many same-key
		// block/source alternatives alive. Without faithful condense, body
		// continuation vs next stab_clause still needs two same-key survivors.
		// With faithful cap-one condense, tied same-key readings are preserved
		// through multi-link GSS nodes, so the steady-state cap can tighten.
		// Keep explicit diagnostic overrides and incremental reparses wider.
		if !parseMaxMergePerKeyEnvConfigured() {
			if glrFaithfulCapOneMerge && mergePerKeyCap > 1 {
				return 1
			}
			if mergePerKeyCap > 2 {
				return 2
			}
		}
	case "typescript", "tsx":
		// TypeScript-family sources in repository indexing workloads are
		// import/query heavy and frequently fork around expression/import
		// ambiguity. Small Aspect-shaped files stay stable with one same-key
		// survivor, while large parser.ts-class sources need the wider default
		// to avoid expensive recovery/result paths.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 && typescriptFullParseCanUseTightMergeCap(sourceLen...) {
			return 1
		}
	case "java":
		// Giant generated string/switch-heavy Java sources can retain millions
		// of redundant GLR survivors under the default per-key budget. Keep one
		// steady-state survivor for full parses. Annotation declaration sources
		// are widened earlier from source text because cap=1 can discard the
		// top-level @interface declaration branch before result selection.
		// Accepted-error retries can still widen this cap when a file proves the
		// steady-state budget is insufficient.
		// Preserve explicit env overrides for diagnosis and parity experiments.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 1 {
			return 1
		}
	}
	return mergePerKeyCap
}

func typescriptFullParseCanUseTightMergeCap(sourceLen ...int) bool {
	// The tight cap applies uniformly regardless of file size. This function
	// previously disengaged the cap above 64KB ("large parser.ts-class sources
	// need the wider default to avoid expensive recovery/result paths",
	// 719cbe90), which created a size-threshold discontinuity: the wide
	// six-survivor budget retains redundant unreduced-spine survivors whose
	// conflict-reduce frontier walk re-forks their entire pending spine at
	// every statement boundary (O(n) transient forks per token, O(n^2) node
	// allocation over the file). Union-type-list-shaped .d.ts sources crossed
	// from "parses in milliseconds" at 64KB-epsilon to "memory_budget with
	// 1548 transient stacks" at 64KB+epsilon. The tight cap is what prevents
	// those survivors from being retained in the first place; typed-arrow and
	// destructured-arrow sources still widen through the dedicated gates in
	// configureParseCaps, and accepted-error retries still widen through
	// fullParseRetryMergePerKeyOverride.
	_ = sourceLen
	return true
}

func dartIncrementalFallbackCanUseTightMergeCap(sourceLen ...int) bool {
	return len(sourceLen) > 0 && sourceLen[0] > dartIncrementalReuseMaxSourceBytes
}

func typeScriptFullParseNeedsTypedArrowMergeWidth(lang *Language, source []byte, reuse *reuseCursor) bool {
	return lang != nil &&
		reuse == nil &&
		!parseMaxMergePerKeyEnvConfigured() &&
		(lang.Name == "typescript" || lang.Name == "tsx") &&
		typeScriptSourceHasTypedArrowParameters(source)
}

func typeScriptFullParseNeedsDestructuredArrowReturnMergeWidth(lang *Language, source []byte, reuse *reuseCursor) bool {
	return lang != nil &&
		reuse == nil &&
		!parseMaxMergePerKeyEnvConfigured() &&
		(lang.Name == "typescript" || lang.Name == "tsx") &&
		typeScriptSourceHasDestructuredArrowReturnType(source)
}

func typeScriptSourceHasTypedArrowParameters(source []byte) bool {
	if len(source) == 0 || !bytes.Contains(source, []byte(":")) {
		return false
	}
	offset := 0
	for {
		rel := bytes.Index(source[offset:], []byte("=>"))
		if rel < 0 {
			return false
		}
		arrow := offset + rel
		i := arrow - 1
		for i >= 0 {
			switch source[i] {
			case ' ', '\t', '\n', '\r':
				i--
				continue
			}
			break
		}
		if i < 0 || source[i] != ')' {
			offset = arrow + len("=>")
			continue
		}
		open := matchingOpenParenBefore(source, i, 2048)
		if open >= 0 && bytes.Contains(source[open:i], []byte(":")) {
			return true
		}
		offset = arrow + len("=>")
	}
}

func matchingOpenParenBefore(source []byte, close int, maxDistance int) int {
	if close < 0 || close >= len(source) || source[close] != ')' {
		return -1
	}
	min := close - maxDistance
	if min < 0 {
		min = 0
	}
	depth := 0
	for j := close; j >= min; j-- {
		switch source[j] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func typeScriptSourceHasDestructuredArrowReturnType(source []byte) bool {
	if len(source) == 0 || !bytes.Contains(source, []byte("=>")) {
		return false
	}
	offset := 0
	for {
		rel := bytes.Index(source[offset:], []byte("=>"))
		if rel < 0 {
			return false
		}
		arrow := offset + rel
		i := arrow - 1
		for i >= 0 {
			switch source[i] {
			case ' ', '\t', '\n', '\r':
				i--
				continue
			}
			break
		}
		colon := -1
		for j := i; j >= 0 && arrow-j <= 512; j-- {
			if source[j] == ':' {
				colon = j
				break
			}
			if source[j] == ')' {
				break
			}
		}
		if colon >= 0 {
			close := colon - 1
			for close >= 0 {
				switch source[close] {
				case ' ', '\t', '\n', '\r':
					close--
					continue
				}
				break
			}
			if open := matchingOpenParenBefore(source, close, 2048); open >= 0 && bytes.ContainsAny(source[open:close], "[{") {
				return true
			}
		}
		offset = arrow + len("=>")
	}
}

func fullParseUsesDeterministicExternalConflicts(lang *Language) bool {
	return lang != nil &&
		lang.ExternalScanner != nil &&
		(lang.Name == "yaml" || lang.Name == "scala")
}

func shouldRepeatExternalScannerFullParse(lang *Language, tree *Tree) bool {
	if lang == nil || lang.ExternalScanner == nil || tree == nil {
		return false
	}
	if lang.Name == "python" || lang.Name == "dart" {
		return false
	}
	// Skip the redundant re-parse when the first attempt already produced a
	// clean tree — retrying a clean parse wastes significant time and memory
	// for grammars with large state tables (e.g. Ruby).
	if treeParseClean(tree) {
		return false
	}
	return true
}

func fullParseRetryMaxStacksOverride(tree *Tree, sourceLen int, initialMaxStacks int) int {
	// An explicit GOT_GLR_MAX_STACKS is a true ceiling: diagnostics and
	// experiments depend on the survivor cap actually holding, and the
	// widening below silently defeated it (2026-07 cliff campaign finding).
	if parseMaxGLRStacksEnvConfigured() {
		return 0
	}
	if fullParseRetryUsesInitialStackCeiling(tree, sourceLen, initialMaxStacks) {
		return 0
	}
	retryMaxStacks := fullParseRetryMaxGLRStacks
	if tree != nil && tree.language != nil && tree.language.Name == "java" {
		retryMaxStacks = javaFullParseRetryMaxGLRStacks
	}
	if initialMaxStacks > retryMaxStacks {
		retryMaxStacks = initialMaxStacks * 2
	}
	if parseMaxGLRStacksValue() >= retryMaxStacks {
		return 0
	}
	if shouldRetryFullParse(tree, sourceLen) ||
		shouldRetryAcceptedErrorParse(tree, sourceLen, initialMaxStacks) ||
		shouldRetryStackPressureCleanFullParse(tree, sourceLen, initialMaxStacks) {
		return retryMaxStacks
	}
	return 0
}

func fullParseRetryUsesInitialStackCeiling(tree *Tree, sourceLen int, initialMaxStacks int) bool {
	if tree == nil || tree.language == nil {
		return false
	}
	switch tree.language.Name {
	case "groovy":
		// The large Groovy corpus cliff accepts at the tuned cap=2 but the
		// widened retry spends the entire file budget and still returns an
		// error-bearing tree. Treat the language-tuned cap as the ceiling for
		// large files. Cap-2 merge retries remain available, and explicit env
		// overrides are handled above as diagnostic ceilings. This contains the
		// timeout/OOM class only; the exact witness remains a tracked C-shape gap.
		return sourceLen >= 64*1024 && initialMaxStacks <= 2
	case "d":
		// D's expressionsem.d accepts under the tuned cap (raised to 3 by the
		// grammar conflict floor) and stays below the prior Go-side RSS cliff.
		// Widening accepted-error retry stacks reintroduces the survivor
		// population blowup, so keep large D retries at the initial ceiling.
		return sourceLen >= dLargeFileRetryMinBytes && initialMaxStacks <= dLargeFileRetryStackCeiling
	default:
		return false
	}
}

func fullParseRetryNodeLimitOverride(tree *Tree, sourceLen int) int {
	if !shouldRetryNodeLimitParse(tree, sourceLen) {
		return 0
	}
	limit := tree.ParseRuntime().NodeLimit
	if limit <= 0 {
		limit = parseNodeLimit(sourceLen)
	}
	return scaledNodeLimit(limit, fullParseRetryNodeLimitScale)
}

func fullParseRetrySecondaryNodeLimitOverride(tree *Tree, sourceLen int) int {
	if tree == nil || sourceLen <= 0 || sourceLen > fullParseRetryMaxSourceBytes {
		return 0
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != ParseStopNodeLimit {
		return 0
	}
	limit := rt.NodeLimit
	if limit <= 0 {
		return 0
	}
	return scaledNodeLimit(limit, fullParseRetrySecondaryNodeLimitScale)
}

func fullParseRetryMergePerKeyOverride(tree *Tree, sourceLen int, initialMaxStacks int) int {
	if tree == nil || sourceLen <= 0 || sourceLen > fullParseRetryMaxSourceBytes {
		return 0
	}
	if treeParseClean(tree) {
		return 0
	}
	rt := tree.ParseRuntime()
	if rt.TokenSourceEOFEarly {
		return 0
	}
	switch rt.StopReason {
	case ParseStopAccepted, ParseStopNoStacksAlive, ParseStopNodeLimit:
	default:
		return 0
	}
	if tree.language != nil && tree.language.Name == "java" && rt.StopReason == ParseStopAccepted && retryTreeHasError(tree) {
		return javaFullParseRetryMaxMergePerKey
	}
	if tree.language != nil && tree.language.Name == "go" && rt.StopReason == ParseStopAccepted && retryTreeHasError(tree) {
		// See goAcceptedErrorMergePerKeyRetry's doc comment and the "go" case
		// in effectiveParseMergePerKeyCap for the full account. TL;DR: the
		// `_automatic_semicolon` external-scanner ASI fix (grammars/go_scanner.go)
		// restructured enough of the LALR table that Go's pre-existing,
		// upstream-intentional dynamic-precedence tie between
		// index_expression and generic_type(composite_literal) (both
		// PrecDynamic(1, ...)) needs more merge-per-key survivors than the
		// steady-state cap=3 on some real files — but ONLY those files, so
		// this is scoped to the retry rung (fires on an accepted-but-erroring
		// fresh parse) rather than a permanent language-wide cap raise, which
		// cost 4-6x on large real files that never needed it (this repo's
		// own parser.go, parser_reduce.go, grammargen/lr.go — all clean at
		// cap=3, never retry, never pay the wider budget) and was itself
		// non-monotonic in the cap value: grammargen/normalize.go parsed
		// clean at cap=3 but produced a false ERROR at every cap from 8
		// through 16 tested (a from-scratch full parse at a fixed, elevated
		// cap can select a WORSE merge winner than one that started at
		// cap=3), so a blanket raise cannot safely replace this rung. Scoped
		// to lang.Name=="go" for now (this retry mechanism is not otherwise
		// language-aware beyond a per-language switch); the same
		// PrecDynamic-tie family shows up in tsx/typescript
		// (`a<b>(c)`-shaped ambiguity, see the "typescript"/"tsx" case just
		// below) and may want the same treatment later.
		return goAcceptedErrorMergePerKeyRetry
	}
	if tree.language != nil && (tree.language.Name == "typescript" || tree.language.Name == "tsx") &&
		rt.StopReason == ParseStopAccepted && retryTreeHasError(tree) && sourceLen > 64*1024 {
		// Large TypeScript-family files keep the wider steady-state cap, but
		// some accepted-error parses recover cleanly only when redundant
		// same-key survivors are pruned. Use a negative override as an exact
		// cap for this retry; positive retry overrides still only widen caps.
		return -4
	}
	if tree.language != nil && tree.language.Name == "cpp" &&
		rt.StopReason == ParseStopAccepted && retryTreeHasError(tree) &&
		!rt.Truncated && !rt.TokenSourceEOFEarly {
		return 0
	}
	if tree.language != nil && tree.language.Name == "bash" &&
		rt.StopReason == ParseStopAccepted && retryTreeHasError(tree) &&
		!rt.Truncated && !rt.TokenSourceEOFEarly {
		return 0
	}
	if initialMaxStacks <= 0 {
		initialMaxStacks = maxGLRStacks
	}
	if rt.MaxStacksSeen < initialMaxStacks {
		if fullParseNoStacksAliveCleanEOFNeedsMergeRetry(tree, rt) {
			return fullParseRetryMaxMergePerKey
		}
		return 0
	}
	if tree.language != nil && tree.language.Name == "java" {
		return javaFullParseRetryMaxMergePerKey
	}
	return fullParseRetryMaxMergePerKey
}

func fullParseNoStacksAliveCleanEOFNeedsMergeRetry(tree *Tree, rt ParseRuntime) bool {
	return rt.StopReason == ParseStopNoStacksAlive &&
		!rt.TokenSourceEOFEarly &&
		!retryTreeHasError(tree)
}

func shouldRunInitialFullParseMergeRetry(tree *Tree) bool {
	if tree == nil {
		return false
	}
	// When the first full parse stops on node_limit, the next useful retry is
	// almost always the wider node budget, not another full parse with the same
	// node cap plus a larger merge bucket. Keep merge-per-key retries available
	// after a widened node-budget pass if the parser still proves ambiguity-
	// bound, but skip the dead intermediate pass up front.
	rt := tree.ParseRuntime()
	if rt.StopReason == ParseStopNodeLimit {
		return false
	}
	if tree.language != nil && tree.language.Name == "c_sharp" &&
		rt.StopReason == ParseStopAccepted &&
		retryTreeHasError(tree) &&
		!rt.Truncated &&
		!rt.TokenSourceEOFEarly {
		// Large C# namespace bodies that accept with an error under the narrow
		// first-pass stack cap have repeatedly spent close to a second in this
		// same-stack merge retry without clearing the error. The existing clean
		// widened-stack retry is the next useful pass; keep later merge retries
		// available if that widened pass still returns an error.
		return false
	}
	return true
}

func (p *Parser) retryFullParse(source []byte, initialMaxStacks int, tree *Tree, runRetry fullParseRetryRunner) *Tree {
	maxStacksOverride := fullParseRetryMaxStacksOverride(tree, len(source), initialMaxStacks)
	maxNodesOverride := fullParseRetryNodeLimitOverride(tree, len(source))
	retryMaxStacks := initialMaxStacks
	if maxStacksOverride > 0 {
		retryMaxStacks = maxStacksOverride
	}

	// retryDeadline caps the cumulative wall time spent across retry
	// iterations. Without it, a pathological input that triggers all four
	// retry branches (initial-merge, node-limit, secondary-node-limit, final
	// merge-per-key) can run far longer than the caller's SetTimeoutMicros
	// budget. The parser polls timeoutMicros inside the parse loop, but between
	// retries the budget was not re-checked. We honor the same budget as a
	// wall-clock deadline shared across retry attempts.
	retryStart := time.Now()
	retryDeadlineExceeded := func() bool {
		if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
			return true
		}
		// Hard pass-count bound, independent of any wall-clock budget (which
		// is a no-op without SetTimeoutMicros — see the KNOWN GAP below).
		// Counted per top-level parse operation across every retryFullParse
		// invocation; see fullParseRetryMaxTotalPasses.
		if p != nil && p.fullParseRetryPassesTaken >= fullParseRetryMaxTotalPasses {
			return true
		}
		// KNOWN GAP (tracked, not fixed here): when the caller never
		// configures a timeout (p.timeoutMicros == 0, the default for a
		// freshly constructed Parser and for every test/benchmark helper in
		// this repo that just calls Parser.Parse), this whole deadline check
		// is a no-op and the retry cascade below has no wall-clock ceiling —
		// only the fixed per-stage caps (fullParseRetryMaxGLRStacks,
		// fullParseRetryMaxMergePerKey, the node-limit scale factors). On
		// most inputs that is fine because each stage still resolves
		// quickly even at its widened cap. But grammargen/parity_test.go (a
		// 110KB file in this repo that already carried a small, pre-existing
		// parse error under the old Go blob) runs past 90s under the current
		// Go blob without a caller-supplied timeout — root cause not
		// isolated: it is not the merge-per-key/stack-cap mechanism itself
		// (widening or narrowing both via GOT_GLR_MAX_MERGE_PER_KEY /
		// GOT_GLR_MAX_STACKS made no difference), and ASCII-substituting the
		// Unicode box-drawing characters near the pre-existing baseline
		// error ruled those out too. Follow-up: either give Parser.Parse a
		// sane default wall-clock budget, or isolate why this specific large,
		// already-imperfect-parsing file drives the retry cascade past any
		// of its per-stage caps without a timeout to fall back on.
		//
		// Related, separately discovered finding: this repo's own
		// grammars/markdown_scanner.go was already HasError=true at the
		// pre-ASI-fix baseline (StopReason accepted, reaches EOF) but is
		// HasError=true differently now (StopReason no_stacks_alive,
		// Truncated=true, stops at byte ~20860 of 37144) — not a
		// clean-to-error flip, but a worse failure shape on an
		// already-broken file, surfaced by the same repo-corpus walk used to
		// validate this fix. Not root-caused in the time available; flagging
		// alongside the parity_test.go gap above rather than leaving it
		// silently undiscovered.
		if p == nil || p.timeoutMicros == 0 {
			return false
		}
		if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
			return true
		}
		if p.parseBudgetDepth > 0 {
			return false
		}
		return time.Since(retryStart) > time.Duration(p.timeoutMicros)*time.Microsecond
	}

	// Each runRetry() produces a fresh Tree + arena. When a candidate loses
	// the compare, release its arena back to the pool immediately so later
	// runRetry() calls in this same retryFullParse can reuse it; otherwise
	// the loser's arena only returns to the pool at GC finalize time, which
	// starves every retry in a warm loop of reusable capacity. Never release
	// the incoming `tree` — it belongs to the caller.
	release := func(t *Tree) {
		if t == nil || t == tree {
			return
		}
		t.Release()
	}
	replaceBest := func(best **Tree, candidate *Tree) {
		if candidate == nil {
			return
		}
		if preferRetryTree(p, candidate, *best) {
			if *best != candidate {
				release(*best)
			}
			*best = candidate
			return
		}
		release(candidate)
	}

	structuralResyncRetry := shouldRetryFullParse(tree, len(source))
	retryDebug := os.Getenv("GOT_RETRY_DEBUG") == "1"
	if retryDebug {
		rt := tree.ParseRuntime()
		fmt.Fprintf(os.Stderr, "RETRYDBG entry stop=%v truncated=%v hasErr=%v maxStacksSeen=%d stacksOverride=%d nodesOverride=%d structResync=%v\n",
			rt.StopReason, rt.Truncated, retryTreeHasError(tree), rt.MaxStacksSeen, maxStacksOverride, maxNodesOverride, structuralResyncRetry)
	}
	// C# namespace recovery can clear this exact accepted-error shape during
	// normal result compatibility; avoid paying the full retry ladder first.
	if csharpAcceptedErrorTreeCanUseNamespaceRecovery(tree, source) {
		return tree
	}
	runRetryAttempt := func(maxStacks int, maxMergePerKeyOverride int, maxNodes int) *Tree {
		if p != nil {
			if p.fullParseRetryPassesTaken >= fullParseRetryMaxTotalPasses {
				// Budget exhausted: a nil candidate is a no-op for every
				// caller (replaceBest ignores nil), so the incumbent best
				// tree flows through unchanged.
				return nil
			}
			p.fullParseRetryPassesTaken++
		}
		var t0 time.Time
		if retryDebug {
			t0 = time.Now()
		}
		var result *Tree
		if !structuralResyncRetry || p == nil || p.forceCleanRetryPass {
			result = runRetry(maxStacks, maxMergePerKeyOverride, maxNodes)
		} else {
			prev := p.retryStructuralTopLevelResync
			p.retryStructuralTopLevelResync = true
			defer func() {
				p.retryStructuralTopLevelResync = prev
			}()
			result = runRetry(maxStacks, maxMergePerKeyOverride, maxNodes)
		}
		if retryDebug {
			stop := ParseStopNone
			hasErr := true
			if result != nil {
				stop = result.ParseRuntime().StopReason
				hasErr = retryTreeHasError(result)
			}
			fmt.Fprintf(os.Stderr, "RETRYDBG pass maxStacks=%d mergePerKey=%d maxNodes=%d took=%v stop=%v hasErr=%v forceClean=%v\n",
				maxStacks, maxMergePerKeyOverride, maxNodes, time.Since(t0), stop, hasErr, p != nil && p.forceCleanRetryPass)
		}
		return result
	}

	bestTree := tree
	if shouldRunInitialFullParseMergeRetry(tree) {
		if initialMergePerKey := fullParseRetryMergePerKeyOverride(tree, len(source), initialMaxStacks); initialMergePerKey != 0 {
			mergeRetryTree := runRetryAttempt(initialMaxStacks, initialMergePerKey, 0)
			replaceBest(&bestTree, mergeRetryTree)
			if treeParseClean(bestTree) {
				return bestTree
			}
		}
	}
	if retryDeadlineExceeded() {
		return bestTree
	}

	nodeRetryTree := tree
	if maxStacksOverride == 0 && maxNodesOverride == 0 {
		return bestTree
	}
	// A widened-stack retry would normally also enable the retry-pass
	// error-recovery behavior (single-stack resurrection on all-stacks-dead),
	// because the override exceeds the small global default budget. The original
	// failure is usually that the narrower prior budget ran every stack dead at
	// a single ambiguity peak; the extra budget alone keeps a winning branch
	// alive to a clean accepted forest. The retry-pass recovery, however,
	// derails the parse into single-stack error recovery and fragments the whole
	// tree into an ERROR root (e.g. bash for/while/case scripts that tree-sitter
	// C parses cleanly). So first try the wider budget as a clean (non-retry)
	// pass; if it parses cleanly we take it. Otherwise we fall through to the
	// retry-pass-enabled retry below, preserving prior recovery behavior.
	if maxStacksOverride > 0 && p != nil && !p.forceCleanRetryPass {
		p.forceCleanRetryPass = true
		cleanRetryTree := runRetryAttempt(retryMaxStacks, 0, maxNodesOverride)
		p.forceCleanRetryPass = false
		// A clean (non-retry-pass) wider-budget parse legitimately ends on
		// ParseStopNoStacksAlive after the winning branch reduces to the start
		// symbol and the remaining survivors die at EOF, so treeParseClean
		// (which requires ParseStopAccepted) under-reports it. Accept any
		// error-free root here; replaceBest/preferRetryTree still pick the best
		// tree if a later pass does better.
		if shouldTakeCleanWideRetry(tree, cleanRetryTree, len(source), initialMaxStacks) {
			cleanMergePerKey := fullParseRetryMergePerKeyOverride(cleanRetryTree, len(source), initialMaxStacks)
			replaceBest(&bestTree, cleanRetryTree)
			if retryTreeCoversExpectedEOF(bestTree) {
				return bestTree
			}
			if cleanMergePerKey != 0 && !retryDeadlineExceeded() {
				p.forceCleanRetryPass = true
				cleanMergeTree := runRetryAttempt(retryMaxStacks, cleanMergePerKey, maxNodesOverride)
				p.forceCleanRetryPass = false
				replaceBest(&bestTree, cleanMergeTree)
				if !retryTreeHasError(bestTree) && retryTreeCoversExpectedEOF(bestTree) {
					return bestTree
				}
			}
		} else {
			release(cleanRetryTree)
		}
		if retryDeadlineExceeded() {
			return bestTree
		}
	}
	if maxStacksOverride > 0 || maxNodesOverride > 0 {
		retryTree := runRetryAttempt(retryMaxStacks, 0, maxNodesOverride)
		// nodeRetryTree is read below for stop-reason inspection, so we hold
		// a pointer to it without handing it through replaceBest until the
		// retry sequence is done. If it doesn't end up bestTree, we release
		// it at function exit via the sentinel below.
		nodeRetryTree = retryTree
		if retryDeadlineExceeded() {
			replaceBest(&bestTree, retryTree)
			return bestTree
		}
		if extraNodeLimit := fullParseRetrySecondaryNodeLimitOverride(retryTree, len(source)); extraNodeLimit > 0 {
			secondaryTree := runRetryAttempt(retryMaxStacks, 0, extraNodeLimit)
			// Fold the primary retry into bestTree before we overwrite
			// nodeRetryTree, so the loser's arena is returned.
			if retryTree != nil {
				if preferRetryTree(p, retryTree, bestTree) {
					if bestTree != retryTree {
						release(bestTree)
					}
					bestTree = retryTree
				} else if retryTree != bestTree {
					release(retryTree)
				}
			}
			nodeRetryTree = secondaryTree
			replaceBest(&bestTree, secondaryTree)
		} else {
			replaceBest(&bestTree, retryTree)
		}
	}

	if treeParseClean(bestTree) {
		if nodeRetryTree != nil && nodeRetryTree != bestTree && nodeRetryTree != tree {
			release(nodeRetryTree)
		}
		return bestTree
	}
	maxMergePerKeyOverride := fullParseRetryMergePerKeyOverride(nodeRetryTree, len(source), initialMaxStacks)
	if maxMergePerKeyOverride == 0 {
		if nodeRetryTree != nil && nodeRetryTree != bestTree && nodeRetryTree != tree {
			release(nodeRetryTree)
		}
		return bestTree
	}
	if retryDeadlineExceeded() {
		return bestTree
	}
	mergeRetryTree := runRetryAttempt(retryMaxStacks, maxMergePerKeyOverride, maxNodesOverride)
	// nodeRetryTree is no longer needed; drop it before potentially replacing
	// bestTree so we don't leak it if it was also the incumbent.
	if nodeRetryTree != nil && nodeRetryTree != bestTree && nodeRetryTree != tree {
		release(nodeRetryTree)
	}
	replaceBest(&bestTree, mergeRetryTree)
	return bestTree
}

func (p *Parser) retryFullParseWithDFA(source []byte, initialMaxStacks int, deterministicExternalConflicts bool, tree *Tree) *Tree {
	result := p.retryFullParse(source, initialMaxStacks, tree, func(maxStacks int, maxMergePerKeyOverride int, maxNodes int) *Tree {
		retryTS := p.acquireParserDFATokenSource(source)
		defer retryTS.Close()
		return p.parseInternal(
			source,
			p.wrapIncludedRanges(retryTS),
			nil,
			nil,
			arenaClassFull,
			nil,
			maxStacks,
			maxNodes,
			maxMergePerKeyOverride,
			deterministicExternalConflicts,
		)
	})
	// retryFullParse releases losing retry trees internally (#34), but when a
	// retry winner replaces the original tree, the original's arena is orphaned.
	// Release it here since the caller will overwrite its tree reference.
	if result != tree {
		tree.Release()
	}
	return result
}

func (p *Parser) retryFullParseWithTokenSource(source []byte, ts TokenSource, initialMaxStacks int, deterministicExternalConflicts bool, tree *Tree) *Tree {
	resettable, ok := ts.(resettableTokenSource)
	if !ok {
		return tree
	}
	result := p.retryFullParse(source, initialMaxStacks, tree, func(maxStacks int, maxMergePerKeyOverride int, maxNodes int) *Tree {
		resettable.Reset(source)
		return p.parseInternal(
			source,
			p.wrapIncludedRanges(ts),
			nil,
			nil,
			arenaClassFull,
			nil,
			maxStacks,
			maxNodes,
			maxMergePerKeyOverride,
			deterministicExternalConflicts,
		)
	})
	// Same as retryFullParseWithDFA: release the original tree if a retry won.
	if result != tree {
		tree.Release()
	}
	return result
}

func (p *Parser) retryIncrementalParseAsFullWithTokenSource(source []byte, ts TokenSource, initialMaxStacks int, tree *Tree, timing *incrementalParseTiming) *Tree {
	if tree == nil {
		return tree
	}
	deterministicExternalConflicts := fullParseUsesDeterministicExternalConflicts(p.language)
	retryStart := time.Now()
	result := p.retryFullParseWithTokenSource(source, ts, initialMaxStacks, deterministicExternalConflicts, tree)
	if result == tree {
		return tree
	}
	if timing != nil {
		timing.totalNanos += time.Since(retryStart).Nanoseconds()
		timing.reuseUnsupported = true
		timing.reuseUnsupportedReason = "incremental_parse_full_retry"
		copyParseRuntimeToTiming(timing, result.ParseRuntime())
	}
	return result
}
