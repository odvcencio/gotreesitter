package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestRepairNoLookaheadLexModes(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	tests := []struct {
		name  string
		load  func() []gotreesitter.LexMode
		state int
	}{
		{
			name:  "scala",
			load:  func() []gotreesitter.LexMode { return ScalaLanguage().LexModes },
			state: 21248,
		},
		{
			name:  "rust",
			load:  func() []gotreesitter.LexMode { return RustLanguage().LexModes },
			state: 3820,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lexModes := tc.load()
			if tc.state >= len(lexModes) {
				t.Fatalf("state %d out of range for %s", tc.state, tc.name)
			}
			if got := lexModes[tc.state].LexState; got != ^uint16(0) {
				t.Fatalf("LexModes[%d].LexState = %d, want %d", tc.state, got, ^uint16(0))
			}
		})
	}
}
