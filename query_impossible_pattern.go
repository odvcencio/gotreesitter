package gotreesitter

import "fmt"

// PatternIssueKind classifies a problem ValidateQueryPatterns can report.
type PatternIssueKind uint8

const (
	// PatternIssueImpossibleChild marks a pattern step that asserts a named
	// node type as a direct child of a parent node type that this
	// language's compiled parse tables prove can never hold it. See the doc
	// comment on ValidateQueryPatterns for exactly what "prove" means here
	// and this analysis's known blind spots.
	PatternIssueImpossibleChild PatternIssueKind = iota
)

// PatternIssue reports one pattern step ValidateQueryPatterns found
// structurally unreachable.
type PatternIssue struct {
	Kind PatternIssueKind
	// PatternIndex is the offending pattern's index, matching
	// QueryMatch.PatternIndex and Query.StartByteForPattern/EndByteForPattern.
	PatternIndex int
	// ParentType and ChildType are the grammar node type names from the
	// pattern: ChildType can never appear as a named child of ParentType.
	ParentType string
	ChildType  string
	// StartByte and EndByte give the offending pattern's full source span
	// (QueryStep does not carry its own byte range, only Pattern does).
	StartByte uint32
	EndByte   uint32
}

// String renders a human-readable diagnostic, e.g. for logging or lint output.
func (i PatternIssue) String() string {
	return fmt.Sprintf("query: impossible pattern: %q can never be a named child of %q (pattern %d, bytes %d-%d)",
		i.ChildType, i.ParentType, i.PatternIndex, i.StartByte, i.EndByte)
}

// ValidateQueryPatterns checks q's compiled patterns against lang's
// compiled grammar tables and reports every pattern step that structurally
// can never match -- gotreesitter's counterpart to the class of query
// tree-sitter's C ts_query_new statically rejects at compile time with
// TSQueryErrorStructure ("Impossible pattern"). q must have been compiled
// against lang (Query does not retain its own Language reference, matching
// every other Query method that takes lang explicitly).
//
// gotreesitter's NewQuery does not reject these by default -- see
// WithStrictPatternValidation for an opt-in that does -- so ValidateQueryPatterns
// exists for callers who want to audit an already-compiled Query on demand,
// listing every offending step rather than stopping at the first the way a
// strict compile would.
//
// # What this checks
//
// For a pattern step of the form "(parent (child) ...)" -- one concrete,
// named node type asserted as a direct, field-less child of another
// concrete, named parent step -- this walks lang's compiled LR parse tables
// (the same tables the parser itself executes: ParseTable/SmallParseTable/
// ParseActions/LargeStateGotos) to determine whether any reduction that
// produces `parent` can ever place `child` among its immediate children. A
// step is flagged only when the tables affirmatively prove no such
// reduction exists among everything they expose; whenever the analysis
// cannot reach a confident answer, it reports nothing for that step rather
// than guessing.
//
// # What this does not check, and why
//
// tree-sitter's own "Impossible pattern" analysis (lib/src/query.c,
// ts_query__analyze_patterns) walks the same generated LR automaton this
// does, but the C tool that builds it still has the grammar's original
// production right-hand sides in scope. gotreesitter only carries that shape
// for languages grammargen assembles at build time
// (Language.ProductionSignatures); every language decoded from a ts2go blob
// -- which includes every core-nine language this repository has ever found
// a dead pattern in, and every language this function's own tests exercise
// -- leaves ProductionSignatures empty, because tree-sitter's generated
// parser.c never embeds production right-hand sides either (the code
// generator throws that shape away once the tables are built). So this
// reconstructs an approximation of "legal child sets" from the compiled
// action/goto tables instead of reproducing C's analysis exactly:
//
//   - Unknown node type names and unknown field names are not re-checked
//     here: NewQuery already rejects both unconditionally, for every caller,
//     before a *Query can exist. There is nothing left for a post-compile
//     validator to add for those two classes.
//   - Field-constrained children ("field: (child)") are never flagged.
//     Legal field/child associations live in per-production field maps
//     keyed by production ID; this analysis tracks which automaton state an
//     edge lands in, not which production ID or child index it belongs to,
//     so it has no ground truth for field legality. Skipped, not guessed.
//   - Alternation branches ("[(a) (b)]") are never flagged, whether the
//     alternation is itself a child step or contains one: QueryStep folds an
//     alternation's branches into step.alternatives rather than ordinary
//     nested steps, and this analysis does not walk that shape at all.
//   - Anchors (".") and quantifiers (?, *, +) are ignored: this only answers
//     "can child ever occur under parent at all", never "at this exact
//     position" or "exactly this many times".
//   - The check is existence-only, not position-aware: it unions together
//     every child position of every production that ever reduces to
//     `parent`. That makes it an over-approximation of the true per-position
//     legal child set -- see Soundness below.
//   - Hidden (invisible) wrapper symbols are resolved transitively (their
//     own legal children substitute for the wrapper, recursively, up to a
//     bounded depth) so ordinary grammar-generated flattening (repeat/choice
//     helpers, hidden alternatives) does not produce false positives. A
//     production's AliasSequences overrides are applied at that exact
//     production only, not propagated through further transitive hidden
//     lookups beyond it.
//   - GLR conflict actions, error-recovery ("RECOVER") transitions, and
//     "extra" (e.g. comment) tokens are read exactly as the tables encode
//     them, with no special-casing. This can only make the computed
//     legal-child set larger (more conservative), never smaller.
//   - A grammar too large for this analysis to walk cleanly (see
//     maxLanguageChildAnalysisStates) is skipped entirely: every step in
//     every one of its queries reports no issue.
//
// # Soundness
//
// This analysis is built to be a (possibly loose) over-approximation of the
// true legal-child relation, not an exact reproduction of it: every signal
// it collects -- raw automaton edges into parent-interior states, this
// language's AliasSequences overrides, and hidden-symbol flattening -- can
// only add candidate children, never remove one the grammar genuinely
// allows. A returned PatternIssue is therefore a strong, tables-backed
// claim, but the absence of one is not proof the pattern is meaningful --
// only that this analysis could not disprove it. That asymmetry is
// deliberate: a false "impossible" verdict would silently break a real,
// working query for an opted-in caller, which this treats as worse than a
// missed detection (the status quo before this function existed).
func ValidateQueryPatterns(lang *Language, q *Query) []PatternIssue {
	if lang == nil || q == nil {
		return nil
	}
	analysis := languageChildAnalysisFor(lang)
	if analysis == nil || !analysis.ok {
		return nil
	}

	var issues []PatternIssue
	for pi := range q.patterns {
		issues = appendPatternIssues(analysis, lang, q, pi, issues)
	}
	return issues
}

