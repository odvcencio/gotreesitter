//go:build !grammar_subset || grammar_subset_tsx

package grammarruntime

import (
	"bytes"
	"math/rand"
	"testing"
	"unicode/utf8"
)

// TestTsxDefaultModeEqualsMainForGeneratedCalls runs the scanner with the
// mode off and a copy of the scanner as it is on main (tsx_oracle_main_test.go,
// identifiers renamed) over generated calls: source text and offset, valid-symbol
// mask. Return value, result symbol, token span and cursor must be equal in every
// call. The scanner keeps no saved state.
func TestTsxDefaultModeEqualsMainForGeneratedCalls(t *testing.T) {
	const calls = 300000
	rng := rand.New(rand.NewSource(1))
	pool := []string{
		"a", "x", "?", "??", "?.", "?.5", " ", "\n", "\t", " ", "\v", "\f", " ", " ", ":", ")", ",", "}", "{", "<", ">", "&", "&amp;", "&#x41;",
		"/", "//c\n", "/*c*/", "/* c", "`", "${", "\\", "\\u{41}", "<!--", "-->", "0.5", ".5", "1", "(", "=", "\"", "'", "\x00", "\xff", "é", "<div>", "</div>", "text", "\r\n",
	}
	oracle := oracleTsxExternalScanner{}
	scanner := TsxExternalScanner{}
	diffs := 0
	for i := 0; i < calls; i++ {
		var b bytes.Buffer
		for n := 1 + rng.Intn(6); n > 0; n-- {
			b.WriteString(pool[rng.Intn(len(pool))])
		}
		src := b.Bytes()
		if rng.Intn(4) == 0 && len(src) > 0 {
			src[rng.Intn(len(src))] = byte(rng.Intn(256))
		}
		pos, row, col := 0, uint32(0), uint32(0)
		if len(src) > 0 && rng.Intn(3) == 0 {
			pos = rng.Intn(len(src))
			for pos < len(src) && !utf8.RuneStart(src[pos]) {
				pos++
			}
			row = uint32(bytes.Count(src[:pos], []byte{'\n'}))
			col = uint32(pos - (bytes.LastIndexByte(src[:pos], '\n') + 1))
		}
		valid := make([]bool, tsxTokenCount)
		switch rng.Intn(4) {
		case 0:
			for j := range valid {
				valid[j] = true
			}
		case 1:
			valid[rng.Intn(tsxTokenCount)] = true
		default:
			for j := range valid {
				valid[j] = rng.Intn(2) == 0
			}
		}
		a := jsDiffRun(scanner, src, pos, row, col, valid)
		o := jsDiffRun(oracle, src, pos, row, col, valid)
		if a != o {
			diffs++
			if diffs <= 5 {
				t.Errorf("call %d: src %q pos %d valid %v: mode off %+v, main %+v", i, src, pos, valid, a, o)
			}
		}
	}
	t.Logf("DIFFCALLS port=tsx calls=%d differences=%d", calls, diffs)
	if diffs != 0 {
		t.Fatalf("%d of %d calls differ from main with the mode off", diffs, calls)
	}
}
