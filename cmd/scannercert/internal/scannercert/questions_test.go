package scannercert

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/incr"
)

// The parser puts optional questions to a scanner by asserting the scanner's
// type, and it does so in every parse, a fresh one as well as an incremental
// one. The recorder stands in for the scanner, so each question reaches the
// recorder first. These tests check that the recorder answers each question as
// the scanner does, and as the parser reads a scanner with no opinion, and that
// a recorded parse puts the scanner the same questions as a production parse.
// probes_test.go lists the questions the root asks.

// capabilityQuestion is one optional interface that the recorder answers.
type capabilityQuestion struct {
	key    string // the name probes_test.go gives the question
	method string // the recorder method that answers it
	absent bool   // what the parser reads from a scanner that lacks the interface
	ask    func(gts.ExternalScanner) (answer, present bool)
}

// asker puts one question to a scanner as the root does: assert the optional
// interface, then call its method.
func asker[T any](call func(T) bool) func(gts.ExternalScanner) (bool, bool) {
	return func(s gts.ExternalScanner) (bool, bool) {
		q, ok := s.(T)
		return ok && call(q), ok
	}
}

// capabilityQuestions lists every question the recorder answers, apart from the
// checkpoint identity (TestRecorderKeepsACheckpointIdentity). absent is the
// value the root takes from a scanner without the interface, except that
// classifyExternalScannerQuiescence reads a false SupportsIncrementalReuse as
// a refusal; probes_test.go explains why that does not matter for a full parse.
var capabilityQuestions = []capabilityQuestion{
	{"SupportsIncrementalReuse", "SupportsIncrementalReuse", false,
		asker(func(q gts.IncrementalReuseExternalScanner) bool { return q.SupportsIncrementalReuse() })},
	{"UsesExternalScannerCheckpoints", "UsesExternalScannerCheckpoints", false,
		asker(func(q gts.CheckpointedExternalScanner) bool { return q.UsesExternalScannerCheckpoints() })},
	{"ExternalScannerIsStateless", "ExternalScannerIsStateless", false,
		asker(func(q gts.StatelessExternalScanner) bool { return q.ExternalScannerIsStateless() })},
	{"PreservesStateOnScanFailure", "PreservesStateOnScanFailure", false,
		asker(func(q gts.FailurePreservingExternalScanner) bool { return q.PreservesStateOnScanFailure() })},
	{"RetainsStateOnScanFailure", "RetainsStateOnScanFailure", false,
		asker(func(q gts.FailureStateRetainingExternalScanner) bool { return q.RetainsStateOnScanFailure() })},
	{"SupportsErrorModeExternalRetry", "SupportsErrorModeExternalRetry", true,
		asker(func(q interface{ SupportsErrorModeExternalRetry() bool }) bool {
			return q.SupportsErrorModeExternalRetry()
		})},
	{"SupportsPackedGSSVersionOrder", "SupportsPackedGSSVersionOrder", true,
		asker(func(q interface{ SupportsPackedGSSVersionOrder(int) bool }) bool {
			return q.SupportsPackedGSSVersionOrder(1)
		})},
	{"SupportsCheckpointedNonLeafReuse", "SupportsCheckpointedNonLeafReuse", true,
		asker(func(q interface{ SupportsCheckpointedNonLeafReuse() bool }) bool {
			return q.SupportsCheckpointedNonLeafReuse()
		})},
	{"type incr.StatelessReadScanner", "SupportsStatelessReadDependencies", false,
		asker(func(q incr.StatelessReadScanner) bool { return q.SupportsStatelessReadDependencies() })},
	{"type incr.LargeStatelessReadScanner", "SupportsLargeStatelessReadDependencies", false,
		asker(func(q incr.LargeStatelessReadScanner) bool { return q.SupportsLargeStatelessReadDependencies() })},
	{"type incr.CheckpointReadScanner", "SupportsCheckpointReadDependencies", false,
		asker(func(q incr.CheckpointReadScanner) bool { return q.SupportsCheckpointReadDependencies() })},
	{"type incr.LargeCheckpointReadScanner", "SupportsLargeCheckpointReadDependencies", false,
		asker(func(q incr.LargeCheckpointReadScanner) bool { return q.SupportsLargeCheckpointReadDependencies() })},
}

