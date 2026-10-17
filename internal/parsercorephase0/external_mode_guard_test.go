package parsercorephase0

import "testing"

func TestExternalModesRejectAllRequiresBothExplicitEmptyRows(t *testing.T) {
	for _, tt := range []struct {
		name               string
		rows               [][]bool
		primary, errorMode uint16
		want               bool
	}{
		{"no table", nil, 0, 0, false},
		{"explicit zero-width row", [][]bool{nil}, 0, 0, true},
		{"empty modes", [][]bool{{false, false}, {false}}, 0, 1, true},
		{"same mode", [][]bool{{false, false}}, 0, 0, true},
		{"primary can scan", [][]bool{{false, true}, {false}}, 0, 1, false},
		{"recovery can scan", [][]bool{{false}, {true}}, 0, 1, false},
		{"primary missing", [][]bool{{false}}, 65535, 0, false},
		{"recovery missing", [][]bool{{false}}, 0, 65535, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExternalModesRejectAll(tt.rows, tt.primary, tt.errorMode); got != tt.want {
				t.Fatalf("got %t want %t", got, tt.want)
			}
		})
	}
}

func TestExternalModesRejectAllPreservesHighestModeID(t *testing.T) {
	rows := make([][]bool, 1<<16)
	if !ExternalModesRejectAll(rows, ^uint16(0), 0) {
		t.Fatal("highest representable mode was narrowed")
	}
	rows[65535] = []bool{true}
	if ExternalModesRejectAll(rows, 0, ^uint16(0)) {
		t.Fatal("highest recovery mode was ignored")
	}
}
