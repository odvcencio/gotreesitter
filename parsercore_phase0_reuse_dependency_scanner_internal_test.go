//go:build gts_parsercorephase0

package gotreesitter

import "testing"

type compactZeroWidthReplayScanner struct {
	diagnosticParserCoreZeroSnapshotScanner
	proven bool
}

func (s compactZeroWidthReplayScanner) SupportsCompactZeroWidthExternalReuse() bool {
	return s.proven
}

func (s compactZeroWidthReplayScanner) SupportsCompactNestedReuseForSymbol(symbol Symbol) bool {
	return s.proven && symbol == 3
}

func TestCompactReuseDependencyZeroWidthReplayBoundary(t *testing.T) {
	for _, proven := range []bool{false, true} {
		compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
		stats, err := compact.Stats(head)
		if err != nil {
			t.Fatal(err)
		}
		scanner := compactZeroWidthReplayScanner{diagnosticParserCoreZeroSnapshotScanner{stateless: true}, proven}
		source := &dfaTokenSource{
			language: &Language{ExternalScanner: scanner}, lexer: NewLexer(nil, []byte("x")),
			hasExternalScanner: true, hasExternalSymbols: true,
		}
		s := &diagnosticParserCoreGenericScheduler{compact: compact, tokenSource: source, headers: []diagnosticParserCoreHeader{{head: head}}}
		s.reuseDependencies.ends = make([]uint32, stats.Subtrees+1)
		token := Token{Symbol: 1, ExternalScannerToken: true, lexerLookaheadEndByte: 1}
		if _, ok := s.beginCompactReuseDependency(token); ok != proven {
			t.Fatalf("mid-source zero-width proof=%t accepted=%t", proven, ok)
		}
		if proven && (s.reuseDependencies.disabled || s.reuseDependencies.frontier != 1) {
			t.Fatal("zero-width proof lost its exact source dependency")
		}
	}
}

func TestCompactNestedReuseZeroWidthResumeRow(t *testing.T) {
	for _, externalValid := range []bool{false, true} {
		p, session, node, _, _, _ := compactBorrowedMaterializationFixture(t)
		p.language.ExternalScanner = compactZeroWidthReplayScanner{diagnosticParserCoreZeroSnapshotScanner{stateless: true}, true}
		p.language.InitialState, p.language.LargeStateCount, p.language.StateCount = 1, 8, 8
		p.language.ParseTable = [][]uint16{nil, nil, nil, nil, {0, 0, 0, 7}}
		p.language.LexModes = make([]LexMode, 8)
		p.language.ExternalLexStates = [][]bool{{externalValid}}
		p.denseLimit = 8
		if !setCompactReuseDependency(node, 0) {
			t.Fatal("fixture dependency was not authenticated")
		}
		parent := session.oldTree.root
		parent.endByte = 3
		parent.setDirty(true)
		session.oldTree.root = newParentNodeInArena(node.ownerArena, 4, true, []*Node{parent}, nil, 0)
		session.oldTree.source = []byte("a x")
		session.oldTree.edits = []InputEdit{{StartByte: 2, OldEndByte: 3, NewEndByte: 3}}
		session.cursor.reset(session.oldTree, []byte("a y"), nil)
		token := Token{Symbol: 1, StartByte: 0, EndByte: 1}
		if _, ok := session.candidateState(p, node, 4, 0, token); ok == externalValid {
			t.Fatalf("resume external row valid=%t accepted=%t", externalValid, ok)
		}
	}
}

func TestCompactReuseDependencyGoProducerCapabilities(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	var source dfaTokenSource
	initDFATokenSourceWithCRecovery(&source, NewLexer(nil, []byte("func a(){_=1}")), p.language, p.lookupActionIndex, nil, nil, nil, false)
	source.noPool = true
	defer source.Close()
	if !source.compactReuseForwardDependenciesOnly() {
		t.Fatalf("Go source is not forward-only: external=%t/%t scanner=%T zero=%t/%t/%t modes=%t/%t/%t/%t/%t/%t", source.hasExternalScanner, source.hasExternalSymbols, p.language.ExternalScanner, source.hasZeroWidthTokens, source.hasZeroWidthStartAccept, source.hasZeroWidthSentinelSymbol, source.isBash, source.isBashGenerated, source.isComment, source.isFortran, source.isScheme, source.isSwift)
	}
}

