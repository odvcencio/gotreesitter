//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type sharedRecoveryAliasTable struct{ genericConflictTable }

func (*sharedRecoveryAliasTable) ProductionAliases(production uint16, count int) ([]core.Symbol, error) {
	if production == 1 && count == 1 {
		return []core.Symbol{3}, nil
	}
	return nil, nil
}

func TestCompactSharedRecoveryAuthenticatesDirectTerminalAlias(t *testing.T) {
	table := &sharedRecoveryAliasTable{genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 1}: {{Type: core.ActionShift, State: 2}},
			{state: 2, symbol: 0}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 3},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer compact.ResetReleasingRetention()
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	shifted, err := compact.Shift(seed, 1, 0, core.Token{Symbol: 1, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	heads, err := compact.Reduce(shifted, 0, 0, core.ForkOrder{})
	if err != nil || len(heads) != 1 {
		t.Fatalf("reduction heads=%v err=%v", heads, err)
	}
	paths, err := compact.Derivations(heads[0])
	if err != nil || len(paths) != 1 {
		t.Fatalf("derivations=%v err=%v", paths, err)
	}
	lang := &Language{
		Name: "shared_alias_fixture", TokenCount: 2, SymbolCount: 4,
		SymbolNames:    []string{"end", "identifier", "root", "type_identifier"},
		SymbolMetadata: []SymbolMetadata{{}, {Visible: true, Named: true}, {Visible: true, Named: true}, {Visible: true, Named: true}},
	}
	parser := &Parser{language: lang, reduceAliasSeq: [][]Symbol{nil, {3}}}
	var scratch parserCoreRunnerScratch
	tree, err := materializeDiagnosticParserCoreAcceptedSelection(compact, heads[0], paths[0].Payloads, parser, []byte("x"), &scratch, false, true)
	if err != nil || tree == nil {
		t.Fatalf("shared alias materialization: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.ChildCount() != 1 || root.Child(0).Symbol() != 3 || root.Child(0).StartByte() != 0 || root.Child(0).EndByte() != 1 {
		t.Fatalf("shared alias projection=%s", root.SExpr(lang))
	}
	if lang.CompactOwnedEOFRecoveryCertified || len(lang.CompactRecoveryTerminalAliasRules) != 0 {
		t.Fatal("fixture unexpectedly supplied artifact alias authority")
	}
}
