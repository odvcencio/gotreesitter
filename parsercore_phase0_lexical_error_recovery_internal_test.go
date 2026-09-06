//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type lexicalRecoveryFaultTable struct {
	recoveryLineageForkTable
	failure error
}

func (table lexicalRecoveryFaultTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	if symbol == 1 && state == 2 {
		return core.NewActionRow([]core.Action{{Type: core.ActionReduce, Symbol: 5, ChildCount: 1}}, false), nil
	}
	if symbol == 1 && state == 3 {
		return core.ActionRow{}, table.failure
	}
	return table.recoveryLineageForkTable.Actions(state, symbol)
}

func TestLexicalErrorRecoveryRollsBackReductionFailure(t *testing.T) {
	failure := errors.New("lexical recovery reduction fault")
	s := newRecoveryLineageForkSchedulerWithTable(t, lexicalRecoveryFaultTable{failure: failure}, true)
	head, err := s.compact.Shift(s.headers[0].head, 2, 0, core.Token{Symbol: 2, StartByte: 1, EndByte: 2}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	s.headers[0].head = head
	s.token = Token{Symbol: errorSymbol, StartByte: 2, EndByte: 3}
	s.tokenSource.lexer.source = []byte("aa?")
	s.receipt = &DiagnosticParserCoreGenericScheduler{Tokens: 7}
	s.verifierHeaderPtr = &s.headers[0]
	s.verifierBound = len(s.headers)
	original := s.headers[0]
	before, err := s.compact.Stats(original.head)
	if err != nil {
		t.Fatal(err)
	}
	work := s.compact.Work()
	receipt := s.receipt
	handled, err := s.tryLexicalErrorRecovery(0)
	if handled || !errors.Is(err, failure) {
		t.Fatalf("recovery handled=%t err=%v, want reduction failure", handled, err)
	}
	requireS5ForkRollback(t, s, original, before, work, receipt)
}

func TestLexicalErrorRecoveryEntryGuardsPreserveState(t *testing.T) {
	cases := []struct {
		name   string
		change func(*diagnosticParserCoreGenericScheduler)
	}{
		{"disabled", func(s *diagnosticParserCoreGenericScheduler) { s.options.Recovery = false }},
		{"real_token", func(s *diagnosticParserCoreGenericScheduler) { s.token.Symbol = 4 }},
		{"empty_token", func(s *diagnosticParserCoreGenericScheduler) { s.token.EndByte = s.token.StartByte }},
		{"missing_token", func(s *diagnosticParserCoreGenericScheduler) { s.token.Missing = true }},
		{"no_lookahead", func(s *diagnosticParserCoreGenericScheduler) { s.token.NoLookahead = true }},
		{"leading_gap", func(s *diagnosticParserCoreGenericScheduler) { s.token.StartByte = 0 }},
		{"prior_region", func(s *diagnosticParserCoreGenericScheduler) { s.s3RegionOpened = true }},
		{"prior_resume", func(s *diagnosticParserCoreGenericScheduler) { s.s3ResumeCount = 1 }},
		{"empty_table", func(s *diagnosticParserCoreGenericScheduler) { s.tokenSource.language.TokenCount = 1 }},
		{"error_is_terminal", func(s *diagnosticParserCoreGenericScheduler) {
			s.tokenSource.language.TokenCount = uint32(errorSymbol) + 1
		}},
		{"multiple_headers", func(s *diagnosticParserCoreGenericScheduler) { s.headers = append(s.headers, s.headers[0]) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			s := newRecoveryLineageForkScheduler(t, true)
			s.token.Symbol = errorSymbol
			test.change(s)
			headers := append([]diagnosticParserCoreHeader(nil), s.headers...)
			before, err := s.compact.Stats(s.headers[0].head)
			if err != nil {
				t.Fatal(err)
			}
			work := s.compact.Work()
			handled, err := s.tryLexicalErrorRecovery(0)
			after, statsErr := s.compact.Stats(s.headers[0].head)
			if err != nil || handled || statsErr != nil || before != after || work != s.compact.Work() || !reflect.DeepEqual(headers, s.headers) {
				t.Fatalf("guard changed state: handled=%t err=%v stats=%v", handled, err, statsErr)
			}
		})
	}
}

func TestLexicalErrorRecoverySummaryKeepsFirstEligibleState(t *testing.T) {
	for _, test := range []struct {
		name      string
		lookahead Symbol
		want      bool
	}{
		{"first_state", 2, true},
		{"later_state", 4, false},
		{"no_state", 99, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newRecoveryLineageForkScheduler(t, true)
			other, err := s.compact.Seed(3, 1)
			if err != nil {
				t.Fatal(err)
			}
			frontier := []diagnosticParserCoreHeader{s.headers[0], {head: other}}
			var absorb diagnosticParserCoreHeader
			err = s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
				var err error
				absorb, err = s.s5AppendAndMergeAbsorberOwned(owner, frontier, 0, 0, &diagnosticParserCoreS5Work{})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			s.headers = []diagnosticParserCoreHeader{absorb}
			got, err := s.lexicalErrorResumeMatchesSummary(absorb, test.lookahead)
			if err != nil || got != test.want {
				t.Fatalf("summary matches=%t want=%t err=%v", got, test.want, err)
			}
		})
	}
}
