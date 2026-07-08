package gotreesitter

func conflictPolicyChoice(lang *Language, tok Token, currentState StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil || len(lang.ConflictPolicies) == 0 {
		return ParseAction{}, false
	}
	for i := range lang.ConflictPolicies {
		policy := &lang.ConflictPolicies[i]
		if policy.State != currentState || policy.Lookahead != tok.Symbol {
			continue
		}
		if chosen, ok := conflictPolicyChoiceForPolicy(lang, policy, actions); ok {
			return chosen, true
		}
	}
	return ParseAction{}, false
}

func conflictPolicyChoiceForPolicy(lang *Language, policy *ConflictPolicy, actions []ParseAction) (ParseAction, bool) {
	if lang == nil || policy == nil {
		return ParseAction{}, false
	}
	if !conflictPolicyReducesMatch(policy, actions) {
		return ParseAction{}, false
	}
	switch policy.Kind {
	case ConflictPolicyRepetitionShift:
		return repetitionShiftConflictChoice(actions)
	case ConflictPolicyShift:
		if len(policy.ReduceSymbols) == 0 {
			return ParseAction{}, false
		}
		return singleShiftConflictChoice(actions)
	default:
		return ParseAction{}, false
	}
}

func conflictPolicyReducesMatch(policy *ConflictPolicy, actions []ParseAction) bool {
	if policy == nil || len(policy.ReduceSymbols) == 0 {
		return true
	}
	foundReduce := false
	for _, act := range actions {
		if act.Type != ParseActionReduce {
			continue
		}
		foundReduce = true
		if !conflictPolicySymbolAllowed(policy.ReduceSymbols, act.Symbol) {
			return false
		}
	}
	return foundReduce
}

func conflictPolicySymbolAllowed(allowed []Symbol, sym Symbol) bool {
	for _, candidate := range allowed {
		if candidate == sym {
			return true
		}
	}
	return false
}

func singleShiftConflictChoice(actions []ParseAction) (ParseAction, bool) {
	if len(actions) < 2 {
		return ParseAction{}, false
	}
	var shift ParseAction
	shiftFound := false
	reduceFound := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			if act.Extra || act.Repetition {
				return ParseAction{}, false
			}
			if shiftFound {
				return ParseAction{}, false
			}
			shift = act
			shiftFound = true
		case ParseActionReduce:
			reduceFound = true
		default:
			return ParseAction{}, false
		}
	}
	if !shiftFound || !reduceFound {
		return ParseAction{}, false
	}
	return shift, true
}

