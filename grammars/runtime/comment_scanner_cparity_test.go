//go:build !grammar_subset || grammar_subset_comment

package grammarruntime

import (
	"testing"
)

// Reproducers from the pinned C scanner (stsewd/tree-sitter-comment 66272d2b,
// src/tree_sitter_comment/parser.c). The expected results follow the C code.
func TestCommentScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		end             uint32
	}
	for _, c := range []tc{
		// parser.c:88-90 invalid_token valid (correction mode, the all-true row)
		// declines even when name is valid too.
		{"correction-mode-declines", "TODO: x", "11", false, 0},
		// parser.c:92-93 name alone is scanned.
		{"name-only", "TODO: x", "10", true, 4},
		{"name-with-user", "TODO(a): x", "10", true, 4},
		// parser.c:88-90 invalid_token alone declines.
		{"invalid-only", "TODO: x", "01", false, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := CommentExternalScanner{}
			valid := make([]bool, len(c.mask))
			for i := range valid {
				valid[i] = c.mask[i] == '1'
			}
			lexer := newPortParityLexer([]byte(c.src), 0, 0, 0)
			portInstallObserver(lexer)
			ok := sc.Scan(nil, lexer, valid)
			r := portObserve(lexer, ok)
			if r.ok != c.found || (c.found && r.end != c.end) {
				t.Errorf("got found=%v end=%d; want found=%v end=%d", r.ok, r.end, c.found, c.end)
			}
		})
	}
}
