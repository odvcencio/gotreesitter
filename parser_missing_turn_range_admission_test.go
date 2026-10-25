package gotreesitter

import (
	"encoding/hex"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/recoveryturn"
)

type missingTurnRangeExclusionScanner struct {
	deny  [32]byte
	calls int
}

func (*missingTurnRangeExclusionScanner) Create() any                           { return nil }
func (*missingTurnRangeExclusionScanner) Destroy(any)                           {}
func (*missingTurnRangeExclusionScanner) Serialize(any, []byte) int             { return 0 }
func (*missingTurnRangeExclusionScanner) Deserialize(any, []byte)               {}
func (*missingTurnRangeExclusionScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (s *missingTurnRangeExclusionScanner) ExcludesMissingVersionTurnsForIncludedRanges(identity [32]byte) bool {
	s.calls++
	return identity == s.deny
}

func missingTurnRangeIdentity(t *testing.T, text string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(text)
	if err != nil || len(b) != 32 {
		t.Fatalf("invalid artifact identity %q", text)
	}
	var identity [32]byte
	copy(identity[:], b)
	return identity
}

func TestMissingTurnRangeExclusionPreservesModeAndArtifactBoundaries(t *testing.T) {
	current := missingTurnRangeIdentity(t, "9ad1f8d7d0f2b9aa32c61bd8295b56177caabb0ae2fb8dd4617559ee0f71fed4")
	previous := missingTurnRangeIdentity(t, "214322327c9ee7dda29febf32e415f7ed9e00a90b7690d3c32522e393211b8da")
	scanner := &missingTurnRangeExclusionScanner{deny: current}
	lang := &Language{RecoveryMissingVersionTurnsCertified: true, ExternalScanner: scanner, grammarBlobSHA256: current, grammarBlobSHA256Valid: true}
	p := &Parser{language: lang}
	if !p.recoveryMissingVersionTurnsCertified() || scanner.calls != 0 {
		t.Fatal("ordinary parsing changed certification or invoked the range hook")
	}
	p.included = []Range{{StartByte: 0, EndByte: 1}}
	if p.recoveryMissingVersionTurnsCertified() || scanner.calls != 1 {
		t.Fatal("current artifact's included-range exclusion was not applied")
	}
	lang.grammarBlobSHA256 = previous
	if !p.recoveryMissingVersionTurnsCertified() {
		t.Fatal("the current-artifact exclusion changed an older artifact")
	}
	lang.grammarBlobSHA256Valid = false
	before := scanner.calls
	if !p.recoveryMissingVersionTurnsCertified() || scanner.calls != before {
		t.Fatal("an unknown artifact acquired an exclusion")
	}
	lang.grammarBlobSHA256, lang.grammarBlobSHA256Valid = current, true
	lang.ExternalScanner = nil
	if !p.recoveryMissingVersionTurnsCertified() {
		t.Fatal("a certified grammar without the optional hook changed policy (including SQL)")
	}
	lang.ExternalScanner = scanner
	lang.RecoveryMissingVersionTurnsCertified = false
	before = scanner.calls
	if p.recoveryMissingVersionTurnsCertified() || scanner.calls != before {
		t.Fatal("the exclusion hook granted certification or ran for an uncertified grammar")
	}
	lang.RecoveryMissingVersionTurnsCertified = true
	p.included = nil
	before = scanner.calls
	if !p.recoveryMissingVersionTurnsCertified() || scanner.calls != before || !lang.RecoveryMissingVersionTurnsCertified {
		t.Fatal("returning to ordinary mode changed the shared language certification")
	}
	var nilParser *Parser
	if nilParser.recoveryMissingVersionTurnsCertified() || (&Parser{}).recoveryMissingVersionTurnsCertified() {
		t.Fatal("a nil parser/language received missing-turn certification")
	}
}

func TestMissingTurnRangeExclusionPreservesNativeAndResyncDispatch(t *testing.T) {
	p := cRecoveryElectionTestParser()
	identity := missingTurnRangeIdentity(t, "9ad1f8d7d0f2b9aa32c61bd8295b56177caabb0ae2fb8dd4617559ee0f71fed4")
	p.language.RecoveryMissingVersionTurnsCertified = true
	p.language.grammarBlobSHA256, p.language.grammarBlobSHA256Valid = identity, true
	p.language.ExternalScanner = &missingTurnRangeExclusionScanner{deny: identity}
	group := &cRecGroup{eagerMissingShiftEnd: 21}
	absorbing, missing := newGLRStack(1), newGLRStack(2)
	absorbing.byteOffset, missing.byteOffset = 21, 21
	absorbing.cRec = &cRecoverState{group: group}
	missing.cRecoverMissingGroup = group
	missing.cRecoveryDispatchPending = recoveryturn.Missing
	stacks := []glrStack{absorbing, missing}
	if p.cBetterVersionExists(stacks, 0, true, cErrCostPerSkippedTree, true) {
		t.Fatal("ordinary missing-version deferral changed")
	}
	p.included = []Range{{StartByte: 0, EndByte: 1}}
	if !p.cBetterVersionExists(stacks, 0, true, cErrCostPerSkippedTree, true) {
		t.Fatal("excluded range mode deferred missing-version competition")
	}
	stacks[1].cRecoveryDispatchPending = recoveryturn.Resync
	if p.cBetterVersionExists(stacks, 0, true, cErrCostPerSkippedTree, true) {
		t.Fatal("range containment disabled independent Resync scheduling")
	}
	stacks[1].cRecoveryDispatchPending = recoveryturn.Missing
	p.language.CompactPackedGSSVersionOrderCertified = true
	p.mergeScratch = &glrMergeScratch{packedGSSVersionOrderActive: true}
	if p.cBetterVersionExists(stacks, 0, true, cErrCostPerSkippedTree, true) {
		t.Fatal("range containment disabled independently certified native version order")
	}
}
