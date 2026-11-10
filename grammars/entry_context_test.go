package grammars

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
)

func TestLangEntryParseContextPreCancelled(t *testing.T) {
	for _, strict := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		entry := LangEntry{Language: func() *gts.Language { t.Fatal("loaded grammar after cancellation"); return nil }}
		var tree *gts.Tree
		var err error
		if strict {
			tree, err = entry.ParseStrict(ctx, nil)
		} else {
			tree, err = entry.ParseContext(ctx, nil)
		}
		if tree != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("tree=%v err=%v", tree, err)
		}
	}
}

// observeEntryCancelStore returns a channel that closes after the cancellation
// bridge has stored the parser's flag. A token source can wait on it without
// sleeps and without relying on goroutine scheduling. It swaps a package-level
// hook until the test ends, so use it only in tests that do not call
// t.Parallel and that run one cancellable parse.
func observeEntryCancelStore(t *testing.T) <-chan struct{} {
	t.Helper()
	applied := make(chan struct{})
	prev := entryAfterFunc
	entryAfterFunc = func(ctx context.Context, f func()) func() bool {
		return prev(ctx, func() { f(); close(applied) })
	}
	t.Cleanup(func() { entryAfterFunc = prev })
	return applied
}

type entryHookTokens struct {
	gts.TokenSource
	once sync.Once
	hook func()
}

func (s *entryHookTokens) Next() gts.Token {
	s.once.Do(s.hook)
	return s.TokenSource.Next()
}

// +case:id=entry_context_stop,desc=`Cancellation and deadlines during tokenization`,expect=`Partial tree with the context error and optional strict error`
func TestLangEntryParseContextDuringParse(t *testing.T) {
	base := entryForTest(t, "c")
	lang := base.Language()
	for _, strict := range []bool{false, true} {
		for _, deadline := range []bool{false, true} {
			name := "cancel"
			if deadline {
				name = "deadline"
			}
			if strict {
				name += "-strict"
			}
			t.Run(name, func(t *testing.T) {
				parent, cancel := context.WithCancel(context.Background())
				want := context.Canceled
				if deadline {
					cancel()
					parent, cancel = context.WithTimeout(context.Background(), time.Second)
					want = context.DeadlineExceeded
				}
				defer cancel()
				ctx := parent
				applied := observeEntryCancelStore(t)
				entered := false
				entry := *base
				entry.TokenSourceFactory = func(src []byte, _ *gts.Language) gts.TokenSource {
					return &entryHookTokens{TokenSource: base.TokenSourceFactory(src, lang), hook: func() {
						entered = true
						if !deadline {
							cancel()
						}
						<-ctx.Done()
						<-applied
					}}
				}
				src := []byte(strings.Repeat("int value;\n", 1000))
				var tree *gts.Tree
				var err error
				if strict {
					tree, err = entry.ParseStrict(ctx, src)
				} else {
					tree, err = entry.ParseContext(ctx, src)
				}
				if tree != nil {
					defer tree.Release()
				}
				if !entered {
					t.Fatal("parser did not reach token source")
				}
				if !errors.Is(err, want) {
					t.Fatalf("error=%v, want %v", err, want)
				}
				if !strict && err != want {
					t.Fatalf("non-strict stop returned %#v, want the bare context error", err)
				}
				if tree == nil || tree.ParseStopReason() != gts.ParseStopCancelled {
					t.Fatal("missing cancelled partial tree")
				}
				if errors.Is(err, gts.ErrParseStoppedEarly) != strict {
					t.Fatalf("strict=%v error=%v", strict, err)
				}
				if strict {
					var stopped *gts.ParseStoppedEarlyError
					if !errors.As(err, &stopped) || stopped.Reason != gts.ParseStopCancelled {
						t.Fatalf("lost stop diagnostics: %v", err)
					}
				}
			})
		}
	}
}

func TestLangEntryParseStrictRoutes(t *testing.T) {
	for _, name := range append([]string{"json", "python"}, entryTokenSourceLanguages...) {
		t.Run(name, func(t *testing.T) {
			entry := entryForTest(t, name)
			src := []byte(ParseSmokeSample(name))
			normal, err := entry.ParseContext(context.Background(), src)
			if normal != nil {
				defer normal.Release()
			}
			if err != nil {
				t.Fatal(err)
			}
			strict, err := entry.ParseStrict(context.Background(), src)
			if strict != nil {
				defer strict.Release()
			}
			if err != nil {
				t.Fatal(err)
			}
			lang := entry.Language()
			if normal.RootNode().SExpr(lang) != strict.RootNode().SExpr(lang) {
				t.Fatal("strict changed lexer selection")
			}
		})
	}
}

