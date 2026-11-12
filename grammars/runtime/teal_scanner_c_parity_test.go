//go:build !grammar_subset || grammar_subset_teal

package grammarruntime

import (
	"bytes"
	"testing"
)

func tealCFaithful(scanner TealExternalScanner) *tealState {
	st := scanner.Create().(*tealState)
	st.SetCFaithful(true)
	return st
}

// TestTealStateBytesMatchPinnedC pins the state bytes of the pinned C scanner
// (tree-sitter-teal src/scanner.c:6-10, :93-102): serialize copies the whole
// 8-byte struct (32-bit count, in_str, opening_quote, two padding bytes) and
// deserialize returns without touching the payload when fewer than 8 bytes are
// given. This is the C-faithful mode; the legacy mode keeps its 6-byte
// checkpoint (see TestTealLegacyEmptySnapshotResets).
func TestTealStateBytesMatchPinnedC(t *testing.T) {
	scanner := TealExternalScanner{}
	state := tealCFaithful(scanner)
	state.openingEqs, state.inStr, state.openingQuote = 0x01020304, true, '"'
	buf := make([]byte, 1024)
	n := scanner.Serialize(state, buf)
	want := []byte{4, 3, 2, 1, 1, '"', 0, 0}
	if !bytes.Equal(buf[:n], want) {
		t.Fatalf("bytes = %v, want %v", buf[:n], want)
	}

	back := tealCFaithful(scanner)
	scanner.Deserialize(back, buf[:n])
	if back.openingEqs != state.openingEqs || back.inStr != state.inStr || back.openingQuote != state.openingQuote {
		t.Fatalf("round trip = %+v, want %+v", back, state)
	}

	// A short saved state leaves the payload as it was (scanner.c:99-100).
	kept := tealCFaithful(scanner)
	kept.openingEqs, kept.inStr, kept.openingQuote = 2, true, '\''
	scanner.Deserialize(kept, nil)
	scanner.Deserialize(kept, buf[:7])
	if kept.openingEqs != 2 || !kept.inStr || kept.openingQuote != '\'' {
		t.Fatalf("short state changed the payload: %+v", kept)
	}
}

// TestTealCopiesPaddingAndSignedQuoteLikeC: C copies
// the whole struct, so the two padding bytes survive a restore and a save, and
// opening_quote is a char (signed on x86-64, unsigned on arm64: cCharIsSigned), so
// 0xff reads back as -1 or 255.
func TestTealCopiesPaddingAndSignedQuoteLikeC(t *testing.T) {
	scanner := TealExternalScanner{}
	st := tealCFaithful(scanner)
	scanner.Deserialize(st, []byte{0, 0, 0, 0, 0, 0xff, 7, 9})
	out := make([]byte, 16)
	n := scanner.Serialize(st, out)
	if !bytes.Equal(out[:n], []byte{0, 0, 0, 0, 0, 0xff, 7, 9}) {
		t.Fatalf("saved %v, want the padding 7 9 preserved", out[:n])
	}
	want := rune(-1)
	if !cCharIsSigned {
		want = 255
	}
	if st.openingQuote != want {
		t.Fatalf("opening quote = %d, want %d (cCharIsSigned = %v)", st.openingQuote, want, cCharIsSigned)
	}
}

// TestTealLegacyEmptySnapshotResets: docs/external-scanners.md
// requires an empty snapshot to reset the payload, so the legacy mode (the
// default engine) clears the payload where the C-faithful mode keeps it.
func TestTealLegacyEmptySnapshotResets(t *testing.T) {
	scanner := TealExternalScanner{}
	valid := make([]bool, tealTokenCount)
	valid[tealTokShortStrStart] = true
	legacy := scanner.Create()
	if got := portScan(scanner, legacy, "\"x", valid); !got.ok {
		t.Fatal("setup: short string start refused")
	}
	scanner.Deserialize(legacy, nil)
	valid = make([]bool, tealTokenCount)
	valid[tealTokShortStrChar] = true
	if got := portScan(scanner, legacy, "x", valid); got.ok {
		t.Fatalf("legacy: payload still inside the string after an empty snapshot, got %+v", got)
	}
	cf := tealCFaithful(scanner)
	vs := make([]bool, tealTokenCount)
	vs[tealTokShortStrStart] = true
	portScan(scanner, cf, "\"x", vs)
	scanner.Deserialize(cf, nil)
	if got := portScan(scanner, cf, "x", valid); !got.ok {
		t.Fatalf("C-faithful: payload should stay inside the string (C returns without touching it), got %+v", got)
	}
}
