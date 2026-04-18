package grammargen

import (
	"fmt"
	"strings"
)

func diagFindAllSymbols(ng *NormalizedGrammar, name string) []int {
	var ids []int
	for i, sym := range ng.Symbols {
		if sym.Name == name {
			ids = append(ids, i)
		}
	}
	return ids
}

func diagFormatProd(ng *NormalizedGrammar, prodIdx, dot int) string {
	prod := &ng.Productions[prodIdx]
	var rhs []string
	for i, sym := range prod.RHS {
		if i == dot {
			rhs = append(rhs, "*")
		}
		rhs = append(rhs, diagSymbolName(ng, sym))
	}
	if dot == len(prod.RHS) {
		rhs = append(rhs, "*")
	}
	return fmt.Sprintf("%s -> %s", diagSymbolName(ng, prod.LHS), strings.Join(rhs, " "))
}

func diagFormatActions(ng *NormalizedGrammar, acts []lrAction) string {
	if len(acts) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(acts))
	for _, act := range acts {
		switch act.kind {
		case lrShift:
			parts = append(parts, fmt.Sprintf("shift(state=%d,lhs=%s)", act.state, diagSymbolName(ng, act.lhsSym)))
		case lrReduce:
			parts = append(parts, fmt.Sprintf("reduce(prod=%d,%s)", act.prodIdx, diagFormatProd(ng, act.prodIdx, len(ng.Productions[act.prodIdx].RHS))))
		case lrAccept:
			parts = append(parts, "accept")
		default:
			parts = append(parts, fmt.Sprintf("kind=%d", act.kind))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func diagSymbolName(ng *NormalizedGrammar, sym int) string {
	if sym < 0 || sym >= len(ng.Symbols) {
		return fmt.Sprintf("sym%d", sym)
	}
	return fmt.Sprintf("%s(%d)", ng.Symbols[sym].Name, sym)
}
