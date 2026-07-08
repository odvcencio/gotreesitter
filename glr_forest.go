package gotreesitter

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"
)

// GSS-FOREST REWRITE (perf/glr-gss-forest) — the only safe cut at the #1
// machinery gap vs tree-sitter C: deep stack-merge node-equivalence is ~46% of
// a fork-heavy parse, because we materialize one tree per stack and must
// deep-compare to dedup. tree-sitter C never compares: its graph-structured
// stack coalesces versions by (state, position) and keeps subtree alternatives
// as forest LINKS (lib/src/stack.c ts_stack_can_merge = 4 scalars + add_link),
// collapsing the forest at finalization by dynamic_precedence/error_cost.
//
// We already have (state, position) merge keys (mergeKeyForStack) and the
// disambiguator (stackCompareMerge: accepted > error-rank > score > shifted).
// The missing piece is a multi-link GSS node. This file builds it behind a flag
// so the default (table-driven dedup) path is untouched until the forest path
// reaches byte-for-byte parity.
//
// STAGED PLAN (see project_glr_merge_design memory + the gss-forest-rewrite
// spore). Stages 1 and 2 are coupled — coalesce produces alternatives that only
// parse correctly once reduce traverses all of them, so parity is expected only
// when BOTH land:
//
//	Stage 0  DONE — instrument. dedup fires 0.2%, fan-out bounded 12-20, so the
//	         forest is narrow (cheap) and the 46% is genuinely wasted compares.
//	Stage 1  DAG node + coalesce-by-(state,position) on push (this file).
//	Stage 2  reduce-over-DAG: enumerate all length-N paths through the links
//	         (C ts_stack_pop_count). The crux; needs error_cost/version bounding.
//	Stage 3  forest finalization: pick per node by score, matching tree-sitter's
//	         dynamic_precedence-then-first-match selection for byte-identical out.

// glrForestEnabled is the master switch for the GSS-forest fast path. ON by
// default: the byte-range-verified languages in builtinForestDefaults (plus any
// Language with WantsForest set, see parserWantsForest) dispatch to the forest
// automatically (with production fallback). Set GOT_GLR_FOREST=0 to disable
// globally; tests/benchmarks toggle via SetGLRForestEnabled. Languages that
// want neither always use production regardless of this switch.
var glrForestEnabled = os.Getenv("GOT_GLR_FOREST") != "0"

// SetGLRForestEnabled toggles the GSS-forest path at runtime (tests/benchmarks).
func SetGLRForestEnabled(on bool) { glrForestEnabled = on }

// nodeCachedHeight returns the subtree height (root = 1), memoized on the node
// (n.subtreeHeight, 0 = uncomputed). Nodes are immutable after build and arena
// slots are zeroed on alloc, so the cache is valid within a parse and never stale
// across parses. Keeps the coalesce dedup tie-break O(1) amortized instead of an
// O(subtree) walk on every score tie (which 7x'd merge-heavy parses).
func nodeCachedHeight(n *Node) int {
	if n == nil {
		return 0
	}
	if n.subtreeHeight != 0 {
		return int(n.subtreeHeight)
	}
	best := 0
	for _, c := range n.children {
		if h := nodeCachedHeight(c); h > best {
			best = h
		}
	}
	h := best + 1
	if h > 255 {
		h = 255
	}
	n.subtreeHeight = uint8(h)
	return h
}

func stackEntrySubtreeHeight(e stackEntry) int {
	if e.kind != stackEntryKindNode || e.node == nil {
		return 0
	}
	return nodeCachedHeight((*Node)(e.node))
}

// forestDedupTieReplace reports whether, on a coalesce dedup score tie, the new
// entry should replace the existing link — true only when the new subtree is
// taller, mirroring tree-sitter C / production stackCompareMerge's post-score
// depth tie-break (go type_instantiation_expression over index_expression under
// the shared `_expression` supertype on `m.T[r.s][r.t]`).
func forestDedupTieReplace(entry, existing stackEntry) bool {
	return stackEntrySubtreeHeight(entry) > stackEntrySubtreeHeight(existing)
}

// glrForestRecover enables EXPERIMENTAL error recovery in the forest parse loop.
// Default OFF — the forest declines (production fallback) on any parse death, so
// this is opt-in for prototyping/measurement only. When on, a token with no valid
// action at any frontier node is absorbed into an error region (the frontier stays
// in its states and advances past the token), instead of declining. The aim is to
// reproduce the production parser's error tree fast; until it is byte-verified
// against production it must stay OFF in any default path.
var glrForestRecover = os.Getenv("GOT_GLR_FOREST_RECOVER") == "1"

// SetGLRForestRecover toggles experimental forest error recovery (tests).
func SetGLRForestRecover(on bool) { glrForestRecover = on }

// languageWantsForestRecover reports whether a forest-dispatched language enables
// the recover-action error_cost recovery path by default (so error-bearing files
// dispatch to the forest instead of declining to production). Restricted to
// languages whose recovered error trees are byte-verified against production.
//   - authzed: 25/25 lock-filtered .zed files (incl. 17 production-error files)
//     produce byte-IDENTICAL trees to production with recovery on.
func languageWantsForestRecover(name string) bool {
	switch name {
	case "authzed", "make", "csv", "fish", "racket", "tlaplus", "beancount":
		// Recovery promoted 2026-06-08 via forest-vs-C (TestForestVsCSources,
		// REPRO_RECOVER=1): with recovery, dispatch is FULL (authzed 40/40, make
		// 20/20, 0 fellback) and introduced=0 — the forest is never worse than
		// production vs C (forest-vs-production "mismatches" are all inherited
		// production C-bugs, not regressions). make's expensive blowup file now
		// dispatches to the forest instead of declining to slow production.
		return true
	case "agda", "org", "ledger", "yuck", "json5", "commonlisp", "vimdoc":
		// Phase 2 recovery promotions 2026-06-08 (tier III->II/I). Production is
		// 14x-609x C (parity-blocked); forest+recovery is 0.95x-2.27x C with
		// introduced=0 vs C on every dispatched real-corpus file (forest-vs-C with
		// REPRO_RECOVER=1: agda 24/30, org 27/30, ledger 4/4, yuck 2/2, json5 30/30;
		// all divergences inherited from production). Recovery is required because
		// these carry error nodes the no-recovery path declines. yuck/json5 are
		// additionally parity-CLEAN vs C (a correctness lift too).
		return true
	}
	// Other grammars: recovery stays opt-in. The recover-action +
	// EOF-error-root recovery (GOT_GLR_FOREST_RECOVER) reproduces production's
	// error tree on the MAJORITY of files (authzed 81/110 byte-identical) but is
	// not yet production-exact across a full corpus (authzed 29/110 diverge), so
	// it stays opt-in until the error-node-placement refinements close that gap.
	_ = name
	return false
}

// forestRecoverCap bounds total error-skip recoveries per parse so a pathological
// file cannot spin (each recovery still advances by one token, so this is a
// belt-and-suspenders guard, not the progress mechanism).
const forestRecoverCap = 1 << 20

// forestLastDeclineReason records why parseForest last declined (diagnostic only,
// not thread-safe; for single-threaded prototyping/measurement).
var forestLastDeclineReason string

const forestDeclineEOFRecoveryConflict = "eof-recovery-conflict"

func forestProgressExtra(frontier, work, nextFrontier []*gssForestNode, curIndex, nextIndex gssForestIndex, processEpoch int32, recoverCount int, reducer *forestReducer, accepted *gssForestNode, more string) string {
	curLen := curIndex.len()
	nextLen := nextIndex.len()
	reducerCapped := false
	reducerSteps := 0
	reducerVisits := 0
	if reducer != nil {
		reducerCapped = reducer.capped
		reducerSteps = reducer.steps
		reducerVisits = reducer.visitCount
	}
	extra := fmt.Sprintf("frontier_len=%d work_len=%d next_frontier_len=%d cur_index_len=%d next_index_len=%d process_epoch=%d recover_count=%d reducer_capped=%t reducer_steps=%d reducer_visits=%d accepted_present=%t",
		len(frontier),
		len(work),
		len(nextFrontier),
		curLen,
		nextLen,
		processEpoch,
		recoverCount,
		reducerCapped,
		reducerSteps,
		reducerVisits,
		accepted != nil,
	)
	if more != "" {
		extra += " " + more
	}
	return extra
}

// ParseForestExperimental parses source with the experimental GSS-forest GLR
// path and returns a releasable tree (or nil,false if the parse dies — the
// forest path has no error recovery yet). Exported so out-of-tree benchmarks
// and validation in packages that attach external scanners (e.g. grammars) can
// drive it; not part of the stable API.
func (p *Parser) ParseForestExperimental(source []byte) (*Tree, bool) {
	// Every other public parse entry point (parser_api.go: Parse,
	// ParseWithTokenSource, ParseIncremental...) establishes the
	// timeout/cancellation deadline via enterParseBudget before doing any
	// work. This is a standalone entry point (out-of-tree benchmarks/
	// validation call it directly, not just via the internal tryForestFastPath
	// dispatch which already runs inside a caller's budget), so without this
	// call SetTimeoutMicros/cancellation would never even compute a deadline
	// for a bare ParseForestExperimental call. Safe to nest: enterParseBudget
	// only (re)computes the deadline at depth 0, so calling this from within
	// an already-budgeted Parse() (were that ever wired up) would not reset it.
	endBudget := p.enterParseBudget()
	defer endBudget()
	arena := acquireNodeArena(arenaClassFull)
	root, ok := p.parseForest(arena, source, true)
	if !ok || root == nil {
		arena.Release()
		if forestLastDeclineReason == forestDeclineEOFRecoveryConflict {
			prev := glrForestEnabled
			glrForestEnabled = false
			tree, err := p.Parse(source)
			glrForestEnabled = prev
			return tree, err == nil && tree != nil
		}
		return nil, false
	}
	p.finalizeForestRoot(root, source)
	return newTreeWithArenas(root, source, p.language, arena, nil), true
}

// ForestDeclineInfo returns where/why the forest fast path last declined (fell
// back to production): the byte offset and lookahead symbol at the decline, a
// short reason code, and (for reason "dead_end") the surviving GLR states. It
// drives language-burndown triage of forest dead-ends without re-instrumenting.
// Valid after a ParseForestExperimental that returned ok=false.
func (p *Parser) ForestDeclineInfo() (offset uint32, sym Symbol, reason string, states []StateID) {
	return p.forestDeclineByte, p.forestDeclineSym, p.forestDeclineReason, p.forestDeclineStates
}

func (p *Parser) recordForestDecline(reason string, tok Token, states []StateID) {
	p.forestDeclineByte = tok.StartByte
	p.forestDeclineSym = tok.Symbol
	p.forestDeclineReason = reason
	p.forestDeclineStates = append(p.forestDeclineStates[:0], states...)
	forestLastDeclineReason = reason
}

// builtinForestDefaults is the curated set of built-in languages that dispatch
// to the GSS-forest GLR fast path by default. Restricted to languages whose
// production GLR parse suffers the super-linear deep-stack-equivalence blowup
// AND that are verified byte-identical to production on their real corpus by
// TestForestCorpusParity (which compares full node BYTE RANGES, not just
// s-expressions — an s-expr-only gate hid systematic span bugs). Measured
// byte-range-clean production-vs-forest speedups on the real corpus: bash
// 803x, erlang 664x, cmake 166x, awk 202x, javascript 36x, css 5x, scss 3x,
// c_sharp 3x. GraphQL is clean against production here too, but stays out
// until the production tree is C-oracle-clean on the ring matrix. The forest
// has no error recovery, so tryForestFastPath falls back to production on any
// decline (failure / error / truncation); that fallback means a language can
// never regress the cases it declines, but does NOT catch a clean-but-different
// tree — so a language joins this list only once its byte-range gate is green.
//
// Verified NOT forest-amenable (2026-06-02 sweep — do NOT re-add as "divergent",
// the older note was stale): python is forest byte-CLEAN (diverged=0) but ~0.8x
// because it has no merge blowup for the forest to amortize the GSS overhead
// against; rust forest TRUNCATES (incomplete) and fails safe to production; dart
// declines every file. ruby is unverified. haskell is NOT forest-amenable: its
// production parse is so pathologically slow (the O(n^2) deep-merge blowup) that
// the forest-vs-production gate times out, and the forest-vs-C oracle gate
// (TestForestVsCOracleParity) shows the forest RELOCATES the blowup — its reduce
// DFS times out on every haskell corpus file.
//
// php is now forest byte-clean vs C — the zero-width recovery ";" missing-flag
// fix (commit e5cf641a) made its production tree C-oracle-clean and the forest
// matches it, so correctness no longer blocks it — but it stays OUT on PERF
// grounds: only ~1/3 of its real corpus dispatches, and the GOT_GLR_FOREST
// on/off A/B is a net-wall LOSS (forest ~1.40ms vs production ~1.21ms over the
// corpus) because the failed forest attempts on the ~2/3 fallback files cost
// more than the dispatched third saves. Re-promote only if the dispatch rate
// rises (e.g. the forest learns the constructs it currently parse_fails on).
//
// Go remains a strong forest canary but is intentionally held out of default
// dispatch. The forest path is correct on curated Go corpora, but the current
// benchmark contract is production-parser performance: default forest dispatch
// makes Go full parse and incremental hot paths pay raw-shape/forest/result
// selection cost that the ordinary path does not need. Keep Go exercised through
// ParseForestExperimental and explicit corpus canaries until the forest path is
// both parity-clean and perf-clean for default Go parsing (commit 6894fc9f;
// that decision stands).
//
// Non-built-in languages opt in per-Language via Language.WantsForest (see
// parserWantsForest) instead of joining this map — e.g. a grammargen consumer
// generating its own grammar (a Pawn grammar, say) sets WantsForest directly
// (or grammargen.Grammar.WantsForest, plumbed through assemble) without
// forking this file. That path bypasses the byte-range parity certification
// this curated set underwent; the decline->production fallback still prevents
// hard failures, but a clean-but-different tree is the consumer's
// responsibility.
var builtinForestDefaults = map[string]bool{
	"bash":       true,
	"erlang":     true,
	"cmake":      true,
	"css":        true,
	"scss":       true,
	"awk":        true,
	"javascript": true,
	"c_sharp":    true,

	// Promoted 2026-06-08 after a full-corpus byte-range gate (forest vs
	// production, lock-filtered real corpus): ZERO divergence on every
	// dispatched file (gitignore 33/44, nix 635/703, squirrel 18/18,
	// prisma 78/78; the rest decline safely to production), AND a net-wall
	// WIN on the corpus (byte-identical trees). All carry the glr_merge
	// deep-stack blowup and their blowup files dispatch cleanly; squirrel and
	// prisma are parity-clean vs C (production ~5.9x), a clean forest speedup.
	//
	// NOT make: it is byte-range clean (19/20, 0 divergence) but net-wall
	// NEUTRAL (1.0x) — its expensive blowup files are precisely the ones that
	// decline (no-shift-death) and fall back to slow production, so the forest
	// only dispatches make's already-cheap files. make promotes once forest
	// error recovery lands (Gate 2 in forest-solution-design.md, moved to
	// gotreesitter-specs (external)).
	"gitignore": true,
	"nix":       true,
	"squirrel":  true,
	"prisma":    true,

	// Phase 2 promotions 2026-06-08: forest+recovery, introduced=0 vs C, large
	// net-wall win (agda 0.95x/prod28x, org 1.70x/25x, ledger 2.27x/345x,
	// yuck 1.28x/14x, json5 1.81x/50x). See languageWantsForestRecover.
	"agda":       true,
	"org":        true,
	"ledger":     true,
	"yuck":       true,
	"json5":      true,
	"commonlisp": true,
	"vimdoc":     true,

	// Promoted 2026-06-08 via the forest-vs-C sweep (TestForestVsCSources):
	// the forest introduces ZERO C-divergences (every divergence is inherited
	// from production) and is a net-wall WIN — bibtex 109.8x, faust 34.5x,
	// arduino 1.3x. faust/arduino also FIX production: the forest matches C on
	// files where the culled production parser does not (faust 108/120,
	// arduino 10/19 production-mismatches that are the forest being C-correct).
	// Held: make (forest=C-clean but net-wall NEUTRAL 1.0x — no lift) and
	// commonlisp (net-wall unverified; rich corpus times out — revisit).
	"bibtex":    true,
	"faust":     true,
	"arduino":   true,
	"authzed":   true,
	"make":      true,
	"csv":       true,
	"fish":      true,
	"racket":    true,
	"tlaplus":   true,
	"beancount": true,

	// Promoted 2026-06-08 against the C ORACLE, not production. gitattributes
	// is parity-blocked (production diverges from C), but the forest matches
	// tree-sitter-C byte-for-byte on every dispatched real-corpus file (10/10
	// via TestForestVsCSources) and is 34.7x faster — so this is a correctness
	// lift (parity-blocked -> parity-clean) AND a speed lift. Production is the
	// wrong promotion baseline for parity-blocked glr-merge grammars.
	//
	// ron/yuck/dtd are ALSO forest=C-clean but held: ron is net-wall NEGATIVE
	// (0.7x on its 3-file corpus), yuck/dtd corpora are too thin (2 and 1
	// dispatched) for a confident promotion. Promote when their corpora grow.
	"gitattributes": true,
}

// parserWantsForest reports whether p's language dispatches to the GSS-forest
// GLR fast path: either the Language opted in directly (WantsForest, set by a
// grammargen consumer) or it is one of the curated built-ins in
// builtinForestDefaults.
func parserWantsForest(p *Parser) bool {
	return p != nil && p.language != nil && (p.language.WantsForest || builtinForestDefaults[p.language.Name])
}