// cRepetitionSkipOptOut lists languages excluded from the global C
// repetition-skip fold (cRepetitionSkipConflictChoice). The list starts
// empty by design: C applies the rule to every grammar, so an entry here
// requires concrete evidence (a failing shape test or a C-oracle parity
// divergence), cited in a comment next to the entry.
var cRepetitionSkipOptOut = map[string]bool{
	// dart: correctness-cap language whose SELECTED trees are cap-sensitive.
	// A/B on the dart corpus (wave-2b, 2026-07-07): with the fold on,
	// dev/a11y_assessments/lib/use_cases/back_button.dart drops from 75/88
	// C-oracle-matching shape chunks to 62/88 (tree-sitter CLI oracle,
	// dart-81638dbbdb76) — pre-error fold choices reshape the downstream
	// recovery wreckage that dart's capped branch selection depends on.
	// dart's error-free giant-list wins (e.g.
	// generated_material_localizations.dart 4.4s->0.9s) are forfeited until
	// dart's selection fidelity is C-clean; re-test before removing.
	"dart": true,
	// c: error-dense real corpus (12/15 spot files carry ERROR) whose
	// recovered shapes are sensitive to pre-error stack layout. A/B
	// (wave-2b, 2026-07-07): with the fold on, archive.c drops from
	// 1354/2040 to 1061/2040 C-oracle-matching shape chunks (tree-sitter
	// CLI oracle, c-ae19b676b13b) even with the legacy c helper restored —
	// eager folds before the first error leave a different stack for
	// recovery to chew on. Clean-lineage-only wins don't outweigh the
	// recovery-shape regression; revisit when c recovery is C-clean.
	"c": true,
	// haskell: the engine-wide fold KILLS a previously clean parse — A/B
	// (wave-2b, 2026-07-07) flips SetupHooks.hs (Cabal-hooks) from
	// accepted/error-free (151ms) to no_stacks_alive at byte ~17k: at some
	// haskell state outside the two proven fold states (9609/10984, see
	// haskellRepeatBoundaryConflictChoice) the repetition-marked shift is
	// NOT re-reachable after the fold, so the fold's losslessness invariant
	// does not hold on this table (huge external-scanner grammar; table
	// fidelity suspect). The scoped 2-state fold helper stays as the proven
	// subset; the memory_budget wins the global fold showed (LicenseId.hs,
	// Licenses.hs ~1.9s->~0.1s) are forfeited until the offending state is
	// isolated.
	"haskell": true,
	// c_sharp: the fold flips a clean parse to ERROR — A/B (wave-2b,
	// 2026-07-07): DeployCommandTests.cs (Bicep) goes from accepted/clean
	// (5.7s, legacy kind-scoped shift helper) to accepted WITH error
	// (0.45s) under the fold. Same failure family as haskell: a fold at
	// some state outside the helper's block/declaration_list kinds is not
	// lossless on this table. c_sharp keeps its kind-scoped helper
	// (csharpRepetitionShiftConflictChoice) and the forest fast path it
	// already dispatches to by default.
	"c_sharp": true,
}

// cRepetitionSkipConflictChoice is C tree-sitter's dispatch rule for
// {repetition SHIFT, REDUCE} conflicts, applied engine-wide. C's action loop
// (parser-repos/tree-sitter/lib/src/parser.c:1625, ts_parser__advance)
// executes `if (action.shift.repetition) break;` — the repetition shift is
// NEVER taken at a conflict entry. Every REDUCE runs (each spawning a stack
// version), and when there is exactly one REDUCE the surviving version is
// renumbered onto the current one and the SAME lookahead re-dispatches from
// the folded state: a deterministic fold, no fork at all. The fold is
// lossless because a repetition-marked shift is by construction re-reachable
// from the post-reduce goto state — after folding, the same lookahead either
// shifts as the list continuation or closes the list, so both futures
// survive. Forking instead (our previous default) builds an O(n) flat spine
// plus a per-boundary frontier refold — O(n^2) on any long repeated list.
//
// Scope: exactly-1 REDUCE + exactly-1 repetition SHIFT (the shape where C's
// semantics are a deterministic fold; with 2+ REDUCEs C itself forks the
// reduce versions, which our GLR fork approximates). Gated per-stack to
// error-free lineages by the sticky !cEverErrored discipline established by
// the recovery-wreckage counterexamples from the original php comma-list fold:
// inside recovery wreckage the
// repetition-shift arm can be the branch the recovery cost competition
// keeps, so wreckage-descended lineages keep the ordinary GLR fork.
func (p *Parser) cRepetitionSkipConflictChoice(s *glrStack, actions []ParseAction) (ParseAction, bool) {
	if p == nil || p.language == nil || cRepetitionSkipOptOut[p.language.Name] {
		return ParseAction{}, false
	}
	if s == nil || s.cRec != nil || s.cPaused || s.cRecoverMissingGroup != nil || s.cEverErrored {
		return ParseAction{}, false
	}
	return singleReduceAgainstRepetitionShiftConflictChoice(actions)
}

func (p *Parser) cRepetitionSkipForestConflictChoice(recoverActive bool, actions []ParseAction) (ParseAction, bool) {
	if p == nil || p.language == nil || cRepetitionSkipOptOut[p.language.Name] || recoverActive {
		return ParseAction{}, false
	}
	return singleReduceAgainstRepetitionShiftConflictChoice(actions)
}

