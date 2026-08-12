package gotreesitter

import (
	"encoding/json"
	"os"
	"sync"
)

// --- A7 dispatcher-arm action-effect transcript ---
//
// dispatcherArmCensus and dispatcherArmSubpassCensus (parser_result_compat.go)
// already answer "did this arm change the tree?" under GTS_DISPATCHER_CENSUS=1.
// This file answers the next question the cohort model needs: *what kind* of
// change did the arm make, and *where*? Grouping arms by shared producer
// defect (election picks the wrong shape, recovery retags the root, an extra
// reattaches to the wrong parent, ...) is a hypothesis until a transcript of
// real invocations backs it: two arms belong in the same cohort only if their
// recorded effects actually look alike.
//
// The transcript reuses the same iterative preorder walk and the same
// dispatcherNodeSignature that dispatcherArmCensus's fingerprint already
// uses, and adds one thing: a root-relative child-index path alongside each
// signature, so a changed invocation can name which node it first diverged
// at, not only that something changed.
//
// It is a strictly separate, independently gated instrument.
// GTS_DISPATCHER_TRANSCRIPT=1 turns it on regardless of GTS_DISPATCHER_CENSUS,
// and GTS_DISPATCHER_CENSUS=1 alone never pays any cost from this file: the
// disabled and census-only branches in dispatcherArmCensus,
// dispatcherArmSubpassCensus, and materializationSubpassCensus.run are
// untouched by this instrument and keep calling the original
// captureDispatcherFingerprint.
//
// Output: one JSON object per line (JSON Lines) appended to the file named
// by GTS_DISPATCHER_TRANSCRIPT_OUT, one line per dispatcher-arm or -subpass
// invocation that changed ctx.root. Records are append-only for the life of
// the process: nothing here truncates a pre-existing file, so a caller that
// wants a clean run removes the file first. dispatcherTranscriptSummary
// exposes the same records' arm-ID -> count totals from memory, without
// re-reading the file, for a caller that only needs the aggregate (for
// example, "did dispatch.hyprlang fire at all against this corpus?").

// dispatcherTranscriptEnvFlag and dispatcherTranscriptEnvOut name the two
// environment variables that gate and target this instrument.
const (
	dispatcherTranscriptEnvFlag = "GTS_DISPATCHER_TRANSCRIPT"
	dispatcherTranscriptEnvOut  = "GTS_DISPATCHER_TRANSCRIPT_OUT"
)

// dispatcherTranscriptEnabledOnce and dispatcherTranscriptOutputPathOnce
// cache the two environment reads below (sync.Once), mirroring
// admissionCensusEnabled's pattern (admission_census.go): each read happens
// at most once per process, so every dispatcher-arm call after the first
// pays one atomic-guarded read instead of a fresh os.Getenv.
var (
	dispatcherTranscriptEnabledOnce sync.Once
	dispatcherTranscriptEnabledVal  bool

	dispatcherTranscriptOutputPathOnce sync.Once
	dispatcherTranscriptOutputPathVal  string
)

// dispatcherTranscriptEnabled reports whether GTS_DISPATCHER_TRANSCRIPT=1 is
// set. The read is cached after the first call (sync.Once): the only cost an
// ordinary parse pays when the transcript is off, after process start, is
// one atomic-guarded boolean read, not a fresh os.Getenv.
func dispatcherTranscriptEnabled() bool {
	dispatcherTranscriptEnabledOnce.Do(func() {
		dispatcherTranscriptEnabledVal = os.Getenv(dispatcherTranscriptEnvFlag) == "1"
	})
	return dispatcherTranscriptEnabledVal
}

// dispatcherTranscriptOutputPath returns the configured JSON-lines output
// path, or "" when unset. An enabled transcript with no output path still
// updates the in-memory summary map; it just has nowhere to write records.
// The read is cached after the first call (sync.Once), the same as
// dispatcherTranscriptEnabled above.
func dispatcherTranscriptOutputPath() string {
	dispatcherTranscriptOutputPathOnce.Do(func() {
		dispatcherTranscriptOutputPathVal = os.Getenv(dispatcherTranscriptEnvOut)
	})
	return dispatcherTranscriptOutputPathVal
}

// dispatcherTranscriptSnapshot pairs one captureDispatcherFingerprint-style
// preorder signature per node with the root-relative child-index path that
// reaches it (path[0] selects a child of the root, path[1] a child of that
// child, and so on; the root itself has an empty, non-nil path). Building
// this is strictly more expensive than captureDispatcherFingerprint (one
// extra int slice per node), so it is only ever produced on the
// GTS_DISPATCHER_TRANSCRIPT=1 path.
type dispatcherTranscriptSnapshot struct {
	signatures []dispatcherNodeSignature
	paths      [][]int
}