// tryForestFastPath attempts a full parse via the GSS-forest path and returns a
// Tree on success, or nil to tell the caller to fall back to the production
// parser. It declines (nil) whenever the forest cannot produce a clean,
// complete tree — it has no error recovery, so any failure, error node, or
// truncation routes to production. Gated by glrForestEnabled (GOT_GLR_FOREST);
// off by default so the production path is unchanged until per-language corpus
// parity is verified and the gate is lifted.
func (p *Parser) tryForestFastPath(source []byte) *Tree {
	if !glrForestEnabled || !parserWantsForest(p) {
		return nil
	}
	if len(p.included) > 0 {
		progress := newParseProgressTelemetry(p, len(source), uint32(len(source)), time.Now())
		if progress.enabled {
			progress.emit(time.Now(), "forest_try_decline", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, fmt.Sprintf("reason=included_ranges count=%d", len(p.included)))
		}
		return nil
	}
	progress := newParseProgressTelemetry(p, len(source), uint32(len(source)), time.Now())
	if progress.enabled {
		progress.emit(time.Now(), "forest_try_begin", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
		progress.beginDetail(time.Now(), "forest_arena_acquire_begin", "forest_arena_acquire_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}
	arena := acquireNodeArena(arenaClassFull)
	allowIncremental := languageAllowsForestIncrementalPath(p.language.Name)
	if progress.enabled {
		progress.endDetail(time.Now(), "forest_arena_acquire_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
		progress.beginDetail(time.Now(), "forest_parse_call_begin", "forest_parse_call_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}
	root, ok := p.parseForest(arena, source, allowIncremental)
	if progress.enabled {
		progress.endDetail(time.Now(), "forest_parse_call_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, fmt.Sprintf("ok=%t root_present=%t decline_reason=%s", ok, root != nil, forestLastDeclineReason))
	}
	if !ok || root == nil {
		if progress.enabled {
			progress.emit(time.Now(), "forest_try_decline", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, fmt.Sprintf("reason=parse_forest_failed ok=%t root_present=%t decline_reason=%s", ok, root != nil, forestLastDeclineReason))
		}
		arena.Release()
		return nil
	}
	if forestRootMustDecline(root) {
		if progress.enabled {
			progress.emit(time.Now(), "forest_try_decline", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "reason=error_root")
		}
		arena.Release()
		return nil
	}
	if root.HasError() && !languageWantsForestRecover(p.language.Name) {
		if progress.enabled {
			progress.emit(time.Now(), "forest_try_decline", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "reason=root_has_error")
		}
		arena.Release()
		return nil // non-recover langs fall back to production on any error
	}
	// Guard against an early-EOF token source: the root must reach the last
	// non-whitespace byte. Trailing whitespace/newlines are extras and may sit
	// outside the root span, so they are excluded from the bound.
	end := len(source)
	for end > 0 {
		switch source[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
			continue
		}
		break
	}
	if root.EndByte() < uint32(end) {
		if progress.enabled {
			progress.emit(time.Now(), "forest_try_decline", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, fmt.Sprintf("reason=incomplete_root root_end=%d expected_non_trivia_end=%d", root.EndByte(), end))
		}
		arena.Release()
		return nil // did not consume the whole input; let production recover it
	}
	if progress.enabled {
		progress.beginDetail(time.Now(), "forest_finalize_begin", "forest_finalize_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, fmt.Sprintf("root_end=%d", root.EndByte()))
	}
	p.finalizeForestRoot(root, source)
	if progress.enabled {
		progress.endDetail(time.Now(), "forest_finalize_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, fmt.Sprintf("root_end=%d", root.EndByte()))
	}
	tree := newTreeWithArenas(root, source, p.language, arena, nil)
	tree.setParseRuntime(forestAcceptedRuntime(root, source))
	tree.forestFastPath = true
	if !allowIncremental {
		tree.incrementalReuseDisabled = true
	}
	if progress.enabled {
		progress.beginDetail(time.Now(), "forest_normalize_begin", "forest_normalize_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}
	p.normalizeReturnedTreeForParse(tree, source)
	if progress.enabled {
		progress.endDetail(time.Now(), "forest_normalize_end", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, fmt.Sprintf("root_end=%d", root.EndByte()))
		progress.emit(time.Now(), "forest_try_success", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, fmt.Sprintf("root_end=%d", root.EndByte()))
	}
	return tree
}

func forestRootMustDecline(root *Node) bool {
	return root != nil && root.IsError()
}

func (p *Parser) finalizeForestRoot(root *Node, source []byte) {
	p.finalizeResultRoot(root, source, nil, false, false)
	extendRootToAcceptedCleanTail(root, source, uint32(len(source)), nil)
}

func forestAcceptedRuntime(root *Node, source []byte) ParseRuntime {
	if root == nil {
		return ParseRuntime{StopReason: ParseStopNone}
	}
	sourceLen := uint32(len(source))
	return ParseRuntime{
		StopReason:       ParseStopAccepted,
		ForestFastPath:   true,
		SourceLen:        sourceLen,
		ExpectedEOFByte:  sourceLen,
		RootEndByte:      root.EndByte(),
		LastTokenEndByte: sourceLen,
		LastTokenSymbol:  0,
		LastTokenWasEOF:  true,
	}
}

// languageAllowsForestIncrementalPath reports forest-default languages whose
// forest-built trees are safe to feed into the normal incremental parser path.
// Some languages still report subtree reuse as unsupported there, but entering
// that path can be much faster than forcing a fresh forest full parse. Languages
// stay disabled until the edited real-corpus matrix proves the path is correct
// and faster than fresh-parse fallback.
//
// 2026-06-03: restricted to {erlang, javascript}. The edited-corpus matrix gate
// the comment above always required was finally written
// (TestForestIncrementalCorrectness) and it found that cmake, css and scss had
// been added here WITHOUT it — their forest-incremental reuse produces
// structurally-wrong, often truncated trees on valid edits (e.g. one scss edit
// yields a 413-byte s-expr vs the correct 377KB). erlang (49/66 valid edits) and
// javascript (13/66) are byte-for-byte incremental==fresh; the rest are demoted
// to fresh-forest-parse fallback on edits (correct) until the reuse bug is fixed.
// A 2026-06-04 Go probe failed the same gate (21/28 valid edits diverged).
// Do NOT re-add a language here without it passing TestForestIncrementalCorrectness.
func languageAllowsForestIncrementalPath(name string) bool {
	switch name {
	case "erlang", "javascript":
		return true
	default:
		return false
	}
}

// gssLink is one alternative predecessor in the forest DAG: the subtree consumed
// to reach this node, and the prior node it was consumed from. A coalesced node
// (one per (state, position)) carries one link per surviving parse that reached
// it — exactly tree-sitter C's StackNode.links[].
type gssLink struct {
	prev *gssForestNode
	// prevDirty is the predecessor dirty version this link last observed. A
	// link can be structurally identical while its predecessor gained a new
	// alternative; downstream reductions must re-run in that case.
	prevDirty int32
	subtree   stackEntry
	// score is the subtree's cumulative dynamic precedence (a reduce's
	// DynamicPrecedence plus its children's scores; 0 for a shifted leaf). The
	// forest defers ambiguity resolution to finalization: among alternatives at
	// one (state, position), the highest-score subtree wins, matching
	// tree-sitter's dynamic_precedence selection.
	score int
	// errorCost is the recovery cost of this specific path. The coalesced node
	// keeps the minimum for queue ordering, but final result selection needs the
	// link-local value so lower-error alternatives beat higher-precedence ones.
	errorCost int
}

func forestNodeDirty(node *gssForestNode) int32 {
	if node == nil {
		return 0
	}
	return node.dirty
}

func forestLinkNoExtraDepth(prev *gssForestNode, entry stackEntry) uint8 {
	if forestStackEntryIsExtra(entry) {
		return 0
	}
	if prev == nil {
		return 1
	}
	if prev.noExtraDepth == ^uint8(0) {
		return prev.noExtraDepth
	}
	return prev.noExtraDepth + 1
}

func forestRecordNoExtraDepth(node *gssForestNode, first bool, depth uint8) {
	if node == nil {
		return
	}
	if first || depth < node.noExtraDepth {
		node.noExtraDepth = depth
	}
}

func forestRecordMinLinkScore(node *gssForestNode, first bool, score int) {
	if node == nil {
		return
	}
	if first || score < node.minLinkScore {
		node.minLinkScore = score
	}
}

func forestRefreshMinLinkScore(node *gssForestNode) {
	if node == nil || len(node.links) == 0 {
		return
	}
	minScore := node.links[0].score
	for i := 1; i < len(node.links); i++ {
		if node.links[i].score < minScore {
			minScore = node.links[i].score
		}
	}
	node.minLinkScore = minScore
}

// gssForestNode is a coalesced graph-structured-stack node: all parses that
// reach (state, byteOffset) share this single node; their differing histories
// are the links. This replaces the singly-linked gssNode{entry, prev} chain in
// the forest path. Link scores carry dynamic-precedence tie-breaks for final
// selection; minLinkScore caches the weakest retained link for cap pruning.
type gssForestNode struct {
	state        StateID
	byteOffset   uint32
	links        []gssLink
	errorCost    int
	minLinkScore int
	// dirty advances whenever a link is appended OR a competing link is
	// replaced by a higher-precedence alternative. Because Nodes are built
	// eagerly at reduce time, a late replacement must re-trigger the reductions
	// that consumed this node so parents rebuild from the winning subtree; the
	// worklist reprocesses a node whenever its dirty count moved past what it
	// last processed. Replacements only happen on a strictly higher score, so
	// dirty advances finitely and the loop terminates.
	dirty          int32
	processedEpoch int32
	processedDirty int32
	noExtraDepth   uint8
	// dupBucket*/worst* memoize the two residents-vs-residents scans
	// forestCapReplacementIndex runs when a capped node's links have no
	// candidate-matching bucket: forestWorstDuplicateRawBucketLink (is any
	// existing link an exact raw-shape duplicate of another) and the
	// residents-only "current worst link" search. Neither depends on the
	// incoming candidate — both are pure functions of node.links' current
	// content — so they are valid until dirty next advances (see the dirty
	// field's doc comment: links only change when dirty advances). Without
	// this, a capped node that keeps losing to new candidates (never
	// replacing a link, so dirty never moves) reruns both O(cap) / O(cap^2)
	// scans on every single insertion attempt; on shapes with many
	// raw-distinct alternatives competing for one slot (e.g. C#
	// designer-style repeated-statement blocks), attempt volume can be much
	// larger than actual replacement volume.
	dupBucketValid bool
	dupBucketDirty int32
	dupBucketIdx   int8
	worstValid     bool
	worstDirty     int32
	worstIdx       int8
}

// cachedDuplicateBucketLink returns forestWorstDuplicateRawBucketLink's result
// for node's current link content, computing and caching it on first use (or
// after node.dirty has advanced since the last cached computation).
func (node *gssForestNode) cachedDuplicateBucketLink(p *Parser, arena *nodeArena) (int, bool) {
	if node == nil {
		return 0, false
	}
	if node.dupBucketValid && node.dupBucketDirty == node.dirty {
		if node.dupBucketIdx < 0 {
			return 0, false
		}
		return int(node.dupBucketIdx), true
	}
	idx, ok := forestWorstDuplicateRawBucketLink(p, arena, node)
	node.dupBucketDirty = node.dirty
	node.dupBucketValid = true
	if ok {
		node.dupBucketIdx = int8(idx)
	} else {
		node.dupBucketIdx = -1
	}
	return idx, ok
}

// cachedWorstResidentLink returns the index of the current worst link among
// node's residents (ignoring any incoming candidate), computing and caching
// it on first use (or after node.dirty has advanced since the last cached
// computation). Requires len(node.links) > 0.
func (node *gssForestNode) cachedWorstResidentLink(p *Parser, arena *nodeArena) int {
	if node.worstValid && node.worstDirty == node.dirty {
		return int(node.worstIdx)
	}
	worst := 0
	for i := 1; i < len(node.links); i++ {
		if forestResultLinkCompare(p, arena, node, &node.links[i], i, &node.links[worst], worst) < 0 {
			worst = i
		}
	}
	node.worstDirty = node.dirty
	node.worstValid = true
	node.worstIdx = int8(worst)
	return worst
}

// coalesceForest merges a parse reaching (state, byteOffset) with subtree `entry`
// from predecessor `prev` into the forest: if a node already exists for that
// (state, byteOffset) it gains a link (O(1), no deep-compare — the heart of the
// win); otherwise a new node is created. `index` maps (state, byteOffset) to the
// node so coalescing is a map lookup, not a stack scan.
//
// Stage 1 scaffold: builds the DAG. Correct trees require Stage 2 (reduce walks
// every link); until then this is exercised only under the flag + parity gate.
func coalesceForest(index *gssForestIndex, slab *gssForestNodeSlab, state StateID, byteOffset uint32, prev *gssForestNode, entry stackEntry, score, errorCost int, linkCapOpt ...int) *gssForestNode {
	linkCap := forestMaxLinksPerNode
	if len(linkCapOpt) > 0 {
		linkCap = linkCapOpt[0]
	}
	return coalesceForestWithRawAndAlternatives(nil, nil, index, slab, state, byteOffset, prev, entry, score, errorCost, linkCap, nil)
}

func coalesceForestWithRaw(p *Parser, arena *nodeArena, index *gssForestIndex, slab *gssForestNodeSlab, state StateID, byteOffset uint32, prev *gssForestNode, entry stackEntry, score, errorCost int) *gssForestNode {
	return coalesceForestWithRawAndAlternatives(p, arena, index, slab, state, byteOffset, prev, entry, score, errorCost, forestMaxLinksPerNode, nil)
}

type forestAlternativeIndex struct {
	nodes   map[*Node]*gssForestNode
	byStart map[uint32][]*Node
	slots   map[forestAlternativeSlotKey]forestAlternativeSlot
}

type forestAlternativeSlotKey struct {
	parent     *Node
	childIndex int
}

type forestAlternativeSlot struct {
	node *gssForestNode
	prev *gssForestNode
}

// forestAlternativeIndexCapacityForSource sizes the alternative-index maps'
// initial capacity from the source length instead of a fixed guess: a fixed
// 1024 undersizes larger, ambiguity-heavy inputs (e.g. C# designer-style
// blocks of hundreds of near-identical statements record one nodes/slots
// entry per direct-reduce child touched), forcing several map-growth/rehash
// cycles over the parse. One entry per ~8 source bytes is a rough, cheap
// upper-bound estimate (grammars with denser ambiguity still just cost a few
// extra rehashes, not a correctness issue either way — this is a capacity
// hint, not a limit).
func forestAlternativeIndexCapacityForSource(sourceLen int) int {
	const minCapacity = 1024
	const maxCapacity = 1 << 20
	capacity := sourceLen / 8
	if capacity < minCapacity {
		return minCapacity
	}
	if capacity > maxCapacity {
		return maxCapacity
	}
	return capacity
}

func newForestAlternativeIndex(capacity int) *forestAlternativeIndex {
	return &forestAlternativeIndex{
		nodes:   make(map[*Node]*gssForestNode, capacity),
		byStart: make(map[uint32][]*Node, max(16, capacity/4)),
		slots:   make(map[forestAlternativeSlotKey]forestAlternativeSlot, capacity),
	}
}

func forestRecordAlternative(alternatives *forestAlternativeIndex, entry stackEntry, node *gssForestNode) {
	if alternatives == nil || node == nil {
		return
	}
	if n := stackEntryNode(entry); n != nil {
		if _, exists := alternatives.nodes[n]; !exists {
			if alternatives.byStart == nil {
				alternatives.byStart = make(map[uint32][]*Node, 16)
			}
			alternatives.byStart[n.startByte] = append(alternatives.byStart[n.startByte], n)
		}
		alternatives.nodes[n] = node
	}
}

func forestAlternativeCandidatesStartingAt(alternatives *forestAlternativeIndex, startByte uint32) []*Node {
	if alternatives == nil {
		return nil
	}
	if len(alternatives.byStart) == 0 && len(alternatives.nodes) > 0 {
		alternatives.byStart = make(map[uint32][]*Node, max(16, len(alternatives.nodes)/4))
		for n := range alternatives.nodes {
			if n != nil {
				alternatives.byStart[n.startByte] = append(alternatives.byStart[n.startByte], n)
			}
		}
	}
	return alternatives.byStart[startByte]
}

func forestRecordParentChildAlternatives(alternatives *forestAlternativeIndex, parent *Node, children []*Node, rawEntries []stackEntry) {
	if alternatives == nil || parent == nil || len(children) == 0 || len(rawEntries) == 0 {
		return
	}
	for i, child := range children {
		if child == nil || forestDirectReduceChildIndex(child, rawEntries) < 0 {
			continue
		}
		forestNode := alternatives.nodes[child]
		if forestNode == nil {
			continue
		}
		prev, ok := forestUniquePrevForSubtreeNode(forestNode, child)
		if !ok {
			continue
		}
		alternatives.slots[forestAlternativeSlotKey{parent: parent, childIndex: i}] = forestAlternativeSlot{
			node: forestNode,
			prev: prev,
		}
	}
}

func forestDirectReduceChildIndex(child *Node, rawEntries []stackEntry) int {
	for i := range rawEntries {
		if stackEntryNode(rawEntries[i]) == child {
			return i
		}
	}
	return -1
}

func forestUniquePrevForSubtreeNode(node *gssForestNode, child *Node) (*gssForestNode, bool) {
	if node == nil || child == nil {
		return nil, false
	}
	var prev *gssForestNode
	found := false
	for i := range node.links {
		if stackEntryNode(node.links[i].subtree) != child {
			continue
		}
		if found && prev != node.links[i].prev {
			return nil, false
		}
		prev = node.links[i].prev
		found = true
	}
	return prev, found
}

func coalesceForestWithRawAndAlternatives(p *Parser, arena *nodeArena, index *gssForestIndex, slab *gssForestNodeSlab, state StateID, byteOffset uint32, prev *gssForestNode, entry stackEntry, score, errorCost int, linkCap int, alternatives *forestAlternativeIndex) *gssForestNode {
	if perfCountersEnabled {
		perfRecordForestCoalesceCall()
	}
	key := gssForestKey{state: state, byteOffset: byteOffset}
	node := index.lookup(key)
	if node == nil {
		node = slab.alloc(state, byteOffset, score, errorCost)
		index.set(key, node)
		if perfCountersEnabled {
			perfRecordForestCoalesceNewNode()
		}
	} else if errorCost < node.errorCost {
		node.errorCost = errorCost
	}
	// Dedup competing alternatives: a link from the same predecessor whose
	// subtree has the same symbol and span is the same reduction reached another
	// way — keep the higher dynamic precedence (tree-sitter's resolution) instead
	// of accumulating a duplicate. This bounds the forest (no exponential link
	// blowup on ambiguous grammars) AND performs Stage-3 disambiguation, cheaply,
	// with no deep-equivalence walk. Only materialized subtrees carry a comparable
	// symbol+span, so the dedup applies to node entries only.
	if entry.kind == stackEntryKindNode && entry.node != nil {
		esym, estart, eend := entrySymSpan(entry)
		// A HIDDEN symbol (SymbolMetadata.Visible == false: generated repeat/seq
		// auxiliaries like `array_repeat1`/`_string_content`, and hidden
		// supertypes like `_value`) never itself appears in the final tree —
		// tree-sitter always splices its children into the enclosing visible
		// parent in place of the hidden node. Two candidates that share the
		// same predecessor AND the same (symbol, span) therefore cover the
		// exact same already-shifted token run and MUST flatten to the exact
		// same sequence of visible descendants, no matter how that run's
		// internal reduces happened to bracket it (e.g. the binary
		// `X_repeat1 -> X_repeat1 X_repeat1 | ...` grouping ambiguity, or a
		// supertype rebuilt over a not-yet-stable child). Their raw shapes can
		// still legitimately differ (different internal nesting), but that
		// difference can never surface, so skip the exact raw-shape gate for
		// them below. Without this, every internal regrouping mints a "new"
		// link at the SAME (prev, symbol, span) slot (since
		// forestRawStackEntriesExactEqual correctly reports them as
		// raw-different), burning the small per-node link cap on redundant
		// duplicates instead of the genuinely distinct (different start, i.e.
		// different coverage) alternatives a later reduce needs to complete
		// the parse — silently forcing an otherwise-valid parse into a dead
		// end once a repeat/string long enough to re-trigger the ambiguity a
		// few times fills the cap with copies of itself (json arrays of 3+
		// items whose last element is a multi-escape string, forest_test
		// dead_end at the closing `]`).
		// Only relax the gate when visibility is actually KNOWN (a real
		// Language with SymbolMetadata) and says hidden — symbolIsVisible
		// returns false for an unknown/nil language too, which must NOT be
		// read as "hidden": that would apply this relaxation to every symbol
		// whenever language metadata is unavailable (e.g. unit tests that
		// coalesce raw *Node fixtures directly against a bare *Parser{}),
		// collapsing genuinely raw-distinct alternatives the caller expected
		// to keep as separate links.
		//
		// Proven premise / what this does NOT cover: "shape never surfaces"
		// is exact for the case actually handled here — two links already tied
		// on (prev, symbol, span), where prev identity means both cover the
		// SAME already-shifted token run, so tree-sitter's hidden-node splice
		// recovers the identical flattened visible-descendant sequence
		// regardless of internal bracketing. It is NOT a general claim that
		// every hidden symbol is safe to collapse across different prev/score/
		// errorCost — only the exact-tie case here relies on it, and the
		// scoring paths above/below this branch (forestResultLinkCompare,
		// forestDedupTieReplace) are left untouched for every other case.
		// Verified empirically (2026-07, local corpus + cgo tree-sitter-C
		// oracle, TestForestCorpusParity/TestForestVsCOracleParity): the
		// languages/files this widens dispatch for (cmake, css, scss, c_sharp,
		// javascript, bash) that showed forest-vs-production divergence all
		// matched the C oracle byte-for-byte (0 errs) — the divergence was
		// production silently truncating, not this gate picking a wrong
		// alternative. The one genuine forest-vs-C divergence found in the
		// sweep (python `not in`/`is not` compound-token leaf span, off by the
		// leading space) reproduces identically with this whole hunk reverted
		// to the original HEAD (pre-existing, unrelated to hidden-symbol
		// dedup — see forest_gap_rejection_test.go / the cap-eviction tiebreak
		// in forestCapReplacementIndex below for the mechanism that made this
		// specific python file reachable enough to expose it).
		hiddenSym := p != nil && p.language != nil && !symbolIsVisible(p.language, esym)
		for i := range node.links {
			l := &node.links[i]
			if l.prev != prev || l.subtree.kind != stackEntryKindNode {
				continue
			}
			lsym, lstart, lend := entrySymSpan(l.subtree)
			if lsym != esym || lstart != estart || lend != eend {
				continue
			}
			rawEqual := true
			if !hiddenSym && p != nil && arena != nil {
				switch forestRawStackEntriesExactEqual(arena, entry, l.subtree) {
				case forestRawEqual:
					rawEqual = true
				case forestRawDifferent, forestRawUnknown:
					rawEqual = false
				}
			}
			if !rawEqual {
				continue
			}
			// Competing reduction reaching the same (prev, symbol, span): keep the
			// result-preferred alternative. A replacement marks the node dirty so
			// the reductions that already consumed the losing subtree re-run and
			// rebuild their parents from the winner.
			replaced := false
			candidate := gssLink{prev: prev, prevDirty: forestNodeDirty(prev), subtree: entry, score: score, errorCost: errorCost}
			switch {
			case hiddenSym:
				// forestResultLinkCompare and forestDedupTieReplace both fall
				// back to raw-shape/subtree-height comparisons to break exact
				// score+errorCost ties — a meaningful tiebreak for a VISIBLE
				// node's competing shapes, but for a hidden node any shape is
				// equally correct (it never surfaces), so that fallback just
				// picks arbitrarily between this arrival and the resident.
				// Every re-derivation of the same (prev, symbol, span) is a
				// FRESH Node (never raw-equal to the last one per the gate
				// above), so an arbitrary tiebreak swaps in place on every
				// single re-derivation, marking the node dirty and forcing
				// every consumer that already built from the resident to
				// rebuild — which can itself re-derive this same slot again,
				// live-locking the reduce worklist (C#'s wider/more ambiguous
				// GLR dispatch blew forestReduceVisitCap on a 16-class
				// benchmark once this path started firing for its hidden
				// declaration-list aux symbols). Comparing on score/errorCost
				// ALONE and treating an exact tie as a no-op keeps a settled
				// hidden link stable across repeated re-derivations instead of
				// thrashing; a real improvement (lower error cost, or higher
				// score at equal cost) still replaces it.
				if errorCost < l.errorCost || (errorCost == l.errorCost && score > l.score) {
					oldScore := l.score
					*l = candidate
					forestRecordAlternative(alternatives, entry, node)
					if oldScore == node.minLinkScore {
						forestRefreshMinLinkScore(node)
					}
					node.dirty++
					replaced = true
				}
			case forestResultLinkCompare(p, arena, node, &candidate, len(node.links), l, i) > 0:
				oldScore := l.score
				*l = candidate
				forestRecordAlternative(alternatives, entry, node)
				if oldScore == node.minLinkScore {
					forestRefreshMinLinkScore(node)
				}
				node.dirty++
				replaced = true
			case score == l.score && forestDedupTieReplace(entry, l.subtree):
				l.subtree = entry
				node.dirty++
				replaced = true
			}
			if prevDirty := forestNodeDirty(prev); l.prevDirty != prevDirty {
				l.prevDirty = prevDirty
				node.dirty++
			}
			if perfCountersEnabled {
				perfRecordForestCoalesceDedupHit(replaced)
			}
			return node
		}
	}
	// Bound the link fan-out per node (tree-sitter caps active versions). Without
	// a cap, a repeated/ambiguous structure accumulates O(n) links on one node and
	// reduceOverForest enumerates O(n^childCount) paths. Keep structural diversity
	// first: a lower-ranked raw-shape-distinct branch can be the only branch that
	// lets an enclosing reduction match C. Pure result rank is still the fallback
	// once the capped set has no duplicate raw-shape bucket to evict.
	linkNoExtraDepth := forestLinkNoExtraDepth(prev, entry)
	if len(node.links) >= linkCap {
		candidate := gssLink{prev: prev, prevDirty: forestNodeDirty(prev), subtree: entry, score: score, errorCost: errorCost}
		if replace, ok := forestCapReplacementIndex(p, arena, node, &candidate, len(node.links)); ok {
			node.links[replace] = candidate
			forestRecordAlternative(alternatives, entry, node)
			forestRefreshMinLinkScore(node)
			forestRecordNoExtraDepth(node, false, linkNoExtraDepth)
			node.dirty++
			if perfCountersEnabled {
				perfRecordForestCoalesceCap(true)
			}
		} else {
			if perfCountersEnabled {
				perfRecordForestCoalesceCap(false)
			}
		}
		return node
	}
	firstLink := len(node.links) == 0
	node.links = append(node.links, gssLink{prev: prev, prevDirty: forestNodeDirty(prev), subtree: entry, score: score, errorCost: errorCost})
	forestRecordAlternative(alternatives, entry, node)
	forestRecordMinLinkScore(node, firstLink, score)
	forestRecordNoExtraDepth(node, firstLink, linkNoExtraDepth)
	if perfCountersEnabled {
		perfRecordForestCoalesceLinkAppend()
	}
	node.dirty++
	return node
}

func forestCapReplacementIndex(p *Parser, arena *nodeArena, node *gssForestNode, candidate *gssLink, candidateOrder int) (int, bool) {
	if node == nil || candidate == nil || len(node.links) == 0 {
		return 0, false
	}
	// A hidden symbol's shape never surfaces in the final tree (see the
	// same-span dedup branch above), so when a candidate ties a resident on
	// BOTH errorCost and score, there is no principled winner between them —
	// falling through to forestResultLinkCompare's raw-shape/height/order
	// tiebreak just picks arbitrarily, and since forestCapReplacementIndex is
	// consulted on every subsequent re-derivation of the same capped slot,
	// "arbitrary" becomes "alternates forever": each swap dirties node,
	// forcing every consumer that already built from the loser to re-derive,
	// which can regenerate an equally-tied candidate and swap back. Observed
	// hitting forestReduceVisitCap on python's module_repeat1 (two DIFFERENT
	// spans tied at score=0/errorCost=0 fighting for the same capped slot for
	// 500+ reduce events, 47 tokens in). Treating the tie as a stable
	// no-op (keep the incumbent) removes the oscillation the same way the
	// dedup branch's tie-as-no-op does; a real difference in errorCost or
	// score still replaces normally.
	//
	// Weaker premise than the dedup branch above, by design: this function is
	// also consulted for candidates with a DIFFERENT (start,end) span than the
	// resident (forestWorstSameRawBucketLink/forestWorstDuplicateRawBucketLink
	// and the "worst" search below both scan every existing link regardless of
	// span), so "same already-shifted token run" is not guaranteed here the
	// way prev-identity guarantees it above — two different partitions of a
	// repeat, tied in score, are not provably interchangeable in the general
	// case, only empirically so on every corpus file this was checked against
	// (see the dedup branch's comment for the validation run: corpus parity +
	// cgo tree-sitter-C oracle across cmake/css/scss/c_sharp/bash/awk/erlang/
	// javascript/python, plus TestForestGapRejection 3/3 under both
	// GOT_C_RECOVERY modes). If a future corpus expansion surfaces a
	// genuine divergence traceable to this specific tie-no-op (unlike the
	// python compound-keyword-span one already ruled out as pre-existing and
	// unrelated), narrow it to the exact bucket the caller already found via
	// forestWorstSameRawBucketLink (same raw shape) rather than the plain
	// same-symbol/tied-score condition used here.
	hiddenSym := false
	if p != nil && p.language != nil && candidate.subtree.kind == stackEntryKindNode && candidate.subtree.node != nil {
		esym, _, _ := entrySymSpan(candidate.subtree)
		hiddenSym = !symbolIsVisible(p.language, esym)
	}
	if p != nil && arena != nil {
		if same, idx := forestWorstSameRawBucketLink(p, arena, node, candidate); same {
			if hiddenSym && candidate.errorCost == node.links[idx].errorCost && candidate.score == node.links[idx].score {
				return idx, false
			}
			return idx, forestResultLinkCompare(p, arena, node, candidate, candidateOrder, &node.links[idx], idx) > 0
		}
		// forestWorstDuplicateRawBucketLink depends only on node.links' own
		// content (no candidate parameter) — cache it per gssForestNode.dirty
		// so repeated losing candidates at an already-saturated node don't
		// re-scan all O(cap^2) resident pairs each time (see cachedDuplicate
		// BucketLink's doc comment).
		if idx, ok := node.cachedDuplicateBucketLink(p, arena); ok {
			return idx, true
		}
	}
	// The residents-only "current worst link" search is likewise independent
	// of candidate — cache it the same way (see cachedWorstResidentLink).
	worst := node.cachedWorstResidentLink(p, arena)
	if hiddenSym && candidate.errorCost == node.links[worst].errorCost && candidate.score == node.links[worst].score {
		return worst, false
	}
	return worst, forestResultLinkCompare(p, arena, node, candidate, candidateOrder, &node.links[worst], worst) > 0
}

func forestWorstSameRawBucketLink(p *Parser, arena *nodeArena, node *gssForestNode, candidate *gssLink) (bool, int) {
	found := false
	worst := -1
	for i := range node.links {
		if forestRawStackEntriesExactEqual(arena, candidate.subtree, node.links[i].subtree) != forestRawEqual {
			continue
		}
		if !found || forestResultLinkCompare(p, arena, node, &node.links[i], i, &node.links[worst], worst) < 0 {
			found = true
			worst = i
		}
	}
	return found, worst
}

func forestWorstDuplicateRawBucketLink(p *Parser, arena *nodeArena, node *gssForestNode) (int, bool) {
	worst := -1
	for i := range node.links {
		if !forestRawBucketHasPeer(arena, node, i) {
			continue
		}
		if worst < 0 || forestResultLinkCompare(p, arena, node, &node.links[i], i, &node.links[worst], worst) < 0 {
			worst = i
		}
	}
	if worst < 0 {
		return 0, false
	}
	return worst, true
}

func forestRawBucketHasPeer(arena *nodeArena, node *gssForestNode, idx int) bool {
	for i := range node.links {
		if i == idx {
			continue
		}
		if forestRawStackEntriesExactEqual(arena, node.links[idx].subtree, node.links[i].subtree) == forestRawEqual {
			return true
		}
	}
	return false
}

type forestRawEquality uint8

const (
	forestRawUnknown forestRawEquality = iota
	forestRawDifferent
	forestRawEqual
)

func forestRawStackEntriesExactEqual(arena *nodeArena, a, b stackEntry) forestRawEquality {
	return forestRawStackEntriesExactEqualRec(arena, a, b, 0)
}

func forestRawStackEntriesExactEqualRec(arena *nodeArena, a, b stackEntry, depth int) forestRawEquality {
	if arena == nil || depth > maxTreeWalkDepth {
		return forestRawUnknown
	}
	if stackEntryHasNode(a) != stackEntryHasNode(b) {
		return forestRawDifferent
	}
	if !stackEntryHasNode(a) {
		return forestRawEqual
	}
	aShape, aHasShape := rawShapeForStackEntry(arena, a)
	bShape, bHasShape := rawShapeForStackEntry(arena, b)
	if aHasShape != bHasShape {
		return forestRawUnknown
	}
	if aHasShape {
		return forestRawShapesExactEqualRec(arena, aShape, bShape, depth+1)
	}
	if stackEntryNodeSymbol(a) != stackEntryNodeSymbol(b) ||
		stackEntryNodeStartByte(a) != stackEntryNodeStartByte(b) ||
		stackEntryNodeEndByte(a) != stackEntryNodeEndByte(b) {
		return forestRawDifferent
	}
	if stackEntryNodeChildCount(a) != 0 || stackEntryNodeChildCount(b) != 0 {
		return forestRawUnknown
	}
	return forestRawEqual
}

func forestRawShapesExactEqualRec(arena *nodeArena, a, b *rawShape, depth int) forestRawEquality {
	if arena == nil || a == nil || b == nil || depth > maxTreeWalkDepth {
		return forestRawUnknown
	}
	if a.symbol != b.symbol || a.productionID != b.productionID || a.childCount != b.childCount {
		return forestRawDifferent
	}
	if a.contentHash != b.contentHash {
		// contentHash folds in strictly more than this function inspects
		// (symbol, productionID, childCount, and every descendant's symbol,
		// span and contentHash — see rawShapeComputeContentHash), so a
		// mismatch proves the subtrees differ somewhere without descending.
		// This is the fast path on shapes with many raw-distinct alternatives
		// (the common case here): only a genuine hash MATCH falls through to
		// the walk below, which still runs unchanged as the exact,
		// collision-proof source of truth.
		return forestRawDifferent
	}
	aChildren := arena.rawShapeChildren(a)
	bChildren := arena.rawShapeChildren(b)
	if len(aChildren) != int(a.childCount) || len(bChildren) != int(b.childCount) || len(aChildren) != len(bChildren) {
		return forestRawUnknown
	}
	for i := range aChildren {
		ae, be := aChildren[i].entry, bChildren[i].entry
		if stackEntryHasNode(ae) != stackEntryHasNode(be) {
			return forestRawDifferent
		}
		if !stackEntryHasNode(ae) {
			continue
		}
		if stackEntryNodeSymbol(ae) != stackEntryNodeSymbol(be) ||
			stackEntryNodeStartByte(ae) != stackEntryNodeStartByte(be) ||
			stackEntryNodeEndByte(ae) != stackEntryNodeEndByte(be) {
			return forestRawDifferent
		}
		aRef, bRef := aChildren[i].shapeRef, bChildren[i].shapeRef
		if aRef == 0 || bRef == 0 {
			if aRef != bRef {
				return forestRawUnknown
			}
			if stackEntryNodeChildCount(ae) != 0 || stackEntryNodeChildCount(be) != 0 {
				return forestRawUnknown
			}
			continue
		}
		aChild, aOK := arena.rawShapeForRef(aRef)
		bChild, bOK := arena.rawShapeForRef(bRef)
		if !aOK || !bOK {
			return forestRawUnknown
		}
		if eq := forestRawShapesExactEqualRec(arena, aChild, bChild, depth+1); eq != forestRawEqual {
			return eq
		}
	}
	return forestRawEqual
}

const forestGotoCacheSize = 8

type forestGotoCache struct {
	states  [forestGotoCacheSize]StateID
	targets [forestGotoCacheSize]StateID
	used    uint8
}

func (c *forestGotoCache) lookup(p *Parser, state StateID, sym Symbol) StateID {
	for i := 0; i < int(c.used); i++ {
		if c.states[i] == state {
			return c.targets[i]
		}
	}
	target := p.lookupGoto(state, sym)
	if c.used < forestGotoCacheSize {
		c.states[c.used] = state
		c.targets[c.used] = target
		c.used++
	}
	return target
}

func forestCoalesceWouldDropForCap(index *gssForestIndex, state StateID, byteOffset uint32, score, errorCost int, linkCap int) bool {
	// This guard runs before the parent node and raw shape are materialized. The
	// cap policy must preserve raw-distinct lower-score branches, so a pre-shape
	// decision cannot prove that a candidate is safe to drop. Always defer to
	// forestCapReplacementIndex after raw shape capture.
	_, _, _, _, _, _ = index, state, byteOffset, score, errorCost, linkCap
	return false
}

// forestMaxLinksPerNode caps the alternative fan-out coalesced at one
// (state, byteOffset) node, bounding reduceOverForest's path enumeration.
const forestMaxLinksPerNode = 8

func forestLinkCapForLanguage(name string) int {
	if name == "cmake" {
		return 2
	}
	return forestMaxLinksPerNode
}

// entrySymSpan returns a materialized node entry's symbol and byte span for cheap
// alternative-deduplication (no deep structural comparison).
func entrySymSpan(e stackEntry) (Symbol, uint32, uint32) {
	n := (*Node)(e.node)
	return n.symbol, n.startByte, n.endByte
}

// collectForestRootAndExtras walks the winning accepted path down from the
// accept node to locate the start-symbol root and gather the root-level extras
// that surround it: extras stacked above it are trailing, extras below it are
// leading. Each group is returned in source order; foldResultRootExtras splits
// them back into leading/trailing by position.
func collectForestRootAndExtras(p *Parser, arena *nodeArena, accepted *gssForestNode, alternatives *forestAlternativeIndex) (*Node, []*Node) {
	if accepted == nil {
		return nil, nil
	}
	var above []*Node // trailing extras, collected latest-first
	var root *Node
	below := (*gssForestNode)(nil)
	for cur := accepted; cur != nil; {
		link := cur.bestAcceptedRootResultLink(p, arena)
		if link == nil {
			return nil, nil
		}
		n := (*Node)(link.subtree.node)
		if n.isExtra() {
			above = append(above, n)
			cur = link.prev
			continue
		}
		root, below = n, link.prev
		break
	}
	if root == nil {
		return nil, nil
	}
	resolveForestChildAlternatives(p, arena, root, alternatives, nil, 0)
	forestPreserveRootVisibleContainerAlternatives(p, arena, root, alternatives)
	var belowExtras []*Node // leading extras, collected latest-first
	for cur := below; cur != nil; {
		link := cur.bestResultLink(p, arena)
		if link == nil {
			break
		}
		n := (*Node)(link.subtree.node)
		if !n.isExtra() {
			break
		}
		belowExtras = append(belowExtras, n)
		cur = link.prev
	}
	if len(above) == 0 && len(belowExtras) == 0 {
		return root, nil
	}
	// Reverse each group into source order, then concatenate (leading first).
	extras := make([]*Node, 0, len(belowExtras)+len(above))
	for i := len(belowExtras) - 1; i >= 0; i-- {
		extras = append(extras, belowExtras[i])
	}
	for i := len(above) - 1; i >= 0; i-- {
		extras = append(extras, above[i])
	}
	return root, extras
}

// forestRootChildrenCoverNonTrivia reports whether the root's direct children
// cover every NON-TRIVIA byte of the root span — i.e. no top-level item was
// dropped or mis-attached into a hole in the middle of the child list. It reads
// the no-materialize child view (stack entries) so the check never forces lazy
// subtrees into existence. bytesAreTrivia is whitespace-only, matching the
// end-coverage check at the accept site: comments are folded in as real
// children, so a correct tree's inter-child gaps are whitespace only. A
// non-trivia gap means the forest took a wrong GLR path at scale and must
// decline rather than dispatch a structurally-incomplete tree.
func forestRootChildrenCoverNonTrivia(root *Node, source []byte) bool {
	if root == nil {
		return true
	}
	prev := root.startByte
	n := nodeChildCountNoMaterialize(root)
	for i := 0; i < n; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(root, i)
		if !ok {
			continue
		}
		start := stackEntryNodeStartByte(entry)
		end := stackEntryNodeEndByte(entry)
		if start > prev && int(start) <= len(source) && !bytesAreTrivia(source[prev:start]) {
			return false
		}
		if end > prev {
			prev = end
		}
	}
	return true
}

func forestNodeBestLinearStack(p *Parser, arena *nodeArena, node *gssForestNode) (glrStack, bool) {
	if node == nil {
		return glrStack{}, false
	}
	reversed := make([]stackEntry, 0, 8)
	score := 0
	for cur := node; cur != nil; {
		link := cur.bestResultLink(p, arena)
		if link == nil {
			break
		}
		reversed = append(reversed, link.subtree)
		score += link.score
		cur = link.prev
	}
	if len(reversed) == 0 {
		return glrStack{}, false
	}
	entries := make([]stackEntry, len(reversed))
	for i := range reversed {
		entries[i] = reversed[len(reversed)-1-i]
	}
	return glrStack{
		entries:    entries,
		score:      score,
		byteOffset: node.byteOffset,
	}, true
}

// forestEOFRecoveryCouldCompete decides whether the forest's accepted path at
// EOF has a competing recovery interpretation nearby that production's cost
// competition might prefer instead. relaxForCleanAcceptLanguages should be the
// SAME (recoverActive || errorCostCompetitionEnabled()) test the caller
// already gates this call on, narrowed to exclude recoverActive: it is true
// only when this parse's dispatch reason is the errorCostCompetitionEnabled()
// half of that OR (bash and friends — languages where the forest only ever
// dispatches a CLEAN, error-free accept; tryForestFastPath rejects any
// root.HasError() result outright for non-recover languages). The
// unvalidated-marker bypass below is verified (TestForestDispatchReportsAcceptedRuntime,
// TestExternalScannerTokenInvariantLeafReuse/bash_number, byte-range parity
// sweeps) ONLY for that clean-accept case. For recoverActive languages
// (languageWantsForestRecover: authzed/make/csv/fish/racket/tlaplus/
// beancount/agda/org/ledger/yuck/json5/commonlisp/vimdoc) the forest's OWN
// accepted path can itself already be an error-bearing recovery, and the
// question this probe answers shifts from "does any recovery beat a clean
// accept" (which the marker's swallowed-error premise covers) to "does a
// DIFFERENT recovery beat OUR recovery" — a genuinely different comparison
// the marker was never validated against, and local corpus coverage for
// those languages (only `make`, 1 file) is too thin to validate empirically.
// So for those languages this keeps the original, unconditionally-conservative
// behavior (any structurally-reachable pop-back declines the forest).
func (p *Parser) forestEOFRecoveryCouldCompete(idx *gssForestIndex, arena *nodeArena, eofByte uint32, relaxForCleanAcceptLanguages bool) bool {
	if p == nil || idx == nil || idx.len() == 0 {
		return false
	}
	var gssScratch gssScratch
	var entryScratch glrEntryScratch
	trackChildErrors := false
	// cRecoverToState (parser_recover_c.go) tags a freshly-recovered fork with
	// cRecoveryUnvalidatedMarker exactly when (a) p.crecoveryHandleErrorSingleStack
	// is set and (b) the popped/recovered span is small (<=
	// crecoverySwallowedErrorMaxFallbackErrorBytes) — production's own signal
	// for "this pop-back is the confirmed swallowed-error pattern: reachable
	// by construction, but never actually beats a clean accept" (see
	// buildResultFromGLR/hasCleanSiblingAtSamePosition in parser_result.go:
	// the marker only ever feeds a diagnostic counter, never the actual
	// stack-selection result — cStackErrorCost-based comparison still always
	// prefers the clean, error-free accept). p.crecoveryHandleErrorSingleStack
	// is only ever set from production's real cHandleError dispatch
	// (len(*stacks)==1), which this forest-only probe never runs, so it
	// defaults to false and the marker never fires here — every reachable
	// pop-back reads as a genuine competitor, declining the forest fast path
	// for ordinary, syntactically valid input purely because SOME
	// structurally-reachable recovery exists (true of most non-trivial
	// parses; a fresh GLR table almost always has a nearby recover-capable
	// state). The forest has no independent []glrStack of its own — every
	// candidate it checks here is, by construction, "the whole probe" in the
	// same sense production's single-stack case is — so prime the flag for
	// the duration of this probe (restored after, so it never leaks into the
	// real production dispatch this same *Parser runs on ParseForestExperimental's
	// fallback) and trust the span-size half of the gate to keep this narrow:
	// a small recovered span behaves exactly like the confirmed swallowed-error
	// class production already treats as non-competing, while a large one
	// still returns true below and declines as before. Only do this priming
	// for the validated clean-accept case; recoverActive languages keep
	// p.crecoveryHandleErrorSingleStack (and hence the marker) untouched, so
	// it stays false and every reachable pop-back still declines as before.
	if relaxForCleanAcceptLanguages {
		prevSingleStack := p.crecoveryHandleErrorSingleStack
		p.crecoveryHandleErrorSingleStack = true
		defer func() { p.crecoveryHandleErrorSingleStack = prevSingleStack }()
	}
	for i := range idx.entries {
		node := idx.entries[i].node
		if node == nil || node.byteOffset != eofByte {
			continue
		}
		stack, ok := forestNodeBestLinearStack(p, arena, node)
		if !ok {
			continue
		}
		entries := cStackEntriesTopFirst(&stack, &gssScratch)
		summary := p.cRecordSummary(entries)
		for _, entry := range summary {
			if entry.state == cErrorState || entry.posBytes == stack.byteOffset {
				continue
			}
			if p.lookupActionIndex(entry.state, 0) == 0 {
				continue
			}
			fork, ok := p.cRecoverToState(&stack, entry.depth, entry.state, arena, &entryScratch, &gssScratch, &trackChildErrors)
			if !ok {
				continue
			}
			if relaxForCleanAcceptLanguages && fork.cRecoveryUnvalidatedMarker {
				continue
			}
			return true
		}
	}
	return false
}

// forestAcceptedIsCleanCompleteParse reports whether the forest's chosen accept
// candidate is a genuinely CLEAN, complete parse of the whole input — the case
// for which the EOF-recovery-competition probe (forestEOFRecoveryCouldCompete)
// is provably unnecessary. It verifies three cheap, independent conditions:
//
//  1. Zero recovery cost along the accept path (accepted.errorCost == 0). Forest
//     errorCost is a PATH cost: a reduce carries its predecessor's errorCost
//     (coalesce ... popTo.errorCost) and only a recovery step ever raises it (by
//     at least a skipped-token width; ERROR_COST_PER_RECOVERY at the stack
//     level). Zero therefore means no recovery happened anywhere on the path —
//     no skipped input, no error region, no missing insertion.
//  2. No ERROR/MISSING nodes: the materialized start-symbol root (built eagerly
//     at reduce time; reached here by the same walk collectForestRootAndExtras
//     uses, skipping trailing extras) has its has-error flag clear.
//     populateParentNode ORs every child's flag up and every ERROR/MISSING leaf
//     sets it, so a clear root flag proves the whole subtree is error-free.
//  3. Full input span: the accept (chosen for MAX coverage, with trailing extras
//     coalesced above it) reaches the last non-whitespace byte. Trailing
//     whitespace/newlines are inter-token trivia outside any node span, so they
//     are excluded from the bound — the same end rule tryForestFastPath applies.
//
// For such a tree the C finished_tree comparison is decisive and needs no probe:
// ts_parser selects the finished version with the LOWEST error_cost, and every
// recovery costs at least ERROR_COST_PER_RECOVERY (cErrCostPerRecovery = 500),
// so a cost-0 accept is unbeatable — the probe's question ("could some EOF
// recovery have competed?") is definitionally "no". Callers gate the probe on
// !this so a clean accept dispatches straight through instead of declining.
func (p *Parser) forestAcceptedIsCleanCompleteParse(arena *nodeArena, accepted *gssForestNode, source []byte) bool {
	// (1) zero recovery cost — the C-model master signal.
	if accepted == nil || accepted.errorCost != 0 {
		return false
	}
	// (3) full input span up to the last non-whitespace byte.
	end := len(source)
	for end > 0 {
		switch source[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
			continue
		}
		break
	}
	if accepted.byteOffset < uint32(end) {
		return false
	}
	// (2) no ERROR/MISSING nodes: walk to the start-symbol root (skipping any
	// trailing extras stacked above the accept, as collectForestRootAndExtras
	// does) and require its propagated has-error flag to be clear.
	var root *Node
	for cur := accepted; cur != nil; {
		link := cur.bestAcceptedRootResultLink(p, arena)
		if link == nil {
			return false
		}
		n := (*Node)(link.subtree.node)
		if n.isExtra() {
			cur = link.prev
			continue
		}
		root = n
		break
	}
	if root == nil || root.hasError() {
		return false
	}
	return true
}

func forestPreserveRootVisibleContainerAlternatives(p *Parser, arena *nodeArena, root *Node, alternatives *forestAlternativeIndex) bool {
	if p == nil || p.language == nil || root == nil || alternatives == nil || resultChildCount(root) == 0 {
		return false
	}
	childCount := resultChildCount(root)
	out := make([]*Node, 0, childCount)
	changed := false
	for i := 0; i < childCount; {
		if candidate, end, ok := forestRootVisibleContainerAlternativeForSlice(p, arena, root, alternatives, i); ok {
			out = append(out, candidate)
			i = end
			changed = true
			continue
		}
		out = append(out, resultChildAt(root, i))
		i++
	}
	if !changed {
		return false
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	replaceNodeChildrenUnfielded(root, out)
	return true
}

func forestRootVisibleContainerAlternativeForSlice(p *Parser, arena *nodeArena, root *Node, alternatives *forestAlternativeIndex, start int) (*Node, int, bool) {
	first := resultChildAt(root, start)
	if first == nil {
		return nil, 0, false
	}
	childCount := resultChildCount(root)
	var best *Node
	bestEnd := 0
	for _, candidate := range forestAlternativeCandidatesStartingAt(alternatives, first.startByte) {
		if !forestVisibleNamedStructuralContainer(p, candidate) || candidate.isExtra() || candidate.isMissing() {
			continue
		}
		for end := childCount; end > start; end-- {
			last := resultChildAt(root, end-1)
			if last == nil || last.endByte != candidate.endByte {
				continue
			}
			if !forestRootSliceMatchesVisibleContainer(p, arena, root, start, end, candidate) {
				continue
			}
			if best == nil || end > bestEnd || (end == bestEnd && resultChildCount(candidate) > resultChildCount(best)) {
				best = candidate
				bestEnd = end
			}
			break
		}
	}
	if best == nil {
		return nil, 0, false
	}
	return best, bestEnd, true
}

func forestRootSliceMatchesVisibleContainer(p *Parser, arena *nodeArena, root *Node, start, end int, candidate *Node) bool {
	if start < 0 || end <= start || candidate == nil {
		return false
	}
	sawFlattenable := false
	flattened := make([]*Node, 0, end-start)
	for i := start; i < end; i++ {
		child := resultChildAt(root, i)
		if child == nil {
			return false
		}
		if child == candidate {
			return false
		}
		if shouldFlattenInvisibleRootChild(child, p.language.SymbolMetadata) {
			sawFlattenable = true
		}
		flattened = appendFlattenedInvisibleRootChild(flattened, child, arena, p.language.SymbolMetadata)
	}
	if !sawFlattenable || len(flattened) != resultChildCount(candidate) {
		return false
	}
	for i := range flattened {
		if !forestNodesHaveSameTreeOrderEnvelope(flattened[i], resultChildAt(candidate, i)) {
			return false
		}
	}
	return true
}

func forestNodesHaveSameTreeOrderEnvelope(a, b *Node) bool {
	if a == nil || b == nil {
		return false
	}
	return a.symbol == b.symbol &&
		a.startByte == b.startByte &&
		a.endByte == b.endByte &&
		a.isExtra() == b.isExtra() &&
		a.isMissing() == b.isMissing() &&
		a.hasError() == b.hasError()
}

func forestAcceptedNodeCompare(p *Parser, arena *nodeArena, a *gssForestNode, aOrder int, b *gssForestNode, bOrder int) int {
	if a == nil || b == nil {
		if a != nil {
			return 1
		}
		if b != nil {
			return -1
		}
		return 0
	}
	aStack, aOK := forestAcceptedNodeResultStack(p, arena, a, aOrder)
	bStack, bOK := forestAcceptedNodeResultStack(p, arena, b, bOrder)
	if aOK && bOK {
		// Root collection intentionally ignores raw-shape ordering for the
		// accepted node's root link. Preserve that same finalization semantic when
		// choosing between accepted forest nodes; local child alternatives still
		// use raw-shape selection below the root.
		if cmp := stackCompareForResultSelectionWithRawShape(p, arena, &aStack, &bStack, false, false); cmp != 0 {
			return cmp
		}
	}
	if aOK != bOK {
		if aOK {
			return 1
		}
		return -1
	}
	if aOrder < bOrder {
		return 1
	}
	if aOrder > bOrder {
		return -1
	}
	return 0
}

func forestAcceptedNodeResultStack(p *Parser, arena *nodeArena, accepted *gssForestNode, order int) (glrStack, bool) {
	if accepted == nil {
		return glrStack{}, false
	}
	var reversed []stackEntry
	totalScore := 0
	cur := accepted
	foundRoot := false
	for cur != nil {
		link := cur.bestAcceptedRootResultLink(p, arena)
		if link == nil {
			return glrStack{}, false
		}
		reversed = append(reversed, link.subtree)
		totalScore += link.score
		if n := stackEntryNode(link.subtree); n != nil && !n.isExtra() {
			cur = link.prev
			foundRoot = true
			break
		}
		cur = link.prev
	}
	if !foundRoot {
		return glrStack{}, false
	}
	for cur != nil {
		link := cur.bestResultLink(p, arena)
		if link == nil {
			break
		}
		n := stackEntryNode(link.subtree)
		if n == nil || !n.isExtra() {
			break
		}
		reversed = append(reversed, link.subtree)
		totalScore += link.score
		cur = link.prev
	}
	entries := make([]stackEntry, len(reversed))
	for i := range reversed {
		entries[i] = reversed[len(reversed)-1-i]
	}
	return glrStack{
		accepted:    true,
		entries:     entries,
		score:       totalScore,
		byteOffset:  accepted.byteOffset,
		branchOrder: uint64(order),
	}, true
}

func resolveForestChildAlternatives(p *Parser, arena *nodeArena, parent *Node, alternatives *forestAlternativeIndex, seen map[*Node]struct{}, depth int) {
	if parent == nil || alternatives == nil || depth > maxTreeWalkDepth {
		return
	}
	if seen == nil {
		seen = make(map[*Node]struct{}, 16)
	}
	if _, ok := seen[parent]; ok {
		return
	}
	seen[parent] = struct{}{}
	defer delete(seen, parent)

	for i := range parent.children {
		child := parent.children[i]
		if child == nil {
			continue
		}
		chosen := child
		if slot, ok := alternatives.slots[forestAlternativeSlotKey{parent: parent, childIndex: i}]; ok {
			if best := slot.node.bestResultLinkForPrev(p, arena, slot.prev); best != nil {
				if bestNode := stackEntryNode(best.subtree); bestNode != nil && forestAlternativeFitsChildSlot(p, child, bestNode) {
					chosen = bestNode
				}
			}
		}
		for {
			direct := forestRecordedUnaryDirectChildAlternative(p, arena, alternatives, chosen, depth+1)
			if direct == nil {
				break
			}
			chosen = direct
		}
		resolveForestChildAlternatives(p, arena, chosen, alternatives, seen, depth+1)
		if chosen != child {
			parent.children[i] = chosen
			chosen.parent = parent
			chosen.childIndex = int32(i)
			replaceRawShapeChildEntry(arena, parent, child, chosen)
			nodeBumpEquivVersion(parent)
		}
	}
}

func forestRecordedUnaryDirectChildAlternative(p *Parser, arena *nodeArena, alternatives *forestAlternativeIndex, wrapper *Node, depth int) *Node {
	if p == nil || arena == nil || alternatives == nil || wrapper == nil || depth > maxTreeWalkDepth {
		return nil
	}
	if len(wrapper.children) != 1 || alternatives.nodes[wrapper] == nil {
		return nil
	}
	direct := wrapper.children[0]
	if direct == nil || len(direct.children) <= 1 || alternatives.nodes[direct] == nil {
		return nil
	}
	if !stackEntryUnaryWrapperContains(p, arena, newStackEntryNode(wrapper.parseState, wrapper), newStackEntryNode(direct.parseState, direct), depth+1) {
		return nil
	}
	return direct
}

func forestAlternativeFitsChildSlot(p *Parser, original, candidate *Node) bool {
	if original == nil || candidate == nil {
		return false
	}
	if original.startByte != candidate.startByte ||
		original.endByte != candidate.endByte ||
		original.isExtra() != candidate.isExtra() ||
		original.isMissing() != candidate.isMissing() {
		return false
	}
	if forestVisibleNamedStructuralContainer(p, original) && !forestVisibleNamedStructuralContainer(p, candidate) {
		return false
	}
	return true
}

func forestVisibleNamedStructuralContainer(p *Parser, node *Node) bool {
	if node == nil || resultChildCount(node) == 0 {
		return false
	}
	if p != nil && p.language != nil {
		if idx := int(node.symbol); idx >= 0 && idx < len(p.language.SymbolMetadata) {
			meta := p.language.SymbolMetadata[idx]
			return meta.Visible || meta.Named
		}
	}
	return node.isNamed()
}

func replaceRawShapeChildEntry(arena *nodeArena, parent, oldChild, newChild *Node) {
	if arena == nil || parent == nil || oldChild == nil || newChild == nil || parent.rawShape == 0 {
		return
	}
	shape, ok := arena.rawShapeForRef(parent.rawShape)
	if !ok {
		return
	}
	children := arena.rawShapeChildren(shape)
	for i := range children {
		if stackEntryNode(children[i].entry) != oldChild {
			continue
		}
		children[i].entry = newStackEntryNode(newChild.parseState, newChild)
		children[i].shapeRef = newChild.rawShape
	}
}

// collectForestErrorRoot builds a synthetic error root from the best partial
// parse in idx when EOF was reached without an accept (error recovery). It
// mirrors production's buildSyntheticRootTree: pick the surviving actor that
// consumed the most input at the lowest error cost, materialize its top-level
// fragment list (the result-link chain down to the start), and wrap it in the
// grammar's expected root symbol — retagged to errorSymbol when a fragment
// carries an error (production's synthetic-root rule). Recovery-only.
func (p *Parser) collectForestErrorRoot(idx *gssForestIndex, arena *nodeArena) *Node {
	if idx == nil || len(idx.entries) == 0 {
		return nil
	}
	var best *gssForestNode
	for i := range idx.entries {
		n := idx.entries[i].node
		if n == nil {
			continue
		}
		if best == nil || forestErrorRootBetter(p, arena, n, best) {
			best = n
		}
	}
	if best == nil {
		return nil
	}
	// Walk the result-preferred chain down to the start, collecting top-level
	// fragments latest-first, then reverse to source order.
	var frags []*Node
	for cur := best; cur != nil; {
		link := cur.bestResultLink(p, arena)
		if link == nil {
			break
		}
		frags = append(frags, (*Node)(link.subtree.node))
		cur = link.prev
	}
	if len(frags) == 0 {
		return nil
	}
	for i, j := 0, len(frags)-1; i < j; i, j = i+1, j-1 {
		frags[i], frags[j] = frags[j], frags[i]
	}
	hasErr := false
	for _, f := range frags {
		if f != nil && (f.symbol == errorSymbol || f.HasError()) {
			hasErr = true
			break
		}
	}
	rootSym := p.rootSymbol
	if hasErr {
		// Forest EOF recovery does not run the result-root parser-table replay
		// policy yet. Keep this path fail-closed on errored fragments until the
		// forest materializer can share that framing check without changing
		// forest recovery selection.
		rootSym = errorSymbol
	}
	// Eager parent-link wiring is safe here (unlike the C-recovery ERROR wrappers
	// fixed via newRecoveryParentNodeInArena): the forest path cannot corrupt the
	// transient-parent sentinel because it never allocates transient parents.
	// parseForest swaps p.reduceScratch to a zero-valued forestReduceScratch for
	// the whole forest parse (glr_forest.go ~:2366-2368, transientParents left
	// nil), and newReduceParentNode only routes to transientParents.allocParent
	// (the slab node whose .parent doubles as the materializer sentinel) when
	// transientParents != nil (parser_reduce.go ~:6660). So every `frags` node
	// built during this forest parse is an ordinary arena/leaf node with a real
	// parent link — there is no {nil/self/other} sentinel to clobber. This
	// collectForestErrorRoot site is the only production caller and runs inside
	// that swap's dynamic extent (glr_forest.go ~:2927).
	root := newParentNodeInArena(arena, rootSym, true, frags, nil, 0)
	if hasErr {
		root.setHasError(true)
	}
	return root
}

// forestErrorRootBetter ranks partial-parse actors for the synthetic error root:
// consumed more input first, then lower error cost, then result-link ordering.
func forestErrorRootBetter(p *Parser, arena *nodeArena, a, b *gssForestNode) bool {
	if a.byteOffset != b.byteOffset {
		return a.byteOffset > b.byteOffset
	}
	if a.errorCost != b.errorCost {
		return a.errorCost < b.errorCost
	}
	la, lb := a.bestResultLink(p, arena), b.bestResultLink(p, arena)
	if la != nil || lb != nil {
		return forestResultLinkCompare(p, arena, a, la, 0, lb, 0) > 0
	}
	return false
}

// bestLink returns the link whose subtree wins tree-sitter's selection:
// highest score (dynamic precedence), then earliest (production order).
// forestCollapsibleNamedKeywordLeaf returns the collapsed LEAF for a unary reduce
// `NamedSym -> single anonymous keyword token` whose token name equals the rule
// name (a keyword-as-named-node: go `false`/`nil`/`true`/`iota`). tree-sitter C
// inlines these to named leaves (ChildCount 0); the production reduce collapses
// them too. Two gates make it forest-safe where the production predicate is not:
//
//   - sameSymbolName only (NOT the broader different-named-child keep path that
//     collapsibleRawUnarySelfReduction also takes): production gates that path on
//     child.parent != nil, but the forest connects nodes via gssLink and never
//     sets node.parent, so it would over-collapse rules C keeps as cc=1 (css
//     universal_selector `*`).
//   - KeywordCaptureToken != 0: only languages with word-token keyword extraction
//     inline a `Named -> 'kw'` rule; languages without it keep the token child
//     even when names match (css `to`/`from`), so the same-name test alone is not
//     enough.
//
// aliasedNodeInArena clones, so the shared child is never mutated. Returns nil
// when not applicable.
// forestGapCollapseSymbols lists, per forest language, the named single-token
// rules tree-sitter C collapses to a LEAF that the sameSymbolName test misses
// (the rule name != the token, so it is not a same-name keyword like false/nil:
// go `blank_identifier` -> '_'). C-ORACLE-SEEDED: the collapse_extract dev tool
// (parse each forest lang + go vs the C oracle, diff forest-keeps-vs-C-collapses
// on single anonymous children) found this is the ONLY such gap across all forest
// languages + go — the 8 allowlisted langs are gap-free. Re-run that tool when
// adding a language. This whitelist is the only safe way to collapse these: they
// are statically indistinguishable from single-token rules C KEEPS (awk
// `pattern`), which production tells apart via child.parent, a contextual signal
// the forest's link-based DAG lacks.
var forestGapCollapseSymbols = map[string]map[string]bool{
	"go": {"blank_identifier": true},
}

func forestGapCollapse(lang *Language, sym Symbol) bool {
	if lang == nil {
		return false
	}
	set := forestGapCollapseSymbols[lang.Name]
	if set == nil {
		return false
	}
	if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
		return false
	}
	return set[lang.SymbolNames[sym]]
}

func (p *Parser) forestCollapsibleNamedKeywordLeaf(act ParseAction, tok Token, arena *nodeArena, entries []stackEntry, start, reducedEnd int) *Node {
	if p == nil || arena == nil || tok.NoLookahead {
		return nil
	}
	if p.language == nil || p.language.KeywordCaptureToken == 0 {
		return nil
	}
	if reducedEnd-start != 1 || start < 0 || reducedEnd > len(entries) {
		return nil
	}
	if p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, arena) || len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		return nil
	}
	child := stackEntryNode(entries[start])
	if child == nil || child.ownerArena != arena || child.parent != nil {
		return nil
	}
	if child.symbol == act.Symbol || child.ChildCount() != 0 {
		return nil
	}
	if !p.canCollapseNamedLeafWrapper(act.Symbol, child.symbol) {
		return nil
	}
	if p.shouldPreserveVisibleUnaryTokenWrapper(act.Symbol) {
		return nil
	}
	if !p.sameSymbolName(act.Symbol, child.symbol) && !forestGapCollapse(p.language, act.Symbol) {
		return nil
	}
	return aliasedNodeInArena(arena, p.language, child, act.Symbol)
}

func (n *gssForestNode) bestLink() *gssLink {
	if n == nil || len(n.links) == 0 {
		return nil
	}
	best := &n.links[0]
	for i := 1; i < len(n.links); i++ {
		if n.links[i].score > best.score {
			best = &n.links[i]
		}
	}
	return best
}

func (n *gssForestNode) bestResultLink(p *Parser, arena *nodeArena) *gssLink {
	return n.bestResultLinkWithRawShape(p, arena, true)
}

func (n *gssForestNode) bestResultLinkForPrev(p *Parser, arena *nodeArena, prev *gssForestNode) *gssLink {
	if n == nil || len(n.links) == 0 {
		return nil
	}
	best := -1
	for i := range n.links {
		if n.links[i].prev != prev {
			continue
		}
		if best < 0 || forestResultLinkCompare(p, arena, n, &n.links[i], i, &n.links[best], best) > 0 {
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	return &n.links[best]
}

func (n *gssForestNode) bestAcceptedRootResultLink(p *Parser, arena *nodeArena) *gssLink {
	return n.bestResultLinkWithRawShape(p, arena, false)
}

func (n *gssForestNode) bestResultLinkWithRawShape(p *Parser, arena *nodeArena, useRawShape bool) *gssLink {
	if n == nil || len(n.links) == 0 {
		return nil
	}
	best := 0
	for i := 1; i < len(n.links); i++ {
		if forestResultLinkCompareWithRawShape(p, arena, n, &n.links[i], i, &n.links[best], best, useRawShape) > 0 {
			best = i
		}
	}
	return &n.links[best]
}

func forestResultLinkCompare(p *Parser, arena *nodeArena, node *gssForestNode, a *gssLink, aOrder int, b *gssLink, bOrder int) int {
	return forestResultLinkCompareWithRawShape(p, arena, node, a, aOrder, b, bOrder, true)
}

func forestResultLinkCompareWithRawShape(p *Parser, arena *nodeArena, node *gssForestNode, a *gssLink, aOrder int, b *gssLink, bOrder int, useRawShape bool) int {
	if a == nil || b == nil {
		if a != nil {
			return 1
		}
		if b != nil {
			return -1
		}
		return 0
	}
	if a.errorCost != b.errorCost {
		if a.errorCost < b.errorCost {
			return 1
		}
		return -1
	}
	// The forest link score is the cumulative dynamic precedence for this
	// path. It can be higher than the materialized subtree node's own dynamic
	// precedence when hidden/supertype reductions are flattened to their
	// visible child, so apply it before stack/raw-shape tie-breaks.
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	if p != nil && arena != nil {
		aStack := glrStack{
			entries:     []stackEntry{a.subtree},
			accepted:    true,
			score:       a.score,
			byteOffset:  node.byteOffset,
			branchOrder: uint64(aOrder),
		}
		bStack := glrStack{
			entries:     []stackEntry{b.subtree},
			accepted:    true,
			score:       b.score,
			byteOffset:  node.byteOffset,
			branchOrder: uint64(bOrder),
		}
		if cmp := stackCompareForResultSelectionWithRawShape(p, arena, &aStack, &bStack, false, useRawShape); cmp != 0 {
			return cmp
		}
	}
	if aOrder < bOrder {
		return 1
	}
	if aOrder > bOrder {
		return -1
	}
	return 0
}

type gssForestKey struct {
	state      StateID
	byteOffset uint32
}

// gssForestIndex maps (state, byteOffset) -> coalesced node for one parse step.
// Profiling showed it holds very few entries per step (p50=1, p90=5, p99=10
// across scss/js/go; rare max ~63), so a Go map was pure overhead: its hashing,
// per-insert mapassign, and per-key delete-on-reset dominated ~15-20% of a
// fork-heavy (scss) forest parse. A linear-scan slice wins at these sizes — no
// hashing, no allocation, O(1) truncate reset. Keys are unique by construction
// (coalesceForest only set()s after a lookup() miss; the per-step seed inserts
// the frontier, which carries unique (state,byteOffset) because the prior step's
// shift-coalesce deduplicated it), so set() appends blindly. lastKey caches the
// hottest repeated lookup (consecutive coalesces of the same actor).
type gssForestEntry struct {
	key  gssForestKey
	node *gssForestNode
}

type gssForestIndex struct {
	entries   []gssForestEntry
	lastKey   gssForestKey
	lastNode  *gssForestNode
	lastValid bool
}

func newGSSForestIndex(capacity int) gssForestIndex {
	return gssForestIndex{entries: make([]gssForestEntry, 0, capacity)}
}

func (idx *gssForestIndex) reset() {
	idx.entries = idx.entries[:0]
	idx.lastValid = false
	idx.lastNode = nil
}

func (idx *gssForestIndex) len() int {
	if idx == nil {
		return 0
	}
	return len(idx.entries)
}

func (idx *gssForestIndex) lookup(key gssForestKey) *gssForestNode {
	if idx.lastValid && idx.lastKey == key {
		return idx.lastNode
	}
	for i := range idx.entries {
		if idx.entries[i].key == key {
			idx.lastKey = key
			idx.lastNode = idx.entries[i].node
			idx.lastValid = true
			return idx.entries[i].node
		}
	}
	return nil
}

func (idx *gssForestIndex) set(key gssForestKey, node *gssForestNode) {
	idx.entries = append(idx.entries, gssForestEntry{key: key, node: node})
	idx.lastKey = key
	idx.lastNode = node
	idx.lastValid = true
}

const (
	gssForestNodeBatchCap            = 4096
	gssForestLinkBatchCap            = 8192
	maxRetainedGSSForestScratchBytes = 32 * 1024 * 1024
)

var gssForestNodeSlabPool = sync.Pool{
	New: func() any {
		return &gssForestNodeSlab{}
	},
}

// gssForestNodeSlab batch-allocates gssForestNodes so the forest doesn't pay one
// heap allocation per coalesced (state, byteOffset) node — the C GSS pools its
// stack nodes the same way. Nodes must outlive the whole parse (the DAG
// references them via links), so batches stay live until parseForest returns,
// then the scratch is cleared and pooled.
type gssForestNodeSlab struct {
	nodeBatches [][]gssForestNode
	nodeBatch   int
	nodeIdx     int
	linkBatches [][]gssLink
	linkBatch   int
	linkIdx     int
}

func acquireGSSForestNodeSlab() *gssForestNodeSlab {
	s := gssForestNodeSlabPool.Get().(*gssForestNodeSlab)
	s.nodeBatch = 0
	s.nodeIdx = 0
	s.linkBatch = 0
	s.linkIdx = 0
	return s
}

func releaseGSSForestNodeSlab(s *gssForestNodeSlab) {
	if s == nil {
		return
	}
	s.resetForRelease()
	s.trimToRetentionCap()
	gssForestNodeSlabPool.Put(s)
}

// trimToRetentionCap drops batches from the tail until the slab is under the
// retention cap, instead of the old all-or-nothing (dropping the ENTIRE slab when
// over cap forced a large parse to re-allocate and re-zero its whole link slab
// every parse — linkSlice was 76% of forest allocations). Keeping a cap's worth
// of batches lets them be reused (acquire resets linkBatch/nodeBatch to 0), so a
// large parse re-allocates only the overflow, at the SAME 32 MiB memory bound.
func (s *gssForestNodeSlab) trimToRetentionCap() {
	nodeSize := int(unsafe.Sizeof(gssForestNode{}))
	linkSize := int(unsafe.Sizeof(gssLink{}))
	total := s.retainedBytes()
	// Link batches dominate; trim them first, then node batches. Always keep at
	// least one batch of each so the slab stays warm.
	for total > maxRetainedGSSForestScratchBytes && len(s.linkBatches) > 1 {
		last := len(s.linkBatches) - 1
		total -= cap(s.linkBatches[last]) * linkSize
		s.linkBatches = s.linkBatches[:last]
	}
	for total > maxRetainedGSSForestScratchBytes && len(s.nodeBatches) > 1 {
		last := len(s.nodeBatches) - 1
		total -= cap(s.nodeBatches[last]) * nodeSize
		s.nodeBatches = s.nodeBatches[:last]
	}
}

func (s *gssForestNodeSlab) alloc(state StateID, byteOffset uint32, _ int, errorCost int) *gssForestNode {
	if len(s.nodeBatches) == 0 {
		s.nodeBatches = append(s.nodeBatches, make([]gssForestNode, gssForestNodeBatchCap))
	} else if s.nodeIdx >= len(s.nodeBatches[s.nodeBatch]) {
		s.nodeBatch++
		s.nodeIdx = 0
		if s.nodeBatch >= len(s.nodeBatches) {
			s.nodeBatches = append(s.nodeBatches, make([]gssForestNode, gssForestNodeBatchCap))
		}
	}
	n := &s.nodeBatches[s.nodeBatch][s.nodeIdx]
	s.nodeIdx++
	n.state = state
	n.byteOffset = byteOffset
	n.errorCost = errorCost
	n.minLinkScore = 0
	n.links = s.linkSlice()
	n.dirty = 0
	n.processedEpoch = 0
	n.processedDirty = 0
	n.noExtraDepth = 0
	// dupBucket*/worst* are pure caches keyed off n.dirty (see their doc
	// comment above): resetForRelease's clear() currently zeroes these too,
	// so this is defense-in-depth, not a behavior change today. Reset them
	// explicitly here so a future resetForRelease optimization (e.g. skipping
	// clear() for slots known not to need it) can't reintroduce a stale
	// dupBucketValid/worstValid hit against a different parse's dirty
	// sequence (cross-parse ABA on a pooled, reused gssForestNode).
	n.dupBucketValid = false
	n.dupBucketDirty = 0
	n.dupBucketIdx = 0
	n.worstValid = false
	n.worstDirty = 0
	n.worstIdx = 0
	return n
}

// linkSlice hands out a zero-length slice backed by the shared link buffer with
// enough capacity for the capped forest fan-out. The pooled slab makes this a
// retained scratch cost and avoids per-node append growth on ambiguous states.
func (s *gssForestNodeSlab) linkSlice() []gssLink {
	const initCap = forestMaxLinksPerNode
	if len(s.linkBatches) == 0 {
		s.linkBatches = append(s.linkBatches, make([]gssLink, gssForestLinkBatchCap))
	} else if s.linkIdx+initCap > len(s.linkBatches[s.linkBatch]) {
		s.linkBatch++
		s.linkIdx = 0
		if s.linkBatch >= len(s.linkBatches) {
			s.linkBatches = append(s.linkBatches, make([]gssLink, gssForestLinkBatchCap))
		}
	}
	buf := s.linkBatches[s.linkBatch]
	sl := buf[s.linkIdx : s.linkIdx : s.linkIdx+initCap]
	s.linkIdx += initCap
	return sl
}

func (s *gssForestNodeSlab) resetForRelease() {
	for i := 0; i <= s.nodeBatch && i < len(s.nodeBatches); i++ {
		used := len(s.nodeBatches[i])
		if i == s.nodeBatch {
			used = s.nodeIdx
		}
		clear(s.nodeBatches[i][:used])
	}
	for i := 0; i <= s.linkBatch && i < len(s.linkBatches); i++ {
		used := len(s.linkBatches[i])
		if i == s.linkBatch {
			used = s.linkIdx
		}
		clear(s.linkBatches[i][:used])
	}
	s.nodeBatch = 0
	s.nodeIdx = 0
	s.linkBatch = 0
	s.linkIdx = 0
}

func (s *gssForestNodeSlab) retainedBytes() int {
	total := 0
	nodeSize := int(unsafe.Sizeof(gssForestNode{}))
	linkSize := int(unsafe.Sizeof(gssLink{}))
	for _, batch := range s.nodeBatches {
		total += cap(batch) * nodeSize
	}
	for _, batch := range s.linkBatches {
		total += cap(batch) * linkSize
	}
	return total
}

// parseForest runs the GSS-forest GLR algorithm end to end: coalesce by
// (state, byteOffset), reduce over the DAG via reduceOverForest, with NO deep
// equivalence walk anywhere — the merge cost that was ~46% of fork-heavy parses
// is structurally gone. Tokens are pulled via nextToken(leadState) (the lexer /
// token-source wiring stays the caller's concern); the accepted root subtree is
// returned, or (nil,false) if the parse dies. This is the forest path the
// GOT_GLR_FOREST flag dispatches into; parity-iteration (extras, recovery,
// external scanners, full GLR-lexing) is layered on this core.
func (p *Parser) parseForest(arena *nodeArena, source []byte, captureExternalCheckpoints bool) (*Node, bool) {
	lang := p.language
	meta := lang.SymbolMetadata
	named := func(sym Symbol) bool { return int(sym) < len(meta) && meta[sym].Named }
	p.forestDeclineReason = ""
	p.forestDeclineByte, p.forestDeclineSym = 0, 0
	p.forestDeclineStates = p.forestDeclineStates[:0]
	forestLastDeclineReason = ""
	progress := newParseProgressTelemetry(p, len(source), uint32(len(source)), time.Now())
	if progress.enabled {
		progress.emit(time.Now(), "forest_parse_begin", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}

	// Reuse ONE child-builder scratch for every reduce in this parse (like the
	// production loop). buildReduceChildrenWithPath calls newReduceBuildScratch,
	// which reuses p.reduceScratch when set, else allocates a fresh scratch +
	// growing node slice PER REDUCE — the dominant forest allocation. One reused
	// scratch turns that into a single up-front allocation.
	prevReduceScratch := p.reduceScratch
	var forestReduceScratch reduceBuildScratch
	p.reduceScratch = &forestReduceScratch
	defer func() { p.reduceScratch = prevReduceScratch }()

	// Drive the production token source so keyword promotion, lex-mode
	// selection, immediate tokens, external scanners and GLR-lexing all match
	// the production parser. State is set per step from the frontier.
	lexer := NewLexer(lang.LexStates, source)
	ts := acquireDFATokenSourceWithCRecovery(lexer, lang, p.lookupActionIndexFunc(), p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState, p.errorCostCompetitionEnabled())
	ts.setExternalScannerCheckpointsEnabled(captureExternalCheckpoints)

	// tree-sitter convention: state 0 is the error state, state 1 is the start.
	start := &gssForestNode{state: 1, byteOffset: 0}
	frontier := []*gssForestNode{start}
	glrStates := make([]StateID, 0, 16)
	reducer := &forestReducer{}
	slab := acquireGSSForestNodeSlab()
	defer releaseGSSForestNodeSlab(slab)
	linkCap := forestLinkCapForLanguage(lang.Name)

	// Honor the same per-parse memory budget the production loop enforces
	// (parser.go: arena.budgetExhausted → ParseStopMemoryBudget). The forest has
	// no partial-tree/error-recovery path, so on exhaustion it declines (returns
	// false) and the production parser re-runs and reports ParseStopMemoryBudget.
	arena.setBudget(parseMemoryBudgetForParser(p, len(source)))

	// Honor the same caller-configured timeout/cancellation the production
	// loop enforces (parser.go's `for iter := ...` checks p.parseStopReasonNow()
	// every iteration). Until this, the forest had NO deadline/cancellation
	// awareness at all — SetTimeoutMicros/cancellation were silently
	// unenforceable for any forest-dispatched language, and the forest's only
	// safety valves (forestReduceStepCap/forestReduceVisitCap/
	// forestWorklistVisitCap) do not bound wall-clock time: a pathological
	// shape (e.g. a wide/deeply-ambiguous C# designer-generated block) can sit
	// reprocessing the same token position for many seconds on only a few
	// hundred reduce-visits, never tripping those caps. stopPoller mirrors
	// parseStopPoller: pollNow() (unmasked) once per TOKEN below, cheap enough
	// at that frequency; poll() (masked to every 1024 calls, parser_timeout.go)
	// inside the reduce/coalesce worklist loop, which can iterate far more
	// often than once per token on an ambiguous shape. Declining here (rather
	// than fabricating a forest-side timeout tree) means the two forest entry
	// points behave differently on a timeout/cancellation decline, matching
	// what each already does for every OTHER decline reason: tryForestFastPath
	// (the real p.Parse() dispatch, parser_api.go) just returns nil to its
	// caller, which falls through to the normal production loop — that loop's
	// own enterParseBudget-established deadline is the SAME one (nesting, not
	// reset), so its very first p.parseStopReasonNow() check fires immediately
	// and returns a proper ParseStopTimeout/ParseStopCancelled tree cheaply.
	// ParseForestExperimental's fallback, by contrast, only re-runs production
	// internally for forestDeclineEOFRecoveryConflict specifically (see below);
	// a timeout/cancellation decline reason does not match that string, so it
	// takes the plain `return nil, false` path with no internal fallback
	// attempt at all — callers of that standalone entry point get a bare
	// failure on timeout, not a production retry.
	stopPoller := parseStopPoller{check: p.activeParseStopCheck()}

	// Per-step scratch reused across every token (cleared, not reallocated): the
	// allocation/GC of fresh maps+slices each step dominated the profile.
	curIndex := newGSSForestIndex(16)
	nextIndex := newGSSForestIndex(16)
	var work, nextFrontier, relex []*gssForestNode
	alternatives := newForestAlternativeIndex(forestAlternativeIndexCapacityForSource(len(source)))
	processEpoch := int32(0)
	noLookaheadSteps := 0
	recoverCount := 0
	recoverActive := glrForestRecover || languageWantsForestRecover(lang.Name)
	if progress.enabled {
		progress.emit(time.Now(), "forest_setup_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0,
			forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, nil, fmt.Sprintf("recover_active=%t", recoverActive)))
	}
	iter := 0
	var tokens uint64

	for {
		iter++
		processEpoch++
		if progress.enabled {
			progress.beginDetail(time.Now(), "forest_step_begin", "", iter, tokens, Token{}, false, nil, 0, 0, 0, true, 0, 0,
				forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, nil, ""))
		}
		if arena.budgetExhausted() {
			// Memory budget hit; decline so the production parser re-runs and
			// reports ParseStopMemoryBudget (the forest has no partial-tree path).
			p.recordForestDecline("budget", Token{StartByte: frontier[len(frontier)-1].byteOffset}, nil)
			if progress.enabled {
				progress.emit(time.Now(), "forest_decline", iter, tokens, Token{}, false, nil, 0, 0, 0, false, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, nil, "decline_reason=budget"))
			}
			return nil, false
		}
		if reason := stopPoller.pollNow(); parseStopReasonIsTerminal(reason) {
			// Timeout/cancellation: decline so the caller's already-established
			// deadline is honored by whichever path re-checks it next (the
			// ParseForestExperimental fallback's p.Parse(source), or the
			// production loop this same *Parser is already inside when
			// dispatched via tryForestFastPath) instead of the forest silently
			// running unbounded.
			p.recordForestDecline(string(reason), Token{StartByte: frontier[len(frontier)-1].byteOffset}, nil)
			if progress.enabled {
				progress.emit(time.Now(), "forest_decline", iter, tokens, Token{}, false, nil, 0, 0, 0, false, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, nil, "decline_reason="+string(reason)))
			}
			return nil, false
		}
		if reducer.capped {
			reason := reducer.capReason
			if reason == "" {
				reason = "reducer_capped"
			}
			p.recordForestDecline(reason, Token{StartByte: frontier[len(frontier)-1].byteOffset}, nil)
			if progress.enabled {
				progress.emit(time.Now(), "forest_decline", iter, tokens, Token{}, false, nil, 0, 0, 0, false, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, nil, "decline_reason="+reason))
			}
			return nil, false
		}
		// GLR-lex over the union of frontier states; lead = the most-advanced.
		glrStates = glrStates[:0]
		for _, n := range frontier {
			glrStates = append(glrStates, n.state)
		}
		ts.SetGLRStates(glrStates)
		ts.SetParserState(frontier[len(frontier)-1].state)
		if progress.enabled {
			progress.beginDetail(time.Now(), "forest_token_next_begin", "forest_token_next_end", iter, tokens, Token{}, false, nil, 0, 0, 0, true, 0, 0,
				forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, nil, fmt.Sprintf("glr_state_count=%d parser_state=%d", len(glrStates), frontier[len(frontier)-1].state)))
		}
		tok := ts.Next()
		tokens++
		p.updateCurrentExternalTokenCheckpoint(ts, tok)
		// A NoLookahead token is a SYNTHETIC EOF the token source emits to force
		// the no-lookahead-state reduction (e.g. completing a multi-token comment
		// extra) — it is NOT real end-of-input. Only Symbol==0 && !NoLookahead is
		// real EOF. Treating the synthetic one as EOF truncated any file whose
		// comment lexes as >1 token (rust/lua/dart starting with a comment).
		eof := tok.Symbol == 0 && !tok.NoLookahead
		if progress.enabled {
			progress.endDetail(time.Now(), "forest_token_next_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
				forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, nil, ""))
		}

		// Reduces coalesce into curIndex (same position, seeded with the
		// frontier so a reduced nonterminal can merge with an existing actor);
		// shifts coalesce into nextIndex (next position).
		curIndex.reset()
		for _, n := range frontier {
			curIndex.set(gssForestKey{n.state, n.byteOffset}, n)
		}
		nextIndex.reset()
		nextFrontier = nextFrontier[:0]
		var accepted *gssForestNode
		acceptedOrder := 0
		acceptedBestOrder := 0

		work = append(work[:0], frontier...)
		if progress.enabled {
			progress.beginDetail(time.Now(), "forest_reduce_worklist_begin", "forest_reduce_worklist_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
				forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, ""))
		}
		reducer.visitCount = 0
		reducer.visitCap = forestReduceVisitCap
		reducer.capReason = ""
		workVisits := 0
		for len(work) > 0 {
			workVisits++
			if workVisits > forestWorklistVisitCap {
				reducer.capped = true
				reducer.capReason = "worklist-cap"
				p.recordForestDecline("worklist-cap", tok, nil)
				if progress.enabled {
					progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
							fmt.Sprintf("decline_reason=worklist-cap work_visits=%d work_cap=%d", workVisits, forestWorklistVisitCap)))
				}
				return nil, false
			}
			// Masked (every 1024 calls, parser_timeout.go) so a hot worklist
			// doesn't pay a time.Now() per visit: this loop can run far more
			// than once per token on an ambiguous shape, unlike the per-token
			// pollNow() above.
			if reason := stopPoller.poll(); parseStopReasonIsTerminal(reason) {
				reducer.capped = true
				reducer.capReason = string(reason)
				p.recordForestDecline(string(reason), tok, nil)
				if progress.enabled {
					progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
							"decline_reason="+string(reason)))
				}
				return nil, false
			}
			node := work[len(work)-1]
			work = work[:len(work)-1]
			// Process a node the first time it is seen, and again whenever it has
			// become dirty (a new link, or a link replaced by a higher-precedence
			// alternative) since it was last processed. Re-running its reductions
			// rebuilds any parents that consumed a now-superseded subtree.
			if node.processedEpoch == processEpoch && node.processedDirty == node.dirty {
				continue
			}
			node.processedEpoch = processEpoch
			node.processedDirty = node.dirty

			nodeActions := p.actionsForParseState(node.state, tok.Symbol, lang.ParseActions)
			nodeActions = p.forestResolveConflict(node.state, tok, nodeActions)
			// C's global repetition-skip fold, mirrored into the forest engine.
			// Production's deterministicConflictChoiceForDispatch resolves a
			// {1 repetition-marked SHIFT, 1 REDUCE} conflict by taking the REDUCE
			// (C's ts_parser__advance runs `if (action.shift.repetition) break;`,
			// parser.c) — a deterministic, lossless fold of the list spine, not a
			// fork. forestResolveConflict did NOT carry this fold, so a long
			// repeated list (a bash `program` is repeat($._statement)) forked at
			// every statement boundary, building an O(n^2) GSS frontier, and
			// reached EOF with only mid-list lineages surviving — none in a
			// list-closed (accept) state — declining eof_no_root
			// (medium__install.sh: forest reached EOF with 2 non-accepting stacks
			// while tree-sitter-C parses the file clean). Applying the same fold
			// lets the forest close the list and accept, byte-matching C. Gated to
			// non-recover forest lineages: with error recovery the repetition-shift
			// arm can be the branch the recovery cost competition keeps (the php
			// comma-list wreckage lesson in conflict_policy.go), so recover-active
			// languages keep the ordinary GLR fork.
			if len(nodeActions) > 1 {
				if folded, ok := p.cRepetitionSkipForestConflictChoice(recoverActive, nodeActions); ok {
					nodeActions = p.forestSingletonActions(folded)
				}
			}
			if progress.enabled {
				reduceActions, shiftActions, acceptActions := 0, 0, 0
				for _, act := range nodeActions {
					switch act.Type {
					case ParseActionReduce:
						reduceActions++
					case ParseActionShift:
						shiftActions++
					case ParseActionAccept:
						acceptActions++
					}
				}
				progress.beginDetail(time.Now(), "forest_actions", "", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
						fmt.Sprintf("state=%d action_count=%d reduce_actions=%d shift_actions=%d accept_actions=%d", node.state, len(nodeActions), reduceActions, shiftActions, acceptActions)))
			}
			for _, act := range nodeActions {
				switch act.Type {
				case ParseActionReduce:
					// Synthetic-EOF containment: a NoLookahead token is the synthetic
					// EOF the token source emits to FLUSH a state stuck mid-extra (a
					// multi-token comment, e.g. rust `///` = `//`+`/`+doc_comment). It
					// must not finalize the whole source unit. Reducing the ROOT symbol
					// (source_file) on it caps a cascade — line_comment → const_item →
					// … → source_file → ACCEPT — that collapses the file mid-parse and
					// strands the item-list continuation, so the next top-level item
					// can no longer shift (rust large__ast.rs dead-ended at a `pub`
					// after a doc comment). The root reduce is valid only at REAL EOF;
					// production re-lexes after each synthetic-EOF reduce (parser.go:
					// needToken=tok.NoLookahead) and meets a real token first.
					if tok.NoLookahead && p.hasRootSymbol && act.Symbol == p.rootSymbol {
						continue
					}
					cc := int(act.ChildCount)
					var gotoCache forestGotoCache
					if progress.enabled {
						progress.beginDetail(time.Now(), "forest_reduce_begin", "forest_reduce_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
							forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
								fmt.Sprintf("state=%d reduce_symbol=%d child_count=%d production_id=%d dynamic_precedence=%d", node.state, act.Symbol, cc, act.ProductionID, act.DynamicPrecedence)))
					}
					reducer.reduce(node, cc, func(children []stackEntry, childScore int, popTo *gssForestNode, noExtras bool) {
						if reducer.capped {
							return
						}
						if progress.enabled {
							popState := StateID(0)
							popOffset := uint32(0)
							if popTo != nil {
								popState = popTo.state
								popOffset = popTo.byteOffset
							}
							progress.beginDetail(time.Now(), "forest_reduce_visit_begin", "", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
								forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
									fmt.Sprintf("reduce_symbol=%d child_count=%d visit_child_len=%d child_score=%d pop_state=%d pop_offset=%d no_extras=%t", act.Symbol, cc, len(children), childScore, popState, popOffset, noExtras)))
						}
						gotoState := gotoCache.lookup(p, popTo.state, act.Symbol)
						if gotoState == 0 {
							if perfCountersEnabled {
								perfRecordForestReduceGotoMiss()
							}
							if progress.enabled {
								progress.beginDetail(time.Now(), "forest_reduce_goto_miss", "", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
									forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
										fmt.Sprintf("reduce_symbol=%d child_count=%d pop_state=%d", act.Symbol, cc, popTo.state)))
							}
							return
						}
						if perfCountersEnabled {
							perfRecordForestReduceGotoHit()
						}
						// Trailing extras (a comment after a complete construct)
						// are not part of the reduced node — they belong to the
						// surrounding context. Trim them here and re-push them on
						// top of the reduced node so the next (outer) reduce attaches
						// them, mirroring reduceWindowFromGSS + the trailing re-push.
						// `children` is the reducer's shared buffer, stable for the
						// duration of this visit (no re-entry until we return), so the
						// node-builder and span helpers read it in place — no per-reduce
						// copy. window = children[0:reducedEnd] (trailing extras trimmed).
						reducedEnd := len(children)
						if !noExtras {
							reducedEnd = reducedEndBeforeTrailingExtras(children)
						}
						score := int(act.DynamicPrecedence) + childScore
						// Coverage rejection: a reduction whose children leave a
						// NON-TRIVIA hole skipped real input and is INVALID. This is
						// the load-bearing fix for tree-sitter's binary repeat
						// (`X_repeat1 -> X_repeat1 X_repeat1`): the forest forks on every
						// grouping of the same statement list, and some binary merges
						// combine two halves with a dropped statement between them
						// (lua `chunk_repeat1[0-99] + chunk_repeat1[162-X]` skipping a
						// `local function` statement). Such a gapped node shares its
						// (symbol, start, end) span with the gap-free grouping, so the
						// (sym,span) dedup merges them and a gapped one can win on score —
						// dropping the statement. Scanning ALL children (extras provide
						// coverage, so an interior comment is NOT a gap) and rejecting any
						// real hole keeps only valid groupings; the gap-free merge then
						// wins. Gap-free reductions (every promoted lang) never trip it.
						if reducedEnd > 0 {
							lastEnd := stackEntryNodeEndByte(children[0])
							for k := 1; k < reducedEnd; k++ {
								cs := stackEntryNodeStartByte(children[k])
								if cs > lastEnd && int(cs) <= len(source) && !bytesAreInterTokenTrivia(source[lastEnd:cs]) {
									return
								}
								if ce := stackEntryNodeEndByte(children[k]); ce > lastEnd {
									lastEnd = ce
								}
							}
						}
						// If the target forest node is already at its fan-out cap and
						// this reduction cannot displace an existing alternative, avoid
						// building the reduced children and parent node just to drop it.
						parentEnd := node.byteOffset
						if reducedEnd < len(children) && reducedEnd > 0 {
							parentEnd = stackEntryNodeEndByte(children[reducedEnd-1])
						}
						if forestCoalesceWouldDropForCap(&curIndex, gotoState, parentEnd, score, popTo.errorCost, linkCap) {
							if perfCountersEnabled {
								perfRecordForestCoalescePreCapDrop()
							}
							if progress.enabled {
								progress.beginDetail(time.Now(), "forest_reduce_precap_drop", "", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
									forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
										fmt.Sprintf("reduce_symbol=%d child_count=%d goto_state=%d parent_end=%d score=%d error_cost=%d", act.Symbol, cc, gotoState, parentEnd, score, popTo.errorCost)))
							}
							return
						}
						// A collapsible named-keyword-leaf reduce (e.g. go `false`->'false',
						// `nil`, `iota`): the named node absorbs its single anonymous keyword
						// token to a LEAF, matching the production reduce (applyReduceAction)
						// and tree-sitter C (ChildCount 0, not 1). aliasedNodeInArena clones,
						// so the shared forest child is never mutated; skip the child build
						// entirely so the child's parent link is untouched and the collapsed
						// leaf keeps the child's span.
						var parent *Node
						var childNodes []*Node
						if collapsed := p.forestCollapsibleNamedKeywordLeaf(act, tok, arena, children, 0, reducedEnd); collapsed != nil {
							if progress.enabled {
								progress.beginDetail(time.Now(), "forest_reduce_parent_begin", "forest_reduce_parent_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
									forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
										fmt.Sprintf("reduce_symbol=%d child_count=%d child_nodes=0 collapsed=true", act.Symbol, cc)))
							}
							parent = collapsed
						} else {
							if progress.enabled {
								progress.beginDetail(time.Now(), "forest_reduce_children_begin", "forest_reduce_children_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
									forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
										fmt.Sprintf("reduce_symbol=%d child_count=%d reduced_end=%d children_len=%d", act.Symbol, cc, reducedEnd, len(children))))
							}
							var fieldIDs []FieldID
							var fieldSources []uint8
							var childPath reduceChildPath
							childNodes, fieldIDs, fieldSources, childPath = p.buildReduceChildrenWithPath(children, 0, reducedEnd, cc, act.Symbol, act.ProductionID, arena)
							if progress.enabled {
								progress.endDetail(time.Now(), "forest_reduce_children_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
									forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
										fmt.Sprintf("reduce_symbol=%d child_count=%d child_nodes=%d", act.Symbol, cc, len(childNodes))))
								progress.beginDetail(time.Now(), "forest_reduce_parent_begin", "forest_reduce_parent_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
									forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
										fmt.Sprintf("reduce_symbol=%d child_count=%d child_nodes=%d", act.Symbol, cc, len(childNodes))))
							}
							parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named(act.Symbol), childNodes, fieldIDs, fieldSources, act.ProductionID)
							// Recover the reduced node's byte span from the full window,
							// mirroring the production reduce. newParentNode spans only the
							// VISIBLE children, so anonymous/invisible tokens that
							// buildReduceChildren drops (e.g. the digits of a css
							// integer_value, or a node with zero visible children) would
							// otherwise leave the span wrong or empty ([0:0]).
							rawSpanApplied := false
							if shouldUseRawSpanForReduction(act.Symbol, childNodes, lang.SymbolMetadata, p.forceRawSpanAll, p.forceRawSpanTable) && reducedEnd > 0 {
								span := computeReduceRawSpan(children, 0, reducedEnd)
								parent.startByte, parent.endByte = span.startByte, span.endByte
								parent.startPoint, parent.endPoint = span.startPoint, span.endPoint
								rawSpanApplied = true
							}
							if !rawSpanApplied && reduceChildPathMayDropSpan(childPath) {
								extendParentSpanToWindow(parent, children, 0, reducedEnd, lang.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols, source)
							}
						}
						// Coalescing tracks parser input position, not necessarily the
						// visible node span. JavaScript blocks can end before dropped
						// anonymous delimiters in the tree, but the stack has still
						// consumed through node.byteOffset. If trailing extras were
						// trimmed, key the parent before those extras so they can be
						// re-pushed on top.
						parent.preGotoState = popTo.state
						parent.parseState = gotoState
						// Mark a reduced EXTRA node (e.g. a multi-token comment like rust's
						// doc_comment, which is parsed as `//`+content then reduced) as
						// extra, mirroring the production reduce (parser_reduce.go:
						// `if tok.NoLookahead && targetState == topState { parent.setExtra }`).
						// A no-lookahead reduce whose goto is transparent (returns to the
						// state it popped to) is an extra completing in place. Without this
						// the comment node sits UNMARKED in the GSS chain, so the next
						// reduce pops it as a real child (wrong popTo, goto=0, dead-end) —
						// the between-item-comment bug for rust/lua/dart.
						if tok.NoLookahead && gotoState == popTo.state {
							parent.setExtra(true)
						}
						parent.dynamicPrecedence = int32(score)
						parent.rawShape = p.captureRawShape(arena, act.Symbol, act.ProductionID, children, 0, reducedEnd)
						if len(childNodes) > 0 {
							forestRecordParentChildAlternatives(alternatives, parent, childNodes, children[:reducedEnd])
						}
						if progress.enabled {
							progress.endDetail(time.Now(), "forest_reduce_parent_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
								forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
									fmt.Sprintf("reduce_symbol=%d parent_start=%d parent_end=%d goto_state=%d", act.Symbol, parent.startByte, parent.endByte, gotoState)))
							progress.beginDetail(time.Now(), "forest_reduce_coalesce_begin", "forest_reduce_coalesce_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
								forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
									fmt.Sprintf("reduce_symbol=%d goto_state=%d parent_end=%d trailing_extras=%d", act.Symbol, gotoState, parentEnd, len(children)-reducedEnd)))
						}
						parentEntry := stackEntry{node: unsafe.Pointer(parent), state: gotoState, kind: stackEntryKindNode}
						// Subtree score = this production's dynamic precedence +
						// the children's accumulated scores.
						top := coalesceForestWithRawAndAlternatives(p, arena, &curIndex, slab, gotoState, parentEnd, popTo,
							parentEntry,
							score, popTo.errorCost, linkCap, alternatives)
						for _, ex := range children[reducedEnd:] {
							extra := (*Node)(ex.node)
							extra.parseState = gotoState
							nodeBumpEquivVersion(extra)
							exEnd := extra.endByte
							top = coalesceForestWithRawAndAlternatives(p, arena, &curIndex, slab, gotoState, exEnd, top,
								stackEntry{node: ex.node, state: gotoState, kind: stackEntryKindNode},
								0, top.errorCost, linkCap, alternatives)
						}
						work = append(work, top)
						if progress.enabled {
							progress.endDetail(time.Now(), "forest_reduce_coalesce_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
								forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
									fmt.Sprintf("reduce_symbol=%d goto_state=%d work_len_after_append=%d", act.Symbol, gotoState, len(work))))
						}
					})
					if reducer.capped {
						reason := reducer.capReason
						if reason == "" {
							reason = "reduce-cap"
						}
						p.recordForestDecline(reason, tok, nil)
						if progress.enabled {
							progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
								forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
									fmt.Sprintf("decline_reason=%s work_visits=%d reduce_symbol=%d child_count=%d visit_cap=%d step_cap=%d", reason, workVisits, act.Symbol, cc, forestReduceVisitCap, forestReduceStepCap)))
						}
						return nil, false
					}
					if progress.enabled {
						progress.endDetail(time.Now(), "forest_reduce_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
							forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
								fmt.Sprintf("state=%d reduce_symbol=%d child_count=%d reducer_steps=%d reducer_capped=%t", node.state, act.Symbol, cc, reducer.steps, reducer.capped)))
					}
				case ParseActionShift:
					if !p.guardForestRealShiftGap(source, node, tok) {
						continue
					}
					if progress.enabled {
						progress.beginDetail(time.Now(), "forest_shift_begin", "forest_shift_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
							forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
								fmt.Sprintf("state=%d shift_state=%d extra=%t", node.state, act.State, act.Extra)))
					}
					leaf := newLeafNodeInArena(arena, tok.Symbol, named(tok.Symbol), tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
					// An extra (comment/whitespace) shifts without advancing the
					// parse state: it stays transparent to the grammar and is
					// attached to the surrounding node as an extra child at the next
					// reduce. extraShiftTargetState keeps the current state when the
					// action carries no explicit target.
					target := act.State
					if act.Extra {
						leaf.setExtra(true)
						target = extraShiftTargetState(node.state, act)
					}
					leaf.preGotoState = node.state
					leaf.parseState = target
					p.recordCurrentExternalLeafCheckpoint(leaf, tok)
					before := nextIndex.len()
					sh := coalesceForestWithRawAndAlternatives(p, arena, &nextIndex, slab, target, tok.EndByte, node,
						stackEntry{node: unsafe.Pointer(leaf), state: target, kind: stackEntryKindNode},
						0, node.errorCost, linkCap, alternatives) // a shifted leaf carries no dynamic precedence
					if nextIndex.len() != before {
						nextFrontier = append(nextFrontier, sh)
					}
					if progress.enabled {
						progress.endDetail(time.Now(), "forest_shift_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
							forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
								fmt.Sprintf("state=%d target_state=%d next_index_before=%d next_index_after=%d shifted=%t", node.state, target, before, nextIndex.len(), nextIndex.len() != before)))
					}
				case ParseActionAccept:
					// Prefer the accept candidate that consumed the MOST input. A
					// trailing multi-token extra (e.g. a single lua `-- comment` at
					// EOF) produces a second accept node ABOVE the root whose
					// byteOffset is larger; the plain root accepts too, and taking the
					// last-seen one drops the trailing comment. Max-coverage keeps it.
					order := acceptedOrder
					acceptedOrder++
					if accepted == nil ||
						node.byteOffset > accepted.byteOffset ||
						(node.byteOffset == accepted.byteOffset && forestAcceptedNodeCompare(p, arena, node, order, accepted, acceptedBestOrder) > 0) {
						accepted = node
						acceptedBestOrder = order
					}
					if progress.enabled {
						progress.beginDetail(time.Now(), "forest_accept_seen", "", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
							forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
								fmt.Sprintf("state=%d", node.state)))
					}
				}
			}
		}
		if progress.enabled {
			progress.endDetail(time.Now(), "forest_reduce_worklist_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
				forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, ""))
		}

		if eof {
			// A genuinely CLEAN forest accept (zero recovery cost, no ERROR/MISSING
			// nodes, full input span) is unbeatable in C's finished_tree selection:
			// ts_parser picks the lowest-error_cost finished version and every
			// recovery costs at least ERROR_COST_PER_RECOVERY (500), so no EOF
			// recovery could compete with a cost-0 accept — forestEOFRecoveryCould-
			// Compete would always answer "no". Skip that probe for such accepts on
			// the !recoverActive languages (bash/c_sharp/cmake/... — every forest
			// language whose accept is required to be error-free), whose outer
			// tryForestFastPath still declines on any root.HasError() as a safety
			// net. recoverActive languages keep the conservative probe: their outer
			// path intentionally accepts error-bearing roots (that HasError net is
			// disabled for them) and forest-vs-production materialization parity on
			// their newly forest-routed clean files is not covered by the local
			// corpus here — the C argument is airtight, but we stay conservative.
			cleanAcceptBeatsRecovery := !recoverActive &&
				p.forestAcceptedIsCleanCompleteParse(arena, accepted, source)
			if accepted != nil && !cleanAcceptBeatsRecovery && (recoverActive || p.errorCostCompetitionEnabled()) && p.forestEOFRecoveryCouldCompete(&curIndex, arena, tok.StartByte, !recoverActive) {
				p.recordForestDecline(forestDeclineEOFRecoveryConflict, tok, nil)
				if progress.enabled {
					progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "decline_reason="+forestDeclineEOFRecoveryConflict))
				}
				return nil, false
			}
			if progress.enabled {
				progress.beginDetail(time.Now(), "forest_collect_root_begin", "forest_collect_root_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, ""))
			}
			root, extras := collectForestRootAndExtras(p, arena, accepted, alternatives)
			if progress.enabled {
				progress.endDetail(time.Now(), "forest_collect_root_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
						fmt.Sprintf("root_present=%t extras_len=%d", root != nil, len(extras))))
			}
			if root == nil {
				if recoverActive {
					if progress.enabled {
						progress.beginDetail(time.Now(), "forest_collect_error_root_begin", "forest_collect_error_root_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
							forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, ""))
					}
					if eroot := p.collectForestErrorRoot(&curIndex, arena); eroot != nil {
						forestPreserveRootVisibleContainerAlternatives(p, arena, eroot, alternatives)
						if int(eroot.endByte) < len(source) && bytesAreTrivia(source[eroot.endByte:]) {
							extendNodeEndTo(eroot, uint32(len(source)), source)
						}
						if progress.enabled {
							progress.endDetail(time.Now(), "forest_collect_error_root_end", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
								forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
									fmt.Sprintf("root_present=true root_end=%d", eroot.EndByte())))
							progress.emit(time.Now(), "forest_return", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
								forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
									fmt.Sprintf("ok=true root_end=%d error_root=true", eroot.EndByte())))
						}
						return eroot, true
					}
					if progress.enabled {
						progress.endDetail(time.Now(), "forest_collect_error_root_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
							forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "root_present=false"))
					}
				}
				p.recordForestDecline("eof_no_root", tok, nil)
				if progress.enabled {
					progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "decline_reason=eof_no_root"))
				}
				return nil, false
			}
			// Leading/trailing extras live outside the start-symbol node (above or
			// below it on the accepted stack); fold them into the root the way the
			// production result builder does, splitting by position.
			if len(extras) > 0 {
				foldResultRootExtras(root, extras, arena)
			}
			// The production root spans the whole input, including trailing
			// trivia; the forest root stops at the last token. Extend to match
			// when the remaining bytes are trivia (whitespace/comments only).
			if int(root.endByte) < len(source) && bytesAreTrivia(source[root.endByte:]) {
				extendNodeEndTo(root, uint32(len(source)), source)
			}
			// Coverage safety: the checks above only validate the END byte. A
			// large-input GLR parse can still take a wrong path that drops or
			// mis-attaches a RUN of top-level items, leaving a non-trivia hole
			// in the MIDDLE of the root's child list (dart's large bindings drop
			// a ~7KB run of typedefs). Dispatching that hands the caller a
			// structurally-incomplete tree. Decline so production re-runs.
			if !forestRootChildrenCoverNonTrivia(root, source) {
				p.recordForestDecline("noncontiguous_root", tok, nil)
				if progress.enabled {
					progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "decline_reason=noncontiguous_root"))
				}
				return nil, false
			}
			if progress.enabled {
				progress.emit(time.Now(), "forest_return", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
						fmt.Sprintf("ok=true root_end=%d extras_len=%d", root.EndByte(), len(extras))))
			}
			return root, true
		}
		if tok.NoLookahead {
			// The no-lookahead reductions ran in the work loop above and advanced
			// the frontier in place (no token was consumed, so nextIndex is for
			// the same position). Re-lex at this position with the states those
			// reductions produced — but DROP states that themselves only emit a
			// no-lookahead reduce, which would re-emit the synthetic EOF and loop.
			relex = relex[:0]
			for i := range curIndex.entries {
				n := curIndex.entries[i].node
				if ts.lexStateForState(n.state) != noLookaheadLexState {
					relex = append(relex, n)
				}
			}
			if len(relex) == 0 {
				p.recordForestDecline("nolook_relex_empty", tok, nil)
				return nil, false
			}
			noLookaheadSteps++
			if noLookaheadSteps > maxForestNoLookaheadSteps {
				p.recordForestDecline("nolook_runaway", tok, nil) // fall back to production
				return nil, false
			}
			frontier = append(frontier[:0], relex...)
			continue
		}
		noLookaheadSteps = 0
		if len(nextFrontier) == 0 {
			curStates := make([]StateID, 0, len(curIndex.entries))
			for i := range curIndex.entries {
				curStates = append(curStates, curIndex.entries[i].node.state)
			}
			// No frontier node could shift this token: the production parser would
			// recover here. EXPERIMENTAL: absorb the token into an error region and
			// keep the frontier alive in its current states, advancing past the
			// token. Consecutive absorbed tokens are deferred to finalization, which
			// wraps the error span(s). Off by default (glrForestRecover).
			if !recoverActive || eof || recoverCount >= forestRecoverCap || tok.EndByte <= tok.StartByte {
				p.recordForestDecline("dead_end", tok, curStates)
				if progress.enabled {
					progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
							fmt.Sprintf("decline_reason=dead_end recover_active=%t recover_cap_hit=%t", recoverActive, recoverCount >= forestRecoverCap)))
				}
				return nil, false
			}
			// error_cost recovery (tree-sitter C model, reusing production's
			// recover-action table): for each stuck frontier node, prefer a
			// grammar RECOVER action (pop to a recover-capable state so reductions
			// can continue toward accept — the piece naive error-skip lacked);
			// otherwise absorb the token into an error leaf at the current state.
			// Each absorbed token raises errorCost by its width; finalization
			// selects the lowest-errorCost path.
			tokWidth := int(tok.EndByte - tok.StartByte)
			nextIndex.reset()
			nextFrontier = nextFrontier[:0]
			if progress.enabled {
				progress.beginDetail(time.Now(), "forest_recovery_begin", "forest_recovery_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted,
						fmt.Sprintf("tok_width=%d", tokWidth)))
			}
			for _, n := range frontier {
				if !p.guardForestRealShiftGap(source, n, tok) {
					continue
				}
				recoverState := n.state
				if act, ok := p.recoverActionForState(n.state, tok.Symbol); ok && act.State != 0 {
					recoverState = act.State
				}
				errLeaf := newLeafNodeInArena(arena, errorSymbol, true, tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
				errLeaf.setHasError(true)
				errLeaf.preGotoState = n.state
				errLeaf.parseState = recoverState
				before := nextIndex.len()
				sh := coalesceForestWithRawAndAlternatives(p, arena, &nextIndex, slab, recoverState, tok.EndByte, n,
					stackEntry{node: unsafe.Pointer(errLeaf), state: recoverState, kind: stackEntryKindNode},
					0, n.errorCost+tokWidth, linkCap, alternatives)
				if nextIndex.len() != before {
					nextFrontier = append(nextFrontier, sh)
				}
			}
			if len(nextFrontier) == 0 {
				p.recordForestDecline("dead_end", tok, curStates)
				if progress.enabled {
					progress.endDetail(time.Now(), "forest_recovery_end", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "recovered=false"))
					progress.emit(time.Now(), "forest_decline", iter, tokens, tok, true, nil, 0, 0, 0, false, 0, 0,
						forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "decline_reason=dead_end"))
				}
				return nil, false
			}
			recoverCount++
			if progress.enabled {
				progress.endDetail(time.Now(), "forest_recovery_end", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "recovered=true"))
			}
			frontier = append(frontier[:0], nextFrontier...)
			if progress.enabled {
				progress.beginDetail(time.Now(), "forest_frontier_advance", "", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
					forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "via=recovery"))
			}
			continue
		}
		// Copy (not alias) so the next step can reset nextFrontier in place;
		// frontier is only read at the top of a step, before that reset.
		frontier = append(frontier[:0], nextFrontier...)
		if progress.enabled {
			progress.beginDetail(time.Now(), "forest_frontier_advance", "", iter, tokens, tok, true, nil, 0, 0, 0, true, 0, 0,
				forestProgressExtra(frontier, work, nextFrontier, curIndex, nextIndex, processEpoch, recoverCount, reducer, accepted, "via=shift"))
		}
	}
}

