//go:build gts_parsercorephase0

package gotreesitter

import (
	"reflect"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCorePerVersionStateKeepsHeaderSize(t *testing.T) {
	if got := unsafe.Sizeof(diagnosticParserCoreHeader{}); got != 152 {
		t.Fatalf("diagnostic parser core header is %d bytes, want 152", got)
	}
}

func TestDiagnosticParserCorePerVersionStatePublishesImmutableCopies(t *testing.T) {
	first := &diagnosticParserCoreS3Region{state: 7, startByte: 2, endByte: 4}
	second := &diagnosticParserCoreS3Region{state: 9, startByte: 4, endByte: 6}
	var header diagnosticParserCoreHeader
	header.openRecoveryRegion(first)
	copyOfHeader := header
	initialState := header.versionState

	header.setRecoveryRegion(second)

	if header.versionState == initialState {
		t.Fatal("setRecoveryRegion reused the previous immutable wrapper")
	}
	if got := header.recoveryRegion(); got == second || !reflect.DeepEqual(got, second) {
		t.Fatalf("updated region=%+v, want an independent copy of %+v", got, second)
	}
	if got := copyOfHeader.recoveryRegion(); got == first || !reflect.DeepEqual(got, first) {
		t.Fatalf("copied header region=%+v, want an independent copy of %+v", got, first)
	}
	if copyOfHeader.versionState != initialState {
		t.Fatal("updating the live header changed the copied header state")
	}
}

func TestDiagnosticParserCorePerVersionStateCloseDoesNotMutateSnapshot(t *testing.T) {
	region := &diagnosticParserCoreS3Region{state: 3, children: []core.SubtreeID{11}}
	var header diagnosticParserCoreHeader
	header.openRecoveryRegion(region)
	snapshot := header

	header.closeRecoveryRegion()

	if header.versionState != nil || header.recoveryRegion() != nil {
		t.Fatal("closeRecoveryRegion left recovery state live")
	}
	if snapshot.versionState == nil || snapshot.recoveryRegion() == region || !reflect.DeepEqual(snapshot.recoveryRegion(), region) {
		t.Fatal("closing the live header changed its copied recovery state")
	}
	if snapshot.versionState.s3Region != snapshot.recoveryRegion() {
		t.Fatal("closing the live header mutated the immutable wrapper")
	}
}

func TestDiagnosticParserCorePerVersionStateRollbackRestoresCopy(t *testing.T) {
	first := &diagnosticParserCoreS3Region{state: 1, startByte: 5, endByte: 7}
	second := &diagnosticParserCoreS3Region{state: 2, startByte: 7, endByte: 9}
	headers := []diagnosticParserCoreHeader{{}}
	headers[0].openRecoveryRegion(first)
	originalState := headers[0].versionState
	var scratch diagnosticParserCoreHeaderRollbackScratch
	if err := scratch.begin(headers); err != nil {
		t.Fatalf("begin rollback snapshot: %v", err)
	}
	headers[0].setRecoveryRegion(second)
	scratch.finish(&headers, true)

	if got := headers[0].recoveryRegion(); got == first || !reflect.DeepEqual(got, first) {
		t.Fatalf("rolled-back region=%+v, want an independent copy of %+v", got, first)
	}
	if headers[0].versionState != originalState {
		t.Fatal("rollback did not restore the original immutable wrapper")
	}
	if scratch.busy || len(scratch.headers) != 0 {
		t.Fatal("rollback scratch remained active after finish")
	}
}

func TestDiagnosticParserCorePerVersionStateSetNilCloses(t *testing.T) {
	var header diagnosticParserCoreHeader
	header.openRecoveryRegion(&diagnosticParserCoreS3Region{state: 4})
	header.setRecoveryRegion(nil)
	if header.versionState != nil || header.recoveryRegion() != nil {
		t.Fatal("setRecoveryRegion(nil) did not close recovery state")
	}
}
