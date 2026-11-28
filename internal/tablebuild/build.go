package tablebuild

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// BuildLanguage runs the whole table build on the bytes of a grammar.json and
// returns the tables as a Language, as the generator's parser.c holds them:
// Prepare, GetVariableInfo, BuildRawTables, BuildTables with OptMergeStates,
// RenderTables and Layout.Language, in the generator's order
// (generate.rs: fn generate_parser_for_grammar_with_opts). abiVersion is the
// ABI the generator writes (ABIVersionMin to ABIVersionMax). Any error that
// the generator reports is returned; the build never falls back to another
// method.
func BuildLanguage(grammarJSON []byte, name string, abiVersion int) (*gotreesitter.Language, error) {
	g, err := Prepare(grammarJSON)
	if err != nil {
		return nil, err
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		return nil, err
	}
	raw, err := BuildRawTables(g, info)
	if err != nil {
		return nil, err
	}
	tables, err := finishTables(g, raw, OptMergeStates)
	if err != nil {
		return nil, err
	}
	layout, err := RenderTables(tables, g, GetSupertypeSymbolMap(g.Syntax, g.DefaultAliases, info), abiVersion)
	if err != nil {
		return nil, err
	}
	return layout.Language(name)
}