// maxForestNoLookaheadSteps bounds consecutive no-lookahead re-lex steps at one
// input position (each should complete a no-lookahead reduction and advance the
// frontier); exceeding it means a runaway chain, so the forest declines to
// production rather than spin.
const maxForestNoLookaheadSteps = 64

func (p *Parser) guardForestRealShiftGap(source []byte, node *gssForestNode, tok Token) bool {
	if node == nil {
		return true
	}
	stack := glrStack{byteOffset: node.byteOffset}
	return p.guardRealShiftGap(source, &stack, tok)
}

// reduceOverForest enumerates every length-childCount path of subtrees ending at
// `node` and invokes visit once per path with the children in left-to-right
// order (children[0] = first/leftmost child) and `popTo` = the predecessor node
// the reduction pops back to. This is Stage 2 — DAG traversal that replaces the
// single-chain reduce so a coalesced node's multiple histories all reduce, with
// no deep-equivalence walk anywhere (the 46% gone). A single-link chain yields
// exactly one path, identical to today's reduce; coalesced nodes yield one path
// per surviving alternative.
//
// `children` is a SHARED buffer reused across paths and across visit calls — the
// visitor must consume or copy it before returning, never retain it. The walk is
// a bounded DFS (depth == childCount); ambiguous grammars are bounded upstream by
// error_cost pruning + the per-(state,position) link cap, mirroring tree-sitter C.
func reduceOverForest(node *gssForestNode, childCount int, visit func(children []stackEntry, childScore int, popTo *gssForestNode)) {
	(&forestReducer{}).reduce(node, childCount, func(children []stackEntry, childScore int, popTo *gssForestNode, _ bool) {
		visit(children, childScore, popTo)
	})
}