type entryEOF struct{}

func (entryEOF) Next() gts.Token { return gts.Token{} }

func TestLangEntryParseStrictEarlyEOF(t *testing.T) {
	// A minimal grammar keeps language-specific recovery from replacing the
	// deliberately truncated token stream with a fresh parse.
	lang := &gts.Language{
		Name: "entry_eof_test", SymbolCount: 3, TokenCount: 2, StateCount: 4, InitialState: 1,
		SymbolNames:    []string{"end", "x", "root"},
		SymbolMetadata: []gts.SymbolMetadata{{Name: "end"}, {Name: "x", Visible: true}, {Name: "root", Visible: true, Named: true}},
		ParseActions: []gts.ParseActionEntry{
			{},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionShift, State: 2}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionReduce, Symbol: 2, ChildCount: 1}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionAccept}}},
		},
		ParseTable: [][]uint16{{0, 0, 0}, {0, 1, 3}, {2, 0, 0}, {3, 0, 0}},
	}
	entry := LangEntry{Language: func() *gts.Language { return lang },
		TokenSourceFactory: func([]byte, *gts.Language) gts.TokenSource { return entryEOF{} }}
	for _, strict := range []bool{false, true} {
		tree, err := entry.parse(context.Background(), []byte("x"), strict)
		if tree != nil {
			defer tree.Release()
		}
		if tree == nil || !tree.ParseStoppedEarly() {
			t.Fatal("expected partial tree")
		}
		if errors.Is(err, gts.ErrParseStoppedEarly) != strict {
			t.Fatalf("strict=%v error=%v", strict, err)
		}
		if !strict && err != nil {
			t.Fatal(err)
		}
	}
}

func TestLangEntryParseCompletedBeforeCancellation(t *testing.T) {
	entry := entryForTest(t, "json")
	tree, err := entry.ParseStrict(context.Background(), []byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := entryParseResult(tree, nil, ctx)
	if got != tree || err != nil {
		t.Fatalf("late cancellation invalidated completed tree: %v", err)
	}
	sentinel := errors.New("independent failure")
	_, err = entryParseResult(nil, sentinel, ctx)
	if err != sentinel {
		t.Fatalf("late cancellation replaced error: %v", err)
	}
}

func TestLangEntryParseContextConcurrent(t *testing.T) {
	entry := entryForTest(t, "json")
	entry.Language() // Warm the grammar before concurrent calls.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if i%2 == 0 {
				cancel()
			}
			tree, err := entry.ParseStrict(ctx, []byte(`{"x":1}`))
			if tree != nil {
				defer tree.Release()
			}
			if i%2 == 0 {
				if !errors.Is(err, context.Canceled) {
					t.Errorf("cancelled parse: %v", err)
				}
			} else if err != nil || tree == nil || tree.RootNode().HasError() {
				t.Errorf("independent parse failed: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestLangEntryParseStrictSyntaxError(t *testing.T) {
	entry := entryForTest(t, "json")
	tree, err := entry.ParseStrict(context.Background(), []byte(`{"x":}`))
	if tree != nil {
		defer tree.Release()
	}
	if err != nil {
		t.Fatalf("syntax error became execution error: %v", err)
	}
	if tree == nil || !tree.RootNode().HasError() {
		t.Fatal("missing syntax error")
	}
}

// +case:id=entry_context_isolation,desc=`Cancel one active parse sharing a LangEntry`,expect=`Other parse completes independently`
func TestLangEntryParseContextIsolation(t *testing.T) {
	base := entryForTest(t, "c")
	base.Language()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := parent
	applied := observeEntryCancelStore(t)
	entered := make(chan struct{})
	done := make(chan error, 1)
	cancelledSource := []byte(strings.Repeat("int cancelled;\n", 1000))
	entry := *base
	entry.TokenSourceFactory = func(src []byte, lang *gts.Language) gts.TokenSource {
		ts := base.TokenSourceFactory(src, lang)
		if string(src) != string(cancelledSource) {
			return ts
		}
		return &entryHookTokens{TokenSource: ts, hook: func() {
			close(entered)
			<-ctx.Done()
			<-applied
		}}
	}
	go func() {
		tree, err := entry.ParseStrict(ctx, cancelledSource)
		if tree != nil {
			tree.Release()
		}
		done <- err
	}()
	defer func() {
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("active parse error: %v", err)
		}
	}()
	<-entered
	tree, err := entry.ParseStrict(context.Background(), []byte("int independent;\n"))
	if tree != nil {
		defer tree.Release()
	}
	if err != nil || tree == nil || tree.RootNode().HasError() {
		t.Fatalf("independent parse failed: %v", err)
	}
}
