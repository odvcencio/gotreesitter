package scannercert

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// Fault injection proves that byte round trips alone cannot certify reuse,
// and that mutation diagnostics distinguish preservation from retention.
func RunContractFaults(t *testing.T, api LexerAPI) {
	for _, tc := range []struct {
		name    string
		scanner certificationFaultScanner
		state   certificationFaultState
		kind    string
	}{
		{name: "codec", scanner: certificationFaultScanner{dropEncoded: true}, state: certificationFaultState{encoded: 1}, kind: "roundtrip"},
		{name: "hidden-decision", scanner: certificationFaultScanner{}, state: certificationFaultState{hidden: true}, kind: "replay"},
		{name: "hidden-span", scanner: certificationFaultScanner{spanOnly: true}, state: certificationFaultState{hidden: true}, kind: "replay"},
		{name: "false-preservation", scanner: certificationFaultScanner{mutateFailure: true, preserving: true}, kind: "failure-mutation"},
		{name: "declared-retention", scanner: certificationFaultScanner{mutateFailure: true, retaining: true}, kind: "failure-mutation"},
		{name: "absent-checkpoint", scanner: certificationFaultScanner{absent: true}, state: certificationFaultState{hidden: true}, kind: "absent-replay"},
		{name: "trusted-panic", scanner: certificationFaultScanner{panicScan: true}, kind: "scan-panic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cert := newScannerCertification(tc.scanner, api)
			defer cert.close()
			cert.Scan(&tc.state, api.New([]byte("x"), 0), []bool{true})
			if _, exists := cert.failures[tc.kind]; !exists {
				t.Fatalf("missed %s contract fault: %v", tc.kind, cert.failures)
			}
			if tc.name == "false-preservation" && !cert.required(tc.kind) {
				t.Fatal("false preservation did not fail certification")
			}
			if (tc.scanner.retaining || tc.scanner.absent) && cert.required(tc.kind) {
				t.Fatal("declared retention or absent checkpoint became a reuse certificate")
			}
		})
	}

	t.Run("dirty-destination", func(t *testing.T) {
		cert := newScannerCertification(certificationFaultScanner{}, api)
		defer cert.close()
		cert.replayPayload.(*certificationFaultState).hidden = true
		cert.Scan(&certificationFaultState{}, api.New(nil, 0), nil)
		if _, exists := cert.failures["replay"]; !exists {
			t.Fatal("missed stale unencoded destination state")
		}
	})

	t.Run("nil-payload-buffer", func(t *testing.T) {
		cert := newScannerCertification(&certificationNilPayloadFaultScanner{}, api)
		defer cert.close()
		cert.Scan(nil, api.New(nil, 0), nil)
		for _, kind := range []string{"stateless-state", "failure-mutation", "replay"} {
			if _, exists := cert.failures[kind]; !exists || !cert.required(kind) {
				t.Fatalf("nil-payload scratch buffer erased %s evidence: %v", kind, cert.failures)
			}
		}
	})
}

type certificationNilPayloadFaultScanner struct{ hidden byte }

func (*certificationNilPayloadFaultScanner) Create() any { return nil }
func (*certificationNilPayloadFaultScanner) Destroy(any) {}
func (s *certificationNilPayloadFaultScanner) Serialize(_ any, buffer []byte) int {
	buffer[0] = s.hidden
	return 1
}
func (*certificationNilPayloadFaultScanner) Deserialize(any, []byte) {}
func (s *certificationNilPayloadFaultScanner) Scan(any, *gts.ExternalLexer, []bool) bool {
	s.hidden++
	return false
}
func (*certificationNilPayloadFaultScanner) ExternalScannerIsStateless() bool  { return true }
func (*certificationNilPayloadFaultScanner) PreservesStateOnScanFailure() bool { return true }

type certificationFaultState struct {
	encoded byte
	hidden  bool
}

type certificationFaultScanner struct {
	dropEncoded, spanOnly, mutateFailure, preserving, retaining, absent, panicScan bool
}

func (certificationFaultScanner) Create() any { return &certificationFaultState{} }
func (certificationFaultScanner) Destroy(any) {}
func (s certificationFaultScanner) Serialize(payload any, buffer []byte) int {
	if s.absent {
		return 0
	}
	buffer[0] = payload.(*certificationFaultState).encoded
	return 1
}
func (s certificationFaultScanner) Deserialize(payload any, buffer []byte) {
	state := payload.(*certificationFaultState)
	state.encoded = 0
	if len(buffer) != 0 && !s.dropEncoded {
		state.encoded = buffer[0]
	}
	// Intentionally omit hidden to model incomplete checkpoint state.
}
func (s certificationFaultScanner) Scan(payload any, lexer *gts.ExternalLexer, _ []bool) bool {
	state := payload.(*certificationFaultState)
	if s.panicScan {
		panic("scanner contract fault")
	}
	if s.mutateFailure {
		state.encoded++
		return false
	}
	if s.spanOnly {
		if state.hidden {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(1)
		return true
	}
	return state.hidden
}
func (certificationFaultScanner) UsesExternalScannerCheckpoints() bool { return true }
func (s certificationFaultScanner) PreservesStateOnScanFailure() bool  { return s.preserving }
func (s certificationFaultScanner) RetainsStateOnScanFailure() bool    { return s.retaining }
