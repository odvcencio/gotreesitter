package gotreesitter_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// incrementalEditStep replaces source[start:oldEnd] with text.
type incrementalEditStep struct {
	start, oldEnd uint32
	text          string
}

// applyIncrementalEditSteps records each step on tree and returns the final
// source. Each step uses the coordinates of the source before that step.
func applyIncrementalEditSteps(tree *gts.Tree, source []byte, steps []incrementalEditStep) []byte {
	current := append([]byte(nil), source...)
	for _, step := range steps {
		next := make([]byte, 0, len(current)-int(step.oldEnd-step.start)+len(step.text))
		next = append(next, current[:step.start]...)
		next = append(next, step.text...)
		next = append(next, current[step.oldEnd:]...)
		newEnd := step.start + uint32(len(step.text))
		tree.Edit(gts.InputEdit{
			StartByte:   step.start,
			OldEndByte:  step.oldEnd,
			NewEndByte:  newEnd,
			StartPoint:  pointForOffset(current, int(step.start)),
			OldEndPoint: pointForOffset(current, int(step.oldEnd)),
			NewEndPoint: pointForOffset(next, int(newEnd)),
		})
		current = next
	}
	return current
}

func requireIncrementalMatchesFresh(t *testing.T, lang *gts.Language, incremental *gts.Tree, source []byte, label string) {
	t.Helper()
	fresh, err := gts.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("%s: fresh parse: %v", label, err)
	}
	defer fresh.Release()
	if d := incrGateFirstDivergence(lang, fresh.RootNode(), incremental.RootNode(), nil); d != nil {
		t.Fatalf("%s: incremental tree differs from fresh parse: %s %s at %s (%s)\nfresh: %s\nincr:  %s",
			label, d.kind, d.nodeType, d.path, d.detail, fresh.RootNode().SExpr(lang), incremental.RootNode().SExpr(lang))
	}
}

// TestParseIncrementalDeleteThenReinsertMatchesFresh checks an edit sequence
// whose net effect is empty. The first edit collapses a leaf. The second edit
// does not restore the leaf span, so the old tree must not be reused whole.
func TestParseIncrementalDeleteThenReinsertMatchesFresh(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lang   *gts.Language
		source string
		steps  []incrementalEditStep
	}{
		{"json", grammars.JsonLanguage(), "[10, 20, 30]", []incrementalEditStep{{5, 7, ""}, {5, 5, "20"}}},
		{"go", grammars.GoLanguage(), "package p\n\nvar a = 10\nvar b = 20\n", []incrementalEditStep{{30, 32, ""}, {30, 30, "20"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := gts.NewParser(tc.lang)
			source := []byte(tc.source)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer old.Release()
			edited := applyIncrementalEditSteps(old, source, tc.steps)
			if string(edited) != tc.source {
				t.Fatalf("edit steps changed the source: %q", edited)
			}
			tree, err := parser.ParseIncremental(edited, old)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer tree.Release()
			requireIncrementalMatchesFresh(t, tc.lang, tree, edited, tc.name)
		})
	}
}

// TestParseIncrementalTypeThenDeleteKeepsUndoReuse checks the lossless undo
// case. A removal that exactly cancels an insertion keeps whole-tree reuse.
func TestParseIncrementalTypeThenDeleteKeepsUndoReuse(t *testing.T) {
	lang := grammars.JsonLanguage()
	source := []byte("[10, 20, 30]")
	parser := gts.NewParser(lang)
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer old.Release()
	edited := applyIncrementalEditSteps(old, source, []incrementalEditStep{{6, 6, "5"}, {6, 7, ""}})
	tree, profile, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer tree.Release()
	requireIncrementalMatchesFresh(t, lang, tree, edited, "type then delete")
	if profile.ReusedBytes == 0 {
		t.Fatalf("type then delete reused no bytes; profile=%+v", profile)
	}
}

// TestParseIncrementalRandomEditSequencesMatchFresh applies short edit
// sequences before one reparse. It compares each incremental tree with a
// fresh parse of the same text. It checks only clean fresh trees, because
// error recovery can differ between routes.
func TestParseIncrementalRandomEditSequencesMatchFresh(t *testing.T) {
	cases := []struct {
		name     string
		lang     *gts.Language
		source   string
		alphabet string
	}{
		{"json", grammars.JsonLanguage(), "[10, [20, 30], {\"k\": 40}, 50]", "0123456789"},
		{"go", grammars.GoLanguage(), "package p\n\nvar alpha = 10\nvar beta = alpha + 20\n\nfunc f() int { return beta }\n", "abcxyz0123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(20260919))
			source := []byte(tc.source)
			parser := gts.NewParser(tc.lang)
			fresh := gts.NewParser(tc.lang)
			checked := 0
			for iter := 0; iter < 300; iter++ {
				steps, edited := randomIncrementalEditSequence(rng, source, tc.alphabet)
				freshTree, err := fresh.Parse(edited)
				if err != nil {
					t.Fatalf("fresh parse: %v", err)
				}
				clean := freshTree.RootNode() != nil && !freshTree.RootNode().HasError() &&
					int(freshTree.RootNode().EndByte()) == len(edited)
				freshTree.Release()
				if !clean {
					continue
				}
				old, err := parser.Parse(source)
				if err != nil {
					t.Fatalf("initial parse: %v", err)
				}
				applyIncrementalEditSteps(old, source, steps)
				tree, err := parser.ParseIncremental(edited, old)
				if err != nil {
					t.Fatalf("incremental parse: %v", err)
				}
				requireIncrementalMatchesFresh(t, tc.lang, tree, edited, fmt.Sprintf("iteration %d steps %+v", iter, steps))
				tree.Release()
				old.Release()
				checked++
			}
			if checked < 50 {
				t.Fatalf("only %d clean edit sequences were checked; want at least 50", checked)
			}
		})
	}
}

