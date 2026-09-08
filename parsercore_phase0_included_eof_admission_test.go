//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import "testing"

func TestIncludedEOFRecoveryRequiresOwnedTurn(t *testing.T) {
	for _, missing := range []string{"grant", "bundle", "ranges", "turn", "acceptance"} {
		s := &diagnosticParserCoreGenericScheduler{}
		s.options.allowCompactIncludedRangeEOFRecovery = true
		s.options.allowCompactRecoveryVersionTurns = true
		s.options.includedRanges = []Range{{StartByte: 0, EndByte: 1}}
		s.recoveryTurns.active = true
		s.work.RecoverEOFAccepts = 1
		if !s.includedRangeEOFRecoveryAdmitted([]byte("x!")) {
			t.Fatal("complete proof declined")
		}
		switch missing {
		case "grant":
			s.options.allowCompactIncludedRangeEOFRecovery = false
		case "bundle":
			s.options.allowCompactRecoveryVersionTurns = false
		case "ranges":
			s.options.includedRanges = nil
		case "turn":
			s.recoveryTurns.active = false
		case "acceptance":
			s.work.RecoverEOFAccepts = 0
		}
		if s.includedRangeEOFRecoveryAdmitted([]byte("x!")) {
			t.Fatalf("admitted without %s", missing)
		}
	}
}

func TestIncludedEOFRecoveryGrantRequiresOwnedBundle(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	for _, owned := range []bool{false, true} {
		for _, ranges := range []bool{false, true} {
			lang := *p.language
			lang.CompactOwnedEOFRecoveryCertified = owned
			lang.CompactIncludedRangeEOFRecoveryCertified = ranges
			runner, err := newAdmissionCandidateRunner(NewParser(&lang))
			if err != nil {
				t.Fatal(err)
			}
			if got := runner.options.allowCompactIncludedRangeEOFRecovery; got != (owned && ranges) {
				t.Fatalf("owned=%t ranges=%t admitted=%t", owned, ranges, got)
			}
		}
	}
}
