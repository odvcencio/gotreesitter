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
//   - that a rejected scan leaves the serialized state unchanged, for a scanner
//     that declares it does (see "Rejected scans" below).
//
// It also counts a Serialize that returns different bytes on two calls in a
// row, a length beyond the 4096-byte buffer the parser provides, and a panic
// during replay. Each failed check is counted. The first witnesses of each kind
// keep the origin, mask, state and both outcomes, so a reader can replay them.
//
// # Rejected scans
//
// What a rejected scan may do to the serialized state depends on the failure
// contract the token source applies to the scanner. The report names it in
// declared.failure_contract:
//
//   - restored, the default. The token source restores the state saved before
//     the scan, so the scanner may change its state freely. The replay checks
//     cover that restore, because they scan from a state rebuilt from bytes.
//   - preserved, for a scanner that declares PreservesStateOnScanFailure. The
//     token source restores nothing, so a rejected scan that changes the state
//     is a finding (checks.failed_scan_state_changes).
//   - retained, for a scanner that declares RetainsStateOnScanFailure. The
//     change stays live by design. Retention wins when a scanner declares both,
//     as it does in the token source.
//
// Under the restored and retained contracts, a rejected scan that changes the
// state is counted in informational.failed_scan_state_allowed and kept as a
// witness of kind failed-scan-state-allowed. The verdict ignores it.
//
// # What a report shows
//
// The verdict no-findings means no check failed on the recorded calls. It does
// not prove the property: a call the corpus never makes is not checked, and the
// replay covers only the states the parser produced. Read the counts beside it:
// how many distinct calls, states and masks the corpus reached, and how many
// scans the scanner accepted. A scanner that accepted nothing was tested only
// on its rejections. The verdict no-evidence means the parser never called the
// scanner. The verdict not-transparent means the parse with the recorder
// differed from the parse without it for at least one source. The recorded
// calls then are not the production parser's, so the report says nothing about
// the scanner, and the command exits nonzero.
//
// # How closely the recorded parse follows production
//
// The recorder implements the optional scanner interfaces that a full parse
// reads. Where the scanner lacks one, it answers as the parser does for a
// scanner without an opinion. Certify parses every source twice, once without
// the recorder and once with it, and compares a digest of each tree. A
// difference gives the verdict not-transparent; the counts under parses show
// how many sources differed.
//
// The comparison sees trees only. A change in which Scan calls the parser makes
// can leave the tree the same, so equal digests do not prove that the recorded
// calls are the production calls. probes_test.go narrows the gap. It reads the
// root package's source with go/types and lists every place the root asks a
// scanner about its type. It fails when the root asks a question that the
// recorder neither answers nor is known to be asked only by incremental reuse,
// so a new probe in a full-parse path fails a test before it becomes a silent
// difference in a report. Two differences remain by design:
//
//   - The recorder does not mirror the interfaces that only incremental reuse
//     reads, because Certify parses each source from scratch with no old tree.
//   - The recorder answers false for five capabilities when the scanner has no
//     opinion (incremental reuse, checkpoints, statelessness, preserving or
//     retaining state on failure), where the parser sees no interface. That is
//     the same answer wherever the root treats an absent interface and a false
//     answer alike. One place does not: classifyExternalScannerQuiescence reads
//     a false SupportsIncrementalReuse as a refusal, and only incremental
//     reuse and the compact route reach it.
//
// The package reaches unexported parts of the root package through go:linkname
// and reflection; see lexer_link.go. lexer_link_test.go fails if those change.
package scannercert
