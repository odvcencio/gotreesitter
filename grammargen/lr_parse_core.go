package grammargen

import (
	"sort"
	"strconv"
)

// yamlReferenceScannerRows confines the reference numbering contract to
// the authored YAML grammar with its external-only lexer.
func yamlReferenceScannerRows(ng *NormalizedGrammar) bool {
	return ng != nil && ng.externalLexerOnly && ng.GrammarName == "yaml"
}

// buildCompactStateCoreKeys records the C-compatible core of each completed
// parser state. It does not change construction or conflict resolution.
func buildCompactStateCoreKeys(ng *NormalizedGrammar, states []lrItemSet) []string {
	keysByProduction := buildCompactProductionDotKeys(ng)
	stateKeys := make([]string, len(states))
	for stateIndex, state := range states {
		items := make(map[string]struct{}, len(state.cores))
		for _, item := range state.cores {
			items[keysByProduction[item.prodIdx][item.dot]] = struct{}{}
		}
		ordered := make([]string, 0, len(items))
		for item := range items {
			ordered = append(ordered, item)
		}
		sort.Strings(ordered)
		stateKeys[stateIndex] = encodeStringList(ordered)
	}
	return stateKeys
}

func buildCompactProductionDotKeys(ng *NormalizedGrammar) [][]string {
	keys := make([][]string, len(ng.Productions))
	fieldCarriers := compactInheritedFieldCarriers(ng)
	for productionIndex, production := range ng.Productions {
		keys[productionIndex] = make([]string, len(production.RHS)+1)
		for dot := 0; dot <= len(production.RHS); dot++ {
			hasInheritedFields := false
			for child := 0; child < dot; child++ {
				symbol := production.RHS[child]
				if symbol >= ng.TokenCount() && fieldCarriers[symbol] {
					hasInheritedFields = true
					break
				}
			}
			keys[productionIndex][dot] = compactProductionDotKey(production, dot, hasInheritedFields)
		}
	}
	return keys
}

func compactProductionDotKey(production Production, dot int, hasInheritedFields bool) string {
	buf := make([]byte, 0, 64+len(production.RHS)*12)
	appendInt := func(value int) {
		buf = strconv.AppendInt(buf, int64(value), 10)
		buf = append(buf, ';')
	}
	appendString := func(value string) {
		appendInt(len(value))
		buf = append(buf, value...)
		buf = append(buf, ';')
	}
	fieldAt := func(child int) string {
		for _, field := range production.Fields {
			if field.ChildIndex == child {
				return field.FieldName
			}
		}
		return ""
	}
	aliasAt := func(child int) (string, bool) {
		for _, alias := range production.Aliases {
			if alias.ChildIndex == child {
				return alias.Name, alias.Named
			}
		}
		return "", false
	}

	appendInt(production.LHS)
	appendInt(dot)
	appendInt(len(production.RHS))
	appendInt(production.Prec)
	if production.HasExplicitPrec {
		appendInt(1)
	} else {
		appendInt(0)
	}
	appendInt(int(production.Assoc))
	appendInt(production.DynPrec)
	if production.IsExtra {
		appendInt(1)
	} else {
		appendInt(0)
	}
	if hasInheritedFields {
		appendInt(1)
	} else {
		appendInt(0)
	}
	for child := range production.RHS {
		if child >= dot || hasInheritedFields {
			appendInt(production.RHS[child])
		}
		alias, named := aliasAt(child)
		appendString(alias)
		if named {
			appendInt(1)
		} else {
			appendInt(0)
		}
		appendString(fieldAt(child))
	}
	return string(buf)
}

func compactInheritedFieldCarriers(ng *NormalizedGrammar) []bool {
	carriers := make([]bool, len(ng.Symbols))
	hasFieldAt := func(production Production, child int) bool {
		for _, field := range production.Fields {
			if field.ChildIndex == child {
				return true
			}
		}
		return false
	}
	isHiddenNonterminal := func(symbol int) bool {
		return symbol >= ng.TokenCount() && symbol < len(ng.Symbols) && !ng.Symbols[symbol].Visible
	}
	for _, production := range ng.Productions {
		if isHiddenNonterminal(production.LHS) && len(production.Fields) > 0 {
			carriers[production.LHS] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, production := range ng.Productions {
			if !isHiddenNonterminal(production.LHS) || carriers[production.LHS] {
				continue
			}
			for child, symbol := range production.RHS {
				if yamlReferenceScannerRows(ng) {
					visible := false
					for _, alias := range production.Aliases {
						if alias.ChildIndex == child {
							visible = true
							break
						}
					}
					if _, ok := ng.defaultAliasDisplays[symbol]; ok {
						visible = true
					}
					if visible {
						continue
					}
				}
				if isHiddenNonterminal(symbol) && carriers[symbol] && !hasFieldAt(production, child) {
					carriers[production.LHS] = true
					changed = true
					break
				}
			}
		}
	}
	return carriers
}

func encodeStringList(values []string) string {
	buf := make([]byte, 0, len(values)*8)
	for _, value := range values {
		buf = strconv.AppendInt(buf, int64(len(value)), 10)
		buf = append(buf, ';')
		buf = append(buf, value...)
	}
	return string(buf)
}
