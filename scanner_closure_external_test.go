//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestStatefulScannerCompactLeafReauthentication exercises the real compact
// candidate route. Python's scanner retains indentation and delimiter stacks,
// so a same-length leaf edit must pass the checkpoint reauthentication gate.
func TestStatefulScannerCompactLeafReauthentication(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source []byte
		old    byte
		new    byte
	}{
		{
			name:   "indentation_sensitive",
			source: []byte("def value():\n    item = 1\n    return item\n"),
			old:    '1',
			new:    '2',
		},
		{
			name:   "delimiter_sensitive",
			source: []byte("def value():\n    items = [1, 2]\n    return items[0]\n"),
			old:    '1',
			new:    '3',
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := grammars.PythonLanguage()
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(true)
			gotreesitter.ResetAdmissionCandidateCountersForTest()
			oldTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatalf("compact candidate parse: %v", err)
			}
			defer oldTree.Release()
			if routed, fallback := gotreesitter.AdmissionCandidateCounters(); routed != 1 || fallback != 0 {
				t.Fatalf("Python fixture did not produce a real compact tree: routed=%d fallback=%d reason=%q", routed, fallback, gotreesitter.AdmissionCandidateLastFallbackReason())
			}

			offset := -1
			for i, b := range tc.source {
				if b == tc.old {
					offset = i
					break
				}
			}
			if offset < 0 {
				t.Fatalf("fixture has no %q marker", tc.old)
			}
			next := append([]byte(nil), tc.source...)
			next[offset] = tc.new
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(tc.source, offset),
				OldEndPoint: pointForOffset(tc.source, offset+1),
				NewEndPoint: pointForOffset(next, offset+1),
			})

			incremental, profile, err := parser.ParseIncrementalProfiled(next, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer incremental.Release()
			if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
				t.Fatalf("stateful compact tree did not reauthenticate scanner checkpoints: %+v", profile)
			}

			freshParser := gotreesitter.NewParser(lang)
			freshParser.SetAdmissionCandidateRoute(false)
			fresh, err := freshParser.Parse(next)
			if err != nil {
				t.Fatalf("forced production fresh parse: %v", err)
			}
			defer fresh.Release()
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		})
	}
}