// randomIncrementalEditSequence builds two or three small edits. Half of the
// sequences end with a step that restores the original text.
func randomIncrementalEditSequence(rng *rand.Rand, source []byte, alphabet string) ([]incrementalEditStep, []byte) {
	current := append([]byte(nil), source...)
	var steps []incrementalEditStep
	apply := func(step incrementalEditStep) {
		next := append([]byte(nil), current[:step.start]...)
		next = append(next, step.text...)
		next = append(next, current[step.oldEnd:]...)
		current = next
		steps = append(steps, step)
	}
	randomText := func() string {
		n := 1 + rng.Intn(2)
		b := make([]byte, n)
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return string(b)
	}
	count := 2 + rng.Intn(2)
	for i := 0; i < count; i++ {
		start := uint32(rng.Intn(len(current) + 1))
		oldEnd := start
		if rng.Intn(2) == 0 && int(start) < len(current) {
			oldEnd = start + uint32(1+rng.Intn(min(3, len(current)-int(start))))
		}
		text := ""
		if oldEnd == start || rng.Intn(2) == 0 {
			text = randomText()
		}
		apply(incrementalEditStep{start, oldEnd, text})
	}
	if rng.Intn(2) == 0 {
		// Restore the original text with one replacement of the changed middle.
		prefix := 0
		for prefix < len(current) && prefix < len(source) && current[prefix] == source[prefix] {
			prefix++
		}
		suffix := 0
		for suffix < len(current)-prefix && suffix < len(source)-prefix &&
			current[len(current)-1-suffix] == source[len(source)-1-suffix] {
			suffix++
		}
		apply(incrementalEditStep{uint32(prefix), uint32(len(current) - suffix), string(source[prefix : len(source)-suffix])})
	}
	return steps, current
}

// TestParseIncrementalIncludedRangeChangeMatchesFresh checks that a changed
// included range forces the parser to read the new ranges. The source does
// not change.
func TestParseIncrementalIncludedRangeChangeMatchesFresh(t *testing.T) {
	lang := grammars.CLanguage()
	source := []byte("int a;\nint b;\n")
	first := []gts.Range{{StartByte: 0, EndByte: 7, EndPoint: gts.Point{Row: 1}}}
	full := []gts.Range{{StartByte: 0, EndByte: uint32(len(source)), EndPoint: gts.Point{Row: 2}}}

	parser := gts.NewParser(lang)
	parser.SetIncludedRanges(first)
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer old.Release()
	parser.SetIncludedRanges(full)
	for _, entry := range []struct {
		name  string
		parse func() (*gts.Tree, error)
	}{
		{"ParseIncremental", func() (*gts.Tree, error) { return parser.ParseIncremental(source, old) }},
		{"ParseIncrementalProfiled", func() (*gts.Tree, error) {
			tree, _, err := parser.ParseIncrementalProfiled(source, old)
			return tree, err
		}},
	} {
		tree, err := entry.parse()
		if err != nil {
			t.Fatalf("%s: %v", entry.name, err)
		}
		fresh := gts.NewParser(lang)
		fresh.SetIncludedRanges(full)
		want, err := fresh.Parse(source)
		if err != nil {
			t.Fatalf("fresh parse: %v", err)
		}
		if d := incrGateFirstDivergence(lang, want.RootNode(), tree.RootNode(), nil); d != nil {
			t.Fatalf("%s: tree ignores the new included range: %s at %s (%s)\nfresh: %s\nincr:  %s",
				entry.name, d.kind, d.path, d.detail, want.RootNode().SExpr(lang), tree.RootNode().SExpr(lang))
		}
		want.Release()
		tree.Release()
	}
}

func TestIncrementalFreshVerificationDoesNotRetryAgain(t *testing.T) {
	before := []byte("t\nfrom s import(eg,)")
	after := []byte("t\nfrom s import(e(,)")
	lang := grammars.DetectLanguageByName("python").Language()
	for _, candidate := range []bool{false, true} {
		for _, profiled := range []bool{false, true} {
			t.Run(fmt.Sprintf("candidate=%t/profiled=%t", candidate, profiled), func(t *testing.T) {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(candidate)
				old, err := p.Parse(before)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				if old.RootNode().HasError() {
					t.Fatal("original must parse cleanly")
				}
				old.Edit(gts.InputEdit{StartByte: 17, OldEndByte: 18, NewEndByte: 18, StartPoint: gts.Point{Row: 1, Column: 15}, OldEndPoint: gts.Point{Row: 1, Column: 16}, NewEndPoint: gts.Point{Row: 1, Column: 16}})
				var next *gts.Tree
				var profile gts.IncrementalParseProfile
				if profiled {
					next, profile, err = p.ParseIncrementalProfiled(after, old)
				} else {
					next, err = p.ParseIncremental(after, old)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer next.Release()
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(candidate)
				fresh, err := freshParser.Parse(after)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 {
					t.Fatalf("incremental=%s fresh=%s", next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
				}
				if !next.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(after)) {
					t.Fatal("replacement must report the syntax error and cover the input")
				}
				allocs := testing.AllocsPerRun(5, func() {
					same, err := p.ParseIncremental(after, next)
					if err != nil {
						t.Fatal(err)
					}
					same.Release()
				})
				if allocs != 0 {
					t.Fatalf("no-edit allocations=%g", allocs)
				}
				if profiled {
					t.Logf("tokens=%d nodes=%d reused_bytes=%d fallback=%s", profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedBytes, profile.ReuseUnsupportedReason)
				}
			})
		}
	}
}

func TestIncrementalFullRetryMatchesFreshTerminalErrorFlags(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "0")
	lang := *grammars.TypescriptLanguage()
	lang.AutomaticForestEnabledByDefault = false
	before := []byte("xxtx=y1xxxxxx\nxF()y)yx;x")
	for _, candidate := range []bool{false, true} {
		for _, profiled := range []bool{false, true} {
			t.Run(fmt.Sprintf("candidate=%t/profiled=%t", candidate, profiled), func(t *testing.T) {
				p := gts.NewParser(&lang)
				p.SetAdmissionCandidateRoute(candidate)
				old, err := p.Parse(before)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				after := applyIncrementalEditSteps(old, before, []incrementalEditStep{{22, 23, "x"}})
				var next *gts.Tree
				if profiled {
					next, _, err = p.ParseIncrementalProfiled(after, old)
				} else {
					next, err = p.ParseIncremental(after, old)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer next.Release()
				freshParser := gts.NewParser(&lang)
				freshParser.SetAdmissionCandidateRoute(candidate)
				fresh, err := freshParser.Parse(after)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				if d := incrGateFirstDivergence(&lang, fresh.RootNode(), next.RootNode(), nil); d != nil {
					t.Fatalf("incremental differs from fresh: %s at %s (%s)", d.kind, d.path, d.detail)
				}
				if !next.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(after)) {
					t.Fatal("recovery must report errors and cover the input")
				}
				if allocs := testing.AllocsPerRun(5, func() {
					same, err := p.ParseIncremental(after, next)
					if err != nil {
						t.Fatal(err)
					}
					same.Release()
				}); allocs != 0 {
					t.Fatalf("no-edit allocations=%g", allocs)
				}
			})
		}
	}
}

