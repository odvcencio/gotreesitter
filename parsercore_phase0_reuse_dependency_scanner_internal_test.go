//go:build gts_parsercorephase0

package gotreesitter

import (
	"os"
	"strings"
	"testing"
)

func TestCompactLegacyReadScratchPooledLargeThenSmall(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	lang := p.language
	// Start with an independent pool so this test proves the size transition.
	lang.compactRunnerPool.Take()
	small, err := os.ReadFile("internal/benchfixtures/testdata/real/go")
	if err != nil {
		t.Fatal(err)
	}
	large := []byte("package p\nvar values = []int{" + strings.Repeat("1,", 2400) + "}\n")
	type observed struct {
		leaves, leafCapacity, ends, endCapacity int
		readBytes                               int64
	}
	parse := func(source []byte) (*Tree, observed, *parserCoreFreshFullRunner) {
		t.Helper()
		runner, borrowed, err := p.borrowAdmissionCandidateRunner()
		if err != nil {
			t.Fatal(err)
		}
		defer p.returnAdmissionCandidateRunner(runner, borrowed)
		tree, ok, reason := p.tryCompactFullParseRoute(source)
		if !ok || tree == nil || tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
			t.Fatalf("compact parse bytes=%d failed: %s", len(source), reason)
		}
		d := &runner.scheduler.reuseDependencies
		if d.reads == nil {
			t.Fatal("compact parse did not capture lexer reads")
		}
		return tree, observed{len(d.leafWords), cap(d.leafWords), len(d.ends), cap(d.ends), d.reads.Bytes()}, runner
	}
	checkIdle := func(runner *parserCoreFreshFullRunner) {
		t.Helper()
		d := &runner.scheduler.reuseDependencies
		bytes := int64(cap(d.ends)+cap(d.leafWords)) * 4
		if d.idleReads != nil {
			bytes += d.idleReads.Bytes()
			if d.idleReads.Recording() || d.idleReads.SourceBytes() != 0 {
				t.Fatal("idle pool retained authorized read history")
			}
		}
		if bytes > 64*1024 || d.reads != nil || d.readsAllocated != 0 || runner.scheduler.tokenSource != nil {
			t.Fatalf("idle pool retained active references or %d dependency bytes", bytes)
		}
	}
	largeTree, largeDemand, runner := parse(large)
	largeTree.Release()
	checkIdle(runner)
	if cap(runner.scheduler.reuseDependencies.leafWords) != compactReuseDependencyRetainedEntries {
		t.Fatalf("large fixture did not fill the shared allowance: active=%+v retained leaves=%d", largeDemand, cap(runner.scheduler.reuseDependencies.leafWords))
	}
	first, firstDemand, sameRunner := parse(small)
	defer first.Release()
	wantTree := first.RootNode().SExpr(lang)
	checkIdle(sameRunner)
	if sameRunner != runner || firstDemand.leafCapacity != largeDemand.leafCapacity || firstDemand.leaves*2 >= firstDemand.leafCapacity {
		t.Fatalf("small parse did not exercise oversized pooled capacity: large=%+v small=%+v", largeDemand, firstDemand)
	}
	if cap(runner.scheduler.reuseDependencies.leafWords) != 0 || runner.scheduler.reuseDependencies.idleReads == nil {
		t.Fatal("small parse kept oversized provenance at the expense of read history")
	}
	readScratch := runner.scheduler.reuseDependencies.idleReads
	second, secondDemand, sameRunner := parse(small)
	second.Release()
	checkIdle(sameRunner)
	if sameRunner != runner || runner.scheduler.reuseDependencies.idleReads != readScratch ||
		secondDemand.leafCapacity >= firstDemand.leafCapacity || secondDemand.readBytes != firstDemand.readBytes {
		t.Fatalf("subsequent small parse did not retain right-sized history: first=%+v second=%+v", firstDemand, secondDemand)
	}
	parseSmall := func() {
		tree, ok, reason := p.tryCompactFullParseRoute(small)
		if !ok {
			t.Fatal(reason)
		}
		tree.Release()
	}
	warm := testing.AllocsPerRun(3, parseSmall)
	cold := testing.AllocsPerRun(3, func() {
		lang.compactRunnerPool.Inspect(func(value any) {
			value.(*parserCoreFreshFullRunner).scheduler.reuseDependencies.idleReads = nil
		})
		parseSmall()
	})
	if cold <= warm {
		t.Fatalf("retained history did not reduce allocations: warm=%g cold=%g", warm, cold)
	}
	if got := first.RootNode().SExpr(lang); got != wantTree {
		t.Fatal("reused scan history changed a live tree")
	}
	t.Logf("large=%+v first small=%+v second small=%+v; warm=%g cold-history=%g allocations", largeDemand, firstDemand, secondDemand, warm, cold)
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
