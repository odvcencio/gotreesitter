package gotreesitter

import (
	"bytes"
	"testing"
)

type c26lCheckpointPayload struct {
	state []byte
}

type c26lCheckpointScanner struct {
	scannerID           []byte
	grammarID           []byte
	identityOK          bool
	checkpoint          bool
	serializeOK         bool
	serializeTooLong    bool
	deserializeMismatch bool
}

func (s *c26lCheckpointScanner) Create() any {
	return &c26lCheckpointPayload{state: []byte{1, 2, 3}}
}

func (*c26lCheckpointScanner) Destroy(any) {}

func (s *c26lCheckpointScanner) Serialize(payload any, buf []byte) int {
	if !s.serializeOK {
		return 0
	}
	if s.serializeTooLong {
		return len(buf) + 1
	}
	p, ok := payload.(*c26lCheckpointPayload)
	if !ok || len(p.state) == 0 || len(p.state) > len(buf) {
		return 0
	}
	return copy(buf, p.state)
}

func (s *c26lCheckpointScanner) Deserialize(payload any, buf []byte) {
	p, ok := payload.(*c26lCheckpointPayload)
	if !ok {
		return
	}
	if s.deserializeMismatch {
		p.state = []byte{9, 9, 9}
		return
	}
	p.state = append(p.state[:0], buf...)
}

func (*c26lCheckpointScanner) Scan(any, *ExternalLexer, []bool) bool {
	return false
}

func (s *c26lCheckpointScanner) UsesExternalScannerCheckpoints() bool {
	return s.checkpoint
}

func (s *c26lCheckpointScanner) CheckpointIdentity() (ExternalScannerCheckpointIdentity, bool) {
	return ExternalScannerCheckpointIdentity{
		Scanner: s.scannerID,
		Grammar: s.grammarID,
	}, s.identityOK
}

type c26lPlainScanner struct{}

func (*c26lPlainScanner) Create() any               { return nil }
func (*c26lPlainScanner) Destroy(any)               {}
func (*c26lPlainScanner) Serialize(any, []byte) int { return 0 }
func (*c26lPlainScanner) Deserialize(any, []byte)   {}
func (*c26lPlainScanner) Scan(any, *ExternalLexer, []bool) bool {
	return false
}

func newC26lCheckpointScanner() *c26lCheckpointScanner {
	return &c26lCheckpointScanner{
		scannerID:   []byte("scanner-c26l"),
		grammarID:   []byte("grammar-c26l"),
		identityOK:  true,
		checkpoint:  true,
		serializeOK: true,
	}
}

func captureC26lRecord(t *testing.T, scanner *c26lCheckpointScanner, payload any) externalScannerCheckpointRecord {
	t.Helper()
	record, ok := captureExternalScannerCheckpointRecord(scanner, payload, 17, Point{Row: 2, Column: 3}, 9, 17, 21)
	if !ok {
		t.Fatal("captureExternalScannerCheckpointRecord rejected a complete checkpoint")
	}
	return record
}

func TestC26lCheckpointRejectsAbsentCapability(t *testing.T) {
	if _, ok := captureExternalScannerCheckpointRecord(&c26lPlainScanner{}, nil, 0, Point{}, 0, 0, 0); ok {
		t.Fatal("capture accepted a scanner without the identity capability")
	}
}

func TestC26lCheckpointRejectsIncompleteIdentityAndSerialization(t *testing.T) {
	tests := map[string]func(*c26lCheckpointScanner){
		"missing identity": func(scanner *c26lCheckpointScanner) {
			scanner.identityOK = false
		},
		"empty scanner identity": func(scanner *c26lCheckpointScanner) {
			scanner.scannerID = nil
		},
		"empty grammar identity": func(scanner *c26lCheckpointScanner) {
			scanner.grammarID = nil
		},
		"oversized scanner identity": func(scanner *c26lCheckpointScanner) {
			scanner.scannerID = make([]byte, externalScannerCheckpointIdentityMaxBytes+1)
		},
		"oversized grammar identity": func(scanner *c26lCheckpointScanner) {
			scanner.grammarID = make([]byte, externalScannerCheckpointIdentityMaxBytes+1)
		},
		"incomplete serialization": func(scanner *c26lCheckpointScanner) {
			scanner.serializeOK = false
		},
		"serialization length exceeds buffer": func(scanner *c26lCheckpointScanner) {
			scanner.serializeTooLong = true
		},
		"checkpoint disabled": func(scanner *c26lCheckpointScanner) {
			scanner.checkpoint = false
		},
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			scanner := newC26lCheckpointScanner()
			configure(scanner)
			if _, ok := captureExternalScannerCheckpointRecord(scanner, scanner.Create(), 0, Point{}, 0, 0, 0); ok {
				t.Fatal("capture accepted an incomplete checkpoint")
			}
		})
	}
}

func TestC26lCheckpointRejectsInvertedTokenSpan(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	if _, ok := captureExternalScannerCheckpointRecord(scanner, scanner.Create(), 0, Point{}, 0, 8, 7); ok {
		t.Fatal("capture accepted an inverted token span")
	}

	record := captureC26lRecord(t, scanner, scanner.Create())
	inverted := record.clone()
	inverted.tokenStartByte = inverted.tokenEndByte + 1
	if inverted.complete() {
		t.Fatal("record completeness accepted an inverted token span")
	}
	if canShareExternalScannerCheckpoint(record, inverted) {
		t.Fatal("share accepted an inverted token span")
	}
}