// TestInjectionParseEmptySpanDoesNotReuseTree reaches an empty injection span
// through InjectionParser.Parse: a missing ";" in C source has zero width. The
// injected tree must not be reused by a plain parser given the document source;
// the incremental parse must equal a fresh parse.
func TestInjectionParseEmptySpanDoesNotReuseTree(t *testing.T) {
	parentLang := grammars.CLanguage()
	childLang := grammars.PythonLanguage()

	ip := gts.NewInjectionParser()
	ip.RegisterLanguage("c", parentLang)
	ip.RegisterLanguage("python", childLang)
	if err := ip.RegisterInjectionQuery("c",
		`((MISSING ";") @injection.content (#set! injection.language "python"))`); err != nil {
		t.Fatal(err)
	}
	source := []byte("int a\n")
	result, err := ip.Parse(source, "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}
	inj := result.Injections[0]
	if len(inj.Ranges) != 1 || inj.Ranges[0].StartByte != inj.Ranges[0].EndByte {
		t.Fatalf("injection ranges = %+v, want one empty range", inj.Ranges)
	}
	if inj.Tree == nil {
		t.Fatal("injection has no tree")
	}

	again, err := gts.NewParser(childLang).ParseIncremental(inj.Tree.Source(), inj.Tree)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := gts.NewParser(childLang).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if again.RootNode() == inj.Tree.RootNode() {
		t.Fatal("ParseIncremental returned the empty injected tree unchanged")
	}
	if a, f := again.RootNode(), fresh.RootNode(); a.StartByte() != f.StartByte() || a.EndByte() != f.EndByte() || a.Type(childLang) != f.Type(childLang) {
		t.Fatalf("ParseIncremental root %s [%d,%d), fresh parse root %s [%d,%d)",
			a.Type(childLang), a.StartByte(), a.EndByte(), f.Type(childLang), f.StartByte(), f.EndByte())
	}
}

// retryStopPythonSource is a small malformed Python input. Without a timeout it
// runs four error-recovery retry passes: initial_merge, clean_wide,
// recovery_wide_or_node and final_merge.
const retryStopPythonSource = "def f(:\n  x = [1,2\n  return )\nclass :\n"

// retryStopFallbackSource runs retry passes even with C-style recovery off,
// which is how the swallowed-error fallback parses.
const retryStopFallbackSource = "int f( { int a = ; return ) }\nstruct { int x }\n"

type retryStopObservation struct {
	rungs       []string
	stop        gts.ParseStopReason
	retryStop   gts.ParseStopReason
	runtimeStop gts.ParseStopReason
	sexpr       string
}

// retryStopRecorder records the rungs that p runs and, after the pass named
// endAfter, expires the deadline (or sets the cancellation flag) of the parser
// that ran the pass.
type retryStopRecorder struct {
	mu       sync.Mutex
	rungs    []string
	endAfter string
	end      func(p *gts.Parser)
}

func (r *retryStopRecorder) observe(p *gts.Parser, rung string) {
	r.mu.Lock()
	r.rungs = append(r.rungs, rung)
	end := r.end
	hit := rung == r.endAfter
	r.mu.Unlock()
	if hit && end != nil {
		end(p)
	}
}

func (r *retryStopRecorder) passes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, rung := range r.rungs {
		if rung != "first_pass" {
			out = append(out, rung)
		}
	}
	return out
}

func observeTree(t *testing.T, lang *gts.Language, tree *gts.Tree, err error) retryStopObservation {
	t.Helper()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	return retryStopObservation{
		stop:        tree.ParseStopReason(),
		retryStop:   tree.RetryStopReason(),
		runtimeStop: tree.ParseRuntime().RetryStopReason,
		sexpr:       tree.RootNode().SExpr(lang),
	}
}

func newRetryStopParser(lang *gts.Language, rec *retryStopRecorder) *gts.Parser {
	p := gts.NewParser(lang)
	p.SetTimeoutMicros(60_000_000)
	gts.SetRetryPassObserverForTests(p, rec.observe)
	return p
}

