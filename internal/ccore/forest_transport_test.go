package ccore

import (
	"slices"
	"testing"
)

func TestForestElectionLifetime(t *testing.T) {
	tables := linTables(t)
	tables.BuildAscii()
	p, err := NewParser(Config{Tables: tables})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"[]", "]"} {
		offset := 0
		e := &ForestElection{WorkLimit: 100, ReduceLimit: 100, CanShift: func(uint32) bool { return true }, Resolve: func(uint16, int) int { return -1 }}
		e.Next = func(states []uint16) (Token, bool, bool, bool) {
			start := uint32(offset)
			tok := Token{StartByte: start, EndByte: start, StartCol: start, EndCol: start}
			if offset < len(input) {
				sym := slices.Index(tables.Names, input[offset:offset+1])
				if sym < 0 {
					t.Fatalf("missing JSON symbol %q", input[offset:offset+1])
				}
				tok.Symbol = Symbol(sym)
				tok.EndByte++
				tok.EndCol++
				offset++
			}
			return tok, false, false, false
		}
		tree, stop := p.ParseFreshForest([]byte(input), e)
		if width := p.Stats().MaxProcessed; width != 1 {
			t.Fatalf("%q processed width=%d, want 1", input, width)
		}
		if p.forestElection != nil {
			t.Fatalf("%q retained the request scanner", input)
		}
		if input == "[]" {
			if tree == nil || stop != "" {
				t.Fatalf("valid JSON stop=%q", stop)
			}
			tree.Release()
		} else if tree != nil || stop != "dead_end" {
			if tree != nil {
				tree.Release()
			}
			t.Fatalf("invalid JSON stop=%q", stop)
		}
	}
	normal, stop := p.Parse([]byte("[]"))
	if normal == nil || stop != "" {
		t.Fatalf("ordinary parse after scanner release: %q", stop)
	}
	normal.Release()
}