func (p *Parser) deterministicConflictChoiceForDispatch(source []byte, s *glrStack, tok Token, currentState StateID, actions []ParseAction, maxStacksSeen int, reuse *reuseCursor) (ParseAction, bool) {
	if p == nil || p.language == nil {
		return ParseAction{}, false
	}
	if p.language.Name == "gomod" {
		if next, ok := gomodRepetitionShiftConflictChoice(p.language, currentState, actions); ok {
			return next, true
		}
	}
	if reuse != nil {
		return ParseAction{}, false
	}
	if chosen, ok := conflictPolicyChoice(p.language, tok, currentState, actions); ok {
		return chosen, true
	}
	// C's global repetition-skip fold. Runs after explicit generated
	// ConflictPolicies (per-state directives outrank the default) but before
	// the grammargen repeat-boundary veto and the per-language arms: the veto
	// exists to keep hand-written SHIFT-preferring shortcuts away from
	// grammars they were never tuned for, whereas this fold is C's own
	// dispatch semantics and applies to every grammar whose tables carry
	// repetition-marked shifts.
	if next, ok := p.cRepetitionSkipConflictChoice(s, actions); ok {
		return next, true
	}
	if generatedRepeatBoundaryConflict(p.language, actions) {
		return ParseAction{}, false
	}
	if p.language.GeneratedByGrammargen {
		return ParseAction{}, false
	}
	// The per-language repetition-boundary arms that used to populate this
	// switch (java/c_sharp/c/rust/typescript/tsx/javascript/python/r/php/perl/
	// sql/dart/hcl/haskell/make/d/clojure/awk/scheme/dot) are retired:
	// cRepetitionSkipConflictChoice above makes the C-faithful choice for the
	// exact {1 repetition-SHIFT + 1 REDUCE} shape those helpers were scoped
	// to. Table-shape analysis (cmd/repskip_shapes) shows every state those
	// helpers covered carries ONLY that exact shape (zero multi-reduce
	// entries), so on error-free lineages the global fold fully shadows them;
	// on wreckage lineages (cEverErrored and friends) the C-faithful behavior
	// is the GLR fork feeding the recovery cost competition — the php
	// TransportResponseTrait lesson — not a deterministic commitment, so the
	// helpers' unconditional firing there was a latent recovery-shape hazard,
	// not coverage worth keeping. The helper functions remain (with their
	// unit tests) as documentation of the profiled states until integration
	// deletes them. Arms that survive below are NOT repetition-boundary
	// policies (or, for gomod above, run where the global fold does not).
	var chosen ParseAction
	var ok bool
	switch p.language.Name {
	case "java":
		// Non-repetition: `case A ->` switch-label disambiguation via a
		// goto/action-table probe on the reduce's landing state.
		chosen, ok = p.javaSwitchArrowConflictChoice(s, tok, actions)
	case "dart":
		// KEPT, and dart is also in cRepetitionSkipOptOut: dart's capped
		// branch selection is tuned around this helper's deterministic
		// repetition shift, INCLUDING on wreckage lineages where the global
		// fold never applies. A/B evidence (wave-2b, 2026-07-07): removing
		// the arm alone flips app_bar.dart's recovered tree and pushes
		// generated_material_localizations.dart from accepted to
		// memory_budget; the global fold instead of it drops
		// back_button.dart from 75/88 to 62/88 C-oracle shape chunks. This
		// is non-C dispatch policy papering over dart's recovery-selection
		// gaps — revisit when dart is C-recovery-clean.
		chosen, ok = dartRepetitionShiftConflictChoice(p.language, currentState, actions)
	case "c":
		// KEPT, and c is also in cRepetitionSkipOptOut: the C-language
		// corpus is error-dense and its recovered shapes depend on this
		// helper's deterministic translation_unit_repeat1 /
		// preproc_if_repeat1 shift on all lineages. A/B evidence (wave-2b,
		// 2026-07-07): retiring the arm dropped archive.c from 1354/2040 to
		// 1061/2040 C-oracle-matching shape chunks, and the fold alone kept
		// it there (pre-error folds reshape the stack recovery chews on).
		// Non-C dispatch policy stabilizing recovery selection; revisit
		// when c recovery is C-clean.
		chosen, ok = cRepetitionShiftConflictChoice(p.language, actions)
	case "haskell":
		// KEPT, and haskell is also in cRepetitionSkipOptOut: this is the
		// proven-safe scoped subset of the fold (REDUCE at states
		// 9609/10984 via singleReduceAgainstRepetitionShiftConflictChoice —
		// the same choice the global rule would make there). The engine-wide
		// fold dead-ends SetupHooks.hs (see the opt-out entry), so haskell
		// keeps only these two certified states.
		chosen, ok = haskellRepeatBoundaryConflictChoice(p.language, currentState, actions)
	case "c_sharp":
		// KEPT, and c_sharp is also in cRepetitionSkipOptOut: the
		// engine-wide fold flips DeployCommandTests.cs from clean to ERROR
		// (see the opt-out entry), so c_sharp keeps the kind-scoped
		// block/declaration_list repetition shift that the designer-block
		// boundedness tests were built around.
		chosen, ok = csharpRepetitionShiftConflictChoice(p.language, tok, actions)
	case "swift":
		// Non-repetition: brace/type-expression and navigable-type reduce
		// selection between distinct nonterminal interpretations.
		chosen, ok = swiftBraceTypeExpressionConflictChoice(p.language, tok, currentState, actions)
	case "kotlin":
		// Non-repetition: suppresses the bundled table's spurious bodiless
		// object_literal reduction (issue #93).
		chosen, ok = kotlinObjectLiteralConflictChoice(p.language, actions)
	case "erlang":
		// Non-repetition: macro invocation args shift (explicitly excludes
		// repetition shifts).
		chosen, ok = erlangMacroCallExprConflictChoice(p.language, actions)
	}
	return chosen, ok
}

