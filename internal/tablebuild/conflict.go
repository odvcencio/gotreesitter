package tablebuild

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports the conflict handling of build_tables/build_parse_table.rs:
// the error types and their messages, and fn ParseTableBuilder::handle_conflict,
// compare_precedence and get_auxiliary_node_info.

// ConflictError is build_parse_table.rs: struct ConflictError
// (ParseTableBuilderError::Conflict).
type ConflictError struct {
	SymbolSequence          []string
	ConflictingLookahead    string
	PossibleInterpretations []Interpretation
	PossibleResolutions     []Resolution
}

func (*ConflictError) parseTableBuilderError() {}

// Interpretation is build_parse_table.rs: struct Interpretation. Precedence
// and Associativity are empty for None.
type Interpretation struct {
	PrecedingSymbols      []string
	VariableName          string
	ProductionStepSymbols []string
	StepIndex             uint32
	Done                  bool
	ConflictingLookahead  string
	Precedence            string
	Associativity         string
	RequiresEOFLookahead  bool
}

// ResolutionKind is the variant of build_parse_table.rs: enum Resolution.
type ResolutionKind uint8

const (
	ResolvePrecedence ResolutionKind = iota
	ResolveAssociativity
	ResolveAddConflict
)

// Resolution is build_parse_table.rs: enum Resolution.
type Resolution struct {
	Kind    ResolutionKind
	Symbols []string
}

// AmbiguousExtraError is build_parse_table.rs: struct AmbiguousExtraError
// (ParseTableBuilderError::AmbiguousExtra).
type AmbiguousExtraError struct {
	ParentSymbols []string
}

func (*AmbiguousExtraError) parseTableBuilderError() {}

// Error is the message of ParseTableBuilderError::Conflict with impl Display
// for ConflictError.
func (e *ConflictError) Error() string {
	var f strings.Builder
	f.WriteString("Unresolved conflict for symbol sequence:\n\n")
	for _, symbol := range e.SymbolSequence {
		fmt.Fprintf(&f, "  %s", symbol)
	}
	fmt.Fprintf(&f, "  •  %s  …\n\n", e.ConflictingLookahead)

	f.WriteString("Possible interpretations:\n\n")
	type interp struct {
		line    string
		prec    string
		hasPrec bool
	}
	interpretations := make([]interp, 0, len(e.PossibleInterpretations))
	maxLength := 0
	for i := range e.PossibleInterpretations {
		it := &e.PossibleInterpretations[i]
		line := it.String()
		var annotations []string
		if it.Precedence != "" && it.Associativity != "" {
			annotations = append(annotations, fmt.Sprintf("(precedence: %s, associativity: %s)", it.Precedence, it.Associativity))
		} else if it.Precedence != "" {
			annotations = append(annotations, fmt.Sprintf("(precedence: %s)", it.Precedence))
		}
		if it.RequiresEOFLookahead {
			annotations = append(annotations, "(reduces only at end of input)")
		}
		x := interp{line: line}
		if len(annotations) > 0 {
			x.prec, x.hasPrec = strings.Join(annotations, "  "), true
		}
		interpretations = append(interpretations, x)
		maxLength = max(maxLength, utf8.RuneCountInString(line))
	}
	// sort_unstable of (String, Option<String>): by line, then None before Some.
	sort.Slice(interpretations, func(a, b int) bool {
		x, y := interpretations[a], interpretations[b]
		if x.line != y.line {
			return x.line < y.line
		}
		if x.hasPrec != y.hasPrec {
			return !x.hasPrec
		}
		return x.prec < y.prec
	})
	for i, x := range interpretations {
		fmt.Fprintf(&f, "  %d:%s", i+1, x.line)
		if x.hasPrec {
			pad := maxLength - utf8.RuneCountInString(x.line)
			if pad < 0 {
				pad = 0
			}
			f.WriteString(strings.Repeat(" ", pad+2))
			f.WriteString(x.prec)
		}
		f.WriteString("\n")
	}

	f.WriteString("\nPossible resolutions:\n\n")
	for i, r := range e.PossibleResolutions {
		fmt.Fprintf(&f, "  %d:  %s\n", i+1, r.String())
	}
	return f.String()
}

