package gotreesitter

import "testing"

func TestLexerUnlexableErrorLeafProvenance(t *testing.T) {
	language := buildArithmeticLanguage()
	source := []byte("\x01\x02+1")
	lexer := NewLexer(language.LexStates, source)
	lexer.hasErrorRunLexState = true
	lexer.errorRunLexState = 0
	direct := lexer.NextWithErrorRuns(0)
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(language.LexStates, source), language, nil, nil, nil, nil, true)
	defer d.Close()
	d.SetParserState(1)
	fromSource := d.Next()
	if fromSource.lexerErrorModeLexed {
		t.Fatal("source fixture relies on the existing error-mode provenance")
	}
	if direct.Symbol != errorSymbol || direct.StartByte != 0 || direct.EndByte != 2 {
		t.Fatalf("internal error-run fixture: %+v", direct)
	}
	if fromSource.Symbol != errorSymbol || fromSource.StartByte != direct.StartByte || fromSource.EndByte != direct.EndByte {
		t.Fatalf("DFA source did not preserve the error run: %+v", fromSource)
	}
	custom := Token{Symbol: direct.Symbol, Text: direct.Text, StartByte: direct.StartByte, EndByte: direct.EndByte, StartPoint: direct.StartPoint, EndPoint: direct.EndPoint}
	external := direct
	external.ExternalScannerToken = true
	missing := direct
	missing.Missing = true
	noLookahead := direct
	noLookahead.NoLookahead = true
	externalLexer := newExternalLexer(source, 0, 0, 0)
	scanner := errorProvenanceExternalScanner{}
	if !scanner.Scan(nil, externalLexer, []bool{true}) {
		t.Fatal("checkpointless scanner did not emit its error token")
	}
	actualExternal, ok := externalLexer.token()
	if !ok || actualExternal.Symbol != errorSymbol || actualExternal.lexerErrorModeLexed || actualExternal.lexerErrorRunLexed {
		t.Fatal("checkpointless scanner acquired internal lexer provenance")
	}
	for _, test := range []struct {
		name          string
		token         Token
		wantLeafError bool
	}{
		{"internal lexer", direct, false},
		{"internal DFA source", fromSource, false},
		{"custom public token", custom, true},
		{"external token", external, true},
		{"checkpointless scanner output", actualExternal, true},
		{"missing token", missing, true},
		{"no-lookahead token", noLookahead, true},
		{"synthetic internal symbol", Token{Symbol: errorSymbol, EndByte: 2, EndPoint: Point{Column: 2}, lexerInternalDFALexed: true}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := NewParser(language)
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			wrapper := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
			wrapper.setHasError(true)
			stack := newGLRStack(1)
			stack.pushEntry(newStackEntryNode(cErrorState, wrapper), nil, nil)
			stack.cRec = &cRecoverState{group: &cRecGroup{}, openErr: wrapper, groupOrder: cPackRecoverGroupOrder(0, true)}
			parser.cAbsorbTokenIntoError(&stack, test.token, nil, arena, nil, nil, nil)
			if !wrapper.HasError() || !wrapper.IsError() || wrapper.ChildCount() != 1 {
				t.Fatalf("error wrapper flags/children changed: has=%t is=%t children=%d", wrapper.HasError(), wrapper.IsError(), wrapper.ChildCount())
			}
			leaf := wrapper.Child(0)
			if leaf.StartByte() != test.token.StartByte || leaf.EndByte() != test.token.EndByte || leaf.StartPoint() != test.token.StartPoint || leaf.EndPoint() != test.token.EndPoint || leaf.IsMissing() {
				t.Fatalf("absorbed error token lost its range or missing flag: %+v", test.token)
			}
			if leaf.HasError() != test.wantLeafError || !leaf.IsError() {
				t.Fatalf("leaf has_error=%t is_error=%t, want has_error=%t and is_error=true", leaf.HasError(), leaf.IsError(), test.wantLeafError)
			}
		})
	}
}

type errorProvenanceExternalScanner struct{}

func (errorProvenanceExternalScanner) Create() any               { return nil }
func (errorProvenanceExternalScanner) Destroy(any)               {}
func (errorProvenanceExternalScanner) Serialize(any, []byte) int { return 0 }
func (errorProvenanceExternalScanner) Deserialize(any, []byte)   {}
func (errorProvenanceExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	lexer.Advance(false)
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(errorSymbol)
	return true
}
