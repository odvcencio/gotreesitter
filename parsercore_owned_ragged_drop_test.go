//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestOwnedRaggedDropRequiresCleanAdvancedSurvivor(t *testing.T) {
	for _, name := range []string{"behind", "overlap", "recovery", "cost", "isolation", "stale", "ahead", "pending", "snapshot"} {
		t.Run(name, func(t *testing.T) {
			compact, err := core.New(&genericConflictTable{}, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			loser, err := compact.Seed(1, 1)
			if err != nil {
				t.Fatal(err)
			}
			winner, err := compact.Seed(2, 4)
			if err != nil {
				t.Fatal(err)
			}
			language := &Language{Name: "ragged-drop"}
			before := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 1)
			after := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 4)
			s := &diagnosticParserCoreGenericScheduler{
				compact:     compact,
				tokenSource: &dfaTokenSource{language: language, lexer: &Lexer{source: []byte("abcd")}},
				headers: []diagnosticParserCoreHeader{
					{head: loser, paused: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: before, lexerRequest: 1}},
					{head: winner, shifted: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: after}},
				},
				electionIndex: 6, epochProgress: true, versionLexerOwnershipActive: true,
				versionLexerRequests: []diagnosticParserCoreVersionLexerRequest{
					newDiagnosticParserCoreOwnedLexerRequest(6, 1, Token{Symbol: 9, StartByte: 1, EndByte: 2}, before, before),
					newDiagnosticParserCoreOwnedLexerRequest(6, 2, Token{Symbol: 9, StartByte: 3, EndByte: 4}, before, after),
				},
			}
			switch name {
			case "overlap":
				s.versionLexerRequests[0].token.EndByte = 4
			case "recovery":
				s.headers[1].markRecoveryLineage()
			case "cost":
				s.headers[1].head, err = compact.ShiftMissingLeaf(winner, 2, 9, 4)
				if err != nil {
					t.Fatal(err)
				}
			case "pending":
				s.headers[1].versionState.lexerRequest = 2
			case "snapshot":
				s.headers[1].versionState.relexSnapshot = before
			case "isolation":
				s.recoveryIsolation = true
			case "stale":
				s.versionLexerRequests[1].electionIndex--
			case "ahead":
				s.headers[0].head, err = compact.Seed(1, 5)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := s.versionLexerNoActionDropEligible([]int{0}); got != (name == "behind") {
				t.Fatalf("eligible=%v", got)
			}
		})
	}
}
