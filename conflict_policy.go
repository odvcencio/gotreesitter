package gotreesitter

func conflictPolicyChoice(lang *Language, tok Token, currentState StateID, actions []ParseAction) (ParseAction, bool) {
	return conflictPolicyChoiceForContext(lang, nil, false, tok, currentState, actions)
}

func conflictPolicyChoiceForDispatch(lang *Language, stack *glrStack, tok Token, currentState StateID, actions []ParseAction) (ParseAction, bool) {
	return conflictPolicyChoiceForContext(lang, stack, true, tok, currentState, actions)
}

func conflictPolicyChoiceForContext(lang *Language, stack *glrStack, allowRecovered bool, tok Token, currentState StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil || len(lang.ConflictPolicies) == 0 {
		return ParseAction{}, false
	}
	for i := range lang.ConflictPolicies {
		policy := &lang.ConflictPolicies[i]
		if policy.State != currentState && policy.State != ConflictPolicyAnyState {
			continue
		}
		if policy.Lookahead != tok.Symbol && policy.Lookahead != ConflictPolicyAnyLookahead {
			continue
		}
		if policy.Kind == ConflictPolicyRecoveredRepetitionReduce {
			if !allowRecovered || stack == nil || !stack.cEverErrored ||
				stack.cRec != nil || stack.cPaused || stack.cRecoverMissingGroup != nil ||
				len(policy.ReduceSymbols) == 0 || !conflictPolicyReducesMatch(policy, actions) {
				continue
			}
			if chosen, ok := singleReduceAgainstRepetitionShiftConflictChoice(actions); ok {
				return chosen, true
			}
			continue
		}
		if chosen, ok := conflictPolicyChoiceForPolicy(lang, policy, actions); ok {
			return chosen, true
		}
	}
	return ParseAction{}, false
}