// String is build_parse_table.rs: impl Display for Interpretation.
func (it *Interpretation) String() string {
	var f strings.Builder
	for _, symbol := range it.PrecedingSymbols {
		fmt.Fprintf(&f, "  %s", symbol)
	}
	fmt.Fprintf(&f, "  (%s", it.VariableName)
	for i, symbol := range it.ProductionStepSymbols {
		if i == int(it.StepIndex) {
			f.WriteString("  •")
		}
		fmt.Fprintf(&f, "  %s", symbol)
	}
	f.WriteString(")")
	if it.Done {
		fmt.Fprintf(&f, "  •  %s  …", it.ConflictingLookahead)
	}
	return f.String()
}

// String is build_parse_table.rs: impl Display for Resolution.
func (r *Resolution) String() string {
	var f strings.Builder
	switch r.Kind {
	case ResolvePrecedence:
		f.WriteString("Specify a higher precedence in ")
		for i, symbol := range r.Symbols {
			if i > 0 {
				f.WriteString(" and ")
			}
			fmt.Fprintf(&f, "`%s`", symbol)
		}
		f.WriteString(" than in the other rules.")
	case ResolveAssociativity:
		f.WriteString("Specify a left or right associativity in ")
		for i, symbol := range r.Symbols {
			if i > 0 {
				f.WriteString(", ")
			}
			fmt.Fprintf(&f, "`%s`", symbol)
		}
	case ResolveAddConflict:
		f.WriteString("Add a conflict for these rules: ")
		for i, symbol := range r.Symbols {
			if i > 0 {
				f.WriteString(", ")
			}
			fmt.Fprintf(&f, "`%s`", symbol)
		}
	}
	return f.String()
}

// Error is the message of ParseTableBuilderError::AmbiguousExtra with impl
// Display for AmbiguousExtraError.
func (e *AmbiguousExtraError) Error() string {
	return "Extra rules must have unambiguous endings. Conflicting rules: " + strings.Join(e.ParentSymbols, ", ")
}

// itemSetOrdered is a BTreeSet of items: sorted by ParseItem order, one item
// per equal class (the first inserted).
type itemSetOrdered []*ParseItem

func (s *itemSetOrdered) insert(it *ParseItem) {
	l := *s
	i := sort.Search(len(l), func(k int) bool { return l[k].compare(it) >= 0 })
	if i < len(l) && l[i].compare(it) == 0 {
		return
	}
	l = append(l, nil)
	copy(l[i+1:], l[i:])
	l[i] = it
	*s = l
}

// shiftPrecedence is one element of handle_conflict's shift_precedence: a
// sorted, duplicate-free Vec<(Precedence, Symbol)>. The generator orders
// named precedences by their string ids; the list is only scanned to set
// three flags, so its order reaches no result. The port orders names by
// string.
type shiftPrecedence struct {
	prec   lexbuild.Precedence
	symbol Symbol
}

func (a shiftPrecedence) cmp(b shiftPrecedence) int {
	if a.prec.Kind != b.prec.Kind {
		return cmpInt(a.prec.Kind, b.prec.Kind)
	}
	switch a.prec.Kind {
	case lexbuild.PrecInteger:
		if c := cmpInt(a.prec.Int, b.prec.Int); c != 0 {
			return c
		}
	case lexbuild.PrecName:
		if c := strings.Compare(a.prec.Ident, b.prec.Ident); c != 0 {
			return c
		}
	}
	return symbolCmp(a.symbol, b.symbol)
}

