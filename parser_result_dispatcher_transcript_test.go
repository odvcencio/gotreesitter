package gotreesitter

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// wolframTranscriptFixture builds the same known-firing witness as
// TestNormalizeWolframSplitInfixRoot (parser_result_wolfram_test.go): a
// hand-built Wolfram source_file tree with a symbol operand followed by a
// split prefix ("a" then "+ b", from source "   a + b\n").
// dispatch.wolfram (runLanguageResultCompatibility's "wolfram" case) merges
// the two into one infix node, replacing the root's two children with one.
// This exercises a different transcript shape than a leaf retag: the first
// divergence is the root's own child_count, not a descendant's node_type.
func wolframTranscriptFixture() (lang *Language, source []byte, root *Node) {
	lang = wolframCompatLang()
	arena := newNodeArena(arenaClassFull)
	source = []byte("   a + b\n")

	left := newLeafNodeInArena(arena, 2, true, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	op := newLeafNodeInArena(arena, 4, false, 5, 6, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 6})
	right := newLeafNodeInArena(arena, 2, true, 7, 8, Point{Row: 0, Column: 7}, Point{Row: 0, Column: 8})
	prefix := newParentNodeInArena(arena, 3, true, []*Node{op, right}, nil, 0)
	root = newParentNodeInArena(arena, 1, true, []*Node{left, prefix}, nil, 0)
	return lang, source, root
}

// jsonLines splits raw JSON-lines content into its non-empty lines.
func jsonLines(t *testing.T, raw []byte) [][]byte {
	t.Helper()
	var lines [][]byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// TestDispatcherArmTranscriptRecordsKnownFiringWitness runs dispatch.wolfram
// (a known-firing arm: it always merges a split infix operand with its
// operator and right operand) through the real dispatcher switch with
// GTS_DISPATCHER_TRANSCRIPT=1 and asserts a transcript record appears in the
// JSON-lines output with the shape the cohort-proof instrument promises:
// language, arm, a node path into the changed subtree, and a compact
// child-count effect.
func TestDispatcherArmTranscriptRecordsKnownFiringWitness(t *testing.T) {
	resetDispatcherTranscriptSummary()
	t.Setenv("GTS_DISPATCHER_CENSUS", "")
	t.Setenv("GTS_DISPATCHER_TRANSCRIPT", "1")
	outPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	t.Setenv("GTS_DISPATCHER_TRANSCRIPT_OUT", outPath)
	resetDispatcherTranscriptEnvCacheForTest()
	t.Cleanup(resetDispatcherTranscriptEnvCacheForTest)

	lang, source, root := wolframTranscriptFixture()
	ctx := resultCompatibilityContext{root: root, source: source, lang: lang}

	if result := runLanguageResultCompatibility(ctx); result.stopReason != ParseStopNone {
		t.Fatalf("dispatcher returned stop reason %v, want none", result.stopReason)
	}

	// The witness fired exactly as it does with no instrumentation at all
	// (TestNormalizeWolframSplitInfixRoot): the transcript never alters
	// normalization, it only observes it.
	if got, want := resultChildCount(root), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got, want := root.Child(0).Type(lang), "infix"; got != want {
		t.Fatalf("merged node type = %q, want %q", got, want)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read transcript output: %v", err)
	}
	lines := jsonLines(t, raw)
	if len(lines) != 1 {
		t.Fatalf("transcript recorded %d lines for dispatch.wolfram's single case body, want 1: %s", len(lines), raw)
	}

	var rec dispatcherTranscriptRecord
	if err := json.Unmarshal(lines[0], &rec); err != nil {
		t.Fatalf("unmarshal transcript line %s: %v", lines[0], err)
	}

	if rec.Language != "wolfram" {
		t.Errorf("record language = %q, want %q", rec.Language, "wolfram")
	}
	if rec.Arm != "dispatch.wolfram" {
		t.Errorf("record arm = %q, want %q", rec.Arm, "dispatch.wolfram")
	}
	if want := []int{}; !slices.Equal(rec.NodePath, want) {
		t.Errorf("record node_path = %v, want %v (root itself, whose child_count changed)", rec.NodePath, want)
	}
	if rec.Effect.NodeType != nil {
		t.Errorf("record node_type = %+v, want nil: the root's own symbol does not change", rec.Effect.NodeType)
	}
	if rec.Effect.ChildCountDelta != -1 {
		t.Errorf("record child_count_delta = %d, want -1 (two root children merge into one)", rec.Effect.ChildCountDelta)
	}
	if rec.Effect.Span != nil {
		t.Errorf("record span = %+v, want nil: the root's own byte range does not move", rec.Effect.Span)
	}
	if rec.Effect.Error != nil || rec.Effect.Missing != nil {
		t.Errorf("record error=%+v missing=%+v, want both nil: this witness carries no recovery flags", rec.Effect.Error, rec.Effect.Missing)
	}
	if rec.Effect.Inserted || rec.Effect.Removed {
		t.Errorf("record inserted=%v removed=%v, want both false: the changed node still exists on both sides", rec.Effect.Inserted, rec.Effect.Removed)
	}

	summary := dispatcherTranscriptSummary()
	if summary["dispatch.wolfram"] != 1 {
		t.Errorf("summary count map[dispatch.wolfram] = %d, want 1: %v", summary["dispatch.wolfram"], summary)
	}
}

// TestDispatcherArmTranscriptDisabledRecordsNothing runs the identical
// witness with GTS_DISPATCHER_TRANSCRIPT unset and asserts: no output file
// is created, the summary count map stays empty, and the normalizer's
// result is byte-for-byte the same as the enabled run above — the
// instrument is diagnostic-only and never changes behavior.
func TestDispatcherArmTranscriptDisabledRecordsNothing(t *testing.T) {
	resetDispatcherTranscriptSummary()
	t.Setenv("GTS_DISPATCHER_CENSUS", "")
	t.Setenv("GTS_DISPATCHER_TRANSCRIPT", "")
	outPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	t.Setenv("GTS_DISPATCHER_TRANSCRIPT_OUT", outPath)
	resetDispatcherTranscriptEnvCacheForTest()
	t.Cleanup(resetDispatcherTranscriptEnvCacheForTest)

	lang, source, root := wolframTranscriptFixture()
	ctx := resultCompatibilityContext{root: root, source: source, lang: lang}

	if result := runLanguageResultCompatibility(ctx); result.stopReason != ParseStopNone {
		t.Fatalf("dispatcher returned stop reason %v, want none", result.stopReason)
	}

	if got, want := resultChildCount(root), 1; got != want {
		t.Fatalf("root child count = %d, want %d (same result as with the flag on)", got, want)
	}
	infix := root.Child(0)
	if got, want := infix.Type(lang), "infix"; got != want {
		t.Fatalf("merged node type = %q, want %q (same result as with the flag on)", got, want)
	}
	if got, want := resultChildCount(infix), 3; got != want {
		t.Fatalf("infix child count = %d, want %d (same result as with the flag on)", got, want)
	}

	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("transcript output file exists with the flag off (stat err = %v); the instrument must write nothing when disabled", err)
	}
	if summary := dispatcherTranscriptSummary(); len(summary) != 0 {
		t.Fatalf("summary count map is non-empty with the flag off: %v", summary)
	}
}