func TestCompactReuseDependencyRejectsUntrackedSourceReads(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*dfaTokenSource, *Token)
	}{
		{"unproven_scanner", func(d *dfaTokenSource, _ *Token) {
			d.language.ExternalScanner = diagnosticParserCoreZeroSnapshotScanner{stateless: false}
			d.hasExternalScanner, d.hasExternalSymbols = true, true
		}},
		{"scanner_source", func(d *dfaTokenSource, _ *Token) { d.hasExternalScanner = true }},
		{"builtin_external_symbols", func(d *dfaTokenSource, _ *Token) { d.hasExternalSymbols = true }},
		{"bash", func(d *dfaTokenSource, _ *Token) { d.isBash = true }},
		{"bash_generated", func(d *dfaTokenSource, _ *Token) { d.isBashGenerated = true }},
		{"comment", func(d *dfaTokenSource, _ *Token) { d.isComment = true }},
		{"fortran", func(d *dfaTokenSource, _ *Token) { d.isFortran = true }},
		{"scheme", func(d *dfaTokenSource, _ *Token) { d.isScheme = true }},
		{"swift", func(d *dfaTokenSource, _ *Token) { d.isSwift = true }},
		{"close_angle_repair", func(d *dfaTokenSource, _ *Token) { d.language.Name = "java" }},
		{"zero_width_sentinel", func(d *dfaTokenSource, _ *Token) { d.hasZeroWidthSentinelSymbol = true }},
		{"zero_width_token", func(_ *dfaTokenSource, token *Token) { token.EndByte = token.StartByte }},
		{"unproven_token", func(_ *dfaTokenSource, token *Token) { token.setLexFlag(tokenFlagInternalDFALexed, false) }},
		{"external_token", func(_ *dfaTokenSource, token *Token) { token.ExternalScannerToken = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
			stats, err := compact.Stats(head)
			if err != nil {
				t.Fatal(err)
			}
			source := &dfaTokenSource{language: &Language{}, lexer: NewLexer(nil, []byte("a"))}
			token := Token{Symbol: 1, EndByte: 1, lexerLookaheadEndByte: 2, lexFlags: tokenFlagInternalDFALexed}
			s := &diagnosticParserCoreGenericScheduler{compact: compact, tokenSource: source, headers: []diagnosticParserCoreHeader{{head: head}}}
			s.reuseDependencies.ends = make([]uint32, stats.Subtrees+1)
			if _, ok := s.beginCompactReuseDependency(token); !ok {
				t.Fatal("raw internal DFA control was not authenticated")
			}
			source.hasZeroWidthTokens, source.hasZeroWidthStartAccept = true, true
			if _, ok := s.beginCompactReuseDependency(token); !ok {
				t.Fatal("metadata-only zero-width capability rejected a real DFA token")
			}
			test.change(source, &token)
			if _, ok := s.beginCompactReuseDependency(token); ok || !s.reuseDependencies.disabled {
				t.Fatal("untracked source reads received a dependency proof")
			}
		})
	}
}

func TestCompactReuseDependencyAllowsCertifiedForwardScanner(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	stats, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	source := &dfaTokenSource{
		language: &Language{ExternalScanner: diagnosticParserCoreZeroSnapshotScanner{stateless: true}},
		lexer:    NewLexer(nil, nil), hasExternalScanner: true, hasExternalSymbols: true,
	}
	s := &diagnosticParserCoreGenericScheduler{compact: compact, tokenSource: source, headers: []diagnosticParserCoreHeader{{head: head}}}
	s.reuseDependencies.ends = make([]uint32, stats.Subtrees+1)
	// Certified scanners may emit zero-width EOF sentinels with a real frontier.
	token := Token{Symbol: 1, ExternalScannerToken: true, lexerLookaheadEndByte: 1}
	if _, ok := s.beginCompactReuseDependency(token); !ok {
		t.Fatal("forward-only scanner contract was rejected")
	}
	source.lexer.source = []byte("x")
	if _, ok := s.beginCompactReuseDependency(token); ok {
		t.Fatal("mid-source zero-width scanner history received a dependency proof")
	}
}
