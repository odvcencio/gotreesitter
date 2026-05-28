package gotreesitter

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

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

func TestParserPoolParseWithTokenSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	src := []byte("1+2+3")

	seedParser := NewParser(lang)
	lexer := NewLexer(lang.LexStates, src)
	ts := acquireDFATokenSource(lexer, lang, seedParser.lookupActionIndex, seedParser.hasKeywordState, seedParser.externalValidByState)

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