type forestReduceVisitor func(children []stackEntry, childScore int, popTo *gssForestNode, noExtras bool)

// forestReducer holds the two scratch slices the reduce DFS reuses across every
// call within one parse, so the hot path allocates nothing: `path` collects the
// current branch most-recent-first (append on descend, truncate on backtrack),
// and `rev` is the left-to-right view handed to visit. The visitor must consume
// children before returning (it copies), and must not re-enter reduce.
// forestReduceStepCap bounds a SINGLE forest reduce's path enumeration. The
// reduce DFS (forestReducer.dfs / .dfsNoExtras) walks every reduce path through
// the GSS forest; on a high-ambiguity grammar (haskell) the path count is
// exponential and the DFS runs effectively unbounded — it times out the
// forest-vs-C oracle gate (TestForestVsCOracleParity). The counter resets per
// reduce() call and counts link iterations (each is a recursion step or a
// coalescing visit, where the real cost lives); when it crosses this cap the
// reducer sets `capped` (sticky for the rest of the parse) and parseForest
// declines, so a pathological input falls back to the production parser instead
// of hanging. A normal reduce enumerates a handful of paths, orders of magnitude
// under this cap, so it never fires for the allowlisted forest languages.
var forestReduceStepCap = 1 << 16

// forestReduceVisitCap bounds reducer output visits per token. It covers the
// specialized no-extra reducers that bypass forestReduceStepCap's DFS step
// counter, so repeated low-child-count reductions cannot enqueue unbounded
// materialization work before the forest declines to production.
const forestReduceVisitCap = 1 << 15

