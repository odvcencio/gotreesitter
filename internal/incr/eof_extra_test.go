package incr

import "testing"

type eofExtraProof struct{ accepted bool }

func (eofExtraProof) EOFExtraTokenProofID() uint8 { return 1 }

func (p eofExtraProof) EOFExtraTokenAppendInvariant([]byte, []byte, uint16, uint32) bool {
	return p.accepted
}

func TestEOFExtraAppendRequiresExactSourceAndLexerProof(t *testing.T) {
	for _, tc := range []struct {
		name, old, next string
		proof           any
		edit            TokenEdit
		start           uint32
		want            bool
	}{
		{"append", "// x", "// xy", eofExtraProof{true}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 0, true},
		{"unknown", "// x", "// xy", struct{}{}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 0, false},
		{"declined", "// x", "// xy", eofExtraProof{}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 0, false},
		{"unreported_prefix", "// x", "/* xy", eofExtraProof{true}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 0, false},
		{"newline", "// x", "// x\n", eofExtraProof{true}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 0, false},
		{"delimiter", "// x", "// x/", eofExtraProof{true}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 0, false},
		{"multiple_bytes", "// x", "// xyz", eofExtraProof{true}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 6}, 0, false},
		{"replacement", "// x", "// xy", eofExtraProof{true}, TokenEdit{Start: 3, OldEnd: 4, NewEnd: 5}, 0, false},
		{"empty_token", "// x", "// xy", eofExtraProof{true}, TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EOFExtraAppend(tc.proof, 1, []byte(tc.old), []byte(tc.next), tc.edit, 1, tc.start); got != tc.want {
				t.Fatalf("proof = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestEOFExtraAppendRequiresAcceptedBackendIdentity(t *testing.T) {
	for _, unknown := range []uint8{0, 2} {
		if EOFExtraAppend(eofExtraProof{true}, unknown, []byte("// x"), []byte("// xy"),
			TokenEdit{Start: 4, OldEnd: 4, NewEnd: 5}, 1, 0) {
			t.Fatalf("backend %d supplied another backend's proof", unknown)
		}
	}
}
