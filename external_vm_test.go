package gotreesitter

import "testing"

func TestExternalVMScannerSimpleToken(t *testing.T) {
	scanner := MustNewExternalVMScanner(ExternalVMProgram{
		Code: []ExternalVMInstr{
			VMRequireValid(0, 5),
			VMIfRuneEq('#', 5),
			VMAdvance(false),
			VMMarkEnd(),
			VMEmit(Symbol(2)),
			VMFail(),
		},
	})

	payload := scanner.Create()
	lexer := newExternalLexer([]byte("#"), 0, 0, 0)

	if !scanner.Scan(payload, lexer, []bool{true}) {
		t.Fatal("expected scan success")
	}
	tok, ok := lexer.token()
	if !ok {
		t.Fatal("expected token after scan")
	}
	if tok.Symbol != Symbol(2) {
		t.Fatalf("token symbol = %d, want %d", tok.Symbol, Symbol(2))
	}
	if tok.Text != "#" {
		t.Fatalf("token text = %q, want %q", tok.Text, "#")
	}
}

func TestExternalVMScannerValidSymbolGate(t *testing.T) {
	scanner := MustNewExternalVMScanner(ExternalVMProgram{
		Code: []ExternalVMInstr{
			VMRequireValid(0, 5),
			VMIfRuneEq('#', 5),
			VMAdvance(false),
			VMMarkEnd(),
			VMEmit(Symbol(2)),
			VMFail(),
		},
	})

	payload := scanner.Create()
	lexer := newExternalLexer([]byte("#"), 0, 0, 0)
	if scanner.Scan(payload, lexer, []bool{false}) {
		t.Fatal("expected scan failure when symbol is invalid")
	}
	if _, ok := lexer.token(); ok {
		t.Fatal("expected no token on failed scan")
	}
}

func TestExternalVMScannerStateRoundTrip(t *testing.T) {
	scanner := MustNewExternalVMScanner(ExternalVMProgram{
		Code: []ExternalVMInstr{
			VMIfRuneEq('[', 5),
			VMAdvance(false),
			VMMarkEnd(),
			VMSetState(1),
			VMEmit(Symbol(10)),
			VMRequireStateEq(1, 10),
			VMIfRuneEq(']', 10),
			VMAdvance(false),
			VMMarkEnd(),
			VMEmit(Symbol(11)),
			VMFail(),
		},
	})

	openPayload := scanner.Create()
	openLexer := newExternalLexer([]byte("["), 0, 0, 0)
	if !scanner.Scan(openPayload, openLexer, nil) {
		t.Fatal("expected open token scan success")
	}
	openToken, ok := openLexer.token()
	if !ok {
		t.Fatal("expected open token")
	}
	if openToken.Symbol != Symbol(10) {
		t.Fatalf("open symbol = %d, want %d", openToken.Symbol, Symbol(10))
	}

	buf := make([]byte, 8)
	n := scanner.Serialize(openPayload, buf)
	if n != 4 {
		t.Fatalf("serialize bytes = %d, want 4", n)
	}

	closePayload := scanner.Create()
	scanner.Deserialize(closePayload, buf[:n])
	closeLexer := newExternalLexer([]byte("]"), 0, 0, 0)
	if !scanner.Scan(closePayload, closeLexer, nil) {
		t.Fatal("expected close token scan success after deserialize")
	}
	closeToken, ok := closeLexer.token()
	if !ok {
		t.Fatal("expected close token")
	}
	if closeToken.Symbol != Symbol(11) {
		t.Fatalf("close symbol = %d, want %d", closeToken.Symbol, Symbol(11))
	}

	freshPayload := scanner.Create()
	freshLexer := newExternalLexer([]byte("]"), 0, 0, 0)
	if scanner.Scan(freshPayload, freshLexer, nil) {
		t.Fatal("expected close token scan failure without restored state")
	}
}

func TestExternalVMScannerLoopGuard(t *testing.T) {
	scanner := MustNewExternalVMScanner(ExternalVMProgram{
		Code: []ExternalVMInstr{
			VMJump(0),
		},
		MaxSteps: 8,
	})

	payload := scanner.Create()
	lexer := newExternalLexer([]byte("#"), 0, 0, 0)
	if scanner.Scan(payload, lexer, []bool{true}) {
		t.Fatal("expected scan failure after hitting max steps")
	}
}

func TestExternalVMScannerInvalidProgram(t *testing.T) {
	_, err := NewExternalVMScanner(ExternalVMProgram{
		Code: []ExternalVMInstr{
			VMJump(1),
		},
	})
	if err == nil {
		t.Fatal("expected invalid jump target error")
	}
}