// answeringScanner is the mini language's hash scanner with an opinion on every
// question in capabilityQuestions, taken from yes by method name. When log is
// set, it counts the questions put to it.
type answeringScanner struct {
	hashScanner
	yes map[string]bool
	log map[string]int
}

func (s answeringScanner) answer(method string) bool {
	if s.log != nil {
		s.log[method]++
	}
	return s.yes[method]
}

func (s answeringScanner) SupportsIncrementalReuse() bool {
	return s.answer("SupportsIncrementalReuse")
}
func (s answeringScanner) UsesExternalScannerCheckpoints() bool {
	return s.answer("UsesExternalScannerCheckpoints")
}
func (s answeringScanner) ExternalScannerIsStateless() bool {
	return s.answer("ExternalScannerIsStateless")
}
func (s answeringScanner) PreservesStateOnScanFailure() bool {
	return s.answer("PreservesStateOnScanFailure")
}
func (s answeringScanner) RetainsStateOnScanFailure() bool {
	return s.answer("RetainsStateOnScanFailure")
}
func (s answeringScanner) SupportsErrorModeExternalRetry() bool {
	return s.answer("SupportsErrorModeExternalRetry")
}
func (s answeringScanner) SupportsPackedGSSVersionOrder(int) bool {
	return s.answer("SupportsPackedGSSVersionOrder")
}
func (s answeringScanner) SupportsCheckpointedNonLeafReuse() bool {
	return s.answer("SupportsCheckpointedNonLeafReuse")
}
func (s answeringScanner) SupportsStatelessReadDependencies() bool {
	return s.answer("SupportsStatelessReadDependencies")
}
func (s answeringScanner) SupportsLargeStatelessReadDependencies() bool {
	return s.answer("SupportsLargeStatelessReadDependencies")
}
func (s answeringScanner) SupportsCheckpointReadDependencies() bool {
	return s.answer("SupportsCheckpointReadDependencies")
}
func (s answeringScanner) SupportsLargeCheckpointReadDependencies() bool {
	return s.answer("SupportsLargeCheckpointReadDependencies")
}

// With every other question answered the opposite way, an answer that the
// recorder takes from the wrong method shows as a difference.
func TestRecorderAnswersEveryQuestionAsTheScannerDoes(t *testing.T) {
	for _, q := range capabilityQuestions {
		for _, value := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s is %v", q.method, value), func(t *testing.T) {
				yes := map[string]bool{}
				for _, other := range capabilityQuestions {
					yes[other.method] = !value
				}
				yes[q.method] = value
				scanner := answeringScanner{yes: yes}
				recorded := newRecorder(scanner).install()
				for _, other := range capabilityQuestions {
					want, _ := other.ask(scanner)
					got, present := other.ask(recorded)
					switch {
					case !present:
						t.Errorf("%s: the recorder has no such method, so the parser sees a scanner without the interface", other.method)
					case got != want:
						t.Errorf("%s: recorder answers %v, scanner answers %v", other.method, got, want)
					}
				}
			})
		}
	}
}

// Where the scanner lacks an interface, the parser takes a fixed answer from
// its absence. The recorder always has the methods, so it must answer that
// value itself.
func TestRecorderAnswersAsTheParserReadsAScannerWithNoOpinion(t *testing.T) {
	recorded := newRecorder(counterScanner{}).install()
	for _, q := range capabilityQuestions {
		if _, present := q.ask(counterScanner{}); present {
			t.Fatalf("counterScanner answers %s; the test needs a scanner with no opinion", q.method)
		}
		got, present := q.ask(recorded)
		switch {
		case !present:
			t.Errorf("%s: the recorder has no such method, so the parser sees a scanner without the interface", q.method)
		case got != q.absent:
			t.Errorf("%s: recorder answers %v for a scanner with no opinion, want %v", q.method, got, q.absent)
		}
	}
	if _, ok := recorded.(gts.ExternalScannerCheckpointIdentityProvider); ok {
		t.Error("the recorder gives a scanner without a checkpoint identity one")
	}
}

// orderScanner answers the packed GSS order question from the version count the
// parser gives it.
type orderScanner struct {
	echoScanner
	upTo int
}