func TestC26lCheckpointRejectsInternallyOverlongRecord(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	record := captureC26lRecord(t, scanner, scanner.Create())
	record.serialized = make([]byte, externalScannerSerializationBufferSize+1)
	if record.complete() {
		t.Fatal("record completeness accepted overlong serialized state")
	}
	if canShareExternalScannerCheckpoint(record, record) {
		t.Fatal("share accepted an overlong serialized state")
	}
}

func TestC26lCheckpointOwnsIdentityAndStateBytes(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	payload := scanner.Create().(*c26lCheckpointPayload)
	record := captureC26lRecord(t, scanner, payload)

	scanner.scannerID[0] = 'X'
	scanner.grammarID[0] = 'Y'
	payload.state[0] = 99
	if !bytes.Equal(record.identity.Scanner, []byte("scanner-c26l")) {
		t.Fatalf("record scanner identity changed through alias: %q", record.identity.Scanner)
	}
	if !bytes.Equal(record.identity.Grammar, []byte("grammar-c26l")) {
		t.Fatalf("record grammar identity changed through alias: %q", record.identity.Grammar)
	}
	if !bytes.Equal(record.serialized, []byte{1, 2, 3}) {
		t.Fatalf("record serialized state changed through alias: %v", record.serialized)
	}
}

func TestC26lCheckpointForkCopiesIndependently(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	record := captureC26lRecord(t, scanner, scanner.Create())
	fork := record.clone()
	fork.identity.Scanner[0] = 'F'
	fork.serialized[0] = 42

	if bytes.Equal(record.identity.Scanner, fork.identity.Scanner) {
		t.Fatal("fork identity aliases the parent")
	}
	if bytes.Equal(record.serialized, fork.serialized) {
		t.Fatal("fork state aliases the parent")
	}
	if canShareExternalScannerCheckpoint(record, fork) {
		t.Fatal("fork with changed state was shareable")
	}
}

func TestC26lCheckpointRequiresExactMergeIdentity(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	record := captureC26lRecord(t, scanner, scanner.Create())
	if !canShareExternalScannerCheckpoint(record, record.clone()) {
		t.Fatal("equal records were not shareable")
	}

	stateMismatch := record.clone()
	stateMismatch.serialized[0]++
	if canShareExternalScannerCheckpoint(record, stateMismatch) {
		t.Fatal("records with different serialized state were shareable")
	}
	identityMismatch := record.clone()
	identityMismatch.identity.Grammar[0] = 'X'
	if canShareExternalScannerCheckpoint(record, identityMismatch) {
		t.Fatal("records with different grammar identity were shareable")
	}
	if canShareExternalScannerCheckpoint(record, externalScannerCheckpointRecord{}) {
		t.Fatal("complete record shared with an incomplete record")
	}
}

func TestC26lCheckpointTransfersElectedRecoveryState(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	selectedPayload := scanner.Create()
	selected := captureC26lRecord(t, scanner, selectedPayload)
	electedPayload := scanner.Create().(*c26lCheckpointPayload)
	electedPayload.state = []byte{8, 8, 8}

	if !selected.clone().restore(scanner, electedPayload) {
		t.Fatal("elected recovery state did not restore")
	}
	if !bytes.Equal(electedPayload.state, []byte{1, 2, 3}) {
		t.Fatalf("elected recovery state = %v, want [1 2 3]", electedPayload.state)
	}
}

func TestC26lCheckpointRestoresAfterFailedScan(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	payload := scanner.Create().(*c26lCheckpointPayload)
	selected := captureC26lRecord(t, scanner, payload)

	payload.state = []byte{7, 7, 7}
	if scanner.Scan(payload, nil, nil) {
		t.Fatal("synthetic scanner unexpectedly succeeded")
	}
	if !selected.restore(scanner, payload) {
		t.Fatal("failed-scan checkpoint did not restore")
	}
	if !bytes.Equal(payload.state, []byte{1, 2, 3}) {
		t.Fatalf("restored state = %v, want [1 2 3]", payload.state)
	}
}

func TestC26lCheckpointRestoreFailsClosedOnIdentityMismatch(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	selected := captureC26lRecord(t, scanner, scanner.Create())
	other := newC26lCheckpointScanner()
	other.grammarID = []byte("other-grammar")
	if selected.restore(other, other.Create()) {
		t.Fatal("restore accepted a different grammar identity")
	}
}

func TestC26lCheckpointRestoreFailsClosedWhenDeserializeDoesNotReproduce(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	selected := captureC26lRecord(t, scanner, scanner.Create())
	scanner.deserializeMismatch = true
	if selected.restore(scanner, scanner.Create()) {
		t.Fatal("restore accepted a payload that did not reproduce the checkpoint")
	}
}

func TestC26lCheckpointExternalScannerControl(t *testing.T) {
	scanner := newC26lCheckpointScanner()
	if _, ok := captureExternalScannerCheckpointRecord(scanner, scanner.Create(), 4, Point{Row: 1}, 3, 4, 5); !ok {
		t.Fatal("external-scanner control did not capture a checkpoint")
	}
}

func TestC26lCheckpointScannerFreeOffMode(t *testing.T) {
	if _, ok := captureExternalScannerCheckpointRecord(nil, nil, 0, Point{}, 0, 0, 0); ok {
		t.Fatal("scanner-free off mode captured a checkpoint")
	}
	if canShareExternalScannerCheckpoint(externalScannerCheckpointRecord{}, externalScannerCheckpointRecord{}) {
		t.Fatal("scanner-free off mode shared an absent checkpoint")
	}
}
