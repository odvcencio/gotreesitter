//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// currentGrammarLockSHA256 hashes the live grammars/languages.lock file and
// returns its SHA-256 as lowercase hex. Dispatcher receipts log or compare
// this value as a lock-drift stamp, not as a per-language precondition:
// each receipt's own digests key to one grammar entry inside the lock, so
// an unrelated grammar bump elsewhere in the same file does not invalidate
// the receipt. Compute the stamp here instead of pinning a literal so the
// receipts stay green across routine grammar bumps that do not touch the
// receipt's own language entry.
func currentGrammarLockSHA256(t *testing.T) string {
	t.Helper()
	got, err := fileSHA256("../grammars/languages.lock")
	if err != nil {
		t.Fatalf("hash grammar lock: %v", err)
	}
	return got
}
