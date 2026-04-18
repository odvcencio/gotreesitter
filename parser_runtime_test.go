package gotreesitter

import (
	"testing"
	"time"
)

func TestParseRuntimeReportsAcceptedOnCompleteParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	rt := tree.ParseRuntime()

	if rt.StopReason != ParseStopAccepted {
		t.Fatalf("StopReason = %q, want %q", rt.StopReason, ParseStopAccepted)
	}
	if tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = true, want false")
	}
	if rt.TokenSourceEOFEarly {
		t.Fatal("TokenSourceEOFEarly = true, want false")
	}
	if rt.Truncated {
		t.Fatal("Truncated = true, want false")
	}
	if rt.IterationLimit <= 0 {
		t.Fatalf("IterationLimit = %d, want > 0", rt.IterationLimit)
	}
	if rt.StackDepthLimit <= 0 {
		t.Fatalf("StackDepthLimit = %d, want > 0", rt.StackDepthLimit)
	}
	if rt.NodeLimit <= 0 {
		t.Fatalf("NodeLimit = %d, want > 0", rt.NodeLimit)
	}
	if rt.Iterations <= 0 {
		t.Fatalf("Iterations = %d, want > 0", rt.Iterations)
	}
}

type eofAtZeroTokenSource struct{}

func (eofAtZeroTokenSource) Next() Token {
	return Token{
		Symbol:    0,
		StartByte: 0,
		EndByte:   0,
	}
}

type slowArithmeticTokenSource struct {
	delay  time.Duration
	tokens []Token
	idx    int
}

func (s *slowArithmeticTokenSource) Next() Token {
	time.Sleep(s.delay)
	if s.idx >= len(s.tokens) {
		return Token{Symbol: 0}
	}
	tok := s.tokens[s.idx]
	s.idx++
	return tok
}

func TestParseRuntimeReportsTokenSourceEOFEarly(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	src := []byte("1+2")

	tree, err := parser.ParseWithTokenSource(src, eofAtZeroTokenSource{})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	rt := tree.ParseRuntime()

	if rt.StopReason != ParseStopTokenSourceEOF {
		t.Fatalf("StopReason = %q, want %q", rt.StopReason, ParseStopTokenSourceEOF)
	}
	if !rt.TokenSourceEOFEarly {
		t.Fatal("TokenSourceEOFEarly = false, want true")
	}
	if rt.LastTokenEndByte != 0 {
		t.Fatalf("LastTokenEndByte = %d, want 0", rt.LastTokenEndByte)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParserCancellationFlagStopsParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	var cancelled uint32 = 1
	parser.SetCancellationFlag(&cancelled)
	if got := parser.CancellationFlag(); got != &cancelled {
		t.Fatalf("CancellationFlag() = %p, want %p", got, &cancelled)
	}

	tree := mustParse(t, parser, []byte("1+2"))
	if got, want := tree.ParseStopReason(), ParseStopCancelled; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParserTimeoutMicrosStopsParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.SetTimeoutMicros(200)
	if got := parser.TimeoutMicros(); got != 200 {
		t.Fatalf("TimeoutMicros() = %d, want 200", got)
	}

	ts := &slowArithmeticTokenSource{
		delay: 2 * time.Millisecond,
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1},
			{Symbol: 0, StartByte: 1, EndByte: 1},
		},
	}
	tree, err := parser.ParseWithTokenSource([]byte("1"), ts)
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	if got, want := tree.ParseStopReason(), ParseStopTimeout; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParserLoggerReceivesEvents(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	var parseEvents int
	var lexEvents int
	parser.SetLogger(func(kind ParserLogType, msg string) {
		if msg == "" {
			t.Fatal("logger message should not be empty")
		}
		switch kind {
		case ParserLogParse:
			parseEvents++
		case ParserLogLex:
			lexEvents++
		}
	})

	if _, err := parser.Parse([]byte("1+2")); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parseEvents == 0 {
		t.Fatal("expected at least one parse log event")
	}
	if lexEvents == 0 {
		t.Fatal("expected at least one lex log event")
	}

	// Nil logger disables logging.
	parser.SetLogger(nil)
	parseEvents = 0
	lexEvents = 0
	if _, err := parser.Parse([]byte("1+2")); err != nil {
		t.Fatalf("Parse() with nil logger error = %v", err)
	}
	if parseEvents != 0 || lexEvents != 0 {
		t.Fatalf("expected no events with nil logger, got parse=%d lex=%d", parseEvents, lexEvents)
	}
}
