//go:build !gts_no_parsercorephase0

package gotreesitter

import "testing"

func TestMilestoneCompactReuseDefersOnlyCleanAmbiguity(t *testing.T) {
	for _, name := range []string{"clean", "empty", "versioned lexer", "recovery isolation", "recovery header", "paused alternative", "sole paused header", "recovery enabled"} {
		t.Run(name, func(t *testing.T) {
			session := &compactIncrementalReuseSession{reusedSubtrees: 1, reusedBytes: 5000}
			scheduler := diagnosticParserCoreGenericScheduler{}
			scheduler.options.compactIncrementalReuse = session
			scheduler.headers = make([]diagnosticParserCoreHeader, 2)
			switch name {
			case "empty":
				scheduler.headers = nil
			case "versioned lexer":
				scheduler.versionLexerOwnershipActive = true
			case "recovery isolation":
				scheduler.recoveryIsolation = true
			case "recovery header":
				scheduler.headers[1].recoveryFlags = diagnosticParserCoreRecoveryCompetitorFlag
			case "recovery enabled":
				scheduler.options.Recovery = true
			case "sole paused header":
				scheduler.headers = scheduler.headers[:1]
				scheduler.headers[0].paused = true
			case "paused alternative":
				scheduler.headers[1].paused = true
			}
			progressed, err := scheduler.tryCompactIncrementalReuse()
			if progressed {
				t.Fatal("borrowed or consumed while deferring")
			}
			if (err == nil) != (name == "clean" || name == "paused alternative") {
				t.Fatalf("error=%v for %s", err, name)
			}
			if session.reusedSubtrees != 1 || session.reusedBytes != 5000 || len(session.nodes) != 0 {
				t.Fatal("deferral changed borrowed ownership")
			}
		})
	}
}
