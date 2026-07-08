package grammargen

import (
	"strings"
	"testing"
)

// TestCheckUint16IndexBoundary pins the exact overflow boundary of the guard
// that protects every uint16-indexed table grammargen writes (parse-action
// group indices, field/supertype-map offsets, reserved-word-set IDs,
// external-lex-state rows). Constructing a real
// >65535-entry grammar is impractical for a unit test, so the bounds check is
// factored into checkUint16Index and its failure path is exercised directly.
func TestCheckUint16IndexBoundary(t *testing.T) {
	cases := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"zero", 0, false},
		{"one", 1, false},
		{"limit-minus-one", maxUint16Index - 1, false},
		{"at-limit", maxUint16Index, false},           // 65535 still fits in uint16
		{"just-over-limit", maxUint16Index + 1, true}, // 65536 wraps to 0
		{"far-over-limit", 1 << 20, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUint16Index("scala", "parse action group", tc.value)
			if tc.wantErr && err == nil {
				t.Fatalf("checkUint16Index(%d) = nil, want overflow error", tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("checkUint16Index(%d) = %v, want nil", tc.value, err)
			}
		})
	}
}

// TestCheckUint16IndexErrorIsActionable verifies the hard-fail error names the
// grammar, the offending field, the count, and the limit — everything a
// downstream author needs to diagnose the failure instead of chasing a
// silently corrupted table.
func TestCheckUint16IndexErrorIsActionable(t *testing.T) {
	err := checkUint16Index("my-huge-lang", "supertype map entry", 70000)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"my-huge-lang", "supertype map entry", "70000", "65535"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

// TestMaxUint16IndexValue guards against an accidental redefinition of the
// limit constant: it must equal the largest value a uint16 can hold.
func TestMaxUint16IndexValue(t *testing.T) {
	if maxUint16Index != int(^uint16(0)) {
		t.Fatalf("maxUint16Index = %d, want %d", maxUint16Index, int(^uint16(0)))
	}
	// The value one past the limit must not survive a uint16 round-trip:
	// this is precisely the silent wrap-to-0 the guard exists to prevent.
	// (Use a variable so the conversion happens at runtime — a constant
	// uint16(65536) is a compile-time error, not a silent wrap.)
	overLimit := maxUint16Index + 1
	if got := uint16(overLimit); got != 0 {
		t.Fatalf("expected uint16(%d) to wrap to 0, got %d", overLimit, got)
	}
}