func TestRetryStopReasonTimeoutDuringRetryPasses(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	src := []byte(retryStopPythonSource)

	baseRec := &retryStopRecorder{}
	base := gts.NewParser(lang)
	gts.SetRetryPassObserverForTests(base, baseRec.observe)
	tree, err := base.Parse(src)
	baseline := observeTree(t, lang, tree, err)
	if got := baseRec.passes(); len(got) != 4 {
		t.Fatalf("unbudgeted retry passes = %v, want 4", got)
	}
	if baseline.stop != gts.ParseStopAccepted || baseline.retryStop != gts.ParseStopNone || baseline.runtimeStop != gts.ParseStopNone {
		t.Fatalf("unbudgeted parse: stop=%s retry=%s runtime retry=%s, want accepted/none/none", baseline.stop, baseline.retryStop, baseline.runtimeStop)
	}

	for _, endAfter := range []string{"initial_merge", "clean_wide", "recovery_wide_or_node"} {
		t.Run("timeout after "+endAfter, func(t *testing.T) {
			t.Parallel()
			rec := &retryStopRecorder{endAfter: endAfter, end: gts.ExpireParseDeadlineForTests}
			tree, err := newRetryStopParser(lang, rec).Parse(src)
			obs := observeTree(t, lang, tree, err)
			if obs.stop != gts.ParseStopAccepted {
				t.Fatalf("ParseStopReason = %s, want accepted (unchanged behavior)", obs.stop)
			}
			if obs.retryStop != gts.ParseStopTimeout || obs.runtimeStop != gts.ParseStopTimeout {
				t.Fatalf("RetryStopReason = %s, ParseRuntime().RetryStopReason = %s, want timeout", obs.retryStop, obs.runtimeStop)
			}
			if len(rec.passes()) >= 4 {
				t.Fatalf("retry passes = %v, want fewer than the unbudgeted 4", rec.passes())
			}
			t.Logf("tree equals unbudgeted tree: %v", obs.sexpr == baseline.sexpr)
		})
	}

	t.Run("deadline after the last pass is not a cut", func(t *testing.T) {
		t.Parallel()
		rec := &retryStopRecorder{endAfter: "final_merge", end: gts.ExpireParseDeadlineForTests}
		tree, err := newRetryStopParser(lang, rec).Parse(src)
		if obs := observeTree(t, lang, tree, err); obs.retryStop != gts.ParseStopNone {
			t.Fatalf("RetryStopReason = %s, want none: every retry pass ran", obs.retryStop)
		}
	})

	t.Run("timeout that is not reached", func(t *testing.T) {
		t.Parallel()
		tree, err := newRetryStopParser(lang, &retryStopRecorder{}).Parse(src)
		if obs := observeTree(t, lang, tree, err); obs.retryStop != gts.ParseStopNone || obs.sexpr != baseline.sexpr {
			t.Fatalf("retry=%s tree equal=%v, want none and the unbudgeted tree", obs.retryStop, obs.sexpr == baseline.sexpr)
		}
	})
}

func TestRetryStopReasonCancellationDuringRetryPasses(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	var flag uint32
	rec := &retryStopRecorder{endAfter: "clean_wide", end: func(*gts.Parser) { atomic.StoreUint32(&flag, 1) }}
	p := gts.NewParser(lang)
	p.SetCancellationFlag(&flag)
	gts.SetRetryPassObserverForTests(p, rec.observe)
	tree, err := p.Parse([]byte(retryStopPythonSource))
	obs := observeTree(t, lang, tree, err)
	if obs.stop != gts.ParseStopAccepted || obs.retryStop != gts.ParseStopCancelled || obs.runtimeStop != gts.ParseStopCancelled {
		t.Fatalf("stop=%s retry=%s runtime retry=%s, want accepted/cancelled/cancelled", obs.stop, obs.retryStop, obs.runtimeStop)
	}
}

func TestRetryStopReasonFirstPassCancellationLeavesNone(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	flag := uint32(1)
	p := gts.NewParser(lang)
	p.SetCancellationFlag(&flag)
	tree, err := p.Parse([]byte(retryStopPythonSource))
	obs := observeTree(t, lang, tree, err)
	if obs.stop != gts.ParseStopCancelled || obs.retryStop != gts.ParseStopNone {
		t.Fatalf("stop=%s retry=%s, want cancelled and none when the first pass is cancelled", obs.stop, obs.retryStop)
	}
}

func TestRetryStopReasonFirstPassTimeoutLeavesNone(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	// The deadline is already over when the parse starts, so the first pass
	// sees it at its first stop check, whatever the machine speed.
	p := gts.NewParser(lang)
	p.SetTimeoutMicros(60_000_000)
	tree, err := gts.ParseWithExpiredDeadlineForTests(p, []byte(retryStopPythonSource))
	obs := observeTree(t, lang, tree, err)
	if obs.stop != gts.ParseStopTimeout || obs.retryStop != gts.ParseStopNone {
		t.Fatalf("stop=%s retry=%s, want timeout and none when the first pass times out", obs.stop, obs.retryStop)
	}
}

func TestRetryStopReasonLadderNeverStartsAfterFirstPass(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	rec := &retryStopRecorder{endAfter: "first_pass", end: gts.ExpireParseDeadlineForTests}
	tree, err := newRetryStopParser(lang, rec).Parse([]byte(retryStopPythonSource))
	obs := observeTree(t, lang, tree, err)
	if got := rec.passes(); len(got) != 0 {
		t.Fatalf("retry passes = %v, want none: the deadline expired before the ladder", got)
	}
	if obs.stop != gts.ParseStopAccepted || obs.retryStop != gts.ParseStopTimeout {
		t.Fatalf("stop=%s retry=%s, want accepted and timeout", obs.stop, obs.retryStop)
	}
}