// forestWorklistVisitCap bounds dirty-node worklist churn for one token. A
// healthy forest worklist drains quickly; crossing this cap means reductions are
// cycling faster than the token stream can advance, so the fast path declines.
const forestWorklistVisitCap = 1 << 15

type forestReducer struct {
	path         []stackEntry
	rev          []stackEntry
	emitChildren []stackEntry
	emits        []forestReduceEmit
	steps        int
	visitCount   int
	visitCap     int
	capReason    string
	capped       bool
}

type forestReduceEmit struct {
	childStart int
	childCount int
	childScore int
	popTo      *gssForestNode
	noExtras   bool
}

// reduce walks back to childCount non-extra subtrees ending at node, including
// any interior extras in the window (they do not count toward childCount —
// mirroring reduceWindowFromGSS), and calls visit once per surviving path with
// the children left-to-right and popTo = the predecessor the reduction pops to.
func (fr *forestReducer) reduce(node *gssForestNode, childCount int, visit forestReduceVisitor) {
	if node == nil {
		return
	}
	fr.steps = 0 // per-reduce budget; fr.capped stays sticky for the whole parse
	fr.emitChildren = fr.emitChildren[:0]
	fr.emits = fr.emits[:0]
	if perfCountersEnabled {
		perfRecordForestReduceCall(childCount)
	}
	if childCount == 0 {
		if perfCountersEnabled {
			perfRecordForestReduceZero()
		}
		fr.visit(nil, 0, node, true, "zero", visit)
		fr.flushVisits(visit)
		return
	}
	if childCount == 1 && fr.reduceOneNoExtras(node, visit) {
		fr.flushVisits(visit)
		return
	}
	if fr.reduceLinearNoExtras(node, childCount, visit) {
		if perfCountersEnabled {
			perfRecordForestReduceLinearNoExtras(childCount)
		}
		fr.flushVisits(visit)
		return
	}
	if fr.reduceForkedLinearNoExtras(node, childCount, visit) {
		if perfCountersEnabled {
			perfRecordForestReduceLinearNoExtras(childCount)
		}
		fr.flushVisits(visit)
		return
	}
	if fr.reduceForkedLinearSinglePath(node, childCount, visit) {
		fr.flushVisits(visit)
		return
	}
	if fr.reduceLinearForkedSinglePath(node, childCount, visit) {
		fr.flushVisits(visit)
		return
	}
	if fr.reduceLinearSinglePath(node, childCount, visit) {
		fr.flushVisits(visit)
		return
	}
	if fr.reduceNoExtrasDFS(node, childCount, visit) {
		fr.flushVisits(visit)
		return
	}
	if perfCountersEnabled {
		perfRecordForestReduceDFS()
	}
	fr.path = fr.path[:0]
	fr.dfs(node, childCount, 0, visit)
	fr.flushVisits(visit)
}

