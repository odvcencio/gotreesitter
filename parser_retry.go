package gotreesitter

import (
	"bytes"
	"time"
)

const (
	// Retry no-stacks-alive full parses with a wider GLR cap. Large real-world
	// files (for example this repo's parser.go) can legitimately need >8 stacks
	// at peak even when parse tables report narrower local conflict widths.
	fullParseRetryMaxGLRStacks = 32
	// Some ambiguity clusters need more survivors per merge bucket even after
	// the global GLR cap is widened. Only enable this on retries for parses
	// that already proved the default merge budget was insufficient.
	fullParseRetryMaxMergePerKey = 24
	// Java's default full-parse merge cap stays intentionally narrow for large
	// generated bodies, but repeatable annotation declarations can need a wider
	// bounded retry to preserve the declaration branch.
	javaFullParseRetryMaxGLRStacks   = 64
	javaFullParseRetryMaxMergePerKey = 6
	javaTightMergeCapSourceLen       = 256 * 1024
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
	root := rawRootOrNil(tree)
	if root == nil || !root.HasError() {
		return false
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != ParseStopAccepted || rt.Truncated || rt.TokenSourceEOFEarly {
		return false
	}
	if initialMaxStacks <= 0 {
		initialMaxStacks = maxGLRStacks
	}
	return rt.MaxStacksSeen >= initialMaxStacks
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

func treeParseClean(tree *Tree) bool {
	if tree == nil {
		return false
	}
	root := rawRootOrNil(tree)
	if root == nil || root.HasError() {
		return false
	}
	rt := tree.ParseRuntime()
	return rt.StopReason == ParseStopAccepted && !rt.Truncated && !rt.TokenSourceEOFEarly
}

func rootOrNil(tree *Tree) *Node {
	if tree == nil {
		return nil
	}
	return tree.RootNode()
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
	return root.HasError()
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

func preferRetryTree(candidate, incumbent *Tree) bool {
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
		if initialMaxStacks < 256 {
			initialMaxStacks = 256
		}
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
	case "javascript":
		// Large JavaScript UMD/runtime bundles need enough survivors to keep the
		// outer call-expression branch alive through long function arguments.
		// Cap 2 is fast on small samples but misrecovers large bundles as ERROR;
		// cap 6 preserves the C-compatible tree without jumping to TSX's wider
		// ambiguity profile.
		if initialMaxStacks == maxGLRStacks {
			initialMaxStacks = 6
		}
	case "tsx":
		// React-heavy TSX still needs a wider steady-state budget than plain
		// JavaScript; lower caps misparse real generic-call cases even when they
		// finish faster.
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
	case "markdown", "markdown_inline":
		// Dense inline-heavy markdown (mixed **bold**/*em*/`code`/tables/
		// footnotes) converges on the winning branch very quickly. Wider
		// steady-state survivor budgets keep equivalent GLR branches alive
		// through the whole parse, and the stack-merge phase dominates CPU
		// (~70% cum in pprof). A tight initial cap of 4 forces early pruning
		// (50x speed-up on the mdpp zero-cgo-parsing.mdpp corpus while keeping
		// link_reference_definition disambiguation working) and still lets the
		// retry-widen cycle handle genuinely harder inputs.
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
	if lang == nil || incremental {
		return mergePerKeyCap
	}
	switch lang.Name {
	case "go":
		// Go's full-tree path is false-equivalence heavy around expression/type
		// ambiguity. Three same-key survivors preserve the current parse,
		// highlight, and query gates, while cap=2 prunes a required branch.
		if !parseMaxMergePerKeyEnvConfigured() && mergePerKeyCap > 3 {
			return 3
		}
	case "c":
		// C's declaration/expression recovery can keep many redundant
		// same-key survivors alive on large full parses. One survivor matches
		// the parity corpus while removing most merge-equivalence churn; keep
		// explicit env overrides available for grammar diagnosis.
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
	return len(sourceLen) == 0 || sourceLen[0] <= 64*1024
}

func tsxFullParseNeedsTypedArrowMergeWidth(lang *Language, source []byte, reuse *reuseCursor) bool {
	return lang != nil &&
		reuse == nil &&
		!parseMaxMergePerKeyEnvConfigured() &&
		lang.Name == "tsx" &&
		typeScriptSourceHasTypedArrowParameters(source)
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
		depth := 0
		for j := i; j >= 0 && i-j <= 2048; j-- {
			switch source[j] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					return bytes.Contains(source[j:i], []byte(":"))
				}
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
	if lang.Name == "python" {
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
	if shouldRetryFullParse(tree, sourceLen) || shouldRetryAcceptedErrorParse(tree, sourceLen, initialMaxStacks) {
		return retryMaxStacks
	}
	return 0
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
	if initialMaxStacks <= 0 {
		initialMaxStacks = maxGLRStacks
	}
	if rt.MaxStacksSeen < initialMaxStacks {
		return 0
	}
	if tree.language != nil && tree.language.Name == "java" {
		return javaFullParseRetryMaxMergePerKey
	}
	return fullParseRetryMaxMergePerKey
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
	return tree.ParseRuntime().StopReason != ParseStopNodeLimit
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
		if p == nil || p.timeoutMicros == 0 {
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
		if preferRetryTree(candidate, *best) {
			if *best != candidate {
				release(*best)
			}
			*best = candidate
			return
		}
		release(candidate)
	}

	bestTree := tree
	if shouldRunInitialFullParseMergeRetry(tree) {
		if initialMergePerKey := fullParseRetryMergePerKeyOverride(tree, len(source), initialMaxStacks); initialMergePerKey > 0 {
			mergeRetryTree := runRetry(initialMaxStacks, initialMergePerKey, 0)
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
	if maxStacksOverride > 0 || maxNodesOverride > 0 {
		retryTree := runRetry(retryMaxStacks, 0, maxNodesOverride)
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
			secondaryTree := runRetry(retryMaxStacks, 0, extraNodeLimit)
			// Fold the primary retry into bestTree before we overwrite
			// nodeRetryTree, so the loser's arena is returned.
			if retryTree != nil {
				if preferRetryTree(retryTree, bestTree) {
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
	mergeRetryTree := runRetry(retryMaxStacks, maxMergePerKeyOverride, maxNodesOverride)
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
		retryLexer := NewLexer(p.language.LexStates, source)
		retryTS := acquireDFATokenSource(retryLexer, p.language, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState)
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