func TestRetryStopReasonTokenSourceLadderNeverStarts(t *testing.T) {
	t.Parallel()
	lang := grammars.JavaLanguage()
	src := []byte("class A { void f( { int a = ; return ) } int x }\n")
	rec := &retryStopRecorder{endAfter: "first_pass", end: gts.ExpireParseDeadlineForTests}
	p := newRetryStopParser(lang, rec)
	ts := grammars.NewJavaTokenSourceOrEOF(src, lang)
	tree, err := p.ParseWithTokenSource(src, ts)
	obs := observeTree(t, lang, tree, err)
	if got := rec.passes(); len(got) != 0 {
		t.Fatalf("retry passes = %v, want none", got)
	}
	if obs.stop != gts.ParseStopAccepted || obs.retryStop != gts.ParseStopTimeout {
		t.Fatalf("stop=%s retry=%s, want accepted and timeout", obs.stop, obs.retryStop)
	}
}

// noResetTokenSource forwards the token methods that the parser loop uses
// (Next, the parser state hints and SkipToByte) to a token source and hides its
// Reset method. A retry pass cannot re-read the source through it, so the
// token-source retry ladder returns at once.
type noResetTokenSource struct {
	inner gts.TokenSource
}

func (s *noResetTokenSource) Next() gts.Token { return s.inner.Next() }

func (s *noResetTokenSource) SetParserState(state gts.StateID) {
	s.inner.(interface{ SetParserState(gts.StateID) }).SetParserState(state)
}

func (s *noResetTokenSource) SetGLRStates(states []gts.StateID) {
	s.inner.(interface{ SetGLRStates([]gts.StateID) }).SetGLRStates(states)
}

func (s *noResetTokenSource) SkipToByte(offset uint32) gts.Token {
	return s.inner.(gts.ByteSkippableTokenSource).SkipToByte(offset)
}

func TestRetryStopReasonTokenSourceWithoutResetNeverCountsTheLadder(t *testing.T) {
	t.Parallel()
	lang := grammars.JavaLanguage()
	src := []byte("class A { void f( { int a = ; return ) } int x }\n")
	if _, ok := grammars.NewJavaTokenSourceOrEOF(src, lang).(interface{ Reset([]byte) }); !ok {
		t.Fatal("control setup: the Java token source has no Reset")
	}
	if _, ok := gts.TokenSource(&noResetTokenSource{}).(interface{ Reset([]byte) }); ok {
		t.Fatal("control setup: the wrapper must not expose Reset")
	}

	// Control: with no deadline the ladder cannot run through this source, so
	// the unbudgeted operation runs no retry pass and reports none.
	free := &retryStopRecorder{}
	base := gts.NewParser(lang)
	gts.SetRetryPassObserverForTests(base, free.observe)
	tree, err := base.ParseWithTokenSource(src, &noResetTokenSource{grammars.NewJavaTokenSourceOrEOF(src, lang)})
	if obs := observeTree(t, lang, tree, err); obs.retryStop != gts.ParseStopNone {
		t.Fatalf("unbudgeted retry=%s, want none", obs.retryStop)
	}
	if got := free.passes(); len(got) != 0 {
		t.Fatalf("unbudgeted retry passes = %v, want none: the source cannot be reset", got)
	}

	// The deadline expires after the first pass. The skipped ladder would have
	// run no pass, so the stop prevented nothing.
	rec := &retryStopRecorder{endAfter: "first_pass", end: gts.ExpireParseDeadlineForTests}
	tree, err = newRetryStopParser(lang, rec).ParseWithTokenSource(src, &noResetTokenSource{grammars.NewJavaTokenSourceOrEOF(src, lang)})
	obs := observeTree(t, lang, tree, err)
	if got := rec.passes(); len(got) != 0 {
		t.Fatalf("retry passes = %v, want none", got)
	}
	if obs.stop != gts.ParseStopAccepted || obs.retryStop != gts.ParseStopNone || obs.runtimeStop != gts.ParseStopNone {
		t.Fatalf("stop=%s retry=%s runtime=%s, want accepted, none and none", obs.stop, obs.retryStop, obs.runtimeStop)
	}
}