func (fr *forestReducer) reduceOneNoExtras(node *gssForestNode, visit forestReduceVisitor) bool {
	if node == nil {
		return true
	}
	if cap(fr.rev) < 1 {
		fr.rev = make([]stackEntry, 1)
	} else {
		fr.rev = fr.rev[:1]
	}
	for i := range node.links {
		if forestStackEntryIsExtra(node.links[i].subtree) {
			return false
		}
	}
	links := node.links
	for i := range links {
		if fr.capped {
			return true
		}
		link := &links[i]
		fr.rev[0] = link.subtree
		fr.visit(fr.rev, link.score, link.prev, true, "oneNoExtras", visit)
	}
	return true
}

func (fr *forestReducer) reduceLinearNoExtras(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 {
		return false
	}
	if cap(fr.rev) < childCount {
		fr.rev = make([]stackEntry, childCount)
	} else {
		fr.rev = fr.rev[:childCount]
	}
	cur := node
	score := 0
	for i := childCount - 1; i >= 0; i-- {
		if cur == nil || len(cur.links) != 1 {
			return false
		}
		link := &cur.links[0]
		if forestStackEntryIsExtra(link.subtree) {
			return false
		}
		fr.rev[i] = link.subtree
		score += link.score
		cur = link.prev
	}
	fr.visit(fr.rev, score, cur, true, "linearNoExtras", visit)
	return true
}