func (s orderScanner) SupportsPackedGSSVersionOrder(n int) bool { return n <= s.upTo }

func TestRecorderPassesTheVersionCountToThePackedOrderQuestion(t *testing.T) {
	got := newRecorder(orderScanner{upTo: 10}).install().(interface{ SupportsPackedGSSVersionOrder(int) bool })
	if !got.SupportsPackedGSSVersionOrder(10) || got.SupportsPackedGSSVersionOrder(11) {
		t.Fatal("the recorder does not give the scanner the version count it was asked about")
	}
}

// questionsAsked parses source with scanner, once as the production parser does
// and once through a recorder, and returns the questions each parse put to the
// scanner, counted by method name. The scanner's log must be set.
func questionsAsked(t *testing.T, scanner answeringScanner, source string) (production, recorded map[string]int) {
	t.Helper()
	sources := [][]byte{[]byte(source)}
	if _, err := parseAll(miniLanguage(scanner), sources, nil, nil); err != nil {
		t.Fatal(err)
	}
	production = maps.Clone(scanner.log)
	clear(scanner.log)

	rec := newRecorder(scanner)
	lang := miniLanguage(nil)
	lang.ExternalScanner = rec.install()
	if _, err := parseAll(lang, sources, rec.begin, nil); err != nil {
		t.Fatal(err)
	}
	return production, maps.Clone(scanner.log)
}

func listAsked(asked map[string]int) []string {
	var out []string
	for method, n := range asked {
		out = append(out, fmt.Sprintf("%s x%d", method, n))
	}
	sort.Strings(out)
	return out
}

// Every parse asks about read certificates, to decide whether to record read
// history and charge it to the parse's memory budget (beginLegacyReuseReads).
// A tree digest cannot show that a recorded parse skipped that work, so the
// recorded parse must put the scanner the questions the production parse puts.
func TestRecordedParseAsksTheScannerWhatTheProductionParseAsks(t *testing.T) {
	large := "1#2" + strings.Repeat(" ", sort.Search(1<<24, incr.RequiresFreshResult))
	for _, tc := range []struct {
		name   string
		yes    map[string]bool
		source string
		asks   []string // questions the production parse must put, so the case tests what it says
	}{
		{"stateless read certificate", map[string]bool{"SupportsStatelessReadDependencies": true}, "1#2",
			[]string{"SupportsStatelessReadDependencies"}},
		{"checkpoint read certificate",
			map[string]bool{"UsesExternalScannerCheckpoints": true, "SupportsCheckpointReadDependencies": true, "SupportsCheckpointedNonLeafReuse": true},
			"1#2", []string{"SupportsCheckpointReadDependencies", "SupportsCheckpointedNonLeafReuse"}},
		{"checkpoint reads with composite reuse declined",
			map[string]bool{"UsesExternalScannerCheckpoints": true, "SupportsCheckpointReadDependencies": true},
			"1#2", []string{"SupportsCheckpointReadDependencies", "SupportsCheckpointedNonLeafReuse"}},
		{"large read certificates", nil, large,
			[]string{"SupportsLargeStatelessReadDependencies", "SupportsLargeCheckpointReadDependencies"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			production, recorded := questionsAsked(t, answeringScanner{yes: tc.yes, log: map[string]int{}}, tc.source)
			for _, method := range tc.asks {
				if production[method] == 0 {
					t.Fatalf("the production parse never asks %s: %q; the case no longer tests it", method, listAsked(production))
				}
			}
			if !maps.Equal(production, recorded) {
				t.Fatalf("production parse asked %q, recorded parse asked %q, want the same questions", listAsked(production), listAsked(recorded))
			}
		})
	}
}

// A question the recorder is tested to answer must be one that probes_test.go
// lists as mirrored. TestRecorderImplementsEveryMirroredProbe checks the other
// direction.
func TestEveryCapabilityQuestionIsAMirroredProbe(t *testing.T) {
	for _, q := range capabilityQuestions {
		if _, ok := mirroredProbes[q.key]; !ok {
			t.Errorf("%s is tested as a recorder answer but is not listed in mirroredProbes", q.key)
		}
	}
}