// An end-of-file append that the Java lexer proves equal to a fresh parse
// needs no fresh verification when no timeout is set. With a timeout the proof
// is off, so the verification runs, and a deadline that skips it prevented work
// only when the proof would not have applied.
func TestRetryStopReasonFreshVerificationSkippedWhereTheProofWouldSkipIt(t *testing.T) {
	t.Parallel()
	lang := grammars.JavaLanguage()
	source := append([]byte("class Main {\n"), bytes.Repeat([]byte("int x = 2;\n"), 40)...)
	source = append(source, []byte("}\n// ")...)
	edited := append(bytes.Clone(source), 'w')

	plain := gts.NewParser(lang)
	old, err := plain.ParseWithTokenSource(source, grammars.NewJavaTokenSourceOrEOF(source, lang))
	if err != nil || old == nil || old.RootNode().HasError() {
		t.Fatalf("initial parse: %v", err)
	}
	defer old.Release()
	old.Edit(gts.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
		StartPoint: gts.Point{Row: 42, Column: 3}, OldEndPoint: gts.Point{Row: 42, Column: 3}, NewEndPoint: gts.Point{Row: 42, Column: 4}})
	next, err := plain.ParseIncrementalWithTokenSource(edited, old, grammars.NewJavaTokenSourceOrEOF(edited, lang))
	if err != nil || next == nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer next.Release()

	freshStreams := 0
	factory := func(src []byte) (gts.TokenSource, error) {
		freshStreams++
		return grammars.NewJavaTokenSourceOrEOF(src, lang), nil
	}
	verify := func(p *gts.Parser, expire bool) *gts.Tree {
		freshStreams = 0
		return gts.RetryCutOperationForTests(p, func() *gts.Tree {
			if expire {
				gts.ExpireParseDeadlineForTests(p)
			}
			ts, _ := grammars.NewJavaTokenSource(edited, lang)
			tree, _ := gts.VerifyIncrementalFreshResultWithTokenSourceForTests(p, edited, old, next, ts, factory)
			return tree
		})
	}

	// Control: without a timeout the proof applies and no fresh stream is built.
	if got := verify(gts.NewParser(lang), false); got.RetryStopReason() != gts.ParseStopNone || freshStreams != 0 {
		t.Fatalf("unbudgeted: RetryStopReason=%s fresh streams=%d, want none and 0 (the proof applies)", got.RetryStopReason(), freshStreams)
	}
	// With a timeout the verification runs when no stop is active.
	live := newRetryStopParser(lang, &retryStopRecorder{})
	verify(live, false)
	if freshStreams != 1 {
		t.Fatalf("budgeted, no stop: fresh streams=%d, want 1 (the proof is off under a timeout)", freshStreams)
	}
	// An expired deadline skips it, but without the timeout the proof would have
	// skipped it too, so nothing was prevented.
	stopped := newRetryStopParser(lang, &retryStopRecorder{})
	if got := verify(stopped, true); got.RetryStopReason() != gts.ParseStopNone || freshStreams != 0 {
		t.Fatalf("expired deadline: RetryStopReason=%s fresh streams=%d, want none and 0", got.RetryStopReason(), freshStreams)
	}
}

// cloneRetryStopLanguage copies the exported fields of a language into a new
// value. A plain struct copy would also copy the sync.Once and the cache state
// that parsers on the shared language use at the same time, which races with
// parallel tests (the same approach as cloneAdmissionTestLanguage).
func cloneRetryStopLanguage(language *gts.Language) *gts.Language {
	source := reflect.ValueOf(language).Elem()
	clone := reflect.New(source.Type()).Elem()
	for i := 0; i < source.NumField(); i++ {
		if source.Type().Field(i).IsExported() {
			clone.Field(i).Set(source.Field(i))
		}
	}
	return clone.Addr().Interface().(*gts.Language)
}

// repeatPolicyPython returns a copy of the Python language whose external
// scanner repeats a full parse that ended with an error tree.
func repeatPolicyPython() *gts.Language {
	lang := cloneRetryStopLanguage(grammars.PythonLanguage())
	lang.ExternalScannerFullParseRetryPolicy = gts.ExternalScannerFullParseRetryDefault
	return lang
}

func TestRetryStopReasonRepeatSuppressedByDeadline(t *testing.T) {
	t.Parallel()
	lang := repeatPolicyPython()
	src := []byte(retryStopPythonSource)

	// Without a deadline the default policy repeats the whole ladder.
	free := &retryStopRecorder{}
	base := gts.NewParser(lang)
	gts.SetRetryPassObserverForTests(base, free.observe)
	tree, err := base.Parse(src)
	if obs := observeTree(t, lang, tree, err); obs.retryStop != gts.ParseStopNone {
		t.Fatalf("unbudgeted retry=%s, want none", obs.retryStop)
	}
	if len(free.passes()) <= 4 {
		t.Fatalf("unbudgeted retry passes = %v, want a repeated ladder", free.passes())
	}

	rec := &retryStopRecorder{endAfter: "final_merge", end: gts.ExpireParseDeadlineForTests}
	tree, err = newRetryStopParser(lang, rec).Parse(src)
	obs := observeTree(t, lang, tree, err)
	if got := rec.passes(); len(got) != 4 {
		t.Fatalf("retry passes = %v, want one ladder of 4 (the repeat is suppressed)", got)
	}
	if obs.stop != gts.ParseStopAccepted || obs.retryStop != gts.ParseStopTimeout {
		t.Fatalf("stop=%s retry=%s, want accepted and timeout", obs.stop, obs.retryStop)
	}
}

func TestRetryStopReasonThroughParserPool(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	pool := gts.NewParserPool(lang, gts.WithParserPoolTimeoutMicros(60_000_000))
	rec := &retryStopRecorder{endAfter: "clean_wide", end: gts.ExpireParseDeadlineForTests}
	gts.SetPoolParserInitForTests(pool, func(p *gts.Parser) { gts.SetRetryPassObserverForTests(p, rec.observe) })
	parse := func() retryStopObservation {
		tree, err := pool.Parse([]byte(retryStopPythonSource))
		return observeTree(t, lang, tree, err)
	}

	cut := parse()
	if cut.stop != gts.ParseStopAccepted || cut.retryStop != gts.ParseStopTimeout || cut.runtimeStop != gts.ParseStopTimeout {
		t.Fatalf("pool parse: stop=%s retry=%s runtime retry=%s, want accepted/timeout/timeout", cut.stop, cut.retryStop, cut.runtimeStop)
	}
	// Every pooled parser carries the observer, so whichever entry the pool
	// hands out next, the next parse must start clean.
	rec.mu.Lock()
	rec.end = nil
	rec.mu.Unlock()
	for i := 0; i < 3; i++ {
		if next := parse(); next.retryStop != gts.ParseStopNone {
			t.Fatalf("later pool parse %d: retry=%s, want none", i, next.retryStop)
		}
	}
}