func (fr *forestReducer) reduceForkedLinearNoExtras(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 1 || node == nil || len(node.links) <= 1 {
		return false
	}
	links := node.links
	for i := range links {
		link := &links[i]
		if forestStackEntryIsExtra(link.subtree) {
			return false
		}
		cur := link.prev
		for child := childCount - 2; child >= 0; child-- {
			if cur == nil || len(cur.links) != 1 {
				return false
			}
			next := &cur.links[0]
			if forestStackEntryIsExtra(next.subtree) {
				return false
			}
			cur = next.prev
		}
	}
	if cap(fr.rev) < childCount {
		fr.rev = make([]stackEntry, childCount)
	} else {
		fr.rev = fr.rev[:childCount]
	}
	for i := range links {
		if fr.capped {
			return true
		}
		link := &links[i]
		score := link.score
		fr.rev[childCount-1] = link.subtree
		cur := link.prev
		for child := childCount - 2; child >= 0; child-- {
			next := &cur.links[0]
			fr.rev[child] = next.subtree
			score += next.score
			cur = next.prev
		}
		fr.visit(fr.rev, score, cur, true, "forkedLinearNoExtras", visit)
	}
	return true
}

func (fr *forestReducer) reduceForkedLinearSinglePath(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 || node == nil || len(node.links) <= 1 {
		return false
	}
	maxPathLen := 0
	links := node.links
	for i := range links {
		pathLen, ok := validateLinearReducePathFromLink(&links[i], childCount)
		if !ok {
			return false
		}
		if pathLen > maxPathLen {
			maxPathLen = pathLen
		}
	}
	if cap(fr.path) < maxPathLen {
		fr.path = make([]stackEntry, 0, maxPathLen)
	}
	if cap(fr.rev) < maxPathLen {
		fr.rev = make([]stackEntry, maxPathLen)
	}
	for i := range links {
		if fr.capped {
			return true
		}
		fr.emitLinearReducePathFromLink(&links[i], childCount, visit)
	}
	return true
}

