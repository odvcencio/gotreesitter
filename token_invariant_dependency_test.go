package gotreesitter

import (
	"sync"
	"testing"
)

type tokenInvariantASCIIClassScanner struct{ primitiveProofSubsetScanner }

func (tokenInvariantASCIIClassScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	switch b {
	case 'a', 'b':
		return 7
	case '1':
		return 1
	case '2':
		return 2
	case 255:
		// The consumer must reject non-ASCII bytes even from a faulty scanner.
		return 7
	default:
		return 0
	}
}

func TestTokenInvariantScannerASCIIClassProof(t *testing.T) {
	for _, tc := range []struct {
		old, next string
		want      bool
	}{
		{"a", "b", true},
		{"ab", "ba", true},
		{"1", "2", false},
		{"a!", "b!", false},
		{"a", "\xff", false},
		{"\xff", "a", false},
		{"", "", false},
		{"ab", "a", false},
	} {
		edit := InputEdit{OldEndByte: uint32(len(tc.old)), NewEndByte: uint32(len(tc.next))}
		got := tokenInvariantScannerASCIIEditEquivalent(tokenInvariantASCIIClassScanner{}, []byte(tc.old), []byte(tc.next), edit)
		if got != tc.want {
			t.Fatalf("%q -> %q: class proof=%t, want %t", tc.old, tc.next, got, tc.want)
		}
	}
	if tokenInvariantScannerASCIIEditEquivalent(primitiveProofSubsetScanner{}, []byte("a"), []byte("b"), InputEdit{OldEndByte: 1, NewEndByte: 1}) {
		t.Fatal("scanner without a class contract supplied a substitution proof")
	}
	for _, edit := range []InputEdit{
		{StartByte: 2, OldEndByte: 1, NewEndByte: 1},
		{StartByte: 1, OldEndByte: 2, NewEndByte: 2},
		{OldEndByte: 2, NewEndByte: 2},
	} {
		if tokenInvariantScannerASCIIEditEquivalent(tokenInvariantASCIIClassScanner{}, []byte("a"), []byte("b"), edit) {
			t.Fatalf("invalid edit supplied a class proof: %+v", edit)
		}
	}
}

func TestTokenInvariantContextualPrimitivesDoNotEnableSubtreeReuse(t *testing.T) {
	for _, name := range []string{"typescript", "tsx", "java"} {
		t.Run(name, func(t *testing.T) {
			d := &dfaTokenSource{language: &Language{Name: name}}
			if !d.tokenInvariantInternalPrimitivesSupported() {
				t.Fatal("contextual language cannot compare its raw lexer primitives")
			}
			if d.tokenInvariantInternalDependenciesForwardOnly() || d.compactReuseForwardDependenciesOnly() {
				t.Fatal("raw primitive support enabled unauthenticated subtree reuse")
			}
		})
	}
}

func TestTokenInvariantDeferredHistoryPublication(t *testing.T) {
	lang := readSpanTestLanguage()
	source := []byte("=")
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil, false)
	d.lexer.Next(0)
	tree := NewTree(&Node{symbol: 1, endByte: 1}, source, lang)
	defer tree.Release()
	tree.parseRuntime.StopReason = ParseStopAccepted
	tree.deferResultCompatibility()
	tree.captureTokenInvariantReadSpan(d)
	d.Close()
	if tree.tokenInvariantReadSpan != 0 {
		t.Fatal("deferred result published history before normalization")
	}
	var wg sync.WaitGroup
	var spans [8]uint32
	for i := range spans {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tree.RootNode()
			spans[i] = tree.tokenInvariantReadSpan
		}()
	}
	wg.Wait()
	for _, span := range spans {
		if span != 2 {
			t.Fatalf("normalized result lost its closed source history: span=%d", span)
		}
	}
	copy := tree.Copy()
	defer copy.Release()
	if copy.tokenInvariantReadSpan != 2 || copy.hasDeferredResultCompatibility() {
		t.Fatal("Copy lost finalized history or retained the finalizer")
	}
}

