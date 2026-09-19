package gotreesitter_test

import (
	"fmt"
	"math/rand"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// incrementalEditStep replaces source[start:oldEnd] with text.
type incrementalEditStep struct {
	start, oldEnd uint32
	text          string
}

func incrementalEditPoint(source []byte, offset uint32) gts.Point {
	var point gts.Point
	for _, b := range source[:offset] {
		if b == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
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
			StartPoint:  incrementalEditPoint(current, step.start),
			OldEndPoint: incrementalEditPoint(current, step.oldEnd),
			NewEndPoint: incrementalEditPoint(next, newEnd),
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
