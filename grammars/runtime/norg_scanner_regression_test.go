//go:build !grammar_subset || grammar_subset_norg

package grammarruntime

import "testing"

func TestNorgDetachedProbeCountsConsumedMarkers(t *testing.T) {
	scanner := NorgExternalScanner{}
	for _, tc := range []struct {
		name, source string
		token, count int
		start, end   uint32
	}{
		{"attached_at_line_start", "  *Press", norgBOLD_OPEN, 1, 2, 3},
		{"weak_paragraph_delimiter", "---\n", norgWEAK_PARAGRAPH_DELIMITER, 3, 0, 4},
		{"heading", "* Heading\n", norgHEADING1, 0, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := scanner.Create().(*norgState)
			defer scanner.Destroy(state)
			valid := make([]bool, norgTokenCount)
			valid[tc.token] = true
			got := portScan(scanner, state, tc.source, valid)
			portWantScan(t, tc.name, got, true, norgDefaultSymTable[tc.token], tc.end)
			if got.start != tc.start || state.parsedChars != tc.count {
				t.Fatalf("token start and consumed markers = %d/%d, want %d/%d",
					got.start, state.parsedChars, tc.start, tc.count)
			}
			if state.lastToken != tc.token {
				t.Fatalf("checkpoint token = %d, want %d", state.lastToken, tc.token)
			}
		})
	}
}

// The pinned scanner marks the @ before examining the tag name, while its
// checkpoint records the last character reached by that examination.
func TestNorgVerbatimTagMarkAndCheckpoint(t *testing.T) {
	scanner := NorgExternalScanner{}
	for _, tc := range []struct {
		name, source  string
		in, out       norgTagType
		token, cursor int
		current       rune
	}{
		{"end_inside_tag", "   @end\n", norgTagInVerbatimTag, norgTagNone, norgRANGED_VERBATIM_TAG_END, 7, 'd'},
		{"end_outside_tag", "   @end\n", norgTagNone, norgTagNone, norgWORD, 7, 'd'},
		{"other_tag_inside_body", "   @ending\n", norgTagInVerbatimTag, norgTagInVerbatimTag, norgWORD, 7, 'd'},
		{"tag_with_partial_end_prefix", "   @enigma\n", norgTagNone, norgTagOnVerbatimTag, norgRANGED_VERBATIM_TAG, 6, 'n'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := scanner.Create().(*norgState)
			defer scanner.Destroy(payload)
			payload.tagContext = tc.in
			valid := make([]bool, norgTokenCount)
			valid[tc.token] = true
			got := portScan(scanner, payload, tc.source, valid)
			portWantScan(t, tc.name, got, true, norgDefaultSymTable[tc.token], 4)
			if got.start != 3 || got.pos != tc.cursor {
				t.Fatalf("token start and scanner cursor = %d/%d, want 3/%d", got.start, got.pos, tc.cursor)
			}

			var checkpoint [19]byte
			size := scanner.Serialize(payload, checkpoint[:])
			restored := scanner.Create().(*norgState)
			defer scanner.Destroy(restored)
			scanner.Deserialize(restored, checkpoint[:size])
			if restored.current != tc.current || restored.tagContext != tc.out || restored.lastToken != tc.token {
				t.Fatalf("restored state = %q/%v/%v, want %q/%v/%v",
					restored.current, restored.tagContext, restored.lastToken, tc.current, tc.out, tc.token)
			}
		})
	}
}
func TestNorgStrongDelimiterMarkAndCheckpoint(t *testing.T) {
	scanner := NorgExternalScanner{}
	for _, tc := range []struct {
		name, source string
		token        int
		start, end   uint32
		cursor       int
		current      rune
	}{
		{"before_lf", "  ===\n", norgSTRONG_PARAGRAPH_DELIMITER, 2, 5, 6, '\n'},
		{"before_crlf", "====\r\n", norgSTRONG_PARAGRAPH_DELIMITER, 0, 4, 5, '\r'},
		{"before_word", "====word", norgWORD, 0, 4, 5, 'w'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := scanner.Create().(*norgState)
			defer scanner.Destroy(state)
			valid := make([]bool, norgTokenCount)
			valid[tc.token] = true
			got := portScan(scanner, state, tc.source, valid)
			portWantScan(t, tc.name, got, true, norgDefaultSymTable[tc.token], tc.end)
			if got.start != tc.start || got.pos != tc.cursor || state.current != tc.current {
				t.Fatalf("token start, scanner cursor, and last character = %d/%d/%q, want %d/%d/%q",
					got.start, got.pos, state.current, tc.start, tc.cursor, tc.current)
			}
		})
	}
}

// The pinned scanner returns false at end of input before it reads any state
// (tree-sitter-norg d89d95af src/scanner.cc:270-274), whatever the tag context
// or the valid tokens. The changed tag, delimiter and detached-marker paths must
// not produce a token or move the checkpoint on empty input.
func TestNorgEmptyInputProducesNoToken(t *testing.T) {
	scanner := NorgExternalScanner{}
	for _, tc := range []struct {
		name string
		in   norgTagType
		last int
	}{
		{"no_tag", norgTagNone, norgWORD},
		{"inside_verbatim_tag", norgTagInVerbatimTag, norgRANGED_VERBATIM_TAG},
		{"on_verbatim_tag", norgTagOnVerbatimTag, norgRANGED_VERBATIM_TAG},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := scanner.Create().(*norgState)
			defer scanner.Destroy(state)
			state.tagContext = tc.in
			state.lastToken = tc.last
			valid := make([]bool, norgTokenCount)
			for i := range valid {
				valid[i] = true
			}
			got := portScan(scanner, state, "", valid)
			portWantScan(t, tc.name, got, false, 0, 0)
			if got.pos != 0 || state.tagContext != tc.in || state.lastToken != tc.last || state.parsedChars != 0 {
				t.Fatalf("after empty input: cursor/tag/last/parsed = %d/%v/%d/%d, want 0/%v/%d/0",
					got.pos, state.tagContext, state.lastToken, state.parsedChars, tc.in, tc.last)
			}
		})
	}
}