// declaredReduceReduceConflictPolicyChoice scans only for
// ConflictPolicyDeclaredReduceReduceHighestSymbol rows, independent of the
// general ConflictPolicies dispatch above. It is safe to call regardless of
// stack or reuse context: the fold reads nothing but the row's own action
// list, so it always reproduces the choice a fresh parse would make at this
// exact (state, lookahead).
func declaredReduceReduceConflictPolicyChoice(lang *Language, currentState StateID, tok Token, actions []ParseAction) (ParseAction, bool) {
	if lang == nil || len(lang.ConflictPolicies) == 0 {
		return ParseAction{}, false
	}
	for i := range lang.ConflictPolicies {
		policy := &lang.ConflictPolicies[i]
		if policy.Kind != ConflictPolicyDeclaredReduceReduceHighestSymbol {
			continue
		}
		if policy.State != currentState && policy.State != ConflictPolicyAnyState {
			continue
		}
		if policy.Lookahead != tok.Symbol && policy.Lookahead != ConflictPolicyAnyLookahead {
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
	case ConflictPolicyRepetitionReduce:
		if len(policy.ReduceSymbols) == 0 {
			return ParseAction{}, false
		}
		return singleReduceAgainstRepetitionShiftConflictChoice(actions)
	case ConflictPolicyDeclaredReduceReduceHighestSymbol:
		if len(policy.ReduceSymbols) < 2 {
			return ParseAction{}, false
		}
		return declaredReduceReduceHighestSymbolConflictChoice(actions)
	default:
		return ParseAction{}, false
	}
}

// declaredReduceReduceHighestSymbolConflictChoice folds a row whose actions
// are all plain REDUCE (no shift, no extra, no repetition) by keeping the
// action that reduces the highest-numbered symbol, and only when every
// competing action already carries equal dynamic precedence. A grammar
// author's explicit prec.dynamic() difference must keep deciding through the
// existing dynamic-precedence tie-break; this fold only replaces the
// otherwise-arbitrary default for rows where the table carries no
// precedence signal at all. See ConflictPolicyDeclaredReduceReduceHighestSymbol
// for the C mechanism this reproduces.
func declaredReduceReduceHighestSymbolConflictChoice(actions []ParseAction) (ParseAction, bool) {
	if len(actions) < 2 {
		return ParseAction{}, false
	}
	best := -1
	for i := range actions {
		act := actions[i]
		if act.Type != ParseActionReduce || act.Extra || act.Repetition {
			return ParseAction{}, false
		}
		if act.DynamicPrecedence != actions[0].DynamicPrecedence {
			return ParseAction{}, false
		}
		if best < 0 || act.Symbol > actions[best].Symbol {
			best = i
		}
	}
	return actions[best], true
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
	// accepted/error-free (151ms) to no_stacks_alive at byte ~17k. At another
	// Haskell state, the repetition shift is not reachable after the fold.
	// The exact conflict-policy rows keep the proven subset.
	// The memory-budget wins from the global fold (LicenseId.hs and
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

// cRecoveredRepetitionSkipOptOut lists languages whose current recovery
// selection still depends on retaining the repetition-shift arm during or
// after recovery. Clean lineages continue to use C's repetition fold.
var cRecoveredRepetitionSkipOptOut = map[string]bool{
	// PHP's recovered Symfony TransportResponseTrait and TwigExtensionTest
	// witnesses change from their C-matching clean result to ERROR when the
	// fold is applied after recovery. Keep the exception recovery-scoped so
	// clean PHP lists retain the linear C path.
	"php": true,
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
// reduce versions, which our GLR fork approximates). The C rule also applies
// while a version is recovering. Languages with a confirmed Go recovery-
// selection counterexample are held out by cRecoveredRepetitionSkipOptOut;
// exact recovered-lineage exceptions are handled by
// conflictPolicyChoiceForDispatch before this global rule.
func (p *Parser) cRepetitionSkipConflictChoice(s *glrStack, actions []ParseAction) (ParseAction, bool) {
	if p == nil || p.language == nil || cRepetitionSkipOptOut[p.language.Name] {
		return ParseAction{}, false
	}
	if s == nil {
		return ParseAction{}, false
	}
	if cRecoveredRepetitionSkipOptOut[p.language.Name] &&
		(s.cRec != nil || s.cPaused || s.cRecoverMissingGroup != nil || s.cEverErrored) {
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
		// go.mod's certified require-list rows apply even during incremental
		// reuse dispatch (unlike the general ConflictPolicies path below,
		// gated off during reuse): go.mod files are small enough that reuse
		// dispatch still benefits from folding the list continuation, and this
		// predates the reuse gate below. See grammars/runtime_profiles.go for
		// the certified rows that replaced gomodRepetitionShiftConflictChoice.
		if next, ok := conflictPolicyChoice(p.language, tok, currentState, actions); ok {
			return next, true
		}
	}
	// ConflictPolicyDeclaredReduceReduceHighestSymbol applies even during
	// incremental reuse dispatch (unlike the general ConflictPolicies path
	// below, gated off during reuse): the fold is a pure function of the
	// table row's own actions (every candidate is a plain REDUCE, tied on
	// dynamic precedence), never of stack or reuse state, so replaying it at
	// a reuse-forced re-dispatch point reaches the exact symbol a fresh
	// parse would reach at the same position. Declared-conflict rows are
	// always marked fragile (table_entry.action_count > 1), so the old
	// subtree at this exact position can never be the one reuse recycles —
	// this is the only choice available once reuse forces a re-dispatch
	// here, not an override of a reuse decision.
	if chosen, ok := declaredReduceReduceConflictPolicyChoice(p.language, currentState, tok, actions); ok {
		return chosen, true
	}
	if reuse == nil {
		if chosen, ok := conflictPolicyChoiceForDispatch(p.language, s, tok, currentState, actions); ok {
			return chosen, true
		}
	}
	// C's global repetition-skip fold. Runs after explicit generated
	// ConflictPolicies (per-state directives outrank the default) but before
	// the grammargen repeat-boundary veto and the per-language arms: the veto
	// exists to keep hand-written SHIFT-preferring shortcuts away from
	// grammars they were never tuned for, whereas this fold is C's own
	// dispatch semantics and applies to every grammar whose tables carry
	// repetition-marked shifts. Unlike language-scoped compatibility choices,
	// the C fold is also valid while dispatching around reused subtrees.
	if next, ok := p.cRepetitionSkipConflictChoice(s, actions); ok {
		return next, true
	}
	if reuse != nil {
		return ParseAction{}, false
	}
	if generatedRepeatBoundaryConflict(p.language, actions) {
		return ParseAction{}, false
	}
	if p.language.GeneratedByGrammargen {
		return ParseAction{}, false
	}
	// The old per-language repetition-boundary shortcuts are gone:
	// cRepetitionSkipConflictChoice above makes the C-faithful choice for their
	// exact {1 repetition-SHIFT + 1 REDUCE} shape. Prior table-shape analysis
	// showed those states carry only that shape. On clean
	// lineages the global fold therefore shadows them completely; on wreckage
	// lineages (cEverErrored and friends), the correct behavior is the GLR fork
	// feeding recovery cost competition, not an unconditional deterministic
	// commitment. Arms that survive below are not retired repetition-boundary
	// policies (or, for gomod above, run where the global fold does not).
	//
	// dart and c are also in cRepetitionSkipOptOut (the engine-wide fold is
	// unsafe for both — see the opt-out entries) but each still has a
	// certified narrow subset in ConflictPolicies (grammars/runtime_profiles.go),
	// checked above by conflictPolicyChoiceForDispatch
	// before this switch is reached, INCLUDING on wreckage lineages: those
	// rows carry no recovery gating, matching the retired
	// dartRepetitionShiftConflictChoice / cRepetitionShiftConflictChoice
	// helpers they replaced.
	var chosen ParseAction
	var ok bool
	switch p.language.Name {
	case "java":
		// Non-repetition: `case A ->` switch-label disambiguation via a
		// goto/action-table probe on the reduce's landing state.
		chosen, ok = p.javaSwitchArrowConflictChoice(s, tok, actions)
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
	// and bail out identically. Recovery-only certified rows do not activate
	// this veto for otherwise unprofiled embedded grammars.
	if !languageHasGeneratorConflictPolicy(lang) {
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

func languageHasGeneratorConflictPolicy(lang *Language) bool {
	if lang == nil {
		return false
	}
	if lang.GeneratedByGrammargen {
		return true
	}
	for i := range lang.ConflictPolicies {
		switch lang.ConflictPolicies[i].Kind {
		case ConflictPolicyRepetitionShift, ConflictPolicyShift:
			return true
		}
	}
	return false
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
