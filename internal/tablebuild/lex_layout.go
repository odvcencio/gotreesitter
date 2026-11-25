package tablebuild

import (
	"errors"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// This file ports the parts of render.rs that number the lexer: fn
// add_lex_function and fn add_lex_state (as the steps of the C code, not its
// text), fn add_lex_modes, fn add_external_scanner_symbol_map, fn
// add_external_scanner_states_list and the keyword capture token of fn
// add_parser_export. layout.go ports fn add_reserved_word_sets and calls these
// functions in the order of fn Generator::generate.

// NoLexState is the lex state of a LexMode that render.rs writes as
// (TSStateId)(-1): the mode of a parse state that ends a non-terminal extra.
const NoLexState = ^uint32(0)

// LexMode is one entry of ts_lex_modes.
type LexMode struct {
	// LexState is the state of ts_lex that the parse state starts in, or
	// NoLexState.
	LexState uint32
	// ExternalLexState is the row of ts_external_scanner_states (0: none).
	ExternalLexState uint32
	// ReservedWordSetID is the row of ts_reserved_words; 0 below ABI 15,
	// where the generator does not write it.
	ReservedWordSetID uint32
}

// LexState is one state of a lex function of parser.c (ts_lex or
// ts_lex_keywords) as the steps that its C code takes. The C code is the one
// that render.rs: fn add_lex_state writes for the state; lexbuild.Table
// evaluates it (lexbuild.Table.Record).
type LexState struct {
	// Accept is the code of the state's ACCEPT_TOKEN, as in
	// lexexact.Func.Accept: 0 without one, otherwise the symbol id plus 1
	// (1 is ts_builtin_sym_end).
	Accept uint32
	// EOFAction is the target of the state's "if (eof) ADVANCE(n);"
	// (tables.rs: LexState::eof_action), -1 without one.
	EOFAction int32
	// OnEOF and OnInvalid are the steps that the C code takes at the end of
	// input and on an ill-formed byte (lookahead -1), in the form of
	// lexexact.Func.OnEOF. OnEOF is EOFAction when the state has one;
	// without one, another statement can step at the end of input, because
	// C reads the lookahead 0 there: an ADVANCE_MAP pair with the key 0.
	OnEOF, OnInvalid uint32
	// Ranges are the steps on the code points 0 to 0x10FFFF with eof false,
	// in order, disjoint, neighbours with the same step joined.
	Ranges []lexexact.Range
}

// tokenID is the C symbol id of a token for lexbuild.Table.Record: the
// value of its identifier in ts_symbol_identifiers. The generator reads
// symbol_ids with an index that panics on a missing key; the port reports a
// missing token to Record, which returns an error.
func (g *generator) tokenID(s lexbuild.Symbol) (uint32, bool) {
	id, ok := g.symbolIDs[s]
	if !ok {
		return 0, false
	}
	v, ok := g.enumValues[id]
	return uint32(v), ok
}

// addLexFunction is render.rs: fn add_lex_function, with fn add_lex_state
// for each state: the lex states of the table in order, each as the steps of
// the code that add_lex_state writes for it.
func (g *generator) addLexFunction(t *lexbuild.Table) ([]LexState, error) {
	rec, err := t.Record(g.tokenID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]LexState, t.NumStates())
	for i := range out {
		eof := int32(-1)
		if target, ok := t.EOFAction(i); ok {
			eof = int32(target)
		}
		out[i] = LexState{Accept: rec.Accept[i], EOFAction: eof, OnEOF: rec.OnEOF[i], OnInvalid: rec.OnInvalid[i], Ranges: rec.Trans[i]}
	}
	return out, nil
}

// addLexFunctions is the part of render.rs: fn Generator::generate that
// writes the lex functions: ts_lex, then ts_lex_keywords when the grammar has
// a word token. The large character sets that generate writes before them
// change no step.
func (g *generator) addLexFunctions() error {
	if g.lexTables == nil {
		return nil
	}
	var err error
	if g.out.MainLex, err = g.addLexFunction(g.lexTables.Main); err != nil {
		return err
	}
	if g.syntaxGrammar.HasWord {
		if g.lexTables.Keyword == nil {
			panicLikeGenerator("the grammar has a word token and no keyword lex table")
		}
		if g.out.KeywordLex, err = g.addLexFunction(g.lexTables.Keyword); err != nil {
			return err
		}
	}
	return nil
}

