package lexbuild

import (
	"fmt"
	"sync/atomic"
)

// Size budgets. The generator has none: some inputs of a few hundred bytes
// ask it for billions of states, productions or sets, and it runs until memory
// is gone. The port counts what it builds in each place where the result or the
// working state can grow faster than the grammar (see growth-audit in the pull
// request) and refuses the grammar with a *SizeError before it builds the item
// that would pass the limit. A limit applies to one construction as named in
// the table below; the limits are variables so that tests can lower them.
//
// Each limit is more than ten times the largest value measured over the
// bundled grammars that could be measured (TestLexCensus prints the maxima).
type budget struct {
	what  string
	limit int64
	peak  atomic.Int64
}

var (
	// Shape of the input, checked first (shape.go). A cap names a quantity of the
	// grammar as given.
	bInputBytes    = &budget{what: "bytes of grammar.json", limit: 1 << 23}
	bRules         = &budget{what: "rules", limit: 1 << 14}
	bExternals     = &budget{what: "external symbols", limit: 1 << 11}
	bPrecOrderings = &budget{what: "precedence orderings", limit: 1 << 9}
	bPrecNames     = &budget{what: "names in one precedence ordering", limit: 1 << 8}
	bNameLen       = &budget{what: "bytes in a name (rule, symbol, field, alias, precedence, external, reserved set)", limit: 1 << 10}
	bStringLen     = &budget{what: "bytes in a string literal", limit: 1 << 11}
	bPatternLen    = &budget{what: "bytes in a pattern source", limit: 1 << 18}
	bRepeatCount   = &budget{what: "largest count of a repetition {n} or {n,m} in a pattern", limit: 1 << 10}
	bListLen       = &budget{what: "entries in a list of the grammar (supertypes, inline names, conflicts and each conflict, extras, reserved sets and each set, members of a rule)", limit: 1 << 14}
	bRuleDepth     = &budget{what: "nesting depth of rules", limit: 1 << 8}
	// Work and state of the passes behind the caps.
	bNormalizeWork   = &budget{what: "rule nodes times dropped rules in the removal of unreachable rules", limit: 1 << 30}
	bCoincidentWork  = &budget{what: "steps of the pairing of tokens that share a parse state", limit: 1 << 32}
	bPrecPairs       = &budget{what: "distinct precedence pairs", limit: 1 << 11}
	bTreeEqWork      = &budget{what: "rule nodes compared while looking for equal rules", limit: 1 << 22}
	bRepeatWork      = &budget{what: "rule nodes evaluated while expanding repeats", limit: 1 << 19}
	bCycleWork       = &budget{what: "steps of the indirect-recursion search", limit: 1 << 15}
	bFlattenKeyBytes = &budget{what: "bytes of production keys retained while flattening rules", limit: 1 << 23}
	bKeywordWork     = &budget{what: "steps of the keyword identification", limit: 1 << 22}
	bGroupingWork    = &budget{what: "steps of the grouping of parse states into lex states", limit: 1 << 34}
	bClosureWork     = &budget{what: "automaton states visited while closing state sets over splits", limit: 1 << 25}
	bGroupWork       = &budget{what: "steps of the grouping of transitions into character sets", limit: 1 << 31}
	bSortWork        = &budget{what: "steps of the ordering of lex states", limit: 1 << 25}
	bRenderWork      = &budget{what: "steps of rendering lex states (large character sets and plans)", limit: 1 << 27}
	bFollowChars     = &budget{what: "character ranges joined for the starting characters of the tokens that follow each token", limit: 1 << 23}
	bRangesWork      = &budget{what: "transition evaluations needed to list the ranges of the lex states", limit: 1 << 26}
	// Prepare.
	bNFAStates     = &budget{what: "automaton states in the lexical grammar", limit: 1 << 18}
	bPatternRanges = &budget{what: "character ranges read into the classes of the patterns of a grammar", limit: 1 << 24}
	bInlineCalls   = &budget{what: "expansion steps in the check of inline rules", limit: 200000}
	bInlineSymbols = &budget{what: "symbols copied in the check of inline rules", limit: 1 << 24}
	bNFARanges     = &budget{what: "character ranges summed over automaton states", limit: 1 << 22}
	bFlattenPaths  = &budget{what: "choice paths explored while flattening rules", limit: 1 << 17}
	bFlattenSteps  = &budget{what: "production steps retained while flattening rules", limit: 1 << 20}
	bFlattenPath1  = &budget{what: "steps in one flattened production", limit: 1 << 12}
	bSymbolCells   = &budget{what: "cells of a token-by-token or rule-by-token matrix", limit: 1 << 24}
	// Analyze.
	bConflictSets = &budget{what: "state sets in one token-pair conflict search", limit: 1 << 17}
	bConflictIDs  = &budget{what: "automaton states retained by one token-pair conflict search", limit: 1 << 21}
	// Lex table construction (one table, the keyword table, or one token).
	bLexStates   = &budget{what: "lex states", limit: 1 << 17}
	bLexIDs      = &budget{what: "automaton states summed over lex states", limit: 1 << 23}
	bLexAdvances = &budget{what: "character transitions summed over lex states", limit: 1 << 20}
	bLexRanges   = &budget{what: "character ranges summed over lex-state transitions", limit: 1 << 23}
	bTransIDs    = &budget{what: "automaton states summed over the transitions of one lex state", limit: 1 << 16}
	bMinCompares = &budget{what: "steps of the comparisons while minimizing a lex table", limit: 1 << 27}
	// Collection of the large character sets (one construction for each token).
	bLargeStates = &budget{what: "lex states summed over the single-token constructions", limit: 1 << 18}
)

