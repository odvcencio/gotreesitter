package parse_test

import (
	"errors"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPublicParseWorkLimits(t *testing.T) {
	limits := gotreesitter.ParseWorkLimits{
		IterationLimit:  1,
		StackDepthLimit: 100,
		NodeLimit:       1_000,
	}
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	parser.SetParseWorkLimits(limits)
	if got := parser.ParseWorkLimits(); got != limits {
		t.Fatalf("ParseWorkLimits = %+v, want %+v", got, limits)
	}

	tree, err := parser.ParseStrict([]byte("package p\nvar x = 1\n"))
	if tree == nil {
		t.Fatal("ParseStrict returned nil partial tree")
	}
	defer tree.Release()
	var stopped *gotreesitter.ParseStoppedEarlyError
	if !errors.As(err, &stopped) {
		t.Fatalf("ParseStrict error = %v, want ParseStoppedEarlyError", err)
	}
	if stopped.Reason != gotreesitter.ParseStopIterationLimit {
		t.Fatalf("stop reason = %q, want %q", stopped.Reason, gotreesitter.ParseStopIterationLimit)
	}
	if stopped.Runtime.IterationLimit != limits.IterationLimit || stopped.Runtime.StackDepthLimit != limits.StackDepthLimit || stopped.Runtime.NodeLimit != limits.NodeLimit {
		t.Fatalf("resolved limits = iterations:%d depth:%d nodes:%d, want %+v", stopped.Runtime.IterationLimit, stopped.Runtime.StackDepthLimit, stopped.Runtime.NodeLimit, limits)
	}
}

func TestPublicParseWorkLimitsIncrementalStrict(t *testing.T) {
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	oldTree, err := parser.Parse([]byte("package p\nvar x = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte: 8, OldEndByte: 9, NewEndByte: 9,
		StartPoint:  gotreesitter.Point{Column: 8},
		OldEndPoint: gotreesitter.Point{Column: 9},
		NewEndPoint: gotreesitter.Point{Column: 9},
	})
	parser.SetParseWorkLimits(gotreesitter.ParseWorkLimits{IterationLimit: 1})
	tree, err := parser.ParseIncrementalStrict([]byte("package q\nvar x = 1\n"), oldTree)
	if tree == nil {
		t.Fatal("incremental parse returned no partial tree")
	}
	defer tree.Release()
	var stopped *gotreesitter.ParseStoppedEarlyError
	if !errors.As(err, &stopped) || stopped.Reason != gotreesitter.ParseStopIterationLimit {
		t.Fatalf("incremental error = %v, want iteration limit", err)
	}
	if tree.ParseRuntime().IterationLimit != 1 {
		t.Fatalf("incremental runtime = %s", tree.ParseRuntime().Summary())
	}
}

func TestPublicParserPoolParseWorkLimits(t *testing.T) {
	limits := gotreesitter.ParseWorkLimits{
		IterationLimit:  1,
		StackDepthLimit: 100,
		NodeLimit:       1_000,
	}
	pool := gotreesitter.NewParserPool(
		grammars.GoLanguage(),
		gotreesitter.WithParserPoolParseWorkLimits(limits),
	)

	for run := 0; run < 2; run++ {
		tree, err := pool.Parse([]byte("package p\nvar x = 1\n"))
		if err != nil {
			t.Fatalf("run %d: Parse error: %v", run, err)
		}
		if tree == nil {
			t.Fatalf("run %d: Parse returned nil partial tree", run)
		}
		runtime := tree.ParseRuntime()
		tree.Release()
		if runtime.StopReason != gotreesitter.ParseStopIterationLimit {
			t.Fatalf("run %d: StopReason = %q, want %q", run, runtime.StopReason, gotreesitter.ParseStopIterationLimit)
		}
		if runtime.IterationLimit != limits.IterationLimit || runtime.StackDepthLimit != limits.StackDepthLimit || runtime.NodeLimit != limits.NodeLimit {
			t.Fatalf("run %d: resolved limits = iterations:%d depth:%d nodes:%d, want %+v", run, runtime.IterationLimit, runtime.StackDepthLimit, runtime.NodeLimit, limits)
		}
	}
}
