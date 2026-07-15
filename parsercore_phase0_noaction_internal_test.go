//go:build gts_parsercorephase0

package gotreesitter

import (
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type parserCoreNoActionTestTables struct{}

func (parserCoreNoActionTestTables) Actions(core.StateID, core.Symbol) ([]core.Action, error) {
	return nil, nil
}
func (parserCoreNoActionTestTables) Goto(core.StateID, core.Symbol) (core.StateID, error) {
	return 0, nil
}
func (parserCoreNoActionTestTables) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}
func (parserCoreNoActionTestTables) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

type parserCoreState22CondenseTestTables struct{}

func (parserCoreState22CondenseTestTables) Actions(state core.StateID, lookahead core.Symbol) ([]core.Action, error) {
	switch {
	case state == 1 && lookahead == 86:
		return []core.Action{{Type: core.ActionShift, State: 5}}, nil
	case state == 2 && lookahead == 86:
		return []core.Action{{Type: core.ActionShift, State: 6}}, nil
	case state == 5 && lookahead == 10:
		return []core.Action{{Type: core.ActionReduce, Symbol: 100, ChildCount: 1, DynamicPrecedence: -10}}, nil
	case state == 6 && lookahead == 10:
		return []core.Action{{Type: core.ActionReduce, Symbol: 101, ChildCount: 1, DynamicPrecedence: -10}}, nil
	default:
		return nil, nil
	}
}
func (parserCoreState22CondenseTestTables) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	switch {
	case state == 1 && symbol == 100:
		return 248, nil
	case state == 2 && symbol == 101:
		return 22, nil
	default:
		return 0, nil
	}
}
func (parserCoreState22CondenseTestTables) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}
func (parserCoreState22CondenseTestTables) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

func frozenNoActionTestInputs(t *testing.T) (*core.Core, Token, []diagnosticParserCoreHeader) {
	t.Helper()
	compact, err := core.New(parserCoreNoActionTestTables{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := compact.Seed(254, 579)
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := compact.Seed(193, 580)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := [32]byte{1, 2, 3}
	return compact, Token{Symbol: 20, StartByte: 579, EndByte: 580}, []diagnosticParserCoreHeader{
		{head: paused, creationSeq: 0, checkpoint: checkpoint},
		{head: preserved, creationSeq: 1, shifted: true, checkpoint: checkpoint},
	}
}

func TestFrozenOracleCondenseValidationIsPureAndFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*core.Core, *Token, *[]diagnosticParserCoreHeader, *DiagnosticParserCoreScannerCheckpoint)
	}{
		{name: "missing-clean-sibling", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			*headers = (*headers)[:1]
		}},
		{name: "extra-paused-header", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			*headers = append(*headers, (*headers)[0])
		}},
		{name: "wrong-lookahead-byte", mutate: func(_ *core.Core, token *Token, _ *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			token.EndByte++
		}},
		{name: "wrong-paused-byte", mutate: func(compact *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			head, err := compact.Seed(254, 578)
			if err != nil {
				panic(err)
			}
			(*headers)[0].head = head
		}},
		{name: "wrong-preserved-byte", mutate: func(compact *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			head, err := compact.Seed(193, 581)
			if err != nil {
				panic(err)
			}
			(*headers)[1].head = head
		}},
		{name: "checkpoint-mismatch", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			(*headers)[1].checkpoint[0]++
		}},
		{name: "checkpoint-length-mismatch", mutate: func(_ *core.Core, _ *Token, _ *[]diagnosticParserCoreHeader, preceding *DiagnosticParserCoreScannerCheckpoint) {
			preceding.Length = 1
		}},
		{name: "extra-runnable-header", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			(*headers)[1].shifted = false
		}},
		{name: "accepted-sibling", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *DiagnosticParserCoreScannerCheckpoint) {
			(*headers)[1].accepted = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compact, token, headers := frozenNoActionTestInputs(t)
			preceding := DiagnosticParserCoreScannerCheckpoint{Length: 0, SHA256: headers[0].checkpoint}
			test.mutate(compact, &token, &headers, &preceding)
			before := append([]diagnosticParserCoreHeader(nil), headers...)
			beforeStats, statsErr := compact.Stats(headers[0].head)
			if statsErr != nil {
				t.Fatal(statsErr)
			}
			_, _, err := validateDiagnosticParserCoreFrozenOracleCondense(compact, token, headers, 0, preceding)
			if err == nil {
				t.Fatal("generalized no-action shape was admitted")
			}
			if !reflect.DeepEqual(headers, before) {
				t.Fatalf("negative guard mutated headers: before=%+v after=%+v", before, headers)
			}
			afterStats, statsErr := compact.Stats(headers[0].head)
			if statsErr != nil {
				t.Fatal(statsErr)
			}
			if afterStats != beforeStats {
				t.Fatalf("negative guard mutated compact core: before=%+v after=%+v", beforeStats, afterStats)
			}
		})
	}
}

