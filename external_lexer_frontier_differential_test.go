package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Lever 3 made ExternalLexer.recordReadFrontier lazy: it skips recomputing
// the frontier/examined maxima once the recorded values already provably
// cover the current position (external_lexer.go). This file proves that
// skip never changes the OBSERVABLE result: replayed over the exact call
// sequence a real scan makes, the lazy path and an independently computed
// eager oracle must land on identical (lookahead, examined) pairs at every
// single call, not just at the end -- across a corpus of YAML, Python, and
// Markdown inputs (all three carry an external scanner), each put through a
// chain of incremental edits so the replayed sequence includes whatever a
// scanner's speculative retries and checkpoint rollbacks produce, not just a
// monotonic forward scan.

// frontierCorpusEdit is one step of an edit chain: replace
// source[StartByte:OldEndByte] with Replacement.
type frontierCorpusEdit struct {
	StartByte, OldEndByte uint32
	Replacement           string
}

// applyFrontierCorpusEdit returns the edited source and the InputEdit that
// describes the change, using byte offsets for every Point field (matching
// this codebase's convention that Point.Column is a byte offset, not a
// code-point count; see ExternalLexer.Column's doc comment).
func applyFrontierCorpusEdit(source []byte, edit frontierCorpusEdit) ([]byte, gts.InputEdit) {
	next := make([]byte, 0, len(source)-int(edit.OldEndByte-edit.StartByte)+len(edit.Replacement))
	next = append(next, source[:edit.StartByte]...)
	next = append(next, edit.Replacement...)
	next = append(next, source[edit.OldEndByte:]...)
	newEnd := edit.StartByte + uint32(len(edit.Replacement))
	inputEdit := gts.InputEdit{
		StartByte:   edit.StartByte,
		OldEndByte:  edit.OldEndByte,
		NewEndByte:  newEnd,
		StartPoint:  gts.Point{Column: edit.StartByte},
		OldEndPoint: gts.Point{Column: edit.OldEndByte},
		NewEndPoint: gts.Point{Column: newEnd},
	}
	return next, inputEdit
}

// captureFrontierPositions installs the test observer, runs fn (which must
// call Parse or ParseIncrementalProfiled exactly once), and returns every
// cursor position recordReadFrontier was called at, in call order --
// including calls the lazy skip short-circuited, since the observer fires
// before that skip decision.
func captureFrontierPositions(t *testing.T, fn func()) []int {
	t.Helper()
	var positions []int
	gts.SetRecordReadFrontierObserverForTest(func(pos int) {
		positions = append(positions, pos)
	})
	defer gts.SetRecordReadFrontierObserverForTest(nil)
	fn()
	return positions
}

// assertFrontierReplayMatches replays positions (captured from a real scan
// over source) through two independent accumulators: RecordReadFrontierForTest,
// which drives the real, unmodified lazy recordReadFrontier one call at a
// time, and a hand-computed eager oracle built from the same pure
// per-position formula (ExternalLexerFrontierAtForTest,
// TokenInvariantExaminedEndForTest) the pre-lever-3 code ran unconditionally
// on every call. It requires equality after every single call, not just the
// final one.
func assertFrontierReplayMatches(t *testing.T, label string, source []byte, positions []int) {
	t.Helper()
	if len(positions) == 0 {
		t.Fatalf("%s: captured zero recordReadFrontier calls; the external scanner path was not exercised", label)
	}
	var lazy gts.ExternalReadFrontierValuesForTest
	var eagerLookahead, eagerExamined uint32
	for i, pos := range positions {
		gts.RecordReadFrontierForTest(&lazy, source, pos)

		frontier := gts.ExternalLexerFrontierAtForTest(source, pos)
		if frontier > eagerLookahead {
			eagerLookahead = frontier
		}
		examinedEnd := gts.TokenInvariantExaminedEndForTest(source, frontier)
		if examinedEnd > eagerExamined {
			eagerExamined = examinedEnd
		}

		if lazy.Lookahead != eagerLookahead || lazy.Examined != eagerExamined {
			t.Fatalf("%s: frontier diverged at call %d (pos=%d): lazy={lookahead:%d examined:%d} eager={lookahead:%d examined:%d}",
				label, i, pos, lazy.Lookahead, lazy.Examined, eagerLookahead, eagerExamined)
		}
	}
}

// frontierCorpusYAML, frontierCorpusPython, and frontierCorpusMarkdown mix
// ASCII structure with non-ASCII text (accented Latin and a multi-byte
// emoji) so the replay exercises both the deterministic (ASCII/EOF) and the
// conservative-margin (non-ASCII) branch of recordReadFrontier's lazy skip.
const frontierCorpusYAML = `title: Café résumé
metadata:
  owner: naïve-team
  tags:
    - "emoji: 🎉 release"
    - café
notes: |
  Multiline block
  with a café mention
  and an emoji 🎉 too
count: 3
`

const frontierCorpusPython = `# café notes 🎉
def greet(name):
    """Say hello, naïvely."""
    return f"héllo, {name} 🎉"


class Café:
    owner = "naïve-team"

    def describe(self):
        return f"{self.owner} runs the café 🎉"
`

const frontierCorpusMarkdown = `# Café Notes 🎉

This is a *naïve* summary of the café project.

- item one: café
- item two: 🎉 celebration
- item three: résumé

> A blockquote about naïve assumptions and café culture 🎉.
`

func frontierCorpusEdits(source string) []frontierCorpusEdit {
	n := uint32(len(source))
	mid := n / 2
	return []frontierCorpusEdit{
		// Insert ASCII near the start.
		{StartByte: 2, OldEndByte: 2, Replacement: "XX"},
		// Replace across a non-ASCII byte sequence in the middle (café's é is
		// two UTF-8 bytes; this edit's boundaries do not need to respect rune
		// boundaries in the ORIGINAL text since StartByte/OldEndByte target
		// byte offsets in the CURRENT source at each step).
		{StartByte: mid, OldEndByte: mid + 2, Replacement: "café"},
		// Delete a short ASCII run near the end.
		{StartByte: n - 3, OldEndByte: n - 1, Replacement: ""},
	}
}

func TestExternalLexerFrontierDifferentialCorpus(t *testing.T) {
	cases := []struct {
		name   string
		lang   *gts.Language
		source string
	}{
		{"yaml", grammars.YamlLanguage(), frontierCorpusYAML},
		{"python", grammars.PythonLanguage(), frontierCorpusPython},
		{"markdown", grammars.MarkdownLanguage(), frontierCorpusMarkdown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			parser := gts.NewParser(tc.lang)

			var tree *gts.Tree
			basePositions := captureFrontierPositions(t, func() {
				var err error
				tree, err = parser.Parse(source)
				if err != nil {
					t.Fatalf("base parse: %v", err)
				}
			})
			defer func() {
				if tree != nil {
					tree.Release()
				}
			}()
			assertFrontierReplayMatches(t, tc.name+"/base", source, basePositions)

			edits := frontierCorpusEdits(tc.source)
			for i, edit := range edits {
				nextSource, inputEdit := applyFrontierCorpusEdit(source, edit)
				tree.Edit(inputEdit)

				var next *gts.Tree
				editPositions := captureFrontierPositions(t, func() {
					var err error
					next, _, err = parser.ParseIncrementalProfiled(nextSource, tree)
					if err != nil {
						t.Fatalf("edit %d incremental parse: %v", i, err)
					}
				})
				if next != tree {
					tree.Release()
				}
				tree = next
				source = nextSource

				assertFrontierReplayMatches(t, tc.name, source, editPositions)
			}
		})
	}
}