func TestRetryStopReasonLaterParseOnSameParserStartsClean(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	src := []byte(retryStopPythonSource)
	rec := &retryStopRecorder{endAfter: "initial_merge", end: gts.ExpireParseDeadlineForTests}
	p := newRetryStopParser(lang, rec)
	tree, err := p.Parse(src)
	if obs := observeTree(t, lang, tree, err); obs.retryStop != gts.ParseStopTimeout {
		t.Fatalf("first parse RetryStopReason = %s, want timeout", obs.retryStop)
	}
	rec.mu.Lock()
	rec.end = nil
	rec.mu.Unlock()
	tree, err = p.Parse(src)
	if obs := observeTree(t, lang, tree, err); obs.retryStop != gts.ParseStopNone || obs.stop != gts.ParseStopAccepted {
		t.Fatalf("later parse: stop=%s retry=%s, want accepted/none", obs.stop, obs.retryStop)
	}
	p.SetTimeoutMicros(0)
	tree, err = p.Parse(src)
	if obs := observeTree(t, lang, tree, err); obs.retryStop != gts.ParseStopNone {
		t.Fatalf("parse without a timeout: RetryStopReason = %s, want none", obs.retryStop)
	}
}

func TestRetryStopReasonDerivedTrees(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	src := []byte(retryStopPythonSource)
	rec := &retryStopRecorder{endAfter: "initial_merge", end: gts.ExpireParseDeadlineForTests}
	p := newRetryStopParser(lang, rec)
	old, err := p.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	if old.RetryStopReason() != gts.ParseStopTimeout {
		t.Fatalf("old tree RetryStopReason = %s, want timeout", old.RetryStopReason())
	}

	// A copy is the same parse, so it keeps the signal.
	cp := old.Copy()
	defer cp.Release()
	if cp.RetryStopReason() != gts.ParseStopTimeout {
		t.Fatalf("copy RetryStopReason = %s, want timeout", cp.RetryStopReason())
	}

	// A reparse of unchanged source returns the old parse, so it keeps the signal.
	same, err := p.ParseIncremental(src, old)
	if err != nil {
		t.Fatal(err)
	}
	if same.RetryStopReason() != gts.ParseStopTimeout {
		t.Fatalf("unchanged-source reparse RetryStopReason = %s, want timeout", same.RetryStopReason())
	}
	same.Release()

	// An incremental reparse is a new parse: it must not inherit the old signal.
	rec.mu.Lock()
	rec.end = nil
	rec.mu.Unlock()
	next := bytes.Replace(src, []byte("class :"), []byte("class C:"), 1)
	at := uint32(bytes.Index(src, []byte("class :")))
	old.Edit(gts.InputEdit{
		StartByte:   at + 6,
		OldEndByte:  at + 6,
		NewEndByte:  at + 7,
		StartPoint:  gts.Point{Row: 3, Column: 6},
		OldEndPoint: gts.Point{Row: 3, Column: 6},
		NewEndPoint: gts.Point{Row: 3, Column: 7},
	})
	reparsed, err := p.ParseIncremental(next, old)
	if err != nil {
		t.Fatal(err)
	}
	defer reparsed.Release()
	if reparsed.RetryStopReason() != gts.ParseStopNone {
		t.Fatalf("incremental reparse RetryStopReason = %s, want none", reparsed.RetryStopReason())
	}
}

func TestRetryStopReasonFreshVerificationKeepsSignal(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	src := []byte(retryStopPythonSource)
	plain, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rec := &retryStopRecorder{endAfter: "clean_wide", end: gts.ExpireParseDeadlineForTests}
	p := newRetryStopParser(lang, rec)
	// The verifier parser inherits the observer and expires its own deadline.
	var verr error
	got := gts.RetryCutOperationForTests(p, func() *gts.Tree {
		var tree *gts.Tree
		tree, verr = gts.VerifyIncrementalFreshResultForTests(p, src, plain, plain)
		return tree
	})
	if verr != nil {
		t.Fatal(verr)
	}
	if got.RetryStopReason() != gts.ParseStopTimeout {
		t.Fatalf("verified tree RetryStopReason = %s, want timeout (the fresh tree was released)", got.RetryStopReason())
	}
	got.Release()
}

func TestRetryStopReasonFreshVerificationSkippedByDeadline(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	src := []byte(retryStopPythonSource)
	plain, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	p := newRetryStopParser(lang, &retryStopRecorder{})
	got := gts.RetryCutOperationForTests(p, func() *gts.Tree {
		gts.ExpireParseDeadlineForTests(p)
		tree, _ := gts.VerifyIncrementalFreshResultForTests(p, src, plain, plain)
		return tree
	})
	if got.RetryStopReason() != gts.ParseStopTimeout {
		t.Fatalf("RetryStopReason = %s, want timeout when the deadline skips the verification", got.RetryStopReason())
	}
	got.Release()
}

func TestRetryStopReasonIncrementalFullFallback(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	src := []byte(retryStopPythonSource)
	// The fallback parses fresh through a hidden parser that runs its own retry passes.
	rec := &retryStopRecorder{endAfter: "clean_wide", end: gts.ExpireParseDeadlineForTests}
	p := newRetryStopParser(lang, rec)
	first, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got := gts.RetryCutOperationForTests(p, func() *gts.Tree { return gts.RetryIncrementalAsFullForTests(p, src, first) })
	if got.RetryStopReason() != gts.ParseStopTimeout {
		t.Fatalf("RetryStopReason = %s, want timeout", got.RetryStopReason())
	}
	got.Release()

	// A deadline that is already over skips the fallback.
	p2 := newRetryStopParser(lang, &retryStopRecorder{})
	first2, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got2 := gts.RetryCutOperationForTests(p2, func() *gts.Tree {
		gts.ExpireParseDeadlineForTests(p2)
		return gts.RetryIncrementalAsFullForTests(p2, src, first2)
	})
	if got2.RetryStopReason() != gts.ParseStopTimeout {
		t.Fatalf("skipped fallback RetryStopReason = %s, want timeout", got2.RetryStopReason())
	}
	got2.Release()
}