func validateLinearReducePathFromLink(link *gssLink, childCount int) (int, bool) {
	remaining := childCount
	pathLen := 0
	for {
		pathLen++
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				return pathLen, true
			}
		}
		cur := link.prev
		if cur == nil || len(cur.links) != 1 {
			return 0, false
		}
		link = &cur.links[0]
	}
}

func (fr *forestReducer) emitLinearReducePathFromLink(link *gssLink, childCount int, visit forestReduceVisitor) {
	fr.path = fr.path[:0]
	remaining := childCount
	score := 0
	for {
		fr.path = append(fr.path, link.subtree)
		score += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				fr.rev = fr.rev[:len(fr.path)]
				for i := range fr.path {
					fr.rev[len(fr.path)-1-i] = fr.path[i]
				}
				fr.visit(fr.rev, score, link.prev, false, "linearFromLink", visit)
				return
			}
		}
		link = &link.prev.links[0]
	}
}

func (fr *forestReducer) reduceLinearForkedSinglePath(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 || node == nil || len(node.links) != 1 {
		return false
	}
	fr.path = fr.path[:0]
	cur := node
	remaining := childCount
	prefixScore := 0
	for cur != nil && len(cur.links) == 1 {
		link := &cur.links[0]
		fr.path = append(fr.path, link.subtree)
		prefixScore += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				return false
			}
		}
		cur = link.prev
	}
	if cur == nil || len(cur.links) <= 1 {
		return false
	}
	prefixLen := len(fr.path)
	maxPathLen := prefixLen
	var branchLens [forestMaxLinksPerNode]int
	links := cur.links
	for i := range links {
		pathLen, ok := validateLinearReducePathFromLink(&links[i], remaining)
		if !ok {
			return false
		}
		branchLens[i] = pathLen
		if prefixLen+pathLen > maxPathLen {
			maxPathLen = prefixLen + pathLen
		}
	}
	if cap(fr.rev) < maxPathLen {
		fr.rev = make([]stackEntry, maxPathLen)
	}
	for i := range links {
		if fr.capped {
			return true
		}
		fr.emitLinearReducePathFromLinkWithPrefix(&links[i], remaining, branchLens[i], prefixLen, prefixScore, visit)
	}
	return true
}

func (fr *forestReducer) emitLinearReducePathFromLinkWithPrefix(link *gssLink, childCount int, branchPathLen, prefixLen int, score int, visit forestReduceVisitor) {
	totalLen := prefixLen + branchPathLen
	fr.rev = fr.rev[:totalLen]
	for i := 0; i < prefixLen; i++ {
		fr.rev[totalLen-1-i] = fr.path[i]
	}
	remaining := childCount
	branchOut := branchPathLen - 1
	for {
		fr.rev[branchOut] = link.subtree
		branchOut--
		score += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				fr.visit(fr.rev, score, link.prev, false, "linearWithPrefix", visit)
				return
			}
		}
		link = &link.prev.links[0]
	}
}

func (fr *forestReducer) reduceLinearSinglePath(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 {
		return false
	}
	fr.path = fr.path[:0]
	cur := node
	remaining := childCount
	score := 0
	for cur != nil {
		if len(cur.links) != 1 {
			return false
		}
		link := &cur.links[0]
		fr.path = append(fr.path, link.subtree)
		score += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				if cap(fr.rev) < len(fr.path) {
					fr.rev = make([]stackEntry, len(fr.path))
				} else {
					fr.rev = fr.rev[:len(fr.path)]
				}
				for i := range fr.path {
					fr.rev[len(fr.path)-1-i] = fr.path[i]
				}
				fr.visit(fr.rev, score, link.prev, false, "linearSinglePath", visit)
				return true
			}
		}
		cur = link.prev
	}
	return false
}

func (fr *forestReducer) reduceNoExtrasDFS(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 || node == nil || int(node.noExtraDepth) < childCount {
		return false
	}
	if cap(fr.rev) < childCount {
		fr.rev = make([]stackEntry, childCount)
	} else {
		fr.rev = fr.rev[:childCount]
	}
	switch childCount {
	case 2:
		fr.dfsNoExtras2(node, 0, visit)
	case 3:
		fr.dfsNoExtras3(node, 0, visit)
	case 4:
		fr.dfsNoExtras4(node, 0, visit)
	default:
		fr.dfsNoExtras(node, childCount, 0, visit)
	}
	return true
}

func (fr *forestReducer) dfsNoExtras2(cur *gssForestNode, score int, visit forestReduceVisitor) {
	links0 := cur.links
	for i := range links0 {
		if fr.capped {
			return
		}
		l0 := &links0[i]
		fr.rev[1] = l0.subtree
		score0 := score + l0.score
		n1 := l0.prev
		links1 := n1.links
		for j := range links1 {
			if fr.capped {
				return
			}
			l1 := &links1[j]
			fr.rev[0] = l1.subtree
			fr.visit(fr.rev, score0+l1.score, l1.prev, true, "dfsNoExtras2", visit)
		}
	}
}

func (fr *forestReducer) dfsNoExtras3(cur *gssForestNode, score int, visit forestReduceVisitor) {
	links0 := cur.links
	for i := range links0 {
		if fr.capped {
			return
		}
		l0 := &links0[i]
		fr.rev[2] = l0.subtree
		score0 := score + l0.score
		n1 := l0.prev
		links1 := n1.links
		for j := range links1 {
			if fr.capped {
				return
			}
			l1 := &links1[j]
			fr.rev[1] = l1.subtree
			score1 := score0 + l1.score
			n2 := l1.prev
			links2 := n2.links
			for k := range links2 {
				if fr.capped {
					return
				}
				l2 := &links2[k]
				fr.rev[0] = l2.subtree
				fr.visit(fr.rev, score1+l2.score, l2.prev, true, "dfsNoExtras3", visit)
			}
		}
	}
}

func (fr *forestReducer) dfsNoExtras4(cur *gssForestNode, score int, visit forestReduceVisitor) {
	links0 := cur.links
	for i := range links0 {
		if fr.capped {
			return
		}
		l0 := &links0[i]
		fr.rev[3] = l0.subtree
		score0 := score + l0.score
		n1 := l0.prev
		links1 := n1.links
		for j := range links1 {
			if fr.capped {
				return
			}
			l1 := &links1[j]
			fr.rev[2] = l1.subtree
			score1 := score0 + l1.score
			n2 := l1.prev
			links2 := n2.links
			for k := range links2 {
				if fr.capped {
					return
				}
				l2 := &links2[k]
				fr.rev[1] = l2.subtree
				score2 := score1 + l2.score
				n3 := l2.prev
				links3 := n3.links
				for m := range links3 {
					if fr.capped {
						return
					}
					l3 := &links3[m]
					fr.rev[0] = l3.subtree
					fr.visit(fr.rev, score2+l3.score, l3.prev, true, "dfsNoExtras4", visit)
				}
			}
		}
	}
}

func (fr *forestReducer) dfsNoExtras(cur *gssForestNode, remaining, score int, visit forestReduceVisitor) {
	out := remaining - 1
	links := cur.links
	for i := range links {
		if fr.capped {
			break
		}
		if fr.steps++; fr.steps > forestReduceStepCap {
			fr.capped = true
			fr.capReason = "reduce-cap"
			break
		}
		link := &links[i]
		fr.rev[out] = link.subtree
		nextScore := score + link.score
		if remaining == 1 {
			fr.visit(fr.rev, nextScore, link.prev, true, "dfsNoExtras", visit)
			continue
		}
		fr.dfsNoExtras(link.prev, remaining-1, nextScore, visit)
	}
}

func (fr *forestReducer) dfs(cur *gssForestNode, remaining, score int, visit forestReduceVisitor) {
	if cur == nil {
		return
	}
	mark := len(fr.path)
	links := cur.links
	for i := range links {
		if fr.capped {
			break
		}
		if fr.steps++; fr.steps > forestReduceStepCap {
			fr.capped = true
			fr.capReason = "reduce-cap"
			break
		}
		link := &links[i]
		extra := forestStackEntryIsExtra(link.subtree)
		if perfCountersEnabled {
			perfRecordForestReduceDFSStep(len(cur.links), extra)
		}
		fr.path = append(fr.path[:mark], link.subtree)
		rem := remaining
		if !extra {
			rem--
		}
		if rem == 0 {
			if perfCountersEnabled {
				perfRecordForestReduceDFSVisit(len(fr.path))
			}
			if cap(fr.rev) < len(fr.path) {
				fr.rev = make([]stackEntry, len(fr.path))
			} else {
				fr.rev = fr.rev[:len(fr.path)]
			}
			for j := range fr.path {
				fr.rev[len(fr.path)-1-j] = fr.path[j]
			}
			fr.visit(fr.rev, score+link.score, link.prev, false, "dfs", visit)
			continue
		}
		fr.dfs(link.prev, rem, score+link.score, visit)
	}
	fr.path = fr.path[:mark]
}

func (fr *forestReducer) visit(children []stackEntry, childScore int, popTo *gssForestNode, noExtras bool, route string, visit forestReduceVisitor) {
	if fr.capped {
		return
	}
	fr.visitCount++
	if fr.visitCap > 0 && fr.visitCount > fr.visitCap {
		fr.capped = true
		fr.capReason = "reduce-visit-cap"
		return
	}
	start := len(fr.emitChildren)
	fr.emitChildren = append(fr.emitChildren, children...)
	fr.emits = append(fr.emits, forestReduceEmit{
		childStart: start,
		childCount: len(children),
		childScore: childScore,
		popTo:      popTo,
		noExtras:   noExtras,
	})
}

func (fr *forestReducer) flushVisits(visit forestReduceVisitor) {
	for i := range fr.emits {
		if fr.capped {
			return
		}
		emit := fr.emits[i]
		children := fr.emitChildren[emit.childStart : emit.childStart+emit.childCount]
		visit(children, emit.childScore, emit.popTo, emit.noExtras)
	}
}

func forestStackEntryIsExtra(e stackEntry) bool {
	return e.kind == stackEntryKindNode && e.node != nil && (*Node)(e.node).isExtra()
}
