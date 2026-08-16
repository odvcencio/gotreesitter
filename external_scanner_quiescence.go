package gotreesitter

// External-scanner quiescence proof (campaign O(edit) workstream W4,
// spec.campaign.oedit; shared artifact with campaign v6.7 Lane 4).
//
// Incremental subtree reuse skips the lexer forward to a reused node's end
// byte and resumes parsing there. For a language with an external scanner,
// this is sound only when the scanner state a fresh parse would hold at the
// reuse boundary matches the state the reuse path resumes with. We call a
// boundary "quiescent" when that match is provable. This file holds the
// per-boundary quiescence predicate. It fails CLOSED: a boundary it cannot
// prove quiescent is refuted, and the reuse admission counts it as
// ReuseRejectScannerUnquiescent and skips reuse.
//
// The predicate replaces a name-allowlist with a proof (invariant 2 of the
// campaign): reuse eligibility is decided per boundary from a scanner-state
// argument, not from a curated language list.
//
// -----------------------------------------------------------------------
// Stateless-scanner proof obligations.
//
// Go is the reference proof below (backed by grammars/go_scanner.go). Other
// scanners may implement StatelessExternalScanner only when the same core
// obligations hold: no persisted payload, Scan depends only on local
// lookahead plus the parser-provided valid-symbol set, and a failed Scan
// preserves the empty state. The production admission matrix and fresh-tree
// differential tests live in docs/external-scanners.md and
// parser_stateless_scanner_incremental_certification_test.go.
//
// Go's only external token is `_automatic_semicolon` (ASI). The scanner that
// resolves it is GoExternalScanner. The proof that every Go reuse boundary is
// quiescent rests on five obligations, each verified against the scanner
// source:
//
//  1. No persisted state. Create returns nil, Serialize writes nothing and
//     returns 0, and Deserialize is a no-op (go_scanner.go). The scanner
//     therefore holds no payload and no cross-token state. There is nothing
//     for a reused subtree to invalidate.
//
//  2. Scan is a pure function of the local byte stream and the valid-symbol
//     set. Scan reads lexer.Lookahead() from the current position, advances
//     over horizontal whitespace only, then branches on '\n', on 0 (end of
//     file), or on any other byte (go_scanner.go). The valid-symbol set is
//     itself a pure function of the LR parser state. Scan reads no other
//     mutable state.
//
//  3. Terminator legality is grammar-gated, not history-gated. Scan runs only
//     in LR states where `_automatic_semicolon` is a valid external symbol.
//     grammargen places the terminator directly after a completed statement,
//     declaration, or spec, so the scanner never needs to remember whether the
//     previous token was an identifier, a literal, or a keyword (go_scanner.go
//     doc, plus the terminator placement in grammargen's Go grammar).
//
//  4. Raw strings never induce scanner state. A Go raw string literal is
//     delimited by back quotes and may span newlines. The DFA lexes it as one
//     raw_string_literal token; the external terminator symbol is never valid
//     between the opening and closing back quote, so Scan never runs inside a
//     raw string. A raw string that spans a newline therefore creates no ASI
//     state and no cross-boundary dependency. A boundary inside a raw string
//     is never a top-level declaration boundary, so reuse never offers it as a
//     candidate. When an edit changes back-quote balance, the byte-equality,
//     stale-boundary, and fragility gates in tryReuseSubtree reject the
//     affected subtree; that hazard is tokenization, not scanner state.
//
//  5. Comments before a declaration never induce scanner state. When Scan
//     meets a comment start, it declines through the default branch; the token
//     source resets the lexer to the start byte, the DFA matches the comment
//     as an extra, and the parser retries the scanner past it. Extras do not
//     change the LR state, so the valid-symbol set at the following boundary
//     is unchanged (go_scanner.go).
//
// From obligations 1 and 2, the Go scanner state at any position is the empty
// state, which equals the state a fresh parse holds at that position.
// Obligations 3 to 5 confirm that the boundaries reuse actually considers --
// top-level declaration boundaries, possibly preceded by comments or by raw
// strings that span newlines -- do not smuggle in hidden state. Therefore
// every Go reuse boundary is quiescent. QED.
//
// GoExternalScanner encodes this proof by implementing StatelessExternalScanner
// and returning true; the classifier below reads that marker and returns
// scannerQuiescenceProven.
//
// -----------------------------------------------------------------------
// Note: ExternalLexer.Previous() and obligation 2.
//
// ExternalLexer.Previous() (external_lexer.go) reads the raw byte before the
// current scan position. That byte can be the tail of a comment or other
// extra the core lexer matched between two scanner invocations, not the
// token that logically precedes this boundary -- Scan never sees extras
// directly. Any scanner that calls Previous() therefore depends on more than
// "local lookahead plus the valid-symbol set" and fails obligation 2 above.
// SwiftExternalScanner (grammars/swift_scanner.go) calls Previous() and
// compensates by tracking the last resolved token-boundary rune across calls
// in its own scanner state, but it does not implement
// StatelessExternalScanner or IncrementalReuseExternalScanner, so it is
// classified scannerQuiescenceUnknown, not scannerQuiescenceProven. Do not
// mark a scanner that calls Previous() as stateless.
// -----------------------------------------------------------------------