func generatedRepeatBoundaryConflict(lang *Language, actions []ParseAction) bool {
	if lang == nil || len(actions) < 2 {
		return false
	}
	// The repeat-boundary rejection exists so grammargen-generated grammars
	// without an explicit ConflictPolicy fork instead of trusting hand-written
	// per-language shortcuts that were never tuned for them. Embedded blobs
	// (c_sharp, java, c, ...) get GeneratedRepeatAux retrofitted by
	// InferGeneratedRepeatAuxMetadata (load_language.go / embedded_loader.go),
	// but their repeat-boundary conflicts are exactly what the per-language
	// deterministic choices below were written for; rejecting here disables
	// those choices and the GLR loop forks on every repetition boundary
	// (C# designer-style blocks grew live stacks linearly with input size:
	// MaxStacksSeen 2064 at 300 statements, arena exhaustion, never accepts).
	// Scope the rejection to languages that actually rely on generated
	// policies. For grammargen languages this is a no-op: both call sites
	// (deterministicConflictChoiceForDispatch below, forestResolveConflict in
	// parser.go) check GeneratedByGrammargen immediately after this predicate
	// and bail out identically.
	if !lang.GeneratedByGrammargen && len(lang.ConflictPolicies) == 0 {
		return false
	}
	shiftFound := false
	generatedReduceFound := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			if !act.Repetition || act.Extra || shiftFound {
				return false
			}
			shiftFound = true
		case ParseActionReduce:
			if languageSymbolIsGeneratedRepeatAux(lang, act.Symbol) {
				generatedReduceFound = true
			}
		default:
			return false
		}
	}
	return shiftFound && generatedReduceFound
}

func languageSymbolIsGeneratedRepeatAux(lang *Language, sym Symbol) bool {
	if lang == nil {
		return false
	}
	idx := int(sym)
	if idx < 0 || idx >= len(lang.SymbolMetadata) {
		return false
	}
	return lang.SymbolMetadata[idx].GeneratedRepeatAux
}
