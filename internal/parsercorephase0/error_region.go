package parsercorephase0

import "errors"

// ---------------------------------------------------------------------------
// error_region.go -- campaign v7 tranche B3 stage S3: native strategy-2
// recovery (error-region absorb and condense-resume) over compact headers.
//
// THE PRODUCTION GO PORT IS THE EXECUTABLE SPECIFICATION for the mechanism
// (spec.compact-recovery-ownership.v1, section 6): parser_recover_c.go's
// cAbsorbTokenIntoError (:3757) and cCondenseAndResume/cRecoverToState
// (:3948,:3596), themselves faithful ports of tree-sitter C's
// ts_parser__recover's strategy-2 tail and ts_parser__recover_to_state. The
// pinned C oracle is the parity arbiter (decision 0007): where the production
// Go port's own construction diverges from the C oracle, this port follows
// the oracle, not the Go port's literal bit -- see ErrorRegionLeaf's doc
// comment for the one confirmed instance (finding
// production-recovery-structural-divergence).
//
// The open error region itself is NOT a record in this file: it lives on the
// driver's header (parsercore_phase0_driver.go), a plain Go slice of already-
// published SubtreeIDs plus span bytes, exactly like glrStack.cRec.openErr
// lives on the stack, not in production's node arena (design section 4,
// stage S2 doc comment, restated for S3). This file publishes the pieces the
// header accumulates: one leaf per absorbed terminal (eager, cheap, and
// itself immutable once published, matching every other compact subtree),
// and -- once the driver decides to resume -- the ERROR container plus the
// new head that carries it, both in one call.
// ---------------------------------------------------------------------------

// ErrorRegionSymbol is tree-sitter's built-in ERROR symbol (ts_builtin_sym_error),
// fixed across every grammar. It matches RecoveryErrorSymbol (recovery_cost.go)
// and the root package's own errorSymbol constant (parser_api.go:997); all
// three names exist because each file documents the constant against its own
// C source citation, not because the value can vary.
const ErrorRegionSymbol Symbol = 65535

// ErrorRegionLeaf publishes one absorbed terminal into the compact subtree
// arena for a driver-owned open error region.
//
// HasError is deliberately never stored or forced true here, even when
// symbol is itself ErrorRegionSymbol (an unlexable byte the token source
// could not classify as any grammar terminal). This mirrors the pinned C
// oracle exactly, and deliberately does NOT mirror production Go's
// cAbsorbTokenIntoError, which calls leaf.setHasError(true) unconditionally
// on every absorbed leaf (parser_recover_c.go:3766) -- a confirmed,
// receipted production/C divergence (finding
// production-recovery-structural-divergence; verified byte-for-byte against
// the locked C oracle in Docker for all ten html_erroneous_end_tag
// witnesses: every remaining structural gap after this port's mechanism ran
// was exactly one absorbed leaf's own HasError flag reading true in Go
// where the C oracle reads false). The wrapping ERROR container (see
// ErrorRegionNode) is materialization's sole per-node HasError trigger for
// this region; ordinary ancestor propagation (populateParentNode, tree.go)
// already ORs a true child flag up through every enclosing reduce with zero
// additional code, so leaving every leaf false here is what makes that
// propagation land in exactly the right place -- on the container, and only
// the container, exactly like the C oracle.
func (c *Core) ErrorRegionLeaf(symbol Symbol, startByte, endByte uint32, extra bool) (id SubtreeID, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	// appendSubtree, not appendSubtreeRecord directly: an absorbed error-
	// region terminal is published outside the grammar-table-authenticated
	// shift seam (shiftClassifiedUncheckpointed and friends), so
	// metadataConstructionAuthenticated must clear the same way any other
	// generic/diagnostic publication does (core.go's own doc comment on the
	// flag) -- this is not just the lexical-provenance ratchet
	// (metadata_auth_ratchet_test.go), it is the correct signal: an S3
	// region's payloads earn full materialization-time metadata validation
	// like any other unauthenticated construction, rather than skipping it
	// on the assumption a grammar-table seam already checked them.
	return c.appendSubtree(subtreeRecord{
		symbol: symbol, startByte: startByte, endByte: endByte, extra: extra, terminal: true,
	}, nil, nil, nil)
}

