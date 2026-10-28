package gotreesitter_test

// Non-env-gated regression coverage for campaign O(edit) workstream W2: the
// cNodeMemoCache adaptive growth trigger (parser_recover_c.go's
// cNodeMemoSlot / cNodeMemoThrashGrowThreshold).
//
// Unlike the retired issue380 wall-clock repro files (env-gated
// demonstrations of the ORIGINAL bug; they remain in git history), these
// tests always run as part of `go test .` and assert only on the deterministic,
// collision-count-driven growth DECISION exposed via
// (*Parser).DebugCNodeMemoCacheStats -- never on wall-clock timing. They are
// cheap (well under a second) and are the permanent guard against the W2
// regression: growth becoming, again, a function of a Parser instance's
// unrelated prior history instead of purely of the (source, edit) being
// parsed right now.
//
// See also TestCNodeMemoSlotAdaptiveGrowFiresExactlyAtThrashThreshold /
// TestCNodeMemoSlotAdaptiveGrowIsNoopOnceAtFullSize (parser_recover_c_test.go,
// internal whitebox package) for a mechanism-level proof of the exact
// threshold-crossing behavior with synthetic colliding nodes -- no real parse
// required. This file instead exercises the real, end-to-end #388 repro
// scenario (this repo's own tree.go, single-char insert near the top) to
// prove the growth DECISION is stable across independent fresh Parser
// instances on identical input, which is the actual property the campaign
// spec asks for ("same input always takes the same growth path").

import (
	"bytes"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// issue380CNodeMemoGrowthScenario runs the exact #388 repro shape (this
// repo's own tree.go, single-char insert at issue380 offset 200, a fresh
// Parser + fresh Tree) and returns the resulting cNodeMemoCache stats. The
// insert forces near-total GLR re-derivation for Go (ReuseRejectRootNonLeaf
// Changed stays high even after campaign O(edit) W1's fragility-gated
// top-level sibling admission, since this repro's blocker is the deeper
// reduce-chain-settling gap W1 documented, not the language allowlist W1
// removed), which is what drives real memo-set contention -- see the
// retired issue380 repro files in git history for the full mechanism.
func issue380CNodeMemoGrowthScenario(t *testing.T) (cacheLen int, thrash uint32) {
	t.Helper()
	src, err := os.ReadFile("tree.go")
	if err != nil {
		t.Fatalf("read tree.go: %v", err)
	}
	lang := grammars.GoLanguage()
	const insertOffset = 200

	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	defer oldTree.Release()

	newSrc := make([]byte, 0, len(src)+1)
	newSrc = append(newSrc, src[:insertOffset]...)
	newSrc = append(newSrc, 'x')
	newSrc = append(newSrc, src[insertOffset:]...)
	startPoint := issue380PointForOffset(src, insertOffset)
	newEndPoint := gotreesitter.Point{Row: startPoint.Row, Column: startPoint.Column + 1}
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(insertOffset),
		OldEndByte:  uint32(insertOffset),
		NewEndByte:  uint32(insertOffset + 1),
		StartPoint:  startPoint,
		OldEndPoint: startPoint,
		NewEndPoint: newEndPoint,
	})

	newTree, err := parser.ParseIncremental(newSrc, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()

	cacheLen, thrash = parser.DebugCNodeMemoCacheStats()
	return cacheLen, thrash
}