// addKeywordCaptureToken is the part of render.rs: fn
// Generator::add_parser_export that numbers a table entry: the symbol id of
// the word token as .keyword_capture_token.
func (g *generator) addKeywordCaptureToken() {
	if g.syntaxGrammar.HasWord {
		g.out.KeywordCaptureToken = g.symbolValue(g.syntaxGrammar.WordToken)
	}
}

// addLexModes is render.rs: fn Generator::add_lex_modes.
func (g *generator) addLexModes() {
	table := g.parseTable
	g.out.LexModes = make([]LexMode, len(table.States))
	for i := range table.States {
		state := &table.States[i]
		if state.isEndOfNonTerminalExtra() {
			g.out.LexModes[i] = LexMode{LexState: NoLexState}
			continue
		}
		m := LexMode{LexState: state.LexStateID, ExternalLexState: state.ExternalLexStateID}
		if g.abiVersion >= abiVersionWithReservedWords {
			m.ReservedWordSetID = uint32(g.reservedWordSetIDsByParseState[i])
		}
		g.out.LexModes[i] = m
	}
}

// addExternalScannerSymbolMap is render.rs: fn
// Generator::add_external_scanner_symbol_map: for each external token, the
// symbol id of its corresponding internal token, or its own.
func (g *generator) addExternalScannerSymbolMap() {
	for i, token := range g.syntaxGrammar.ExternalTokens {
		idToken := lexbuild.ExternalSym(i)
		if token.HasCorresponding {
			idToken = token.CorrespondingInternal
		}
		g.out.ExternalScannerSymbolMap = append(g.out.ExternalScannerSymbolMap, g.symbolValue(idToken))
	}
}

// addExternalScannerStatesList is render.rs: fn
// Generator::add_external_scanner_states_list: one row per external lex
// state, a flag per external token.
func (g *generator) addExternalScannerStatesList() {
	n := len(g.syntaxGrammar.ExternalTokens)
	for _, set := range g.parseTable.ExternalLexStates {
		row := make([]bool, n)
		for _, token := range set.Symbols() {
			if !token.IsExternal() || int(token.Index) >= n {
				panicLikeGenerator("external lex state holds %v, not one of %d external tokens", token, n)
			}
			row[token.Index] = true
		}
		g.out.ExternalScannerStates = append(g.out.ExternalScannerStates, row)
	}
}

// RenderTables is render.rs: fn render_c_code and fn Generator::generate, for
// the numbered tables of a whole build: Render with the lex functions of the
// lex tables.
func RenderTables(t *Tables, g *PreparedGrammar, supertypeSymbolMap []SupertypeSubtypes, abiVersion int) (*Layout, error) {
	if t.LexTables == nil {
		return nil, errors.New("tablebuild: RenderTables: the tables hold no lex tables")
	}
	return render(t.ParseTable, t.LexTables, g, supertypeSymbolMap, abiVersion)
}

// LexExact returns the lexexact record of the layout's lex functions: per
// lex state the accept code and the steps at the end of input and on an
// ill-formed byte, without ranges (Trans and Start nil), as for a Language
// whose lex states are converted from parser.c. It is nil when the layout
// has no lex functions.
func (l *Layout) LexExact() *lexexact.Tables {
	if l.MainLex == nil {
		return nil
	}
	f := func(states []LexState) lexexact.Func {
		out := lexexact.Func{Accept: make([]uint32, len(states)), OnEOF: make([]uint32, len(states)), OnInvalid: make([]uint32, len(states))}
		for i, s := range states {
			out.Accept[i], out.OnEOF[i], out.OnInvalid[i] = s.Accept, s.OnEOF, s.OnInvalid
		}
		return out
	}
	t := &lexexact.Tables{Version: lexexact.Version, Main: f(l.MainLex)}
	if l.KeywordLex != nil {
		t.Keyword = f(l.KeywordLex)
	}
	return t
}
