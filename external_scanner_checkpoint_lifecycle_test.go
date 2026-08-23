package gotreesitter

import (
	"bytes"
	"testing"
)

type c26nCheckpointPayload struct {
	state []byte
}

type c26nCheckpointScanner struct {
	scannerID           []byte
	grammarID           []byte
	destroyed           int
	deserializeMismatch bool
}

func (s *c26nCheckpointScanner) Create() any {
	return &c26nCheckpointPayload{state: []byte{1, 2, 3}}
}

func (s *c26nCheckpointScanner) Destroy(any) { s.destroyed++ }

func (s *c26nCheckpointScanner) Serialize(payload any, buf []byte) int {
	p, ok := payload.(*c26nCheckpointPayload)
	if !ok || len(p.state) == 0 || len(p.state) > len(buf) {
		return 0
	}
	return copy(buf, p.state)
}

func (s *c26nCheckpointScanner) Deserialize(payload any, buf []byte) {
	p, ok := payload.(*c26nCheckpointPayload)
	if !ok {
		return
	}
	if s.deserializeMismatch {
		p.state = []byte{9, 9, 9}
		return
	}
	p.state = append(p.state[:0], buf...)
}

func (*c26nCheckpointScanner) Scan(any, *ExternalLexer, []bool) bool {
	return false
}

func (*c26nCheckpointScanner) UsesExternalScannerCheckpoints() bool {
	return true
}

func (s *c26nCheckpointScanner) CheckpointIdentity() (ExternalScannerCheckpointIdentity, bool) {
	return ExternalScannerCheckpointIdentity{
		Scanner: s.scannerID,
		Grammar: s.grammarID,
	}, true
}

func newC26nCheckpointScanner() *c26nCheckpointScanner {
	return &c26nCheckpointScanner{
		scannerID: []byte("scanner-c26n"),
		grammarID: []byte("grammar-c26n"),
	}
}

func c26nPayloadState(t *testing.T, payload any) []byte {
	t.Helper()
	p, ok := payload.(*c26nCheckpointPayload)
	if !ok {
		t.Fatalf("payload type = %T, want *c26nCheckpointPayload", payload)
	}
	return append([]byte(nil), p.state...)
}

func TestC26nCheckpointLifecycleRequiresExplicitOptIn(t *testing.T) {
	scanner := newC26nCheckpointScanner()
	if lifecycle, ok := newExternalScannerCheckpointLifecycle(scanner, false); ok || lifecycle != nil {
		t.Fatal("disabled lifecycle allocated an ownership ledger")
	}
	if lifecycle, ok := newExternalScannerCheckpointLifecycle(nil, true); ok || lifecycle != nil {
		t.Fatal("nil scanner enabled a lifecycle")
	}
}

func TestC26nCheckpointLifecycleDiscardsFailedRestore(t *testing.T) {
	scanner := newC26nCheckpointScanner()
	lifecycle, ok := newExternalScannerCheckpointLifecycle(scanner, true)
	if !ok {
		t.Fatal("explicit lifecycle opt-in was rejected")
	}

	rootPayload := scanner.Create()
	rootKey, ok := lifecycle.addRoot(rootPayload, 10, Point{Row: 1}, 7, 10, 10)
	if !ok {
		t.Fatal("root checkpoint was rejected")
	}
	scanner.deserializeMismatch = true
	if _, ok := lifecycle.elect(rootKey, 10, Point{Row: 1}, 7, func(payload any) (Token, bool) {
		payload.(*c26nCheckpointPayload).state = []byte{9, 9, 9}
		return Token{}, false
	}); ok {
		t.Fatal("failed election unexpectedly succeeded")
	}
	if _, exists := lifecycle.versions[rootKey]; exists {
		t.Fatal("failed election retained an unverified payload")
	}
	if scanner.destroyed != 1 {
		t.Fatalf("destroy count after failed election = %d, want 1", scanner.destroyed)
	}

	scanner.deserializeMismatch = false
	resumePayload := scanner.Create()
	resumeKey, ok := lifecycle.addRoot(resumePayload, 11, Point{Row: 1}, 7, 11, 11)
	if !ok {
		t.Fatal("resume checkpoint was rejected")
	}
	scanner.deserializeMismatch = true
	if _, ok := lifecycle.resume(resumeKey, 11, Point{Row: 1}, 7, func(any) (Token, bool) {
		return Token{}, false
	}); ok {
		t.Fatal("failed resume unexpectedly succeeded")
	}
	if _, exists := lifecycle.versions[resumeKey]; exists {
		t.Fatal("failed resume retained an unverified payload")
	}
	if scanner.destroyed != 2 {
		t.Fatalf("destroy count after failed resume = %d, want 2", scanner.destroyed)
	}
	lifecycle.close()
	if scanner.destroyed != 2 {
		t.Fatalf("close destroyed discarded payloads again: count = %d", scanner.destroyed)
	}
}

