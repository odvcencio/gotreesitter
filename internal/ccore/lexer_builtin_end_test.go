package ccore

import "testing"

func TestScanBuiltinEndWithUnreadSource(t *testing.T) {
	states := []LexDFAState{
		{AcceptEnd: true, Default: -1, EOF: -1, Trans: []LexTrans{
			{Lo: 'a', Hi: 'a', Next: 1},
			{Lo: ' ', Hi: ' ', Next: 2},
		}},
		{AcceptToken: 4, Default: -1, EOF: -1},
		{Skip: true, Default: -1, EOF: -1},
		{AcceptEOF: true, Default: -1, EOF: -1},
	}
	tables := &Tables{LexStates: states, LexAscii: BuildLexAscii(states)}
	for _, tc := range []struct {
		name, src string
		state     uint32
		matched   bool
		tok       Token
	}{
		{"early end", "!", 0, true, Token{BuiltinEnd: true, LookaheadEnd: 1}},
		{"real token wins", "a!", 0, true, Token{Symbol: 4, EndByte: 1, EndCol: 1, LookaheadEnd: 2}},
		{"skip wins", " !", 0, true, Token{EndByte: 1, EndCol: 1, LookaheadEnd: 2}},
		{"physical EOF only", "!", 3, false, Token{LookaheadEnd: 1}},
		{"physical EOF accepted", "", 3, true, Token{LookaheadEnd: 1}},
		{"invalid UTF8 frontier", "\xff", 0, true, Token{BuiltinEnd: true, LookaheadEnd: 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, provider := range []bool{false, true} {
				p := Parser{t: tables, src: []byte(tc.src)}
				if provider {
					p.scanProv = NewTableScan(tables, p.src)
				}
				var got Token
				if ok := p.scan(tc.state, 0, 0, 0, &got); ok != tc.matched || got != tc.tok {
					t.Fatalf("provider=%t ok=%t token=%+v, want %t %+v", provider, ok, got, tc.matched, tc.tok)
				}
			}
		})
	}
}

func TestRecoveryScanRecognizesEarlyBuiltinEnd(t *testing.T) {
	states := []LexDFAState{{AcceptEnd: true, Default: -1, EOF: -1}}
	tables := &Tables{LexStates: states, LexAscii: BuildLexAscii(states)}
	src := []byte("??x")
	for _, provider := range []bool{false, true} {
		p := Parser{t: tables, src: src}
		if provider {
			p.scanProv = NewTableScan(tables, src)
		}
		if found, _, frontier := p.scanFrom(LexMode{}, NewLength(1, 0, 1)); !found || frontier != 2 {
			t.Fatalf("provider=%t found=%t frontier=%d", provider, found, frontier)
		}
	}
}
