//go:build !grammar_subset || grammar_subset_hcl

package grammarruntime

import (
	"strings"
	"testing"
)

// TestHclSerializeRefusesAtBufferSizeLikeC pins tree-sitter-hcl src/scanner.c:
// 134-136. C returns 0 (no saved state) when a context would bring the state
// to TREE_SITTER_SERIALIZATION_BUFFER_SIZE (1024) bytes or more, so a state of
// exactly 1024 bytes is refused and one of 1023 bytes is kept. The limit is
// C's constant, not the length of the buffer the caller passes.
func TestHclSerializeRefusesAtBufferSizeLikeC(t *testing.T) {
	scanner := HclExternalScanner{}
	// Eight heredoc contexts: 4 bytes of count, then 8 bytes plus the
	// identifier for each. Seven identifiers have 119 bytes (7*127 = 889);
	// the eighth is sized so that the whole state has the wanted size.
	build := func(total int) *hclState {
		s := &hclState{}
		s.SetCFaithful(true)
		sum := 4
		for i := 0; i < 7; i++ {
			s.contextStack = append(s.contextStack, hclContext{ctxType: hclCtxHeredocTemplate, heredocIdentifier: strings.Repeat("a", 119)})
			sum += 8 + 119
		}
		last := total - sum - 8
		if last < 0 || last > 127 {
			t.Fatalf("test setup: identifier of %d bytes", last)
		}
		s.contextStack = append(s.contextStack, hclContext{ctxType: hclCtxHeredocTemplate, heredocIdentifier: strings.Repeat("a", last)})
		return s
	}
	buf := make([]byte, 4096)
	if n := scanner.Serialize(build(1024), buf); n != 0 {
		t.Fatalf("state of exactly 1024 bytes serialized to %d bytes, want 0", n)
	}
	if n := scanner.Serialize(build(1023), buf); n != 1023 {
		t.Fatalf("state of 1023 bytes serialized to %d bytes, want 1023", n)
	}
	// With the mode off the limit is the length of buf, as on main: a state of
	// exactly 1024 bytes is kept when the buffer holds it.
	off := build(1024)
	off.SetCFaithful(false)
	if n := scanner.Serialize(off, buf); n != 1024 {
		t.Fatalf("mode off: state of 1024 bytes serialized to %d bytes, want 1024", n)
	}
}

// TestHclHeredocIdentifierIsAsciiLikeC pins the iswalnum call of the pinned C
// scanner (tree-sitter-hcl src/scanner.c:315). In the C locale only ASCII
// letters and digits are alphanumeric, so the identifier "aé" ends after "a".
func TestHclHeredocIdentifierIsAsciiLikeC(t *testing.T) {
	scanner := HclExternalScanner{}
	valid := make([]bool, hclTokenCount)
	valid[hclTokHeredocIdentifier] = true
	got := portScan(scanner, cFaithfulPayload(scanner), "aé\n", valid)
	portWantScan(t, "identifier", got, true, hclDefaultSymTable[hclTokHeredocIdentifier], 1)
	// With the mode off the identifier test is main's: letters and digits of Unicode.
	got = portScan(scanner, scanner.Create(), "aé\n", valid)
	portWantScan(t, "identifier (mode off)", got, true, hclDefaultSymTable[hclTokHeredocIdentifier], 3)
}

// TestHclCharMaxFollowsTheArchitectureInTheMode. C tests context counts and
// identifier lengths against CHAR_MAX (tree-sitter-hcl src/scanner.c:126 and :137),
// which is 127 where char is signed (x86-64) and 255 where it is unsigned (Linux
// arm64). With the mode on the port follows cCharIsSigned, the build-time rule for
// the architecture; with the mode off it is 127 everywhere, as on main.
func TestHclCharMaxFollowsTheArchitectureInTheMode(t *testing.T) {
	scanner := HclExternalScanner{}
	limit := 127
	if !cCharIsSigned {
		limit = 255
	}
	for _, mode := range []bool{true, false} {
		want := 127
		if mode {
			want = limit
		}
		s := &hclState{}
		s.SetCFaithful(mode)
		for _, id := range []int{want, want + 1} {
			s.contextStack = []hclContext{{ctxType: hclCtxHeredocTemplate, heredocIdentifier: strings.Repeat("a", id)}}
			n := scanner.Serialize(s, make([]byte, 4096))
			if id == want && n != 4+8+id {
				t.Errorf("mode %v: identifier of %d bytes: serialized %d bytes, want %d", mode, id, n, 4+8+id)
			}
			if id == want+1 && n != 0 {
				t.Errorf("mode %v: identifier of %d bytes: serialized %d bytes, want 0 (CHAR_MAX %d)", mode, id, n, want)
			}
		}
		s.contextStack = make([]hclContext, want+1)
		if n := scanner.Serialize(s, make([]byte, 4096)); n != 0 {
			t.Errorf("mode %v: %d contexts: serialized %d bytes, want 0 (CHAR_MAX %d)", mode, want+1, n, want)
		}
	}
}

// TestHclWhitespaceIsCLocaleInTheModeLikeC: the
// white space skipped before a token is iswspace of the C locale
// (scanner.c:198, :237, :337). The legacy mode skips Unicode white space.
func TestHclWhitespaceIsCLocaleInTheModeLikeC(t *testing.T) {
	scanner := HclExternalScanner{}
	valid := make([]bool, hclTokenCount)
	valid[hclTokQuotedTemplateStart] = true
	cf := &hclState{}
	cf.SetCFaithful(true)
	if got := portScan(scanner, cf, " \"", valid); got.ok {
		t.Errorf("C-faithful: the no-break space was skipped, got %+v", got)
	}
	if got := portScan(scanner, &hclState{}, " \"", valid); !got.ok {
		t.Errorf("legacy mode: expected the no-break space skipped, got %+v", got)
	}
}