// captureDispatcherTranscriptSnapshot walks root in the same iterative
// preorder captureDispatcherFingerprint uses (parser_result_compat.go; same
// node signature, same child order) and records each node's path alongside
// its signature. The two functions must keep visiting nodes in the same
// order: firstDivergentTranscriptPath compares this snapshot's signatures
// against a plain captureDispatcherFingerprint-shaped slice position by
// position, exactly as diffDispatcherFingerprint does.
func captureDispatcherTranscriptSnapshot(root *Node) dispatcherTranscriptSnapshot {
	if root == nil {
		return dispatcherTranscriptSnapshot{}
	}
	type pending struct {
		entry   stackEntry
		arena   *nodeArena
		fieldID FieldID
		path    []int
	}
	snap := dispatcherTranscriptSnapshot{
		signatures: make([]dispatcherNodeSignature, 0, 64),
		paths:      make([][]int, 0, 64),
	}
	stack := make([]pending, 0, 64)
	stack = append(stack, pending{
		entry: newStackEntryNode(root.parseState, root),
		arena: root.ownerArena,
		path:  []int{},
	})
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !stackEntryHasNode(top.entry) {
			continue
		}
		childCount := stackEntryNodeChildCount(top.entry)
		snap.signatures = append(snap.signatures, dispatcherNodeSignature{
			symbol:       stackEntryNodeSymbol(top.entry),
			startByte:    stackEntryNodeStartByte(top.entry),
			endByte:      stackEntryNodeEndByte(top.entry),
			startPoint:   stackEntryNodeStartPoint(top.entry),
			endPoint:     stackEntryNodeEndPoint(top.entry),
			parseState:   stackEntryNodeParseState(top.entry),
			preGotoState: stackEntryNodePreGotoState(top.entry),
			childCount:   int32(childCount),
			fieldID:      top.fieldID,
			flags:        dispatcherFingerprintEntryFlags(top.entry),
		})
		snap.paths = append(snap.paths, top.path)
		for i := childCount - 1; i >= 0; i-- {
			child, childArena, fieldID, ok := dispatcherFingerprintChild(top.entry, top.arena, i)
			if !ok {
				continue
			}
			childPath := make([]int, len(top.path)+1)
			copy(childPath, top.path)
			childPath[len(top.path)] = i
			stack = append(stack, pending{entry: child, arena: childArena, fieldID: fieldID, path: childPath})
		}
	}
	return snap
}

// dispatcherTranscriptRecord is one JSON-lines entry: one dispatcher-arm (or
// -subpass) invocation that changed the tree.
type dispatcherTranscriptRecord struct {
	Language string                 `json:"language"`
	Arm      string                 `json:"arm"`
	NodePath []int                  `json:"node_path"`
	Effect   dispatcherEffectRecord `json:"effect"`
}

// dispatcherEffectRecord is the compact before/after description of the
// first node (in preorder) whose signature differs between the snapshot
// taken immediately before a dispatcher arm ran and the one taken
// immediately after. Every change kind is a separate, omitempty field so a
// cohort's shared shape is visible at a glance across many JSON-lines
// records: a cohort whose members only ever set NodeType reads differently
// from one that only ever moves a Field.
//
// Inserted and Removed mark the case where the divergence is a pure length
// change (the tree gained or lost a node at this position and every node
// before it is identical): NodeType and Span then describe the one side
// that exists, and the other change kinds do not apply.
type dispatcherEffectRecord struct {
	Inserted        bool                    `json:"inserted,omitempty"`
	Removed         bool                    `json:"removed,omitempty"`
	NodeType        *dispatcherStringChange `json:"node_type,omitempty"`
	Span            *dispatcherSpanChange   `json:"span,omitempty"`
	Error           *dispatcherBoolChange   `json:"error,omitempty"`
	Missing         *dispatcherBoolChange   `json:"missing,omitempty"`
	ChildCountDelta int32                   `json:"child_count_delta,omitempty"`
	Field           *dispatcherStringChange `json:"field,omitempty"`
}

// dispatcherStringChange records a before/after pair of display strings
// (a node type name or a field name).
type dispatcherStringChange struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// dispatcherBoolChange records a before/after pair for one node flag.
type dispatcherBoolChange struct {
	Before bool `json:"before"`
	After  bool `json:"after"`
}

// dispatcherSpanChange records a before/after byte range [start, end).
type dispatcherSpanChange struct {
	Before [2]uint32 `json:"before"`
	After  [2]uint32 `json:"after"`
}