// ErrorRegionResume publishes the ERROR container over the region's
// accumulated children (parity obligation 1, design section 6: spans start
// at the first real token; span writes mirror cSetNodeSpan,
// parser_recover_c.go:3585 -- startByte/endByte come from the caller's own
// tracked region bounds, not recomputed here) and condenses it onto
// preErrorHead at preErrorState, publishing the resumed head strategy 2's
// absorb-then-resume step produces (the compact equivalent of production's
// pushStackNode(fork, goal, errNode, ...), cRecoverToState,
// parser_recover_c.go:3743). The resumed head is keyed as a shift boundary
// (shiftedBoundaryKey), matching the shape of every other terminal-carrying
// attachment onto a head (scheduler_owned.go's shiftClassifiedUncheckpointed):
// the ERROR container is, from the boundary index's point of view, one more
// unit attached at this state and byte position, not a reduction replacing
// existing children.
//
// children must already be published (via ErrorRegionLeaf or, for the rare
// case production's cRecoverToState splices an existing closed subtree in
// front, an already-live SubtreeID the caller owns). len(children)==0 is
// rejected: an ERROR region with no absorbed content has nothing to resume.
//
// KNOWN UNVERIFIED DIVERGENCE (adversarial review, MINOR item b): every
// entry in children is always wrapped INSIDE the published ERROR container.
// Production's own cRecoverToState (parser_recover_c.go:3638-3644, "Split
// trailing extras (re-pushed after the ERROR per C)") instead splits its
// popped node sequence at the last non-extra entry and keeps any TRAILING
// extras (comments, whitespace absorbed at the tail of the region) OUTSIDE
// the ERROR node, re-pushed onto the stack as siblings after it, mirroring
// tree-sitter C's own ts_parser__recover: an error region's trailing extras
// are not semantically part of the error content, so C does not nest them
// under it.
//
// This port does not implement that split. It is unreached, not simply
// untested, for every grammar S3 is certified against today: html's own
// comment token always resumes via an ordinary EXTRA SHIFT onto the
// pre-error head (s3TokenIsExtraShift's own resume-action check,
// parsercore_phase0_driver.go) before it would ever need absorbing as one
// of this region's children in the first place -- so no committed
// html_erroneous_end_tag witness ever calls this function with a trailing
// extra in children, and CompactStrategy2ErrorRegionCertified is html-only
// (grammars/runtime_profiles.go). A grammar whose lexer can produce a
// trailing extra inside an absorbed error region -- rather than always
// resuming before absorbing one -- would need this split reproduced (or an
// explicit decline guarding it) before certifying; do not assume this gap
// is closed by extrapolation from html's own witnesses.
func (c *Core) errorRegionResumeUncheckpointed(
	preErrorHead Head,
	preErrorState StateID,
	startByte, endByte uint32,
	children []SubtreeID,
	cost ReductionOutputCostFunc,
) (out Head, err error) {
	if len(children) == 0 {
		return Head{}, errors.New("parser-core phase zero: error region resume has no absorbed children")
	}
	if _, err := c.node(preErrorHead.Node); err != nil {
		return Head{}, err
	}
	// extra:true (mirrors cRecoverToState's errNode.setExtra(true),
	// parser_recover_c.go) is load-bearing, not cosmetic: popPaths (core.go)
	// skips extra payloads when counting a production's structural
	// ChildCount, exactly like an ordinary whitespace/comment extra between
	// two real children. Without it, an ordinary enclosing reduce (e.g.
	// end_tag's '</' tag_name '>') would count this region as one of its own
	// structural children, popping one link short and landing its GOTO
	// lookup on the wrong ancestor state entirely (observed directly: "no
	// goto from state N for reduced symbol" once this region sits between
	// two real children of a fixed-arity production).
	// appendSubtree, not appendSubtreeRecord directly: same rationale as
	// ErrorRegionLeaf above -- the ERROR container is generic/diagnostic-
	// seam publication, not grammar-table-authenticated reduction, so it
	// must clear metadataConstructionAuthenticated the same way.
	payload, err := c.appendSubtree(subtreeRecord{
		symbol: ErrorRegionSymbol, startByte: startByte, endByte: endByte, extra: true,
	}, children, nil, nil)
	if err != nil {
		return Head{}, err
	}
	in := linkInput{prev: preErrorHead.Node, payload: payload}
	if cost != nil {
		storedErrorCost, costErr := cost(in.prev, in.payload)
		if costErr != nil {
			return Head{}, costErr
		}
		in.storedErrorCost = storedErrorCost
		in.hasStoredErrorCost = true
	}
	return c.condense(c.shiftedBoundaryKey(preErrorState, endByte), in)
}

// ErrorRegionResume publishes one ERROR container over the region's existing
// children. It keeps the historical no-cost wrapper for clean callers.
func (c *Core) ErrorRegionResume(preErrorHead Head, preErrorState StateID, startByte, endByte uint32, children []SubtreeID) (out Head, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	return c.errorRegionResumeUncheckpointed(preErrorHead, preErrorState, startByte, endByte, children, nil)
}

// ErrorRegionResumeWithCost publishes the authenticated cumulative recovery
// cost before the new ERROR boundary can condense or publish.
func (c *Core) ErrorRegionResumeWithCost(
	preErrorHead Head,
	preErrorState StateID,
	startByte, endByte uint32,
	children []SubtreeID,
	cost ReductionOutputCostFunc,
) (out Head, err error) {
	if cost == nil {
		return Head{}, errors.New("parser-core phase zero: error-region cost callback is required")
	}
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	return c.errorRegionResumeUncheckpointed(preErrorHead, preErrorState, startByte, endByte, children, cost)
}
