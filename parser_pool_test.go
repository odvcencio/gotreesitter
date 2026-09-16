package gotreesitter

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestParserPoolReleaseScrubsPendingStackBuffers(t *testing.T) {
	pool := NewParserPool(buildArithmeticLanguage())
	parser := pool.checkout()

	small := make([]glrStack, 1, maxRetainedPendingStackCap)
	small[0].gss.head = &gssNode{}
	large := make([]glrStack, 1, maxRetainedPendingStackCap+1)
	large[0].gss.head = &gssNode{}
	parser.pendingForkStacks = small
	parser.pendingFrontierForkStacks = large

	pool.release(parser)

	if parser.pendingForkStacks == nil || len(parser.pendingForkStacks) != 0 || cap(parser.pendingForkStacks) != cap(small) {
		t.Fatalf("pooled small pending buffer = len %d, cap %d, want len 0, cap %d", len(parser.pendingForkStacks), cap(parser.pendingForkStacks), cap(small))
	}
	if parser.pendingForkStacks[:cap(parser.pendingForkStacks)][0].gss.head != nil {
		t.Fatal("pooled small pending buffer retained a stack reference")
	}
	if parser.pendingFrontierForkStacks != nil {
		t.Fatalf("pooled oversized pending buffer cap = %d, want nil", cap(parser.pendingFrontierForkStacks))
	}
	if parser.forestDeclineMemo != nil {
		t.Fatal("pool release allocated the cold sidecar")
	}
}

func TestCRecoverSummaryArenaResetsAtParseBoundary(t *testing.T) {
	parser := &Parser{}
	scratch := acquireParserScratch()
	entries := recoverySummaryTestEntries(1)
	if _, reason := parser.cRecordSummaryWithScratch(entries, &scratch.gss, nil); reason != ParseStopNone {
		t.Fatalf("summary stopped: %s", reason)
	}
	if len(scratch.gss.summaryChunks) == 0 || scratch.gss.summaryChunks[0].used == 0 {
		t.Fatal("summary arena did not allocate before release")
	}
	releaseParserScratch(scratch, false)
	reacquired := acquireParserScratch()
	defer releaseParserScratch(reacquired, false)
	for _, chunk := range reacquired.gss.summaryChunks {
		if chunk.used != 0 {
			t.Fatal("summary chunk retained live slots after parser-scratch release")
		}
	}
}

func TestParserPoolReleaseScrubsColdPendingStackReserves(t *testing.T) {
	pool := NewParserPool(buildArithmeticLanguage())
	parser := pool.checkout()
	cold := parser.ensureParserColdState()
	forkReserve := make([]glrStack, 1, maxRetainedPendingStackCap)
	forkReserve[0].gss.head = &gssNode{}
	cold.pendingForkStackReserve = forkReserve[:0]
	parser.pendingForkStacks = cold.pendingForkStackReserve
	frontierReserve := make([]glrStack, 1, maxRetainedPendingStackCap)
	frontierReserve[0].gss.head = &gssNode{}
	cold.pendingFrontierForkStackReserve = frontierReserve[:0]
	frontierBacking := make([]glrStack, 1, maxRetainedPendingStackCap+1)
	frontierHead := &gssNode{}
	frontierBacking[0].gss.head = frontierHead
	parser.pendingFrontierForkStacks = frontierBacking[:0]

	pool.release(parser)

	if frontierBacking[0].gss.head != frontierHead {
		t.Fatal("pool release scanned the oversized active buffer")
	}
	if got := cap(parser.pendingForkStacks); got != maxRetainedPendingStackCap {
		t.Fatalf("pooled pending fork capacity = %d, want %d", got, maxRetainedPendingStackCap)
	}
	if got := cap(parser.pendingFrontierForkStacks); got != maxRetainedPendingStackCap {
		t.Fatalf("pooled pending frontier capacity = %d, want %d", got, maxRetainedPendingStackCap)
	}
	if forkReserve[0].gss.head != nil || frontierReserve[0].gss.head != nil {
		t.Fatal("pool release retained stack references in the cold reserves")
	}
	if cold.pendingForkStackReserve[:cap(cold.pendingForkStackReserve)][0].gss.head != nil ||
		cold.pendingFrontierForkStackReserve[:cap(cold.pendingFrontierForkStackReserve)][0].gss.head != nil {
		t.Fatal("cold reserve state retained stack references")
	}

	reacquired := pool.checkout()
	if len(reacquired.pendingForkStacks) != 0 || len(reacquired.pendingFrontierForkStacks) != 0 {
		t.Fatal("reacquired parser retained pending stacks")
	}
	if cap(reacquired.pendingForkStacks) > maxRetainedPendingStackCap ||
		cap(reacquired.pendingFrontierForkStacks) > maxRetainedPendingStackCap {
		t.Fatal("reacquired parser retained oversized pending capacity")
	}
	pool.release(reacquired)
}