func TestTokenInvariantDeferredHistoryRejectsChangedResult(t *testing.T) {
	for _, invalidSource := range []bool{false, true} {
		lang := readSpanTestLanguage()
		source := []byte("=")
		d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil, false)
		d.lexer.Next(0)
		tree := NewTree(&Node{symbol: 1, endByte: 1}, source, lang)
		tree.parseRuntime.StopReason = ParseStopAccepted
		tree.deferResultCompatibility()
		tree.captureTokenInvariantReadSpan(d)
		d.Close()
		if invalidSource {
			// A replacement attempt cannot retain the previous source's receipt.
			tree.captureTokenInvariantReadSpan(nil)
		} else {
			tree.parseRuntime.Truncated = true
		}
		_ = tree.RootNode()
		if tree.tokenInvariantReadSpan != 0 {
			t.Fatal("unknown source or incomplete result published lexical history")
		}
		tree.Release()
	}
}

func TestTokenInvariantDependencyCaptureAndLifecycle(t *testing.T) {
	lang := readSpanTestLanguage()
	source := []byte("=")
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil, false)
	defer d.Close()
	d.lexer.Next(0)
	tree := NewTree(&Node{symbol: 1, endByte: 1, endPoint: Point{Column: 1}}, source, lang)
	tree.parseRuntime = ParseRuntime{StopReason: ParseStopAccepted, MaxStacksSeen: 1}
	tree.captureTokenInvariantReadSpan(d)
	if tree.tokenInvariantReadSpan != 2 {
		t.Fatalf("captured span=%d, want token plus EOF probe", tree.tokenInvariantReadSpan)
	}
	copy := tree.Copy()
	defer copy.Release()
	if copy.tokenInvariantReadSpan != 2 || copy.root == tree.root {
		t.Fatal("Copy lost independent nodes or lexical coverage")
	}
	reused := reuseTreeWithNewSource(copy, source, copy.root, false)
	if reused.tokenInvariantReadSpan != 2 {
		t.Fatal("whole-tree reuse lost lexical coverage")
	}
	reused.Release()
	if reused.tokenInvariantReadSpan != 0 {
		t.Fatal("Release retained lexical coverage")
	}
	scratch := &Tree{tokenInvariantReadSpan: 123}
	resetTreeForReuse(scratch, nil, nil, nil, nil, nil)
	if scratch.tokenInvariantReadSpan != 0 {
		t.Fatal("pool reset retained lexical coverage")
	}
}

func TestTokenInvariantDependencyCaptureDeclinesIncompleteHistory(t *testing.T) {
	lang := readSpanTestLanguage()
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("=")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	d.lexer.Next(0)
	for _, tc := range []struct {
		name  string
		alter func(*Tree)
	}{
		{"error", func(tree *Tree) { tree.root.flags |= nodeFlagHasError }},
		{"stopped", func(tree *Tree) { tree.parseRuntime.StopReason = ParseStopNoStacksAlive }},
		{"truncated", func(tree *Tree) { tree.parseRuntime.Truncated = true }},
		{"recovery", func(tree *Tree) { tree.parseRuntime.CRecoveryEnteredErrorState = true }},
		{"ranges", func(tree *Tree) { tree.includedRanges = []Range{{EndByte: 1}} }},
		{"deferred", func(tree *Tree) { tree.resultCompatibilityFinalizer = &treeResultCompatibilityFinalizer{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := NewTree(&Node{symbol: 1, endByte: 1}, []byte("="), lang)
			tree.parseRuntime = ParseRuntime{StopReason: ParseStopAccepted, MaxStacksSeen: 1}
			tree.tokenInvariantReadSpan = 123
			tc.alter(tree)
			tree.captureTokenInvariantReadSpan(d)
			if tree.tokenInvariantReadSpan != 0 {
				t.Fatal("incomplete history retained lexical coverage")
			}
		})
	}
}

func TestTokenInvariantDependencyRangeWrapperCapture(t *testing.T) {
	lang := readSpanTestLanguage()
	source := []byte("=")
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil, false)
	defer d.Close()
	ranges := []Range{{EndByte: 1, EndPoint: Point{Column: 1}}}
	wrapped := newIncludedRangeTokenSource(d, ranges)
	wrapped.Next()
	tree := NewTree(&Node{symbol: 1, endByte: 1}, source, lang)
	tree.parseRuntime.StopReason = ParseStopAccepted
	tree.includedRanges = ranges
	tree.captureTokenInvariantReadSpan(wrapped)
	if tree.tokenInvariantReadSpan != 2 || tokenInvariantDFASource(wrapped, ranges) != d {
		t.Fatal("known range filtering lost the underlying read history")
	}
	if tokenInvariantDFASource(wrapped, nil) != nil || tokenInvariantDFASource(d, ranges) != nil {
		t.Fatal("source composition or range mismatch supplied a dependency proof")
	}
	tree.Release()
}
