//go:build gts_workcount

package gotreesitter

import (
	"testing"
)

func TestReferenceAtlasTraceBoundsAndSequences(t *testing.T) {
	BeginDiagnosticReferenceAtlasTrace()
	for i := 0; i < DiagnosticReferenceAtlasTraceMaxEvents+2; i++ {
		referenceAtlasTraceAppend(DiagnosticReferenceAtlasEvent{
			SourceStartByte: uint32(i),
			SourceEndByte:   uint32(i),
			ParseState:      7,
			Symbol:          9,
			CallSite:        "test",
			EventKind:       "parser.shift",
			Outcome:         "dispatched",
		})
	}
	trace := EndDiagnosticReferenceAtlasTrace()
	if trace.Contract != DiagnosticReferenceAtlasTraceContract || trace.MaxEvents != DiagnosticReferenceAtlasTraceMaxEvents {
		t.Fatalf("trace contract=%q max=%d", trace.Contract, trace.MaxEvents)
	}
	if trace.EventsSeen != uint64(DiagnosticReferenceAtlasTraceMaxEvents)+2 || trace.EventsDropped != 2 || trace.ArithmeticOverflow {
		t.Fatalf("trace bounds: seen=%d dropped=%d overflow=%v", trace.EventsSeen, trace.EventsDropped, trace.ArithmeticOverflow)
	}
	if len(trace.Events) != DiagnosticReferenceAtlasTraceMaxEvents {
		t.Fatalf("retained events=%d, want %d", len(trace.Events), DiagnosticReferenceAtlasTraceMaxEvents)
	}
	for index, event := range trace.Events {
		if event.EventSeq != uint64(index+1) {
			t.Fatalf("event %d sequence=%d", index, event.EventSeq)
		}
	}
}

func TestReferenceAtlasTraceMapsOrderedGoEvents(t *testing.T) {
	BeginDiagnosticReferenceAtlasTrace()
	lookup := DiagnosticSemanticPhaseEvent{
		LookaheadStartByte: 4,
		LookaheadEndByte:   7,
		State:              11,
		LookaheadSymbol:    13,
		ActionOrdinal:      0,
		Phase:              "action_cell",
		Outcome:            "candidate",
	}
	referenceAtlasTraceRecordActionLookup(lookup)
	referenceAtlasTraceRecordActionExecution(lookup, ParseAction{Type: ParseActionShift})
	referenceAtlasTraceRecordActionExecution(lookup, ParseAction{Type: ParseActionReduce, Symbol: 17})
	referenceAtlasTraceRecordActionExecution(lookup, ParseAction{Type: ParseActionAccept})
	referenceAtlasTraceRecordActionExecution(lookup, ParseAction{Type: ParseActionRecover})
	referenceAtlasTraceRecordDecision(DiagnosticSemanticPhaseEvent{
		LookaheadStartByte: 8,
		LookaheadEndByte:   9,
		State:              19,
		LookaheadSymbol:    23,
		Phase:              workCountConvergencePhaseBoundaryGSS,
		Outcome:            workCountConvergenceOutcomePacked,
	}, nil, nil)
	trace := EndDiagnosticReferenceAtlasTrace()

	wantKinds := []string{
		"action_table.lookup",
		"parser.shift",
		"parser.reduce",
		"parser.accept",
		"parser.recover",
		"head.merge",
	}
	if len(trace.Events) != len(wantKinds) {
		t.Fatalf("event count=%d want=%d", len(trace.Events), len(wantKinds))
	}
	for index, event := range trace.Events {
		if event.EventSeq != uint64(index+1) || event.EventKind != wantKinds[index] || event.CallSite == "" {
			t.Fatalf("event %d=%+v want kind=%q", index, event, wantKinds[index])
		}
		if event.SourceEndByte < event.SourceStartByte {
			t.Fatalf("event %d has reversed span: %+v", index, event)
		}
	}
	if trace.Events[2].Symbol != 17 {
		t.Fatalf("reduce symbol=%d want=17", trace.Events[2].Symbol)
	}
}