// allBudgets lists the budgets in the order of the documentation table.
var allBudgets = []*budget{bInputBytes, bRules, bExternals, bPrecOrderings, bPrecNames, bNameLen, bStringLen, bPatternLen, bRepeatCount, bRuleDepth, bListLen, bPrecPairs, bCoincidentWork, bNormalizeWork, bTreeEqWork, bRepeatWork, bCycleWork, bFlattenKeyBytes, bKeywordWork, bGroupingWork, bClosureWork, bGroupWork, bSortWork, bRenderWork, bRangesWork, bFollowChars, bPatternRanges, bInlineCalls, bInlineSymbols, bNFAStates, bNFARanges, bFlattenPaths, bFlattenSteps, bFlattenPath1, bSymbolCells,
	bConflictSets, bConflictIDs, bLexStates, bLexIDs, bLexAdvances, bLexRanges, bTransIDs, bMinCompares, bLargeStates}

// SizeError reports that a construction passed a size limit of this package.
type SizeError struct {
	What  string // what was counted
	Limit int64
}

func (e *SizeError) Error() string {
	return fmt.Sprintf("lexbuild: size limit: more than %d %s", e.Limit, e.What)
}

type lexTooLarge struct{ err *SizeError }

// check records n as a value seen and refuses it if it passes the limit.
// Callers pass the value that the item they are about to build would reach.
func (b *budget) check(n int) { b.checkWork(int64(n)) }

// checkWork is check for a count that can pass the range of int on a 32-bit build.
func (b *budget) checkWork(n int64) {
	for {
		old := b.peak.Load()
		if n <= old || b.peak.CompareAndSwap(old, n) {
			break
		}
	}
	if n > b.limit {
		panic(lexTooLarge{&SizeError{b.what, b.limit}})
	}
}

// guardSize runs f and turns the panic of a passed size limit into an error.
func guardSize(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if l, ok := r.(lexTooLarge); ok {
				err = l.err
				return
			}
			panic(r)
		}
	}()
	f()
	return nil
}

// mulWork is a*b for a work estimate. Both operands are converted before the
// multiplication, so the product does not wrap on a 32-bit int (sizes of
// 65,536 and 65,536 give 4.3 x 10^9).
func mulWork(a, b int) int64 { return int64(a) * int64(b) }

// cells is a*b for a dimension check. It cannot overflow: a product above the
// limit of bSymbolCells is returned as one more than that limit.
func cells(a, b int) int {
	over := int(bSymbolCells.limit) + 1
	if a < 0 || b < 0 {
		return over
	}
	if a != 0 && b > over/a {
		return over
	}
	return a * b
}

// addCells is the sum of two cell counts, saturating at one more than the limit.
func addCells(a, b int) int {
	over := int(bSymbolCells.limit) + 1
	if a > over-b {
		return over
	}
	return a + b
}

// cells2 is a*b for a work estimate, saturating at one more than the limit of bNormalizeWork.
func cells2(a, b int) int {
	over := int(bNormalizeWork.limit) + 1
	if a != 0 && b > over/a {
		return over
	}
	return a * b
}