func TestReleaseSnippetParserScrubsPendingStackBuffers(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())

	small := make([]glrStack, 1, maxRetainedPendingStackCap)
	small[0].gss.head = &gssNode{}
	large := make([]glrStack, 1, maxRetainedPendingStackCap+1)
	large[0].gss.head = &gssNode{}
	parser.pendingForkStacks = small
	parser.pendingFrontierForkStacks = large

	releaseSnippetParser(parser)

	if parser.pendingForkStacks == nil || len(parser.pendingForkStacks) != 0 || cap(parser.pendingForkStacks) != cap(small) {
		t.Fatalf("snippet small pending buffer = len %d, cap %d, want len 0, cap %d", len(parser.pendingForkStacks), cap(parser.pendingForkStacks), cap(small))
	}
	if parser.pendingForkStacks[:cap(parser.pendingForkStacks)][0].gss.head != nil {
		t.Fatal("snippet small pending buffer retained a stack reference")
	}
	if parser.pendingFrontierForkStacks != nil {
		t.Fatalf("snippet oversized pending buffer cap = %d, want nil", cap(parser.pendingFrontierForkStacks))
	}
	if parser.forestDeclineMemo != nil {
		t.Fatal("snippet release allocated the cold sidecar")
	}
}

func TestParserPoolParseConcurrent(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	src := []byte("1 + 2 + 3 + 4")

	const workers = 16
	const iters = 64

	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				tree, err := pool.Parse(src)
				if err != nil {
					errs <- err
					return
				}
				if tree == nil || tree.RootNode() == nil {
					errs <- fmt.Errorf("nil parse tree")
					return
				}
				if tree.RootNode().HasError() {
					errs <- fmt.Errorf("unexpected parse error: %s", tree.RootNode().SExpr(lang))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent parse failed: %v", err)
	}
}

func TestParserPoolAppliesParseWorkLimits(t *testing.T) {
	limits := ParseWorkLimits{
		IterationLimit:  1,
		StackDepthLimit: 100,
		NodeLimit:       1_000,
	}
	pool := NewParserPool(buildArithmeticLanguage(), WithParserPoolParseWorkLimits(limits))

	for run := 0; run < 2; run++ {
		tree, err := pool.Parse([]byte("1+2+3+4"))
		if err != nil {
			t.Fatalf("run %d: Parse error: %v", run, err)
		}
		if tree == nil {
			t.Fatalf("run %d: Parse returned nil tree", run)
		}
		runtime := tree.ParseRuntime()
		tree.Release()
		if runtime.StopReason != ParseStopIterationLimit {
			t.Fatalf("run %d: StopReason = %q, want %q (%s)", run, runtime.StopReason, ParseStopIterationLimit, runtime.Summary())
		}
		if runtime.IterationLimit != limits.IterationLimit || runtime.StackDepthLimit != limits.StackDepthLimit || runtime.NodeLimit != limits.NodeLimit {
			t.Fatalf("run %d: resolved limits = iterations:%d depth:%d nodes:%d, want %+v", run, runtime.IterationLimit, runtime.StackDepthLimit, runtime.NodeLimit, limits)
		}
	}

	parser := pool.checkout()
	if got := parser.ParseWorkLimits(); got != limits {
		t.Fatalf("checked-out ParseWorkLimits = %+v, want %+v", got, limits)
	}
	parser.SetParseWorkLimits(ParseWorkLimits{NodeLimit: 7})
	pool.release(parser)
	parser = pool.checkout()
	if got := parser.ParseWorkLimits(); got != limits {
		t.Fatalf("reused ParseWorkLimits = %+v, want %+v", got, limits)
	}
	pool.release(parser)
}

func TestParserPoolParseWithTokenSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	src := []byte("1+2+3")

	seedParser := NewParser(lang)
	lexer := NewLexer(lang.LexStates, src)
	ts := acquireDFATokenSource(lexer, lang, seedParser.lookupActionIndex, seedParser.hasKeywordState, seedParser.externalValidByState, seedParser.externalValidMaskByState)

	tree, err := pool.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("ParseWithTokenSource returned nil tree")
	}
	if tree.RootNode().HasError() {
		t.Fatalf("expected parse without errors, got: %s", tree.RootNode().SExpr(lang))
	}
}

func TestParserPoolParseUTF16Conveniences(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	source := utf16Units("1+2+3")

	tree, err := pool.ParseUTF16(source)
	if err != nil {
		t.Fatalf("ParseUTF16 failed: %v", err)
	}
	if got, want := tree.SourceEncoding(), InputEncodingUTF16; got != want {
		t.Fatalf("SourceEncoding = %s, want %s", got, want)
	}
	if got, want := tree.RootNode().Text(tree.Source()), "1+2+3"; got != want {
		t.Fatalf("ParseUTF16 root text = %q, want %q", got, want)
	}

	byteTree, err := pool.ParseUTF16Bytes(utf16BytesForTest(t, "1+2+3", UTF16LittleEndian), UTF16LittleEndian)
	if err != nil {
		t.Fatalf("ParseUTF16Bytes failed: %v", err)
	}
	if got, want := byteTree.RootNode().SExpr(lang), tree.RootNode().SExpr(lang); got != want {
		t.Fatalf("ParseUTF16Bytes SExpr = %q, want %q", got, want)
	}
}

func TestParserPoolParseUTF16TokenSourceFactoryConveniences(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	seedParser := NewParser(lang)
	factory := dfaTokenSourceFactoryForTest(t, seedParser)

	oldSource := utf16Units("1+2")
	oldTree, err := pool.ParseUTF16WithTokenSourceFactory(oldSource, factory)
	if err != nil {
		t.Fatalf("ParseUTF16WithTokenSourceFactory failed: %v", err)
	}

	newSource := utf16Units("1+3")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}
	incrTree, err := pool.ParseIncrementalUTF16WithTokenSourceFactory(newSource, oldTree, factory)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16WithTokenSourceFactory failed: %v", err)
	}
	if got, want := incrTree.RootNode().Text(incrTree.Source()), "1+3"; got != want {
		t.Fatalf("incremental UTF16 factory root text = %q, want %q", got, want)
	}

	byteTree, err := pool.ParseUTF16BytesWithTokenSourceFactory(utf16BytesForTest(t, "1+3", UTF16BigEndian), UTF16BigEndian, factory)
	if err != nil {
		t.Fatalf("ParseUTF16BytesWithTokenSourceFactory failed: %v", err)
	}
	if got, want := byteTree.RootNode().SExpr(lang), incrTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("ParseUTF16BytesWithTokenSourceFactory SExpr = %q, want %q", got, want)
	}

	if ok := incrTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, utf16Units("1+4")); !ok {
		t.Fatal("second EditUTF16 returned false")
	}
	byteIncrTree, err := pool.ParseIncrementalUTF16BytesWithTokenSourceFactory(utf16BytesForTest(t, "1+4", UTF16BigEndian), incrTree, UTF16BigEndian, factory)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16BytesWithTokenSourceFactory failed: %v", err)
	}
	if got, want := byteIncrTree.RootNode().Text(byteIncrTree.Source()), "1+4"; got != want {
		t.Fatalf("incremental UTF16 byte factory root text = %q, want %q", got, want)
	}
}

func TestParserPoolAppliesLoggerOption(t *testing.T) {
	lang := buildArithmeticLanguage()
	var logCount atomic.Int64
	pool := NewParserPool(
		lang,
		WithParserPoolLogger(func(kind ParserLogType, msg string) {
			logCount.Add(1)
		}),
	)

	if _, err := pool.Parse([]byte("1+2")); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if logCount.Load() == 0 {
		t.Fatal("expected parser logger to receive at least one log")
	}
}
