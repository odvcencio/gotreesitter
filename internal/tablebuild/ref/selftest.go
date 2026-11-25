package ref

import (
	"fmt"
	"strings"

	gts "github.com/odvcencio/gotreesitter"
)

// BlobOmits lists the sections that a Language decoded from a ts2go blob
// cannot have, with the reason. A comparison of such a blob with a parser.c
// leaves them out of both sides and must say so.
var BlobOmits = map[string]string{
	"L2 max-alias-sequence-length": "a Language keeps MAX_ALIAS_SEQUENCE_LENGTH only as the row length of its alias sequences",
	"L4 primary-states":            "BuildLanguage does not copy ts_primary_state_ids",
	"L4 symbol-map":                "a Language has no ts_symbol_map",
}

// InputSections reports whether a section is read from grammar.json,
// node-types.json or parser-noopt.c, which a Language does not hold.
func InputSections(section string) bool {
	return strings.HasPrefix(section, "L0 ") || strings.HasPrefix(section, "L3 ") || section == "L1 node-type"
}

// Without returns the lines whose section fails drop.
func Without(lines []string, drop func(section string) bool) []string {
	var out []string
	for _, l := range lines {
		if !drop(SectionOf(l)) {
			out = append(out, l)
		}
	}
	return out
}

// RestoreEndAccept sets AcceptEOF on a main lex state that accepts nothing.
// Blobs written before ts2go kept the end-token accept (commit 38c452fe7,
// 22 Sep 2026) lack it on the state where C accepts ts_builtin_sym_end. It
// fails when the state already has an accept, so a test that calls it stops
// once the blob is regenerated.
func RestoreEndAccept(lang *gts.Language, state int) error {
	if state < 0 || state >= len(lang.LexStates) {
		return fmt.Errorf("lex state %d: the blob has %d", state, len(lang.LexStates))
	}
	st := &lang.LexStates[state]
	if st.AcceptEOF || st.AcceptToken != 0 || st.Skip {
		return fmt.Errorf("lex state %d already accepts (end %v, token %d, skip %v): the blob no longer has the known difference", state, st.AcceptEOF, st.AcceptToken, st.Skip)
	}
	st.AcceptEOF = true
	return nil
}

// ChangeOneCell changes one terminal cell of the first large state after the
// error state to the action list of another cell of the same state, for the
// comparer's self-tests. It returns the state and the field index of the cell
// in that state's L4 record, and the cell's text before and after.
func ChangeOneCell(lang *gts.Language) (state, field int, before, after string, err error) {
	if lang.LargeStateCount < 2 {
		return 0, 0, "", "", fmt.Errorf("no large state after the error state")
	}
	state = 1
	row := append([]uint16(nil), lang.ParseTable[state]...)
	var terms []int
	for sym := 0; sym < int(lang.TokenCount) && sym < len(row); sym++ {
		if row[sym] != 0 {
			terms = append(terms, sym)
		}
	}
	for k, sym := range terms {
		for _, other := range terms {
			if row[other] == row[sym] {
				continue
			}
			before = cellText(lang, cell{sym, int(row[sym])})
			row[sym] = row[other]
			lang.ParseTable[state] = row
			after = cellText(lang, cell{sym, int(row[sym])})
			// Fields: layer, kind, state, then one per non-empty cell in symbol
			// order; terminals come first.
			return state, 3 + k, before, after, nil
		}
	}
	return 0, 0, "", "", fmt.Errorf("state 1 has no two terminal cells with different action lists")
}
