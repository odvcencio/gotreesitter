package ccore_test

import (
	"errors"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// scanStub is a hook that never finds a token; it only has to be non-nil.
type scanStub struct{}

func (scanStub) Scan([]byte, int, uint32, uint32, []byte, []bool, []byte, *ccore.ExtScan) bool {
	return false
}
func (scanStub) Close() {}

// TestNewParserAcceptsGoldens: the tables the goldens carry pass the checks.
func TestNewParserAcceptsGoldens(t *testing.T) {
	for _, g := range loadGoldens(t) {
		if _, err := ccore.NewParser(ccore.Config{Tables: g.Tables}); err != nil {
			t.Errorf("%s: %v", g.Name, err)
		}
	}
}

// TestNewParserRefuses changes one fact of valid tables at a time. Each case
// must be refused with the error of its kind.
func TestNewParserRefuses(t *testing.T) {
	base := loadGoldens(t)[0].Tables
	clone := func() *ccore.Tables { c := *base; return &c }
	cases := []struct {
		name string
		want error
		make func() ccore.Config
	}{
		{"nil tables", ccore.ErrNoTables, func() ccore.Config { return ccore.Config{} }},
		{"initial state 0", ccore.ErrInitialState, func() ccore.Config {
			c := clone()
			c.InitialState = 0
			return ccore.Config{Tables: c}
		}},
		{"initial state 2", ccore.ErrInitialState, func() ccore.Config {
			c := clone()
			c.InitialState = 2
			return ccore.Config{Tables: c}
		}},
		{"no states", ccore.ErrNoStates, func() ccore.Config {
			c := clone()
			c.StateCount = 0
			return ccore.Config{Tables: c}
		}},
		{"65,536 states", ccore.ErrTooManyStates, func() ccore.Config {
			c := clone()
			c.StateCount = 65536
			return ccore.Config{Tables: c}
		}},
		{"65,536 tokens", ccore.ErrTooManyTokens, func() ccore.Config {
			c := clone()
			c.TokenCount = 65536
			return ccore.Config{Tables: c}
		}},
		{"65,536 symbols", ccore.ErrTooManySymbols, func() ccore.Config {
			c := clone()
			c.Meta = make([]ccore.SymbolMeta, 65536)
			return ccore.Config{Tables: c}
		}},
		{"65,535 symbols (collides with C's error symbols)", ccore.ErrTooManySymbols, func() ccore.Config {
			c := clone()
			c.Meta = make([]ccore.SymbolMeta, 65535)
			return ccore.Config{Tables: c}
		}},
		{"lex modes one short", ccore.ErrLexModesShort, func() ccore.Config {
			c := clone()
			c.LexModes = c.LexModes[:c.StateCount-1]
			return ccore.Config{Tables: c}
		}},
		{"no lex automaton", ccore.ErrNoLexAutomaton, func() ccore.Config {
			c := clone()
			c.LexStates = nil
			return ccore.Config{Tables: c}
		}},
		{"keyword capture 256", ccore.ErrKeywordCapture, func() ccore.Config {
			c := clone()
			c.KeywordLexStates = c.LexStates[:1]
			c.KeywordCapture = 256
			return ccore.Config{Tables: c}
		}},
		{"keyword capture 255", ccore.ErrKeywordCapture, func() ccore.Config {
			c := clone()
			c.KeywordLexStates = c.LexStates[:1]
			c.KeywordCapture = 255
			return ccore.Config{Tables: c}
		}},
		{"shift to the state count", ccore.ErrShiftTarget, func() ccore.Config {
			c := clone()
			c.Actions = []ccore.ActionEntry{{Actions: []ccore.Action{{Type: ccore.ActionShift, State: uint32(len(c.LexModes))}}}}
			return ccore.Config{Tables: c}
		}},
		{"goto in a dense row at the state count", ccore.ErrGotoTarget, func() ccore.Config {
			c := clone()
			row := make([]uint16, c.TokenCount+1)
			row[c.TokenCount] = uint16(len(c.LexModes))
			c.Dense = [][]uint16{nil, row}
			return ccore.Config{Tables: c}
		}},
		{"goto in the compressed small table at the state count", ccore.ErrGotoTarget, func() ccore.Config {
			c := clone()
			c.Small = []uint16{1, uint16(len(c.LexModes)), 1, uint16(c.TokenCount)}
			c.SmallMap = []uint32{0}
			return ccore.Config{Tables: c}
		}},
		{"goto in a derived small-state row at the state count", ccore.ErrGotoTarget, func() ccore.Config {
			c := clone()
			row := make([]uint16, c.TokenCount+1)
			row[c.TokenCount] = uint16(len(c.LexModes))
			c.SmallTokenLookup = [][]uint16{row}
			return ccore.Config{Tables: c}
		}},
		{"goto in a sparse small lookup at the state count", ccore.ErrGotoTarget, func() ccore.Config {
			c := clone()
			c.SmallLookup = [][]ccore.SmallPair{{{Sym: uint16(c.TokenCount), Val: uint16(len(c.LexModes))}}}
			return ccore.Config{Tables: c}
		}},
		{"goto in the large-state map at the state count", ccore.ErrGotoTarget, func() ccore.Config {
			c := clone()
			c.LargeStateGotos = map[uint64]uint32{1<<32 | uint64(c.TokenCount): uint32(len(c.LexModes))}
			return ccore.Config{Tables: c}
		}},
		{"large-state goto above 65,535", ccore.ErrGotoTooLarge, func() ccore.Config {
			c := clone()
			c.LargeStateGotos = map[uint64]uint32{1<<32 | uint64(c.TokenCount): 65536}
			return ccore.Config{Tables: c}
		}},
		{"shift to state 65,536", ccore.ErrShiftState, func() ccore.Config {
			c := clone()
			c.Actions = []ccore.ActionEntry{{Actions: []ccore.Action{{Type: ccore.ActionShift, State: 65536}}}}
			return ccore.Config{Tables: c}
		}},
		{"external valid-symbol row shorter than the external symbols", ccore.ErrExternalRow, func() ccore.Config {
			c := clone()
			c.ExternalSymbols = []ccore.Symbol{1, 2}
			c.ExternalLexStates = [][]bool{nil, {true}}
			return ccore.Config{Tables: c, Scanner: scanStub{}}
		}},
		{"external symbols without a scanner", ccore.ErrScannerMissing, func() ccore.Config {
			c := clone()
			c.ExternalSymbols = []ccore.Symbol{1}
			return ccore.Config{Tables: c}
		}},
	}
	for _, tc := range cases {
		p, err := ccore.NewParser(tc.make())
		if err == nil || p != nil {
			t.Errorf("%s: accepted (parser %v, error %v)", tc.name, p, err)
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: error %q is not %q", tc.name, err, tc.want)
		}
	}
	// The same tables with a scanner hook and a keyword capture of 254 pass.
	c := clone()
	c.ExternalSymbols = []ccore.Symbol{1}
	c.KeywordLexStates = c.LexStates[:1]
	c.KeywordCapture = 254
	if _, err := ccore.NewParser(ccore.Config{Tables: c, Scanner: scanStub{}}); err != nil {
		t.Errorf("tables with a scanner and keyword capture 254 refused: %v", err)
	}
}

// TestRefusalTextsDiffer: the fifteen refusals have fifteen different texts.
func TestRefusalTextsDiffer(t *testing.T) {
	errs := []error{ccore.ErrNoTables, ccore.ErrInitialState, ccore.ErrNoStates, ccore.ErrTooManyStates, ccore.ErrTooManyTokens,
		ccore.ErrTooManySymbols, ccore.ErrLexModesShort, ccore.ErrNoLexAutomaton, ccore.ErrKeywordCapture, ccore.ErrShiftState, ccore.ErrScannerMissing,
		ccore.ErrShiftTarget, ccore.ErrGotoTarget, ccore.ErrGotoTooLarge, ccore.ErrExternalRow}
	seen := map[string]bool{}
	for _, e := range errs {
		if seen[e.Error()] {
			t.Errorf("duplicate text %q", e)
		}
		seen[e.Error()] = true
	}
}