// TestDispatcherTranscriptRequiresExactOne locks in GTS_DISPATCHER_TRANSCRIPT's
// gating contract: only the exact string "1" turns the instrument on,
// matching GTS_DISPATCHER_CENSUS's existing contract
// (TestDispatcherCensusRequiresExactOne).
func TestDispatcherTranscriptRequiresExactOne(t *testing.T) {
	t.Cleanup(resetDispatcherTranscriptEnvCacheForTest)
	for _, value := range []string{"", "0", "false", "2"} {
		t.Setenv("GTS_DISPATCHER_TRANSCRIPT", value)
		// dispatcherTranscriptEnabled caches its read (sync.Once); reset
		// before each value so this loop still exercises a fresh read every
		// iteration instead of pinning the first iteration's result.
		resetDispatcherTranscriptEnvCacheForTest()
		if dispatcherTranscriptEnabled() {
			t.Fatalf("GTS_DISPATCHER_TRANSCRIPT=%q enabled the transcript", value)
		}
	}
	t.Setenv("GTS_DISPATCHER_TRANSCRIPT", "1")
	resetDispatcherTranscriptEnvCacheForTest()
	if !dispatcherTranscriptEnabled() {
		t.Fatal("GTS_DISPATCHER_TRANSCRIPT=1 did not enable the transcript")
	}
}

// TestCaptureDispatcherTranscriptSnapshotPaths pins the path convention
// (root-relative child-index chain, root itself is []) against a small hand
// built tree, independent of any real dispatcher arm.
func TestCaptureDispatcherTranscriptSnapshotPaths(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf1 := newLeafNodeInArena(arena, 10, true, 0, 3, Point{Column: 0}, Point{Column: 3})
	leaf2 := newLeafNodeInArena(arena, 11, true, 3, 6, Point{Column: 3}, Point{Column: 6})
	root := newParentNodeInArena(arena, 1, true, []*Node{leaf1, leaf2}, nil, 0)

	snap := captureDispatcherTranscriptSnapshot(root)
	if len(snap.signatures) != 3 || len(snap.paths) != 3 {
		t.Fatalf("snapshot visited %d signatures / %d paths, want 3/3 (root + 2 leaves)", len(snap.signatures), len(snap.paths))
	}
	want := [][]int{{}, {0}, {1}}
	for i, wantPath := range want {
		if !slices.Equal(snap.paths[i], wantPath) {
			t.Errorf("paths[%d] = %v, want %v", i, snap.paths[i], wantPath)
		}
	}
}

