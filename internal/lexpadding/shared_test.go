package lexpadding

import "testing"

func TestSharedExternalPaddingProof(t *testing.T) {
	for _, test := range []struct {
		name, source                       string
		start, end, skipStart, skipEnd     uint32
		external, skipped, stateless, want bool
	}{
		{"newline", "\nY", 0, 1, 0, 1, true, true, true, true},
		{"crlf", "\r\n Y", 0, 2, 0, 3, true, true, true, true},
		{"concatenating_space", " \tY", 0, 2, 0, 2, true, true, true, false},
		{"content", "\nx", 0, 2, 0, 2, true, true, true, false},
		{"partial_skip", "\r\nY", 0, 2, 0, 1, true, true, true, false},
		{"different_skip", "\nY", 0, 1, 1, 2, true, true, true, false},
		{"unproved_skip", "\nY", 0, 1, 0, 1, true, false, true, false},
		{"stateful_scanner", "\nY", 0, 1, 0, 1, true, true, false, false},
		{"internal_token", "\nY", 0, 1, 0, 1, false, true, true, false},
		{"zero_width", "\nY", 0, 0, 0, 0, true, true, true, false},
		{"out_of_source", "\nY", 0, 3, 0, 3, true, true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := SharedExternalSkipped([]byte(test.source), test.start, test.end, test.skipStart, test.skipEnd, test.external, test.skipped, test.stateless); got != test.want {
				t.Fatalf("padding proof=%t, want %t", got, test.want)
			}
		})
	}
}

func TestContinuationGapCannotDropContent(t *testing.T) {
	for _, source := range []string{"", " \r\n\t", "#", "\nx", "\\\n"} {
		want := source == "" || source == " \r\n\t"
		if got := Whitespace([]byte(source), 0, uint32(len(source))); got != want {
			t.Fatalf("gap %q padding=%t, want %t", source, got, want)
		}
	}
}