// firstDivergentTranscriptPath locates the earliest preorder position where
// before and after disagree, using exactly diffDispatcherFingerprint's own
// notion of "changed" (see that function's comment for the same caveat: a
// structural insertion or deletion shifts every later position, so the
// reported node is not always the literal node an arm edited, but its
// position is exact and reproducible from the two snapshots). beforeSig or
// afterSig is nil when the position only exists in the other snapshot (a
// pure insertion/deletion at the tail).
func firstDivergentTranscriptPath(before, after dispatcherTranscriptSnapshot) (path []int, beforeSig, afterSig *dispatcherNodeSignature, found bool) {
	shorter := len(before.signatures)
	if len(after.signatures) < shorter {
		shorter = len(after.signatures)
	}
	for i := 0; i < shorter; i++ {
		if before.signatures[i] != after.signatures[i] {
			b, a := before.signatures[i], after.signatures[i]
			return before.paths[i], &b, &a, true
		}
	}
	if len(before.signatures) > shorter {
		b := before.signatures[shorter]
		return before.paths[shorter], &b, nil, true
	}
	if len(after.signatures) > shorter {
		a := after.signatures[shorter]
		return after.paths[shorter], nil, &a, true
	}
	return nil, nil, nil, false
}

// dispatcherSymbolTypeName resolves a raw Symbol to its display type name,
// matching (*Node).Type's convention (tree.go): the ERROR symbol reports
// "ERROR", and an out-of-range symbol reports "".
func dispatcherSymbolTypeName(lang *Language, sym Symbol) string {
	if lang == nil {
		return ""
	}
	if sym == errorSymbol {
		return "ERROR"
	}
	if int(sym) >= 0 && int(sym) < len(lang.SymbolNames) {
		return unescapePunctuationSymbolName(lang.SymbolNames[sym])
	}
	return ""
}

// dispatcherFieldName resolves a raw FieldID to its display name, matching
// (*Node).FieldNameForChild's convention (tree.go): field 0 ("no field") and
// an out-of-range field both report "".
func dispatcherFieldName(lang *Language, fieldID FieldID) string {
	if lang == nil || fieldID == 0 || int(fieldID) >= len(lang.FieldNames) {
		return ""
	}
	return lang.FieldNames[fieldID]
}

// buildDispatcherEffectRecord reduces a pair of node signatures (or one
// signature and a nil, for a pure insertion/deletion) to the compact change
// description the transcript records.
func buildDispatcherEffectRecord(lang *Language, before, after *dispatcherNodeSignature) dispatcherEffectRecord {
	var eff dispatcherEffectRecord
	switch {
	case before == nil && after != nil:
		eff.Inserted = true
		eff.NodeType = &dispatcherStringChange{After: dispatcherSymbolTypeName(lang, after.symbol)}
		eff.Span = &dispatcherSpanChange{After: [2]uint32{after.startByte, after.endByte}}
		return eff
	case after == nil && before != nil:
		eff.Removed = true
		eff.NodeType = &dispatcherStringChange{Before: dispatcherSymbolTypeName(lang, before.symbol)}
		eff.Span = &dispatcherSpanChange{Before: [2]uint32{before.startByte, before.endByte}}
		return eff
	case before == nil || after == nil:
		// Both absent: firstDivergentTranscriptPath never produces this: it
		// is guarded here only so this function never dereferences a nil.
		return eff
	}
	if before.symbol != after.symbol {
		eff.NodeType = &dispatcherStringChange{
			Before: dispatcherSymbolTypeName(lang, before.symbol),
			After:  dispatcherSymbolTypeName(lang, after.symbol),
		}
	}
	if before.startByte != after.startByte || before.endByte != after.endByte {
		eff.Span = &dispatcherSpanChange{
			Before: [2]uint32{before.startByte, before.endByte},
			After:  [2]uint32{after.startByte, after.endByte},
		}
	}
	if beforeErr, afterErr := before.flags&nodeFlagHasError != 0, after.flags&nodeFlagHasError != 0; beforeErr != afterErr {
		eff.Error = &dispatcherBoolChange{Before: beforeErr, After: afterErr}
	}
	if beforeMissing, afterMissing := before.flags&nodeFlagMissing != 0, after.flags&nodeFlagMissing != 0; beforeMissing != afterMissing {
		eff.Missing = &dispatcherBoolChange{Before: beforeMissing, After: afterMissing}
	}
	if before.childCount != after.childCount {
		eff.ChildCountDelta = after.childCount - before.childCount
	}
	if before.fieldID != after.fieldID {
		eff.Field = &dispatcherStringChange{
			Before: dispatcherFieldName(lang, before.fieldID),
			After:  dispatcherFieldName(lang, after.fieldID),
		}
	}
	return eff
}