// TestFirstDivergentTranscriptPath exercises the shapes a divergence can
// take: a same-length interior mismatch, a real child insertion (which, for
// this signature shape, always surfaces as an interior mismatch on the
// inserted node's parent), the trailing-only-length fallback branch in
// isolation, and no divergence at all.
func TestFirstDivergentTranscriptPath(t *testing.T) {
	build := func() (root, leaf1, leaf2 *Node) {
		arena := newNodeArena(arenaClassFull)
		leaf1 = newLeafNodeInArena(arena, 10, true, 0, 3, Point{Column: 0}, Point{Column: 3})
		leaf2 = newLeafNodeInArena(arena, 11, true, 3, 6, Point{Column: 3}, Point{Column: 6})
		root = newParentNodeInArena(arena, 1, true, []*Node{leaf1, leaf2}, nil, 0)
		return root, leaf1, leaf2
	}

	t.Run("interior mismatch", func(t *testing.T) {
		root, _, leaf2 := build()
		before := captureDispatcherTranscriptSnapshot(root)
		leaf2.symbol = 99
		after := captureDispatcherTranscriptSnapshot(root)

		path, b, a, found := firstDivergentTranscriptPath(before, after)
		if !found {
			t.Fatal("expected a divergence")
		}
		if !slices.Equal(path, []int{1}) {
			t.Errorf("path = %v, want [1]", path)
		}
		if b == nil || a == nil {
			t.Fatalf("expected both sides present, got before=%v after=%v", b, a)
		}
		if b.symbol != 11 || a.symbol != 99 {
			t.Errorf("signatures = before %d after %d, want 11 -> 99", b.symbol, a.symbol)
		}
	})

	t.Run("child inserted under root", func(t *testing.T) {
		// Appending a child always changes its parent's own childCount, and
		// the parent (here, root) appears earlier in preorder than the new
		// child, so the interior-mismatch scan (above) finds root itself,
		// not a trailing-only divergence. This is the shape every real
		// dispatcher-arm insertion takes: an ancestor's signature always
		// changes at or before the inserted node's position.
		root, _, _ := build()
		before := captureDispatcherTranscriptSnapshot(root)
		extra := newLeafNodeInArena(root.ownerArena, 12, true, 6, 9, Point{Column: 6}, Point{Column: 9})
		root.children = append(root.children, extra)
		after := captureDispatcherTranscriptSnapshot(root)

		path, b, a, found := firstDivergentTranscriptPath(before, after)
		if !found {
			t.Fatal("expected a divergence")
		}
		if !slices.Equal(path, []int{}) {
			t.Errorf("path = %v, want [] (root itself, whose child_count changed)", path)
		}
		if b == nil || a == nil {
			t.Fatalf("expected both sides present, got before=%v after=%v", b, a)
		}
		if b.childCount != 2 || a.childCount != 3 {
			t.Errorf("root child counts = before %d after %d, want 2 -> 3", b.childCount, a.childCount)
		}
	})

	t.Run("trailing-only length divergence", func(t *testing.T) {
		// firstDivergentTranscriptPath's trailing-only branch: every
		// position in the common prefix compares equal, and only the
		// longer snapshot's own length exposes the change. Built directly
		// (rather than from a tree mutation) because, for this signature
		// shape, an ordinary insertion is always caught earlier as an
		// interior mismatch on the inserted node's parent (see the
		// preceding subtest) — this subtest instead pins the fallback
		// branch's own contract.
		before := dispatcherTranscriptSnapshot{
			signatures: []dispatcherNodeSignature{{symbol: 1, childCount: 1}, {symbol: 10}},
			paths:      [][]int{{}, {0}},
		}
		after := dispatcherTranscriptSnapshot{
			signatures: []dispatcherNodeSignature{{symbol: 1, childCount: 1}, {symbol: 10}, {symbol: 12}},
			paths:      [][]int{{}, {0}, {0, 0}},
		}

		path, b, a, found := firstDivergentTranscriptPath(before, after)
		if !found {
			t.Fatal("expected a divergence")
		}
		if !slices.Equal(path, []int{0, 0}) {
			t.Errorf("path = %v, want [0 0]", path)
		}
		if b != nil {
			t.Errorf("before signature = %+v, want nil (pure insertion)", b)
		}
		if a == nil || a.symbol != 12 {
			t.Errorf("after signature = %v, want symbol 12", a)
		}

		// Symmetric removal: after is the shorter snapshot this time.
		path, b, a, found = firstDivergentTranscriptPath(after, before)
		if !found {
			t.Fatal("expected a divergence")
		}
		if !slices.Equal(path, []int{0, 0}) {
			t.Errorf("path = %v, want [0 0]", path)
		}
		if a != nil {
			t.Errorf("after signature = %+v, want nil (pure removal)", a)
		}
		if b == nil || b.symbol != 12 {
			t.Errorf("before signature = %v, want symbol 12", b)
		}
	})

	t.Run("no divergence", func(t *testing.T) {
		root, _, _ := build()
		before := captureDispatcherTranscriptSnapshot(root)
		after := captureDispatcherTranscriptSnapshot(root)
		if _, _, _, found := firstDivergentTranscriptPath(before, after); found {
			t.Fatal("expected no divergence for two snapshots of the same unmodified tree")
		}
	})
}
