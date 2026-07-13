package main

import (
	"testing"
	"unicode/utf16"
)

func units(value string) []uint16 {
	return utf16.Encode([]rune(value))
}

func TestUTF16EditBetween(t *testing.T) {
	tests := []struct {
		name                  string
		oldSource             string
		newSource             string
		start, oldEnd, newEnd uint32
		changed               bool
	}{
		{name: "same", oldSource: "hello", newSource: "hello", start: 5, oldEnd: 5, newEnd: 5},
		{name: "insert", oldSource: "ab", newSource: "aXb", start: 1, oldEnd: 1, newEnd: 2, changed: true},
		{name: "replace", oldSource: "hello", newSource: "hullo", start: 1, oldEnd: 2, newEnd: 2, changed: true},
		{name: "delete", oldSource: "abc", newSource: "ac", start: 1, oldEnd: 2, newEnd: 1, changed: true},
		{name: "emoji", oldSource: "a😀z", newSource: "a😁z", start: 1, oldEnd: 3, newEnd: 3, changed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			edit, changed := utf16EditBetween(units(test.oldSource), units(test.newSource))
			if changed != test.changed || edit.StartCodeUnit != test.start || edit.OldEndCodeUnit != test.oldEnd || edit.NewEndCodeUnit != test.newEnd {
				t.Fatalf("edit = %+v, changed = %v", edit, changed)
			}
		})
	}
}