// dispatcherTranscriptState is the process-wide, mutex-guarded home for the
// summary count map this instrument keeps. There is deliberately no
// process-wide open *os.File: appendDispatcherTranscriptLine opens, writes,
// and closes the output file per record. That only pays a cost on the
// already-expensive GTS_DISPATCHER_TRANSCRIPT=1 path and needs no shutdown
// hook to flush.
var dispatcherTranscriptState struct {
	mu     sync.Mutex
	counts map[string]uint64
}

// dispatcherTranscriptSummary returns a snapshot of how many transcript
// records this process has recorded so far, keyed by dispatcher arm/subpass
// ID (the same "dispatch.<name>" / "dispatch.<name>.<subpass>" IDs the
// records themselves carry as Arm, matching
// testdata/result_compat_ownership_v1.json's entry IDs). It is safe to call
// whether or not GTS_DISPATCHER_TRANSCRIPT is set: the map is empty when the
// transcript never ran.
func dispatcherTranscriptSummary() map[string]uint64 {
	dispatcherTranscriptState.mu.Lock()
	defer dispatcherTranscriptState.mu.Unlock()
	out := make(map[string]uint64, len(dispatcherTranscriptState.counts))
	for k, v := range dispatcherTranscriptState.counts {
		out[k] = v
	}
	return out
}

// resetDispatcherTranscriptSummary clears the in-memory summary count map.
// It exists for tests: GTS_DISPATCHER_TRANSCRIPT fixtures in this package
// share the map across the whole test binary, so a test that asserts an
// exact count resets first.
func resetDispatcherTranscriptSummary() {
	dispatcherTranscriptState.mu.Lock()
	defer dispatcherTranscriptState.mu.Unlock()
	dispatcherTranscriptState.counts = nil
}

// resetDispatcherTranscriptEnvCacheForTest clears
// dispatcherTranscriptEnabled's and dispatcherTranscriptOutputPath's cached
// reads (their sync.Once guards). It exists for tests: a fixture that sets
// GTS_DISPATCHER_TRANSCRIPT or GTS_DISPATCHER_TRANSCRIPT_OUT with t.Setenv
// needs the next read to see that value immediately, regardless of whether
// an earlier test in the same binary already read and cached one; call it
// again (for example via t.Cleanup) after the environment variable is
// restored, so later tests re-read rather than inherit this test's value.
func resetDispatcherTranscriptEnvCacheForTest() {
	dispatcherTranscriptEnabledOnce = sync.Once{}
	dispatcherTranscriptEnabledVal = false
	dispatcherTranscriptOutputPathOnce = sync.Once{}
	dispatcherTranscriptOutputPathVal = ""
}

// recordDispatcherArmTranscript is the sole entry point dispatcherArmCensus,
// dispatcherArmSubpassCensus, and materializationSubpassCensus.run call once
// they already know (GTS_DISPATCHER_TRANSCRIPT=1, and
// diffDispatcherFingerprint found a rewrite) that a transcript record
// belongs to this invocation. It locates the first divergent node, builds
// the compact effect record, always updates the in-memory summary, and
// appends one JSON line to GTS_DISPATCHER_TRANSCRIPT_OUT when that path is
// set.
func recordDispatcherArmTranscript(lang *Language, armID string, before, after dispatcherTranscriptSnapshot) {
	path, beforeSig, afterSig, found := firstDivergentTranscriptPath(before, after)
	if !found {
		return
	}
	languageName := ""
	if lang != nil {
		languageName = lang.Name
	}
	record := dispatcherTranscriptRecord{
		Language: languageName,
		Arm:      armID,
		NodePath: path,
		Effect:   buildDispatcherEffectRecord(lang, beforeSig, afterSig),
	}

	dispatcherTranscriptState.mu.Lock()
	if dispatcherTranscriptState.counts == nil {
		dispatcherTranscriptState.counts = make(map[string]uint64)
	}
	dispatcherTranscriptState.counts[armID]++
	dispatcherTranscriptState.mu.Unlock()

	outPath := dispatcherTranscriptOutputPath()
	if outPath == "" {
		return
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	line = append(line, '\n')
	appendDispatcherTranscriptLine(outPath, line)
}

// appendDispatcherTranscriptLine opens outPath in append mode, writes line,
// and closes it. Errors are swallowed: this is a diagnostic sink, and it
// must never turn a bad path or a full disk into a parse failure.
func appendDispatcherTranscriptLine(outPath string, line []byte) {
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line)
}
