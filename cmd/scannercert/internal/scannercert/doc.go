// Package scannercert reports whether an external scanner gives the same
// answers when it resumes from its serialized state as when it carries on from
// the live state that produced those bytes.
//
// Incremental reuse depends on that property. C tree-sitter compares serialized
// scanner states to decide whether a subtree is reusable, and the checkpoint
// route here restores a scanner from recorded bytes. Both are sound only if
// Scan(s) equals Scan(Deserialize(Serialize(s))) for every state s the parser
// reaches. This package samples the property. It is the report-only first
// slice of item E-E1 in the v1 design: it grants nothing and changes no
// scanner's declared capabilities.
//
// # What Certify does
//
// Certify parses a grammar's pinned real-corpus sample, then the sources of
// the sample's pinned editing session. A recorder stands in for the grammar's
// scanner and notes every Scan call: the serialized state before the call, the
// byte origin, the valid-symbol mask, and the outcome. Certify then replays
// each distinct call on a fresh scanner payload restored from the recorded
// bytes and compares:
//
//   - the result of Scan;
//   - the result symbol, when the results match;
//   - the token's start, end and end point, when both scans accept;
//   - the serialized state after the scan;
//   - that Deserialize followed by Serialize returns the recorded bytes;
//   - that a rejected scan leaves the serialized state unchanged.
//
// It also counts a Serialize that returns different bytes on two calls in a
// row, a length beyond the 4096-byte buffer the parser provides, and a panic
// during replay. Each failed check is counted. The first witnesses of each kind
// keep the origin, mask, state and both outcomes, so a reader can replay them.
//
// # What a report shows
//
// The verdict no-findings means no check failed on the recorded calls. It does
// not prove the property: a call the corpus never makes is not checked, and the
// replay covers only the states the parser produced. Read the counts beside it:
// how many distinct calls, states and masks the corpus reached, and how many
// scans the scanner accepted. A scanner that accepted nothing was tested only
// on its rejections. The verdict no-evidence means the parser never called the
// scanner.
//
// The recorder implements the optional scanner interfaces that a full parse
// reads, so the parse is the same with it as without it. Certify checks this by
// comparing a digest of every tree and reports the result as parses.transparent.
// The recorder does not mirror the interfaces that only incremental reuse
// reads. The package reaches unexported parts of the root package through
// go:linkname and reflection; see lexer_link.go. lexer_link_test.go fails if
// those change.
package scannercert
