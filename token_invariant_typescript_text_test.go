package gotreesitter

import "testing"

type tokenInvariantTypeScriptDigitScanner struct{ primitiveProofSubsetScanner }

func (tokenInvariantTypeScriptDigitScanner) Scan(any, *ExternalLexer, []bool) bool { return false }

func (tokenInvariantTypeScriptDigitScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

func TestTypeScriptTextInvariantNumberRequiresDependencyProof(t *testing.T) {
	for _, name := range []string{"missing-history", "missing-scanner-equivalence", "changed-scanner-equivalence", "changed-raw-token"} {
		t.Run(name, func(t *testing.T) {
			lang := primitiveProofDigitLanguage()
			lang.Name = "typescript"
			lang.SymbolNames = []string{"ERROR", "number", "other"}
			lang.ExternalScanner = tokenInvariantTypeScriptDigitScanner{}
			span := uint32(2)
			switch name {
			case "missing-history":
				span = 0
			case "missing-scanner-equivalence":
				lang.ExternalScanner = primitiveProofSubsetScanner{}
			case "changed-scanner-equivalence":
				lang.ExternalScanner = tokenInvariantASCIIClassScanner{}
			case "changed-raw-token":
				lang.LexStates = []LexState{
					{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '1', Hi: '1', NextState: 1}, {Lo: '2', Hi: '2', NextState: 2}}},
					{Default: -1, EOF: -1, AcceptToken: 1},
					{Default: -1, EOF: -1, AcceptToken: 2},
				}
			}
			leaf := &Node{symbol: 1, flags: nodeFlagNamed, endByte: 1, endPoint: Point{Column: 1}, ownerArena: &nodeArena{}}
			if !leaf.ownerArena.externalScannerLeafCheckpointIdentityMatches(lang) {
				t.Fatal("fixture failed scanner identity admission")
			}
			edit := InputEdit{OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}}
			tree := &Tree{root: leaf, source: []byte("1"), language: lang, lastEditedLeaf: leaf,
				edits: []InputEdit{edit}, tokenInvariantReadSpan: span, compactMaterialized: true, incrementalReuseDisabled: true}
			parser := &Parser{language: lang}
			if !parser.canReuseLanguageTextInvariantNode([]byte("2"), tree, leaf, edit) {
				t.Fatal("fixture did not reach numeric text admission")
			}
			d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("2")), lang, nil, nil, nil, nil, false)
			defer d.Close()
			var timing incrementalParseTiming
			if reused, ok := parser.tryTokenInvariantLeafEdit([]byte("2"), tree, d, &timing); ok || reused != nil {
				t.Fatal("numeric text admission bypassed dependency authentication")
			}
			if name == "changed-raw-token" && timing.tokenInvariantDependencyChecks != 1 {
				t.Fatal("fixture did not reach primitive comparison")
			}
		})
	}
}

func TestTypeScriptTextInvariantNumberRejectsSyntaxAndOtherNodes(t *testing.T) {
	lang := &Language{Name: "typescript", SymbolNames: []string{"number", "identifier"}}
	leaf := &Node{flags: nodeFlagNamed, endByte: 1}
	tree := &Tree{source: []byte("1")}
	parser := &Parser{language: lang}
	edit := InputEdit{OldEndByte: 1, NewEndByte: 1}
	if parser.canReuseLanguageTextInvariantNode([]byte("+"), tree, leaf, edit) {
		t.Fatal("numeric text admission accepted punctuation")
	}
	leaf.symbol = 1
	if parser.canReuseLanguageTextInvariantNode([]byte("2"), tree, leaf, edit) {
		t.Fatal("numeric text admission accepted an identifier")
	}
	leaf.symbol = 0
	leaf.flags |= nodeFlagHasError
	if parser.canReuseLanguageTextInvariantNode([]byte("2"), tree, leaf, edit) {
		t.Fatal("numeric text admission accepted an error")
	}
}
