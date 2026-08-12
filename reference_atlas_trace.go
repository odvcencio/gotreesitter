//go:build gts_workcount

package gotreesitter

import (
	"math"
	"strconv"
)

// DiagnosticReferenceAtlasEvent is one ordered Go event for the G2 atlas.
// It contains semantic fields only. It contains no pointer or arena identity.
type DiagnosticReferenceAtlasEvent struct {
	EventSeq        uint64 `json:"event_seq"`
	SourceStartByte uint32 `json:"source_start_byte"`
	SourceEndByte   uint32 `json:"source_end_byte"`
	ParseState      uint32 `json:"parse_state"`
	Symbol          uint32 `json:"symbol"`
	CallSite        string `json:"call_site"`
	EventKind       string `json:"event_kind"`
	Outcome         string `json:"outcome"`
}

// DiagnosticReferenceAtlasTrace is a bounded, diagnostic-only event stream.
// Aggregate counters remain the authority until both engines emit this form.
type DiagnosticReferenceAtlasTrace struct {
	Contract           string                          `json:"contract"`
	MaxEvents          uint32                          `json:"max_events"`
	EventsSeen         uint64                          `json:"events_seen"`
	EventsDropped      uint64                          `json:"events_dropped"`
	ArithmeticOverflow bool                            `json:"arithmetic_overflow"`
	Events             []DiagnosticReferenceAtlasEvent `json:"events"`
}

const (
	DiagnosticReferenceAtlasTraceContract  = "gts-reference-atlas-events/v1"
	DiagnosticReferenceAtlasTraceMaxEvents = 65536
)

var activeDiagnosticReferenceAtlasTrace *DiagnosticReferenceAtlasTrace

// BeginDiagnosticReferenceAtlasTrace starts one bounded Go event stream.
// Pair it with the semantic phase trace during parser instrumentation.
func BeginDiagnosticReferenceAtlasTrace() {
	if activeDiagnosticReferenceAtlasTrace != nil {
		panic("gotreesitter: reference-atlas trace already active")
	}
	activeDiagnosticReferenceAtlasTrace = &DiagnosticReferenceAtlasTrace{
		Contract:  DiagnosticReferenceAtlasTraceContract,
		MaxEvents: DiagnosticReferenceAtlasTraceMaxEvents,
		Events:    make([]DiagnosticReferenceAtlasEvent, 0, DiagnosticReferenceAtlasTraceMaxEvents),
	}
}

// EndDiagnosticReferenceAtlasTrace returns the event stream and disables it.
func EndDiagnosticReferenceAtlasTrace() DiagnosticReferenceAtlasTrace {
	if activeDiagnosticReferenceAtlasTrace == nil {
		panic("gotreesitter: reference-atlas trace is not active")
	}
	out := *activeDiagnosticReferenceAtlasTrace
	out.Events = append([]DiagnosticReferenceAtlasEvent(nil), out.Events...)
	activeDiagnosticReferenceAtlasTrace = nil
	return out
}

func referenceAtlasTraceAppend(event DiagnosticReferenceAtlasEvent) {
	trace := activeDiagnosticReferenceAtlasTrace
	if trace == nil {
		return
	}
	if trace.EventsSeen == math.MaxUint64 {
		trace.ArithmeticOverflow = true
		return
	}
	trace.EventsSeen++
	event.EventSeq = trace.EventsSeen
	if uint64(len(trace.Events)) < uint64(trace.MaxEvents) {
		trace.Events = append(trace.Events, event)
		return
	}
	if trace.EventsDropped == math.MaxUint64 {
		trace.ArithmeticOverflow = true
		return
	}
	trace.EventsDropped++
}

func referenceAtlasTraceRecordActionLookup(event DiagnosticSemanticPhaseEvent) {
	referenceAtlasTraceAppend(DiagnosticReferenceAtlasEvent{
		SourceStartByte: event.LookaheadStartByte,
		SourceEndByte:   event.LookaheadEndByte,
		ParseState:      event.State,
		Symbol:          event.LookaheadSymbol,
		CallSite:        referenceAtlasActionCallSite(event.Phase, event.ActionOrdinal),
		EventKind:       "action_table.lookup",
		Outcome:         event.Outcome,
	})
}

func referenceAtlasTraceRecordActionExecution(event DiagnosticSemanticPhaseEvent, action ParseAction) {
	eventKind, ok := referenceAtlasActionEventKind(action.Type)
	if !ok {
		return
	}
	symbol := event.LookaheadSymbol
	if action.Type == ParseActionReduce {
		symbol = uint32(action.Symbol)
	}
	referenceAtlasTraceAppend(DiagnosticReferenceAtlasEvent{
		SourceStartByte: event.LookaheadStartByte,
		SourceEndByte:   event.LookaheadEndByte,
		ParseState:      event.State,
		Symbol:          symbol,
		CallSite:        referenceAtlasActionCallSite(event.Phase, event.ActionOrdinal),
		EventKind:       eventKind,
		Outcome:         event.Outcome,
	})
}

func referenceAtlasActionEventKind(action ParseActionType) (string, bool) {
	switch action {
	case ParseActionShift:
		return "parser.shift", true
	case ParseActionReduce:
		return "parser.reduce", true
	case ParseActionAccept:
		return "parser.accept", true
	case ParseActionRecover:
		return "parser.recover", true
	default:
		return "", false
	}
}

func referenceAtlasActionCallSite(phase string, ordinal int16) string {
	return phase + ":action[" + strconv.FormatInt(int64(ordinal), 10) + "]"
}

func referenceAtlasTraceRecordDecision(event DiagnosticSemanticPhaseEvent, target, candidate *glrStack) {
	if event.Phase != workCountConvergencePhasePostReducePrimary &&
		event.Phase != workCountConvergencePhasePostReducePending &&
		event.Phase != workCountConvergencePhaseBoundaryGSS &&
		event.Phase != workCountConvergencePhaseBoundaryEquivalence {
		return
	}
	stack := target
	if stack == nil {
		stack = candidate
	}
	start, end, state, symbol := event.LookaheadStartByte, event.LookaheadEndByte, event.State, event.LookaheadSymbol
	if stack != nil && stack.depth() > 0 {
		entry := stack.top()
		start = stackEntryNodeStartByte(entry)
		end = stackEntryNodeEndByte(entry)
		state = uint32(entry.state)
		symbol = uint32(stackEntryNodeSymbol(entry))
	}
	referenceAtlasTraceAppend(DiagnosticReferenceAtlasEvent{
		SourceStartByte: start,
		SourceEndByte:   end,
		ParseState:      state,
		Symbol:          symbol,
		CallSite:        event.Phase,
		EventKind:       "head.merge",
		Outcome:         event.Outcome,
	})
}
