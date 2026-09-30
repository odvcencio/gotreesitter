package lex

import "testing"

func TestRecoveryExternalProgressIncludesPaddingAndScannerState(t *testing.T) {
	for _, test := range []struct {
		name         string
		start, end   uint32
		stateChanged bool
		want         bool
	}{
		{"padding", 2, 3, false, true},
		{"empty", 3, 3, false, false},
		{"state_change", 3, 3, true, true},
		{"before_start", 3, 2, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := KeepRecoveryExternalToken(test.start, test.end, test.stateChanged); got != test.want {
				t.Fatalf("keep = %t, want %t", got, test.want)
			}
		})
	}
}
