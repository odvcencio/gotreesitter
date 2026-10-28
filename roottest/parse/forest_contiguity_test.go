package parse_test

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestForestNoNonTriviaRootGap is a SAFETY invariant: the forest fast path
// must never DISPATCH a structurally-incomplete tree — one with a non-trivia
// hole between top-level children. At scale a wrong GLR path can drop or
// mis-attach a run of top-level items (dart's large generated bindings drop a
// ~7KB run of typedefs); the forest's end-byte coverage check does not catch a
// hole in the MIDDLE of the child list. Declining (falling back to production)
// is correct; dispatching a gapped tree hands the caller a wrong parse.
func TestForestNoNonTriviaRootGap(t *testing.T) {
	cases := []struct {
		name string
		path string
		lang func() *gts.Language
	}{
		{"dart_large", "../../cgo_harness/corpus_real/dart/large__generated_bindings.dart", grammars.DartLanguage},
		// Promoted langs must STILL dispatch — contiguously (regression guard).
		{"scss_large", "../../cgo_harness/corpus_real/scss/large__github.com.scss", grammars.ScssLanguage},
		{"go_large", "../../cgo_harness/corpus_real/go/large__proc.go", grammars.GoLanguage},
	}
	for _, c := range cases {
		src, err := os.ReadFile(c.path)
		if err != nil {
			t.Logf("%s: corpus absent, skipping (%v)", c.name, err)
			continue
		}
		lang := c.lang()
		tr, ok := gts.NewParser(lang).ParseForestExperimental(src)
		if !ok {
			t.Logf("%s: forest declined (safe)", c.name)
			continue
		}
		// Dispatched -> the tree is USED, so it must cover all non-trivia bytes.
		r := tr.RootNode()
		prev := r.StartByte()
		for i := 0; i < int(r.ChildCount()); i++ {
			ch := r.Child(i)
			if ch.StartByte() > prev && !allTrivia(src[prev:ch.StartByte()]) {
				t.Errorf("%s: forest DISPATCHED a tree with a non-trivia gap before child[%d] %s (bytes %d-%d): forest must decline, not dispatch a structurally-incomplete tree",
					c.name, i, ch.Type(lang), prev, ch.StartByte())
				break
			}
			if ch.EndByte() > prev {
				prev = ch.EndByte()
			}
		}
		tr.Release()
	}
}

func allTrivia(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r', '\f':
		default:
			return false
		}
	}
	return true
}