// TestCNodeMemoCacheAdaptiveGrowthDeterministicAcrossFreshParsers is the
// primary W2 regression: independent fresh Parser instances (never sharing
// any prior parse) parsing the IDENTICAL (source, edit) must reach the
// IDENTICAL growth decision every time. Before W2, this property held only
// for the sticky cHandleError trigger (which this scenario never reaches --
// AcceptedErrorRetryAttempts is 0 and the input is syntactically valid, see
// the retired repro files), so the cache stayed stuck at
// cNodeMemoCacheInitialSize (128) for every one of these fresh, "cold"
// instances -- correct, but pathologically slow (see the retired env-gated
// repros in git history for the wall-clock evidence). After W2, the adaptive
// trigger fires from THIS parse's own memo-set contention alone, so the
// decision is reached the same way regardless of the instance's (nonexistent,
// here) prior history.
func TestCNodeMemoCacheAdaptiveGrowthDeterministicAcrossFreshParsers(t *testing.T) {
	const iterations = 5
	var firstCacheLen int
	for i := 0; i < iterations; i++ {
		cacheLen, thrash := issue380CNodeMemoGrowthScenario(t)
		if i == 0 {
			firstCacheLen = cacheLen
		} else if cacheLen != firstCacheLen {
			t.Fatalf("iter=%d cacheLen=%d, want %d (same as iter=0) -- growth decision is not input-deterministic", i, cacheLen, firstCacheLen)
		}
		// The adaptive trigger must actually have fired for this scenario:
		// a repro known to force heavy GLR re-derivation that never grows
		// the cache would silently regress back to the original bug.
		if cacheLen != 16384 {
			t.Fatalf("iter=%d cacheLen=%d, want 16384 (cNodeMemoCacheSize) -- adaptive growth did not fire on the #388 repro scenario", i, cacheLen)
		}
		if thrash == 0 {
			t.Fatalf("iter=%d thrash=0, want nonzero -- growth fired without any observed collision, which should be impossible", i)
		}
	}
}

// TestCNodeMemoCacheStaysSmallForCleanFullParse guards the companion
// requirement: a plain, non-pathological full parse -- no incremental edit,
// no forced re-derivation -- must NOT grow the cache. This is the
// "small-footprint-for-typical-files" property the campaign spec calls out
// explicitly: the adaptive trigger must not turn every parse into a
// permanent 16384-entry allocation.
func TestCNodeMemoCacheStaysSmallForCleanFullParse(t *testing.T) {
	files := []string{"tree.go", "parser.go", "lexer.go"}
	lang := grammars.GoLanguage()
	for _, f := range files {
		f := f
		t.Run(f, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Skipf("read %s: %v", f, err)
			}
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}
			defer tree.Release()
			cacheLen, thrash := parser.DebugCNodeMemoCacheStats()
			// The robust gate is cacheLen, not thrash: cacheLen<=128
			// (cNodeMemoCacheInitialSize) is exactly the "did adaptive growth
			// fire" decision this test exists to guard, and it is what
			// growCNodeMemoCache/cNodeMemoSlot actually promise to keep
			// input-stable. The raw thrash count is heap-address-dependent
			// (cNodeMemoCacheIndex hashes node pointers, which move under
			// ASLR/allocator layout across hosts/processes), so this is only
			// a sanity bound: real clean parses measure exactly 0 on this box
			// (see cNodeMemoThrashGrowThreshold's provenance comment,
			// parser_recover_c.go), but a different layout could in
			// principle incur a benign second-way fill without changing the
			// decision -- so assert well under the 512 growth threshold
			// (cNodeMemoThrashGrowThreshold), not exact equality to 0.
			if cacheLen > 128 {
				t.Fatalf("%s: cacheLen=%d after a plain clean full parse, want <=128 (cNodeMemoCacheInitialSize) -- adaptive growth fired on a typical file", f, cacheLen)
			}
			if thrash >= 512 {
				t.Fatalf("%s: thrash=%d after a plain clean full parse, want <512 (cNodeMemoThrashGrowThreshold)", f, thrash)
			}
		})
	}
}

func issue380PointForOffset(src []byte, off int) gotreesitter.Point {
	prefix := src[:off]
	row := bytes.Count(prefix, []byte{'\n'})
	col := off
	if idx := bytes.LastIndexByte(prefix, '\n'); idx >= 0 {
		col = off - idx - 1
	}
	return gotreesitter.Point{Row: uint32(row), Column: uint32(col)}
}