// handleConflict is build_parse_table.rs: fn ParseTableBuilder::handle_conflict.
func (b *parseTableBuilder) handleConflict(itemSet *ParseItemSet, stateID uint32, precedingSymbols []Symbol, precedingAuxiliarySymbols []auxiliarySymbolInfo, conflictingLookahead Symbol, reductionInfo *reductionInfo) error {
	sg := b.sg
	if reductionInfo == nil {
		panicLikeGenerator("no reduction info for a conflicting lookahead (reduction_infos.get(&symbol).unwrap())")
	}
	state := &b.parseTable.States[stateID]
	entry := b.lookup(state, conflictingLookahead)

	// Determine which items in the set conflict with each other, and the
	// precedences of the Shift and Reduce actions. There are no Reduce
	// actions with different precedences (add_actions sorts those out), but
	// there can still be Reduce/Reduce conflicts where all actions have the
	// same precedence, and there can still be Shift/Reduce conflicts.
	consideredAssociativity := false
	var shiftPrec []shiftPrecedence
	var conflictingItems itemSetOrdered
	for k := range itemSet.Entries {
		ent := &itemSet.Entries[k]
		item := &ent.Item
		if step, ok := item.step(sg); ok {
			if item.StepIndex > 0 && b.itemSetBuilder.firstSet(step.Symbol).Contains(conflictingLookahead) {
				if item.VariableIndex != startProductionID {
					conflictingItems.insert(item)
				}
				p := shiftPrecedence{item.precedence(sg), lexbuild.NonTerminalSym(int(item.VariableIndex))}
				i := sort.Search(len(shiftPrec), func(k int) bool { return shiftPrec[k].cmp(p) >= 0 })
				if i == len(shiftPrec) || shiftPrec[i].cmp(p) != 0 {
					shiftPrec = append(shiftPrec, shiftPrecedence{})
					copy(shiftPrec[i+1:], shiftPrec[i:])
					shiftPrec[i] = p
				}
			}
		} else if b.itemSetBuilder.lookaheads.get(ent.lookaheads).Contains(conflictingLookahead) && item.VariableIndex != startProductionID {
			conflictingItems.insert(item)
		}
	}

	if last := &entry.Actions[len(entry.Actions)-1]; last.Kind == ActionShift {
		// If all of the items in the conflict have the same parent symbol, and
		// that parent symbol is auxiliary, then this is just the intentional
		// ambiguity of a repeat rule. Resolve it by leaving it in the parse
		// table, but marking the Shift action with an is_repetition flag.
		if len(conflictingItems) == 0 {
			panicLikeGenerator("a Shift conflict with no conflicting item (conflicting_items.iter().next().unwrap())")
		}
		conflictingVariableIndex := conflictingItems[0].VariableIndex
		if sg.Variables[conflictingVariableIndex].Kind == lexbuild.VarAuxiliary {
			all := true
			for _, it := range conflictingItems {
				if it.VariableIndex != conflictingVariableIndex {
					all = false
					break
				}
			}
			if all {
				last.IsRepetition = true
				return nil
			}
		}

		// If the Shift action has higher precedence, remove all the Reduce
		// actions.
		shiftIsLess, shiftIsEqual, shiftIsMore := false, false, false
		for _, p := range shiftPrec {
			switch comparePrecedence(sg, p.prec, []Symbol{p.symbol}, reductionInfo.precedence, reductionInfo.symbols) {
			case 1:
				shiftIsMore = true
			case -1:
				shiftIsLess = true
			default:
				shiftIsEqual = true
			}
		}

		pureRight := !reductionInfo.hasLeftAssoc && !reductionInfo.hasNonAssoc && reductionInfo.hasRightAssoc
		switch {
		case shiftIsMore && !shiftIsLess:
			keepLast(entry)
		case shiftIsLess && !shiftIsMore:
			// If the Reduce actions have higher precedence, remove the Shift
			// action. Exception: if one Shift interpretation ties the Reduce
			// actions in precedence while another has lower precedence, and the
			// Reduce actions are purely right associative, honor that right
			// associativity by shifting rather than reducing. The
			// lower-precedence interpretation coexists with the tying one, so on
			// its own it must not force a Reduce that would flip the tie to left
			// associative.
			if shiftIsEqual && pureRight {
				keepLast(entry)
			} else {
				entry.Actions = entry.Actions[:len(entry.Actions)-1]
				conflictingItems = retainDone(conflictingItems)
			}
		case !shiftIsLess && !shiftIsMore:
			// If the Shift and Reduce actions have the same precedence,
			// consider the Reduce actions' associativity: if all are left
			// associative remove the Shift action, if all are right
			// associative remove the Reduce actions.
			consideredAssociativity = true
			switch {
			case reductionInfo.hasLeftAssoc && !reductionInfo.hasNonAssoc && !reductionInfo.hasRightAssoc:
				entry.Actions = entry.Actions[:len(entry.Actions)-1]
				conflictingItems = retainDone(conflictingItems)
			case pureRight:
				keepLast(entry)
			}
		}
	}

	// If all of the actions but one have been eliminated, there is no
	// problem.
	if len(entry.Actions) == 1 {
		return nil
	}

	// Determine the set of parent symbols involved in this conflict.
	var actualConflict []Symbol
	for _, item := range conflictingItems {
		symbol := lexbuild.NonTerminalSym(int(item.VariableIndex))
		if sg.Variables[symbol.Index].Kind == lexbuild.VarAuxiliary {
			found := false
			for k := len(precedingAuxiliarySymbols) - 1; k >= 0; k-- {
				if precedingAuxiliarySymbols[k].auxiliarySymbol == symbol {
					actualConflict = append(actualConflict, precedingAuxiliarySymbols[k].parentSymbols...)
					found = true
					break
				}
			}
			if !found {
				panicLikeGenerator("no preceding auxiliary symbol info for %s (find_map(..).unwrap())", sg.Variables[symbol.Index].Ident)
			}
		} else {
			actualConflict = append(actualConflict, symbol)
		}
	}
	sort.Slice(actualConflict, func(i, j int) bool { return lexbuild.SymbolLess(actualConflict[i], actualConflict[j]) })
	actualConflict = dedupSymbols(actualConflict)

	// If this set of symbols has been declared, there is no error.
	for _, c := range sg.ExpectedConflicts {
		if equalSymbols(c, actualConflict) {
			delete(b.actualConflicts, conflictKey(actualConflict))
			return nil
		}
	}

	conflictError := &ConflictError{}
	for _, symbol := range precedingSymbols {
		conflictError.SymbolSequence = append(conflictError.SymbolSequence, b.symbolName(symbol))
	}
	conflictError.ConflictingLookahead = b.symbolName(conflictingLookahead)

	for _, item := range conflictingItems {
		// preceding_symbols.len() - step_index wraps in the generator's
		// release build when the item has more steps than the state's path
		// (states of non-terminal extras start at step 1 with an empty path);
		// take() then keeps every symbol.
		n := len(precedingSymbols)
		if int(item.StepIndex) <= n {
			n -= int(item.StepIndex)
		}
		var preceding []string
		for _, s := range precedingSymbols[:n] {
			preceding = append(preceding, b.symbolName(s))
		}
		var stepSymbols []string
		for _, step := range item.production(sg).steps {
			stepSymbols = append(stepSymbols, b.symbolName(step.Symbol))
		}
		interp := Interpretation{
			PrecedingSymbols:      preceding,
			VariableName:          sg.Variables[item.VariableIndex].Ident,
			ProductionStepSymbols: stepSymbols,
			StepIndex:             item.StepIndex,
			Done:                  item.isDone(),
			ConflictingLookahead:  b.symbolName(conflictingLookahead),
			RequiresEOFLookahead:  item.production(sg).requiresEOF,
		}
		if p := item.precedence(sg); p.Kind != lexbuild.PrecNone {
			interp.Precedence = precDisplay(p)
		}
		switch item.associativity(sg) {
		case lexbuild.AssocLeft:
			interp.Associativity = "Left"
		case lexbuild.AssocRight:
			interp.Associativity = "Right"
		}
		conflictError.PossibleInterpretations = append(conflictError.PossibleInterpretations, interp)
	}

	var shiftItems, reduceItems itemSetOrdered
	for _, item := range conflictingItems {
		if item.isDone() {
			reduceItems = append(reduceItems, item)
		} else {
			shiftItems = append(shiftItems, item)
		}
	}
	// conflicting_items is sorted, so both lists already are (sort_unstable
	// of distinct items).

	getRuleNames := func(items itemSetOrdered) []string {
		var result []string
		last, has := uint32(0), false
		for _, item := range items {
			if has && last == item.VariableIndex {
				continue
			}
			last, has = item.VariableIndex, true
			result = append(result, b.symbolName(lexbuild.NonTerminalSym(int(item.VariableIndex))))
		}
		return result
	}

	if len(actualConflict) > 1 {
		if len(shiftItems) > 0 {
			conflictError.PossibleResolutions = append(conflictError.PossibleResolutions, Resolution{ResolvePrecedence, getRuleNames(shiftItems)})
		}
		for _, item := range reduceItems {
			conflictError.PossibleResolutions = append(conflictError.PossibleResolutions, Resolution{ResolvePrecedence, []string{b.symbolName(lexbuild.NonTerminalSym(int(item.VariableIndex)))}})
		}
	}

	if consideredAssociativity {
		conflictError.PossibleResolutions = append(conflictError.PossibleResolutions, Resolution{ResolveAssociativity, getRuleNames(reduceItems)})
	}

	var names []string
	for _, s := range actualConflict {
		names = append(names, b.symbolName(s))
	}
	conflictError.PossibleResolutions = append(conflictError.PossibleResolutions, Resolution{ResolveAddConflict, names})

	b.actualConflicts[conflictKey(actualConflict)] = actualConflict
	return conflictError
}

