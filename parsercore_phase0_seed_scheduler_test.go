//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDiagnosticParserCoreSeedSchedulerShadowsFrozenPrefixAtEditsElection(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	legacy, legacyErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source, gotreesitter.DiagnosticParserCorePrefixOptions{},
	)
	if legacyErr == nil || legacy.CachedDotClosure == nil || len(legacy.Elections) != 103 {
		t.Fatalf("frozen prefix did not reach its edits-election barrier: result=%+v err=%v", legacy, legacyErr)
	}

	shadow, err := gotreesitter.RunDiagnosticParserCoreSeedShadowForTest(grammars.GoExternalScanner{}, source)
	if err != nil {
		t.Fatalf("seed-owned scheduler diverged before edits election: %v", err)
	}
	if shadow.Receipt == nil {
		t.Fatal("seed-owned scheduler returned no receipt")
	}
	if shadow.Receipt.StartElectionIndex != -1 || shadow.Receipt.StartToken != (gotreesitter.Token{}) ||
		len(shadow.Receipt.StartHeaders) != 1 || shadow.Receipt.StartHeaders[0].Header.State != 1 ||
		shadow.Receipt.StartHeaders[0].Header.ByteOffset != 0 || shadow.Receipt.StartHeaders[0].Header.Shifted ||
		shadow.Receipt.StartHeaders[0].Header.Accepted || shadow.Receipt.StartHeaders[0].Header.CreationSeq != 0 ||
		shadow.Receipt.StartHeaders[0].Header.ExactPaths != 1 {
		t.Fatalf("seed lifecycle identity drifted: %+v", shadow.Receipt.StartHeaders)
	}
	if !reflect.DeepEqual(shadow.Receipt.Elections, legacy.Elections) {
		limit := min(len(shadow.Receipt.Elections), len(legacy.Elections))
		for index := 0; index < limit; index++ {
			if !reflect.DeepEqual(shadow.Receipt.Elections[index], legacy.Elections[index]) {
				t.Fatalf("first election divergence at %d\nseed=%+v\nfrozen=%+v", index, shadow.Receipt.Elections[index], legacy.Elections[index])
			}
		}
		t.Fatalf("election count diverged: seed=%d frozen=%d", len(shadow.Receipt.Elections), len(legacy.Elections))
	}
	if shadow.Receipt.Tokens != 103 || shadow.Receipt.Dispatches != 196 || shadow.BranchOrder != 4 || shadow.NextCreationSeq != 6 ||
		shadow.Receipt.GlobalBranchOrder != 4 || shadow.Receipt.NextCreationSeq != 6 ||
		!shadow.SingleHeadGLRNil || !reflect.DeepEqual(shadow.MultiHeadStates, []gotreesitter.StateID{164, 194}) ||
		!reflect.DeepEqual(shadow.Election, legacy.Elections[102]) {
		t.Fatalf("edits election scheduler totals drifted: shadow=%+v election=%+v", shadow.Receipt, shadow.Election)
	}
	if shadow.Stats.Nodes != 208 || shadow.Stats.Links != 207 || shadow.Stats.Subtrees != 196 || shadow.Stats.Children != 184 || shadow.Stats.CurrentExactPaths != 2 {
		t.Fatalf("edits election physical receipt drifted: %+v", shadow.Stats)
	}
	// The generic scheduler records and drops the two no-action siblings after
	// real sibling progress. Relative to the frozen route, this avoids two
	// legacy pre-lookup dispatch charges and ends with two fewer dead
	// speculative subtrees; the semantic frontier remains identical.
	if legacy.Dispatches-shadow.Receipt.Dispatches != 2 || legacy.CachedDotClosure.SubtreesAfter-shadow.Stats.Subtrees != 2 ||
		legacy.CachedDotClosure.NodesAfter != shadow.Stats.Nodes || legacy.CachedDotClosure.LinksAfter != shadow.Stats.Links || legacy.CachedDotClosure.ChildrenAfter != shadow.Stats.Children ||
		len(shadow.Receipt.NoActionDrops) != 2 || shadow.Receipt.NoActionDrops[0].ElectionIndex != 67 || shadow.Receipt.NoActionDrops[0].Header.Header.State != 254 || shadow.Receipt.NoActionDrops[0].Header.Header.ByteOffset != 579 ||
		shadow.Receipt.NoActionDrops[1].ElectionIndex != 97 || shadow.Receipt.NoActionDrops[1].Header.Header.State != 22 || shadow.Receipt.NoActionDrops[1].Header.Header.ByteOffset != 730 {
		t.Fatalf("generic no-action work reduction drifted: legacy_dispatches=%d seed_dispatches=%d legacy_stats=%d/%d/%d/%d seed_stats=%+v drops=%+v",
			legacy.Dispatches, shadow.Receipt.Dispatches,
			legacy.CachedDotClosure.NodesAfter, legacy.CachedDotClosure.LinksAfter, legacy.CachedDotClosure.SubtreesAfter, legacy.CachedDotClosure.ChildrenAfter,
			shadow.Stats, shadow.Receipt.NoActionDrops)
	}
	wantHeaders := append([]gotreesitter.DiagnosticParserCoreHeaderPathReceipt(nil), legacy.CachedDotClosure.Headers...)
	for index := range wantHeaders {
		wantHeaders[index].Header.Shifted = false
		wantHeaders[index].Header.Checkpoint = shadow.Election.ScannerAfter.SHA256
	}
	if !reflect.DeepEqual(shadow.Headers, wantHeaders) {
		t.Fatalf("edits election frontier diverged\nseed=%+v\nfrozen=%+v", shadow.Headers, wantHeaders)
	}
	if len(shadow.Headers) != 2 || shadow.Headers[0].Header.State != 164 || shadow.Headers[0].Header.ByteOffset != 742 || shadow.Headers[0].Header.Shifted || shadow.Headers[0].Header.CreationSeq != 1 ||
		shadow.Headers[1].Header.State != 194 || shadow.Headers[1].Header.ByteOffset != 742 || shadow.Headers[1].Header.Shifted || shadow.Headers[1].Header.CreationSeq != 4 ||
		shadow.Headers[0].Header.Checkpoint != shadow.Election.ScannerAfter.SHA256 || shadow.Headers[1].Header.Checkpoint != shadow.Election.ScannerAfter.SHA256 {
		t.Fatalf("edits election reset frontier identity drifted: %+v", shadow.Headers)
	}
	wantGates := []struct {
		byte       uint32
		dispatches uint64
		nodes      uint32
		links      uint32
		subtrees   uint32
		children   uint32
	}{
		{116, 23, 24, 23, 23, 18},
		{580, 120, 127, 126, 121, 111},
		{725, 180, 189, 188, 182, 173},
		{732, 187, 196, 195, 188, 181},
		{742, 196, 208, 207, 196, 184},
	}
	if len(shadow.ClosedGates) != len(wantGates) {
		t.Fatalf("seed closed gates=%+v, want=%v", shadow.ClosedGates, wantGates)
	}
	for index, gate := range shadow.ClosedGates {
		want := wantGates[index]
		if gate.Byte != want.byte || gate.Dispatches != want.dispatches ||
			gate.Stats.Nodes != want.nodes || gate.Stats.Links != want.links || gate.Stats.Subtrees != want.subtrees || gate.Stats.Children != want.children || len(gate.Headers) == 0 {
			t.Fatalf("seed closed gate %d=%+v, want=%+v", index, gate, want)
		}
	}
}
