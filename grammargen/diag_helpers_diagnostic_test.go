//go:build diagnostic

package grammargen

import "context"

func buildLRTablesWithProvenanceCtx(bgCtx context.Context, ng *NormalizedGrammar) (*LRTables, *lrContext, error) {
	return buildLRTablesInternal(bgCtx, ng, true)
}

func diagProductionMentionsNames(ng *NormalizedGrammar, prod *Production, names []string) bool {
	nameSet := make(map[string]bool, len(names))
	for _, name := range names {
		nameSet[name] = true
	}
	if prod.LHS >= 0 && prod.LHS < len(ng.Symbols) && nameSet[ng.Symbols[prod.LHS].Name] {
		return true
	}
	for _, sym := range prod.RHS {
		if sym >= 0 && sym < len(ng.Symbols) && nameSet[ng.Symbols[sym].Name] {
			return true
		}
	}
	return false
}

func diagStateMentionsNames(ng *NormalizedGrammar, set *lrItemSet, names []string) bool {
	for _, ce := range set.cores {
		if diagProductionMentionsNames(ng, &ng.Productions[ce.prodIdx], names) {
			return true
		}
	}
	return false
}

func diagMergeCount(ctx *lrContext, state int) int {
	if ctx == nil || ctx.provenance == nil {
		return 0
	}
	return len(ctx.provenance.origins(state))
}