// keepLast is tables.rs: fn ActionList::keep_last.
func keepLast(e *ParseTableEntry) {
	if len(e.Actions) > 1 {
		e.Actions = []ParseAction{e.Actions[len(e.Actions)-1]}
	}
}

// retainDone is conflicting_items.retain(|item| item.is_done()).
func retainDone(s itemSetOrdered) itemSetOrdered {
	out := s[:0]
	for _, it := range s {
		if it.isDone() {
			out = append(out, it)
		}
	}
	return out
}

func dedupSymbols(s []Symbol) []Symbol {
	if len(s) < 2 {
		return s
	}
	out := s[:1]
	for _, x := range s[1:] {
		if x != out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}

func equalSymbols(a, b []Symbol) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// comparePrecedence is build_parse_table.rs: fn
// ParseTableBuilder::compare_precedence. It returns -1, 0 or 1 for Less,
// Equal and Greater.
func comparePrecedence(grammar *lexbuild.SyntaxGrammar, left lexbuild.Precedence, leftSymbols []Symbol, right lexbuild.Precedence, rightSymbols []Symbol) int {
	precedenceEntryMatches := func(entry lexbuild.PrecedenceEntry, precedence lexbuild.Precedence, symbols []Symbol) bool {
		if !entry.IsSymbol {
			return precedence.Kind == lexbuild.PrecName && precedence.Ident == entry.Ident
		}
		for _, s := range symbols {
			if int(s.Index) >= len(grammar.Variables) {
				panicLikeGenerator("compare_precedence reads variable %d of %d", s.Index, len(grammar.Variables))
			}
			if grammar.Variables[s.Index].Ident == entry.Ident {
				return true
			}
		}
		return false
	}

	switch {
	// Integer precedences can be compared to other integer precedences, and
	// to the default precedence, which is zero.
	case left.Kind == lexbuild.PrecInteger && right.Kind == lexbuild.PrecInteger && (left.Int != 0 || right.Int != 0):
		return cmpInt(left.Int, right.Int)
	case left.Kind == lexbuild.PrecInteger && right.Kind == lexbuild.PrecNone && left.Int != 0:
		return cmpInt(left.Int, 0)
	case left.Kind == lexbuild.PrecNone && right.Kind == lexbuild.PrecInteger && right.Int != 0:
		return cmpInt(0, right.Int)
	}

	// Named precedences can be compared to other named precedences.
	for _, list := range grammar.PrecedenceOrderings {
		sawLeft, sawRight := false, false
		for _, entry := range list {
			matchesLeft := precedenceEntryMatches(entry, left, leftSymbols)
			matchesRight := precedenceEntryMatches(entry, right, rightSymbols)
			if matchesLeft {
				sawLeft = true
				if sawRight {
					return -1
				}
			} else if matchesRight {
				sawRight = true
				if sawLeft {
					return 1
				}
			}
		}
	}
	return 0
}

// getAuxiliaryNodeInfo is build_parse_table.rs: fn
// ParseTableBuilder::get_auxiliary_node_info.
func (b *parseTableBuilder) getAuxiliaryNodeInfo(itemSet *ParseItemSet, symbol Symbol) auxiliarySymbolInfo {
	sg := b.sg
	var parents []Symbol
	for k := range itemSet.Entries {
		item := &itemSet.Entries[k].Item
		if s, ok := item.symbol(sg); ok && s == symbol && sg.Variables[item.VariableIndex].Kind != lexbuild.VarAuxiliary {
			parents = append(parents, lexbuild.NonTerminalSym(int(item.VariableIndex)))
		}
	}
	return auxiliarySymbolInfo{auxiliarySymbol: symbol, parentSymbols: parents}
}
