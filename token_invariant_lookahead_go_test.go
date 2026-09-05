//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"testing"
)

func TestTokenInvariantLookaheadGoControl(t *testing.T) {
	testTokenInvariantGoControl(t, "1", "int_literal")
}

func TestTokenInvariantLookaheadGoFloatControl(t *testing.T) {
	testTokenInvariantGoControl(t, "0e1", "float_literal")
}

func testTokenInvariantGoControl(t *testing.T, value, nodeType string) {
	for _, compact := range []bool{false, true} {
		name := "legacy"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			p := newAdmissionCandidateGoParser(t)
			p.SetAdmissionCandidateRoute(compact)
			source := []byte("package p\nfunc f() { _ = " + value + " }\n")
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			root := old.RootNode()
			if old.tokenInvariantReadSpan == 0 {
				t.Fatalf("full parse did not capture lexical coverage: stop=%s stacks=%d recovery=%t deferred=%t", old.parseRuntime.StopReason, old.parseRuntime.MaxStacksSeen, old.parseRuntime.CRecoveryEnteredErrorState, old.hasDeferredResultCompatibility())
			}
			for _, replacement := range []string{"2", "1", "2"} {
				valueStart := bytes.Index(source, []byte("_ = ")) + 4
				offset := valueStart + len(value) - 1
				edited, edit := compactExecutionEdit(source, offset, offset+1, replacement)
				old.Edit(edit)
				next, profile, err := p.ParseIncrementalProfiled(edited, old)
				if err != nil {
					t.Fatal(err)
				}
				old.Release()
				old = next
				if next.RootNode() != root || profile.ReparseNanos != 0 || profile.NewNodesAllocated != 0 {
					t.Fatalf("ordinary literal edit lost its fast path: same_root=%t profile=%+v", next.RootNode() == root, profile)
				}
				if profile.TokenInvariantDependencyChecks != 1 || next.tokenInvariantReadSpan == 0 {
					t.Fatal("ordinary literal reuse did not execute and retain its lexical proof")
				}
				if next.RootNode().HasError() || profile.ReusedBytes != uint64(len(edited)) {
					t.Fatal("ordinary literal edit lost its complete valid tree")
				}
				literal := compactExecutionNode(next.RootNode(), p.language, nodeType, uint32(valueStart))
				if literal == nil || literal.Text(edited) != string(edited[valueStart:offset+1]) {
					t.Fatal("literal text was not updated")
				}
				source = edited
			}
		})
	}
}

func TestLexerReadSpanFailedCompactRelex(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates[1].AcceptToken = 0
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("=i")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	scheduler := &diagnosticParserCoreGenericScheduler{tokenSource: d}
	shared := Token{Symbol: 1, EndByte: 1, EndPoint: Point{Column: 1}}
	if _, ok := scheduler.relexTokenForState(0, shared); ok {
		t.Fatal("failed relex unexpectedly selected a token")
	}
	if d.tokenInvariantReadSpan() != 3 || d.relexProbeLexer.tokenInvariantReadSpanMax != nil {
		t.Fatal("failed relex lost coverage or retained its aggregate owner")
	}
}