// externalScannerQuiescence is the per-boundary classification result.
type externalScannerQuiescence uint8

const (
	// scannerQuiescenceProven: the scanner state a fresh parse holds at the
	// boundary is provably identical to the state the reuse path resumes with.
	// Reuse across the scanner is sound.
	scannerQuiescenceProven externalScannerQuiescence = iota

	// scannerQuiescenceUnknown: the W4 classifier neither proves nor refutes
	// quiescence. The legacy per-language admission
	// (languageSupportsIncrementalReuse, checked before tryReuseSubtree runs)
	// stays the authority. This keeps the predicate neutral for every
	// production external-scanner language that has not yet migrated to a
	// per-boundary proof.
	scannerQuiescenceUnknown

	// scannerQuiescenceRefuted: the scanner carries cross-token state and this
	// boundary has no matching checkpoint, so reuse is unsound. The caller
	// fails closed: it rejects reuse and counts ReuseRejectScannerUnquiescent.
	scannerQuiescenceRefuted
)

// classifyExternalScannerQuiescence decides, from the language's scanner alone,
// whether reuse across the scanner is provably sound, provably unsound, or
// undecided. Checkpoint languages are reported as undecided here; the
// checkpoint match in canReuseNodeWithExternalScannerCheckpoint decides them
// per boundary from recorded state.
func classifyExternalScannerQuiescence(lang *Language) externalScannerQuiescence {
	if lang == nil || lang.ExternalScanner == nil {
		// No external scanner means no scanner state to reconstruct.
		return scannerQuiescenceProven
	}
	if stateless, ok := lang.ExternalScanner.(StatelessExternalScanner); ok && stateless.ExternalScannerIsStateless() {
		// A scanner satisfying the stateless proof is quiescent at every
		// boundary. See the proof obligations at the top of this file and the
		// per-scanner certification matrix.
		return scannerQuiescenceProven
	}
	if reusable, ok := lang.ExternalScanner.(IncrementalReuseExternalScanner); ok && !reusable.SupportsIncrementalReuse() {
		// The scanner opts out of incremental reuse because it holds serialized
		// mutable state, such as Python's indentation stack. Refute per boundary
		// as a fail-closed backstop behind the language-level gate.
		return scannerQuiescenceRefuted
	}
	if languageUsesExternalScannerCheckpoints(lang) {
		// A checkpoint match decides these boundaries, not this classifier.
		return scannerQuiescenceUnknown
	}
	// A scanner the classifier cannot decide keeps the legacy blanket
	// admission. This branch is neutral: the language-level gate already
	// vetted the scanner before reuse could reach this point.
	return scannerQuiescenceUnknown
}

// externalScannerBoundaryQuiescentWithoutCheckpoint reports whether a
// non-checkpoint external-scanner language may reuse a subtree at a candidate
// boundary. It returns false only when the classifier refutes quiescence, so a
// caller that rejects on false is fail-closed while staying neutral for every
// language the classifier reports as proven or undecided.
func externalScannerBoundaryQuiescentWithoutCheckpoint(lang *Language) bool {
	return classifyExternalScannerQuiescence(lang) != scannerQuiescenceRefuted
}

// compactIncrementalReuseProvenForLanguage keeps the classifier's Unknown
// result neutral only when the legacy language admission permits reuse.
// Refuted scanners remain fail-closed.
func compactIncrementalReuseProvenForLanguage(lang *Language) bool {
	return externalScannerBoundaryQuiescentWithoutCheckpoint(lang) &&
		languageSupportsIncrementalReuse(lang)
}
