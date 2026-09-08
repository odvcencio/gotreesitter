//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"sync"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestRecoverySymbolPolicyCachePreservesDefaults(t *testing.T) {
	if diagnosticParserCoreRecoverySymbolPolicy(nil) != nil {
		t.Fatal("nil language has a policy")
	}
	for _, tc := range []struct {
		name     string
		metadata []SymbolMetadata
		names    int
		count    uint32
		want     int
	}{
		{"metadata-width", []SymbolMetadata{{Visible: false, Named: true}, {Visible: true}}, 1, 1, 2},
		{"name-width", []SymbolMetadata{{Named: true}}, 4, 2, 4},
		{"count-width", []SymbolMetadata{{Visible: true, Named: true}}, 2, 5, 5},
		{"empty", nil, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := &Language{SymbolMetadata: tc.metadata, SymbolNames: make([]string, tc.names), SymbolCount: tc.count}
			policy := diagnosticParserCoreRecoverySymbolPolicy(lang)
			if len(policy) != tc.want {
				t.Fatalf("width=%d, want %d", len(policy), tc.want)
			}
			for i, got := range policy {
				want := core.SelectedSymbolPolicy{Visible: true}
				if i < len(tc.metadata) {
					want.Visible = tc.metadata[i].Visible
					want.Named = tc.metadata[i].Named
				}
				if got != want {
					t.Fatalf("symbol %d=%+v, want %+v", i, got, want)
				}
			}
			again := diagnosticParserCoreRecoverySymbolPolicy(lang)
			if len(policy) > 0 && &again[0] != &policy[0] {
				t.Fatal("same language rebuilt the policy")
			}
			if cap(policy) != len(policy) {
				t.Fatal("policy retained spare capacity")
			}
		})
	}
}

func TestRecoverySymbolPolicyCacheTracksMetadataReplacement(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{Visible: true}}, SymbolCount: 2}
	first := diagnosticParserCoreRecoverySymbolPolicy(lang)
	lang.SymbolMetadata = []SymbolMetadata{{Named: true}}
	second := diagnosticParserCoreRecoverySymbolPolicy(lang)
	if second[0].Visible || !second[0].Named || !first[0].Visible || first[0].Named {
		t.Fatal("replacement changed an old policy or reused stale metadata")
	}
	lang.SymbolCount = 3
	third := diagnosticParserCoreRecoverySymbolPolicy(lang)
	if len(third) != 3 || !third[2].Visible || third[2].Named {
		t.Fatal("symbol width change retained a short policy")
	}
	lang.SymbolMetadata = append(lang.SymbolMetadata, SymbolMetadata{Named: true}, SymbolMetadata{})
	fourth := diagnosticParserCoreRecoverySymbolPolicy(lang)
	if fourth[1].Visible || !fourth[1].Named || fourth[2].Visible {
		t.Fatal("metadata growth did not replace visible defaults")
	}
}

func TestRecoverySymbolPolicyCacheConcurrentFirstUse(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{Visible: true, Named: true}}, SymbolCount: 257}
	const workers = 32
	results := make([][]core.SelectedSymbolPolicy, workers)
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	for i := range results {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			results[i] = diagnosticParserCoreRecoverySymbolPolicy(lang)
		}(i)
	}
	start.Done()
	done.Wait()
	for _, policy := range results {
		if &policy[0] != &results[0][0] || !policy[256].Visible {
			t.Fatal("first use did not publish one complete shared policy")
		}
	}
	bytes := uintptr(cap(results[0])) * unsafe.Sizeof(core.SelectedSymbolPolicy{})
	if bytes != 514 {
		t.Fatalf("policy retained %d bytes, want 514", bytes)
	}
}

func TestRecoverySymbolPolicyCacheBoundsRetention(t *testing.T) {
	lang := &Language{SymbolCount: 2}
	diagnosticParserCoreRecoverySymbolPolicy(lang)
	lang.SymbolCount = 65537
	policy := diagnosticParserCoreRecoverySymbolPolicy(lang)
	if len(policy) != 65537 || !policy[65536].Visible {
		t.Fatal("oversized projection changed its width or visibility")
	}
	if lang.compactRecoverySymbols.Load() != nil {
		t.Fatal("cache retained an oversized policy or stale metadata")
	}
}