func TestRetryStopReasonSwallowedErrorFallbackRejected(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	clean, err := gts.NewParser(lang).Parse([]byte("x = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	// The fallback parses malformed source and runs its retry passes. The
	// cancellation flag is set after its last pass, where no later checkpoint
	// of the retry loop looks at it (a nested timeout scope would restore an
	// expired deadline when it closes); the shared stop then rejects the
	// fallback.
	var flag uint32
	rec := &retryStopRecorder{endAfter: "recovery_wide_or_node", end: func(*gts.Parser) { atomic.StoreUint32(&flag, 1) }}
	p := gts.NewParser(lang)
	p.SetCancellationFlag(&flag)
	gts.SetRetryPassObserverForTests(p, rec.observe)
	got := gts.RetryCutOperationForTests(p, func() *gts.Tree {
		return gts.ResolveSwallowedErrorForTests(p, []byte(retryStopFallbackSource), clean)
	})
	if got != clean {
		t.Fatalf("fallback was adopted; the test needs a rejected fallback (passes %v, flag %d)", rec.passes(), atomic.LoadUint32(&flag))
	}
	if got.RetryStopReason() != gts.ParseStopCancelled {
		t.Fatalf("RetryStopReason = %s, want cancelled", got.RetryStopReason())
	}
	got.Release()
}

func TestRetryStopReasonUnsetReportsNone(t *testing.T) {
	t.Parallel()
	var nilTree *gts.Tree
	if nilTree.RetryStopReason() != gts.ParseStopNone || nilTree.ParseRuntime().RetryStopReason != gts.ParseStopNone {
		t.Fatal("nil tree must report none")
	}
	lang := grammars.PythonLanguage()
	tree, err := gts.NewParser(lang).Parse([]byte("x = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.RetryStopReason() != gts.ParseStopNone || tree.ParseRuntime().RetryStopReason != gts.ParseStopNone {
		t.Fatalf("parse without a timeout: %q / %q, want none", tree.RetryStopReason(), tree.ParseRuntime().RetryStopReason)
	}
	copied := tree.Copy()
	defer copied.Release()
	if copied.RetryStopReason() != gts.ParseStopNone {
		t.Fatalf("copy: %q, want none", copied.RetryStopReason())
	}
}

func TestRetryStopReasonSwallowedFallbackRejectedForDeterministicLimit(t *testing.T) {
	t.Parallel()
	lang := grammars.PythonLanguage()
	clean, err := gts.NewParser(lang).Parse([]byte("x = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	// The fallback hits an explicit node limit on its first pass and is
	// rejected for that reason. The cancellation flag is set right after that
	// pass, but the unbudgeted operation would reject the fallback as well, so
	// the value stays none.
	var flag uint32
	rec := &retryStopRecorder{endAfter: "first_pass", end: func(*gts.Parser) { atomic.StoreUint32(&flag, 1) }}
	p := gts.NewParser(lang)
	p.SetCancellationFlag(&flag)
	p.SetParseWorkLimits(gts.ParseWorkLimits{NodeLimit: 20})
	gts.SetRetryPassObserverForTests(p, rec.observe)
	got := gts.RetryCutOperationForTests(p, func() *gts.Tree {
		return gts.ResolveSwallowedErrorForTests(p, []byte(retryStopFallbackSource), clean)
	})
	if got != clean {
		t.Fatal("fallback was adopted; the test needs a rejected fallback")
	}
	if len(rec.passes()) != 0 {
		t.Fatalf("retry passes = %v, want none", rec.passes())
	}
	if got.RetryStopReason() != gts.ParseStopNone {
		t.Fatalf("RetryStopReason = %s, want none: the limit rejected the fallback", got.RetryStopReason())
	}
}

func TestRetryStopReasonForestRescueDeclinedByStop(t *testing.T) {
	t.Parallel()
	lang := grammars.CLanguage()
	src := []byte(retryStopFallbackSource)
	incumbent, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if incumbent.ParseStopReason() != gts.ParseStopAccepted || !incumbent.RootNode().HasError() {
		t.Fatalf("test setup: want an accepted tree with an error, got %s", incumbent.ParseStopReason())
	}

	// Control: an uninterrupted rescue declines because the input has errors.
	free := gts.NewParser(lang)
	got := gts.RetryCutOperationForTests(free, func() *gts.Tree {
		tree, _ := gts.ForestRescueForTests(free, src, incumbent)
		return tree
	})
	if got != incumbent || got.RetryStopReason() != gts.ParseStopNone {
		t.Fatalf("uninterrupted rescue: same=%v retry=%s, want the incumbent with none", got == incumbent, got.RetryStopReason())
	}

	flag := uint32(1)
	p := gts.NewParser(lang)
	p.SetCancellationFlag(&flag)
	got = gts.RetryCutOperationForTests(p, func() *gts.Tree {
		tree, replaced := gts.ForestRescueForTests(p, src, incumbent)
		if replaced {
			t.Error("an interrupted rescue must not replace the incumbent")
		}
		return tree
	})
	if got != incumbent || got.RetryStopReason() != gts.ParseStopCancelled {
		t.Fatalf("interrupted rescue: same=%v retry=%s, want the incumbent with cancelled", got == incumbent, got.RetryStopReason())
	}
	got.Release()
}