func appendPatternIssues(analysis *languageChildAnalysis, lang *Language, q *Query, patternIndex int, issues []PatternIssue) []PatternIssue {
	pat := &q.patterns[patternIndex]
	steps := pat.steps
	for i := range steps {
		child := &steps[i]
		if !isCheckableNamedStep(child) || child.depth == 0 || child.field != 0 {
			continue
		}
		parentIdx := findImmediateParentStep(steps, i)
		if parentIdx < 0 {
			continue
		}
		parent := &steps[parentIdx]
		if !isCheckableNamedStep(parent) {
			continue
		}

		parentSym := lang.PublicSymbolForNamedness(parent.symbol, true)
		children, ok := analysis.legalNamedChildren(parentSym)
		if !ok {
			continue
		}
		childSym := lang.PublicSymbolForNamedness(child.symbol, true)
		if children[childSym] {
			continue
		}
		issues = append(issues, PatternIssue{
			Kind:         PatternIssueImpossibleChild,
			PatternIndex: patternIndex,
			ParentType:   symbolDisplayName(lang, parent.symbol),
			ChildType:    symbolDisplayName(lang, child.symbol),
			StartByte:    pat.startByte,
			EndByte:      pat.endByte,
		})
	}
	return issues
}

// isCheckableNamedStep reports whether step asserts one concrete, named
// grammar symbol -- excluding wildcards ("_"), the synthetic ERROR symbol,
// and MISSING markers, none of which describe an ordinary grammar
// reduction this analysis can reason about.
func isCheckableNamedStep(step *QueryStep) bool {
	return step.isNamed && step.symbol != 0 && !step.isMissing &&
		step.symbol != errorSymbol && len(step.alternatives) == 0
}

// findImmediateParentStep returns the index of the nearest preceding step
// whose depth is exactly one less than steps[idx]'s, i.e. idx's direct
// parent in the pattern's preorder-with-depth encoding (see
// (*Query).matchStepsWithParentPredicates in query_matcher.go for the
// forward counterpart this mirrors: a step's direct children are the run of
// immediately following steps at depth+1). Returns -1 if idx is a pattern
// root (depth 0) or the structure is otherwise malformed.
func findImmediateParentStep(steps []QueryStep, idx int) int {
	want := steps[idx].depth - 1
	for i := idx - 1; i >= 0; i-- {
		if steps[i].depth == want {
			return i
		}
	}
	return -1
}

func symbolDisplayName(lang *Language, sym Symbol) string {
	if lang == nil || int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
		return fmt.Sprintf("<symbol %d>", sym)
	}
	return lang.SymbolNames[sym]
}

// QueryOption configures optional NewQuery compile-time behavior. The zero
// value of every option is a no-op, so existing two-argument NewQuery(source,
// lang) call sites are unaffected.
type QueryOption func(*queryCompileOptions)

type queryCompileOptions struct {
	strictPatternValidation bool
}

// WithStrictPatternValidation opts NewQuery into rejecting a query whose
// compiled patterns ValidateQueryPatterns proves structurally impossible,
// mirroring tree-sitter's C ts_query_new: a query with at least one such
// pattern fails to compile at all, reporting the first offending pattern
// (source order) as the error, exactly as C reports only the first
// "Impossible pattern" it hits and rejects the whole multi-pattern query.
//
// This is opt-in, not the default, for two reasons: gotreesitter's default
// NewQuery behavior must not change for existing callers, and dozens of this
// repository's own currently-shipping fleet grammars' inferred queries this
// analysis flags at least one dead pattern in today (see
// grammars/tags_query_infer.go's "Impossible-pattern fixes" note for the
// core-nine languages already audited and fixed) -- enabling this by default
// would break compilation for languages nobody has audited yet. See
// ValidateQueryPatterns for exactly what the underlying analysis proves and
// its known blind spots; this option can reject a query the analysis
// confidently disproves, but it can also let a genuinely dead pattern
// through undetected the way NewQuery always has.
func WithStrictPatternValidation() QueryOption {
	return func(c *queryCompileOptions) {
		c.strictPatternValidation = true
	}
}
