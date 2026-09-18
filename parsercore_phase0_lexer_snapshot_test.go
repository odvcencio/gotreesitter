//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func newDiagnosticParserCoreOwnedLexerSnapshot(
	t *testing.T,
	compact *core.Core,
	language *Language,
	lexerPosition int,
) *diagnosticParserCoreVersionLexerSnapshot {
	t.Helper()
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var snapshotErr error
		snapshot, snapshotErr = newDiagnosticParserCoreVersionLexerSnapshot(
			compact, language, owner,
			dfaRelexSnapshot{lexerPos: lexerPosition}, 0, 0,
		)
		return snapshotErr
	})
	if err != nil {
		t.Fatalf("construct owned lexer snapshot: %v", err)
	}
	return snapshot
}
