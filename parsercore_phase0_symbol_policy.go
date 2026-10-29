//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreSymbolPolicy supplies symbol metadata to recovery and selected-store materialization.
// Cover the complete symbol space. Missing metadata means visible and unnamed.
// Both consumers must use the same defaults to preserve recovery selection.
func parserCoreSymbolPolicy(lang *Language) []core.SelectedSymbolPolicy {
	if lang == nil {
		return nil
	}
	width := max(len(lang.SymbolMetadata), len(lang.SymbolNames), int(lang.SymbolCount))
	out := make([]core.SelectedSymbolPolicy, width)
	for index := range out {
		visible, named := true, false
		if index < len(lang.SymbolMetadata) {
			visible = lang.SymbolMetadata[index].Visible
			named = lang.SymbolMetadata[index].Named
		}
		out[index] = core.SelectedSymbolPolicy{Visible: visible, Named: named}
	}
	return out
}

// recoverySymbolPolicy shares the symbol projection within one parse.
// The scheduler reset discards it before the next parse reads language metadata.
func (s *diagnosticParserCoreGenericScheduler) recoverySymbolPolicy() []core.SelectedSymbolPolicy {
	if s.recoverySymbols == nil {
		s.recoverySymbols = parserCoreSymbolPolicy(s.tokenSource.language)
	}
	return s.recoverySymbols
}

func buildParserCoreSelectedStorePolicy(parser *Parser) (core.SelectedStorePolicy, error) {
	if parser == nil || parser.language == nil || !parser.hasRootSymbol {
		return core.SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store policy requires an authenticated parser root")
	}
	lang := parser.language
	symbols := parserCoreSymbolPolicy(lang)
	width := len(symbols)
	if width != 0 && width > math.MaxInt/width {
		return core.SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store unary policy overflow")
	}
	unary := make([]core.SelectedUnaryRule, width*width)
	for parent := 0; parent < width; parent++ {
		for child := 0; child < width; child++ {
			parentSymbol, childSymbol := Symbol(parent), Symbol(child)
			rule := core.SelectedUnaryKeep
			switch {
			case parentSymbol == childSymbol && !parser.isSharedVisibleAnonymousToken(childSymbol):
				rule = core.SelectedUnaryPass
			case parser.canCollapseInvisibleUnaryWrapperSymbol(parentSymbol):
				rule = core.SelectedUnaryPass
			case parser.canCollapseNamedLeafWrapper(parentSymbol, childSymbol) &&
				!parser.shouldPreserveVisibleUnaryTokenWrapper(parentSymbol) &&
				!parser.shouldKeepVisibleAnonymousTokenChild(parentSymbol, childSymbol):
				rule = core.SelectedUnaryRenameLeaf
			}
			unary[parent*width+child] = rule
		}
	}
	policy, err := core.NewSelectedStorePolicy(symbols, unary, core.Symbol(parser.rootSymbol))
	if err != nil {
		return core.SelectedStorePolicy{}, err
	}
	var supertypeMasks []uint32
	for index := 0; index < width; index++ {
		if mask := lang.supertypeBit(Symbol(index)); mask != 0 {
			if supertypeMasks == nil {
				supertypeMasks = make([]uint32, width)
			}
			supertypeMasks[index] = mask
		}
	}
	if err := policy.SetSupertypeMasks(supertypeMasks); err != nil {
		return core.SelectedStorePolicy{}, err
	}
	retainedAliases := make([]core.SelectedAliasChildPair, 0, len(parser.collapsedChildOccurrencePairs))
	for _, pair := range parser.collapsedChildOccurrencePairs {
		retainedAliases = append(retainedAliases, core.SelectedAliasChildPair{Alias: core.Symbol(pair.parent), Child: core.Symbol(pair.child)})
	}
	policy.SetRetainedAliasChildren(retainedAliases)
	policy.ProjectSyntheticSiblingRoot = true
	syms, _ := goCompatibilitySymbolsForLanguage(lang)
	containers := make([]bool, width)
	for _, symbol := range syms.semiContainers[:syms.semiContainerLen] {
		if int(symbol) < width {
			containers[symbol] = true
		}
	}
	cases := make([]bool, width)
	for _, symbol := range [...]Symbol{syms.expressionCase, syms.defaultCase, syms.typeCase, syms.communicationCase} {
		if symbol != 0 && int(symbol) < width {
			cases[symbol] = true
		}
	}
	statementLists := make([]bool, width)
	for _, symbol := range [...]Symbol{syms.statementList, syms.statementListTail} {
		if symbol != 0 && int(symbol) < width {
			statementLists[symbol] = true
		}
	}
	if err := policy.SetGoCompatibility(core.Symbol(syms.semicolon), core.Symbol(syms.semicolonSentinel), containers, cases, statementLists); err != nil {
		return core.SelectedStorePolicy{}, err
	}
	return policy, nil
}

// The profile is enabled by the authenticated output contract at its caller.
func parserCoreTypeArgumentPolicy(lang *Language) core.SelectedTypeArgumentPolicy {
	symbol := func(name string) core.Symbol {
		value, _ := lang.symbolByNamePreferNamed(name)
		return core.Symbol(value)
	}
	field := func(name string) core.FieldID { value, _ := lang.FieldByName(name); return core.FieldID(value) }
	return core.SelectedTypeArgumentPolicy{
		Call: symbol("call_expression"), Arguments: symbol("argument_list"), Identifier: symbol("identifier"), TypeIdentifier: symbol("type_identifier"),
		Comment: symbol("comment"), TypeArguments: symbol("type_arguments"), Selector: symbol("selector_expression"), Unary: symbol("unary_expression"),
		Qualified: symbol("qualified_type"), Pointer: symbol("pointer_type"), PackageIdentifier: symbol("package_identifier"), Parenthesized: symbol("parenthesized_expression"), ParenthesizedType: symbol("parenthesized_type"),
		FunctionField: field("function"), ArgumentsField: field("arguments"), OperandField: field("operand"), OperatorField: field("operator"), SelectorField: field("field"), PackageField: field("package"), NameField: field("name"),
	}
}