func TestFrozenOracleCondenseRecordsSkippedTreeCostDecision(t *testing.T) {
	compact, token, headers := frozenNoActionTestInputs(t)
	preceding := DiagnosticParserCoreScannerCheckpoint{Length: 0, SHA256: headers[0].checkpoint}
	receipt, preserved, err := validateDiagnosticParserCoreFrozenOracleCondense(compact, token, headers, 0, preceding)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.OraclePinned || receipt.PausedEffectiveCost != cErrCostPerSkippedTree || receipt.PreservedEffectiveCost != 0 || !receipt.PausedDropped || receipt.PausedResumed || receipt.PrecedingScannerAfter != preceding || receipt.Paused.State != 254 || receipt.Preserved.State != 193 || preserved != headers[1] {
		t.Fatalf("frozen no-action selection=%+v preserved=%+v", receipt, preserved)
	}
}

func TestGenericNoActionDropRequiresSiblingBackedEpochProgress(t *testing.T) {
	tests := []struct {
		name          string
		headers       []diagnosticParserCoreHeader
		paused        []int
		epochProgress bool
		want          bool
	}{
		{name: "sole-no-action", headers: []diagnosticParserCoreHeader{{}}, paused: []int{0}},
		{name: "all-no-action-after-progress", headers: []diagnosticParserCoreHeader{{}, {}}, paused: []int{0, 1}, epochProgress: true},
		{name: "closed-sibling-without-progress", headers: []diagnosticParserCoreHeader{{shifted: true}, {}}, paused: []int{1}},
		{name: "shifted-sibling-after-progress", headers: []diagnosticParserCoreHeader{{shifted: true}, {}}, paused: []int{1}, epochProgress: true, want: true},
		{name: "accepted-sibling-after-progress", headers: []diagnosticParserCoreHeader{{accepted: true}, {}}, paused: []int{1}, epochProgress: true, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := diagnosticParserCoreGenericNoActionDropEligible(test.headers, test.paused, test.epochProgress); got != test.want {
				t.Fatalf("eligibility=%t, want %t", got, test.want)
			}
		})
	}
}

