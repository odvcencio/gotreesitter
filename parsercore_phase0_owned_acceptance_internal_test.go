//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func compactGoMidfileMissingOperandSource() []byte {
	var clean, source bytes.Buffer
	for _, out := range []*bytes.Buffer{&clean, &source} {
		out.WriteString("package main\n\nimport \"fmt\"\n\n")
	}
	for i := 0; clean.Len() < 24<<10; i++ {
		fmt.Fprintf(&clean, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
		if i == 7 {
			fmt.Fprintf(&source, "func f%d(a int, b int) int {\n\tx := a +\n\treturn x\n}\n\n", i)
		} else {
			fmt.Fprintf(&source, "func f%d(a int, b int) int {\n\tx := a + b\n\treturn x\n}\n\n", i)
		}
	}
	return source.Bytes()
}

func TestOwnedOrdinaryEOFAcceptanceRequiresLiveProof(t *testing.T) {
	source := compactGoMidfileMissingOperandSource()
	p := newCompactRecoveryVersionTurnGoParser(t)
	runner, err := newAdmissionCandidateRunner(p)
	if err != nil {
		t.Fatal(err)
	}
	s, tokenSource, err := runner.executeSchedulerOpen(source, runner.compact, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tokenSource.Close()
	if !s.recoveryTurns.active || s.work.RecoverEOFAccepts != 0 || !s.hasOwnedOrdinaryEOFAcceptance(source) {
		t.Fatalf("fixture did not reach owned ordinary EOF acceptance: active=%t eof=%d accepts=%d owned=%t", s.recoveryTurns.active, s.work.RecoverEOFAccepts, s.work.Accepts, s.versionLexerOwnershipActive)
	}
	for _, name := range []string{
		"clean", "shared_lexer", "no_accept_action", "no_receipt", "no_acceptance",
		"unaccepted_header", "wrong_head", "invalid_request", "stale_election",
		"non_eof", "missing", "no_lookahead", "wrong_offset",
		"receipt_token", "receipt_election", "receipt_state", "receipt_position", "receipt_creation", "receipt_count",
	} {
		t.Run(name, func(t *testing.T) {
			saved := *s
			headers := append([]diagnosticParserCoreHeader(nil), s.headers...)
			requests := append([]diagnosticParserCoreVersionLexerRequest(nil), s.versionLexerRequests...)
			receipt := *s.receipt
			acceptance := *s.receipt.Acceptance
			s.receipt.Acceptance = &acceptance
			defer func() {
				*s = saved
				copy(s.headers, headers)
				copy(s.versionLexerRequests, requests)
				*s.receipt = receipt
			}()
			index := -1
			for i := range s.headers {
				if s.headers[i].accepted && s.headers[i].head == s.acceptedHead {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatal("accepted header is absent")
			}
			request := s.versionLexerRequestForHeader(index)
			if request == nil {
				t.Fatal("accepted header has no owned request")
			}
			switch name {
			case "receipt_token":
				acceptance.Token.Symbol = 1
			case "receipt_election":
				acceptance.ElectionIndex--
			case "receipt_state":
				acceptance.Header.Header.State++
			case "receipt_position":
				acceptance.Header.Header.ByteOffset--
			case "receipt_creation":
				acceptance.Header.Header.CreationSeq++
			case "receipt_count":
				acceptance.Accepts++
			case "shared_lexer":
				s.versionLexerOwnershipActive = false
			case "no_accept_action":
				s.work.Accepts = 0
			case "no_receipt":
				s.receipt = nil
			case "no_acceptance":
				s.receipt.Acceptance = nil
			case "unaccepted_header":
				s.headers[index].accepted = false
			case "wrong_head":
				s.acceptedHead.Node++
			case "invalid_request":
				request.valid = false
			case "stale_election":
				request.electionIndex--
			case "non_eof":
				request.token.Symbol = 1
			case "missing":
				request.token.Missing = true
			case "no_lookahead":
				request.token.NoLookahead = true
			case "wrong_offset":
				request.token.StartByte--
			}
			if got := s.hasOwnedOrdinaryEOFAcceptance(source); got != (name == "clean") {
				t.Fatalf("owned ordinary EOF proof=%t", got)
			}
			if name != "clean" {
				if tree, err := runner.materializeSelection(source, nil, s); tree != nil || err == nil ||
					!strings.Contains(err.Error(), "requires an executed EOF turn") {
					t.Fatalf("invalid proof reached materialization: tree=%v err=%v", tree, err)
				}
			}
		})
	}
}