func TestC26nCheckpointLifecycle(t *testing.T) {
	scanner := newC26nCheckpointScanner()
	lifecycle, ok := newExternalScannerCheckpointLifecycle(scanner, true)
	if !ok {
		t.Fatal("explicit lifecycle opt-in was rejected")
	}
	defer lifecycle.close()

	rootPayload := scanner.Create()
	rootKey, ok := lifecycle.addRoot(rootPayload, 10, Point{Row: 1, Column: 2}, 7, 10, 10)
	if !ok {
		t.Fatal("root checkpoint was rejected")
	}

	selectedToken, ok := lifecycle.elect(rootKey, 10, Point{Row: 1, Column: 2}, 7, func(payload any) (Token, bool) {
		p := payload.(*c26nCheckpointPayload)
		p.state = []byte{4, 5, 6}
		return Token{Symbol: 11, StartByte: 10, EndByte: 12}, true
	})
	if !ok || selectedToken.Symbol != 11 || selectedToken.EndByte != 12 {
		t.Fatalf("election = (%+v, %t), want symbol 11 and success", selectedToken, ok)
	}
	if got := c26nPayloadState(t, rootPayload); !bytes.Equal(got, []byte{4, 5, 6}) {
		t.Fatalf("elected payload = %v, want [4 5 6]", got)
	}
	rootRecord := lifecycle.versions[rootKey].checkpoint.clone()

	failedToken, ok := lifecycle.elect(rootKey, 12, Point{Row: 1, Column: 4}, 7, func(payload any) (Token, bool) {
		p := payload.(*c26nCheckpointPayload)
		p.state = []byte{9, 9, 9}
		return Token{}, false
	})
	if ok || failedToken != (Token{}) {
		t.Fatalf("failed election = (%+v, %t), want zero token and failure", failedToken, ok)
	}
	if got := c26nPayloadState(t, rootPayload); !bytes.Equal(got, []byte{4, 5, 6}) {
		t.Fatalf("failed-election payload = %v, want [4 5 6]", got)
	}
	if !lifecycle.versions[rootKey].checkpoint.equal(rootRecord) {
		t.Fatal("failed election changed the owned checkpoint")
	}

	exactFork, ok := lifecycle.fork(rootKey)
	if !ok {
		t.Fatal("exact fork was rejected")
	}
	if !lifecycle.canShare(rootKey, exactFork) {
		t.Fatal("exact fork did not share its copied checkpoint")
	}
	childPayload := lifecycle.versions[exactFork].payload
	if got := c26nPayloadState(t, childPayload); !bytes.Equal(got, []byte{4, 5, 6}) {
		t.Fatalf("fork payload = %v, want [4 5 6]", got)
	}
	childPayload.(*c26nCheckpointPayload).state[0] = 99
	if got := c26nPayloadState(t, rootPayload); !bytes.Equal(got, []byte{4, 5, 6}) {
		t.Fatalf("fork mutation changed root payload = %v, want [4 5 6]", got)
	}
	if !lifecycle.restoreAndVerify(lifecycle.versions[exactFork]) {
		t.Fatal("exact fork did not restore after an independent payload mutation")
	}
	staleFork, ok := lifecycle.fork(rootKey)
	if !ok {
		t.Fatal("stale fork was rejected")
	}
	lifecycle.versions[staleFork].payload.(*c26nCheckpointPayload).state[0] = 77
	if lifecycle.merge(rootKey, staleFork) {
		t.Fatal("stale checkpoint merge was accepted")
	}
	if _, exists := lifecycle.versions[staleFork]; exists {
		t.Fatal("stale checkpoint remained shareable")
	}
	if !lifecycle.merge(rootKey, exactFork) {
		t.Fatal("exact checkpoint merge was rejected")
	}
	if _, exists := lifecycle.versions[exactFork]; exists {
		t.Fatal("merged fork was not deleted")
	}
	if got := c26nPayloadState(t, rootPayload); !bytes.Equal(got, []byte{4, 5, 6}) {
		t.Fatalf("root payload after merge = %v, want [4 5 6]", got)
	}

	mismatchedFork, ok := lifecycle.fork(rootKey)
	if !ok {
		t.Fatal("mismatched fork was rejected")
	}
	if _, ok := lifecycle.elect(mismatchedFork, 12, Point{Row: 1, Column: 4}, 7, func(payload any) (Token, bool) {
		p := payload.(*c26nCheckpointPayload)
		p.state = []byte{8, 8, 8}
		return Token{Symbol: 12, StartByte: 12, EndByte: 13}, true
	}); !ok {
		t.Fatal("mismatched election was rejected")
	}
	if lifecycle.merge(rootKey, mismatchedFork) {
		t.Fatal("mismatched checkpoint merge was accepted")
	}
	if _, exists := lifecycle.versions[mismatchedFork]; !exists {
		t.Fatal("mismatched version was dropped after merge rejection")
	}

	if _, ok := lifecycle.condense(rootKey, []externalScannerCheckpointVersionKey{mismatchedFork}); ok {
		t.Fatal("condense dropped a live mismatched version")
	}
	if !lifecycle.markDead(mismatchedFork) {
		t.Fatal("mismatched version was not marked dead")
	}
	if !lifecycle.deleteDead(mismatchedFork) {
		t.Fatal("dead version was not deleted")
	}
	if _, exists := lifecycle.versions[mismatchedFork]; exists {
		t.Fatal("dead version remains in the ledger")
	}

	condenseFork, ok := lifecycle.fork(rootKey)
	if !ok {
		t.Fatal("condense fork was rejected")
	}
	if _, ok := lifecycle.condense(rootKey, []externalScannerCheckpointVersionKey{condenseFork}); !ok {
		t.Fatal("exact condense selection was rejected")
	}
	if _, exists := lifecycle.versions[condenseFork]; exists {
		t.Fatal("exact condense sibling was not deleted")
	}

	resumedToken, ok := lifecycle.resume(rootKey, 13, Point{Row: 1, Column: 5}, 7, func(payload any) (Token, bool) {
		p := payload.(*c26nCheckpointPayload)
		if !bytes.Equal(p.state, []byte{4, 5, 6}) {
			return Token{}, false
		}
		p.state = []byte{7, 7, 7}
		return Token{Symbol: 13, StartByte: 13, EndByte: 14}, true
	})
	if !ok || resumedToken.Symbol != 13 {
		t.Fatalf("recovery resume = (%+v, %t), want symbol 13 and success", resumedToken, ok)
	}
	if got := c26nPayloadState(t, rootPayload); !bytes.Equal(got, []byte{7, 7, 7}) {
		t.Fatalf("resumed payload = %v, want [7 7 7]", got)
	}
}