func frozenState22CondenseTestInputs(t *testing.T) (*core.Core, Token, []diagnosticParserCoreHeader) {
	t.Helper()
	compact, err := core.New(parserCoreState22CondenseTestTables{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	preservedSeed, err := compact.Seed(1, 725)
	if err != nil {
		t.Fatal(err)
	}
	preservedShift, err := compact.Shift(preservedSeed, 86, 0, core.Token{Symbol: 86, StartByte: 725, EndByte: 732}, core.ForkOrder{Value: 1, Present: true})
	if err != nil {
		t.Fatal(err)
	}
	preservedFrontier, err := compact.Reduce(preservedShift, 10, 0, core.ForkOrder{})
	if err != nil || len(preservedFrontier) != 1 {
		t.Fatalf("build preserved state248: frontier=%+v err=%v", preservedFrontier, err)
	}
	pausedSeed, err := compact.Seed(2, 725)
	if err != nil {
		t.Fatal(err)
	}
	pausedShift, err := compact.Shift(pausedSeed, 86, 0, core.Token{Symbol: 86, StartByte: 725, EndByte: 730}, core.ForkOrder{Value: 2, Present: true})
	if err != nil {
		t.Fatal(err)
	}
	pausedFrontier, err := compact.Reduce(pausedShift, 10, 0, core.ForkOrder{})
	if err != nil || len(pausedFrontier) != 1 {
		t.Fatalf("build paused state22: frontier=%+v err=%v", pausedFrontier, err)
	}
	checkpoint := parserCoreCheckpoint(nil).SHA256
	return compact, Token{Symbol: 10, Text: "=", StartByte: 731, EndByte: 732}, []diagnosticParserCoreHeader{
		{head: preservedFrontier[0], creationSeq: 1, shifted: true, checkpoint: checkpoint},
		{head: pausedFrontier[0], creationSeq: 2, checkpoint: checkpoint},
	}
}

func TestState22CondenseValidationIsPureAndFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*core.Core, *Token, *[]diagnosticParserCoreHeader, *int, *int)
	}{
		{name: "wrong-token", mutate: func(_ *core.Core, token *Token, _ *[]diagnosticParserCoreHeader, _ *int, _ *int) { token.EndByte++ }},
		{name: "reordered", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *int, _ *int) {
			(*headers)[0], (*headers)[1] = (*headers)[1], (*headers)[0]
		}},
		{name: "missing-paused", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *int, _ *int) {
			*headers = (*headers)[:1]
		}},
		{name: "checkpoint-mismatch", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *int, _ *int) {
			(*headers)[1].checkpoint[0]++
		}},
		{name: "wrong-paused-index", mutate: func(_ *core.Core, _ *Token, _ *[]diagnosticParserCoreHeader, pausedIndex *int, _ *int) {
			*pausedIndex = 0
		}},
		{name: "wrong-election", mutate: func(_ *core.Core, _ *Token, _ *[]diagnosticParserCoreHeader, _ *int, electionIndex *int) {
			*electionIndex = 96
		}},
		{name: "resumed-paused", mutate: func(_ *core.Core, _ *Token, headers *[]diagnosticParserCoreHeader, _ *int, _ *int) {
			(*headers)[1].shifted = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compact, token, headers := frozenState22CondenseTestInputs(t)
			pausedIndex, electionIndex := 1, 97
			test.mutate(compact, &token, &headers, &pausedIndex, &electionIndex)
			before := append([]diagnosticParserCoreHeader(nil), headers...)
			_, _, err := validateDiagnosticParserCoreState22Condense(compact, token, headers, pausedIndex, electionIndex)
			if err == nil {
				t.Fatal("generalized state22 condense shape was admitted")
			}
			if !reflect.DeepEqual(headers, before) {
				t.Fatalf("negative state22 guard mutated headers: before=%+v after=%+v", before, headers)
			}
		})
	}
}

func TestState22CondenseRecordsCostDecompositionAndOrder(t *testing.T) {
	compact, token, headers := frozenState22CondenseTestInputs(t)
	receipt, kept, err := validateDiagnosticParserCoreState22Condense(compact, token, headers, 1, 97)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Executed || !receipt.OraclePinned || !receipt.PausedDropped || receipt.PausedResumed ||
		receipt.PausedOpenRecoveryCost != cErrCostPerRecovery || receipt.PausedSkippedTreeCost != cErrCostPerSkippedTree ||
		receipt.PausedEffectiveCost != cErrCostPerRecovery+cErrCostPerSkippedTree || receipt.PreservedEffectiveCost != 0 ||
		len(kept) != 1 || kept[0] != headers[0] || len(receipt.BeforeBoundaries) != 2 || len(receipt.AfterBoundaries) != 1 ||
		receipt.BeforeBoundaries[0].Score != -10 || receipt.BeforeBoundaries[0].BranchOrder != 1 ||
		receipt.BeforeBoundaries[1].Score != -10 || receipt.BeforeBoundaries[1].BranchOrder != 2 ||
		receipt.AfterBoundaries[0].Score != -10 || receipt.AfterBoundaries[0].BranchOrder != 1 {
		t.Fatalf("state22 condense receipt=%+v kept=%+v", receipt, kept)
	}
}
