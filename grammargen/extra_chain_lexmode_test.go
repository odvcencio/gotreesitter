package grammargen

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNonterminalExtraChainLexModesDoNotInheritTerminalExtras(t *testing.T) {
	g := NewGrammar("extra_chain_lexmode")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`.`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build LR tables: %v", err)
	}
	addNonterminalExtraChains(tables, ng, ctx)

	slashStarSyms := diagFindAllSymbols(ng, "/*")
	if len(slashStarSyms) != 1 {
		t.Fatalf("expected one /* symbol, got %v", slashStarSyms)
	}
	whitespaceSyms := diagFindAllSymbols(ng, "_whitespace")
	if len(whitespaceSyms) != 1 {
		t.Fatalf("expected one _whitespace symbol, got %v", whitespaceSyms)
	}
	closeCommentSyms := diagFindAllSymbols(ng, "*/")
	if len(closeCommentSyms) != 1 {
		t.Fatalf("expected one */ symbol, got %v", closeCommentSyms)
	}

	acts := tables.ActionTable[0][slashStarSyms[0]]
	if len(acts) != 1 || acts[0].kind != lrShift {
		t.Fatalf("expected synthetic extra-chain shift on /*, got %s", diagFormatActions(ng, acts))
	}
	target := acts[0].state
	if target < tables.ExtraChainStateStart {
		t.Fatalf("expected synthetic state >= %d, got %d", tables.ExtraChainStateStart, target)
	}

	lexModes, stateToMode := computeLexModes(
		tables.StateCount,
		ng.TokenCount(),
		func(state, sym int) bool {
			if bySym, ok := tables.ActionTable[state]; ok {
				if acts, ok := bySym[sym]; ok && len(acts) > 0 {
					return true
				}
			}
			return false
		},
		computeStringPrefixExtensions(ng.Terminals),
		ng.ExtraSymbols,
		tables.ExtraChainStateStart,
		map[int]bool{},
		ng.ExternalSymbols,
		ng.WordSymbolID,
		map[int]bool{},
	)

	initialMode := lexModes[stateToMode[0]]
	if !initialMode.skipWhitespace {
		t.Fatal("initial state should still skip whitespace extras")
	}
	if !initialMode.validSymbols[whitespaceSyms[0]] {
		t.Fatal("initial state should keep terminal extra valid")
	}

	chainMode := lexModes[stateToMode[target]]
	if chainMode.skipWhitespace {
		t.Fatal("synthetic extra-chain state should not skip whitespace")
	}
	if chainMode.validSymbols[whitespaceSyms[0]] {
		t.Fatal("synthetic extra-chain state should not inherit terminal extra symbols")
	}
	if !chainMode.validSymbols[closeCommentSyms[0]] {
		t.Fatal("synthetic extra-chain state should still accept the explicit comment terminator token")
	}
}

func TestNonterminalExtraChainRuntimeProducesReducedExtraNode(t *testing.T) {
	g := NewGrammar("extra_chain_runtime")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tree, err := gotreesitter.NewParser(report.Language).Parse([]byte("/**/foo"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 16))
	}
	if root.EndByte() != 7 {
		t.Fatalf("root end byte = %d, want 7", root.EndByte())
	}
	if root.ChildCount() < 2 {
		t.Fatalf("root child count = %d, want at least 2", root.ChildCount())
	}
	if got := root.Child(0).Type(report.Language); got != "block_comment" {
		t.Fatalf("child[0] type = %q, want block_comment; sexpr=%s", got, safeSExpr(root, report.Language, 16))
	}
	if got := root.Child(1).Type(report.Language); got != "item" {
		t.Fatalf("child[1] type = %q, want item; sexpr=%s", got, safeSExpr(root, report.Language, 16))
	}
}
