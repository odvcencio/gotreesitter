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
