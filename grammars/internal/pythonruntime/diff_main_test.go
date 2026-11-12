package pythonruntime

import (
	"bytes"
	"math/rand"
	"reflect"
	"testing"
	"unicode/utf8"
	"unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

//go:linkname diffLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func diffLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)

//go:linkname diffLookaheadEnd github.com/odvcencio/gotreesitter.(*ExternalLexer).lookaheadEndByteAtCursor
func diffLookaheadEnd(*gotreesitter.ExternalLexer) uint32

type diffScanner interface {
	Create() any
	Destroy(any)
	Scan(any, *gotreesitter.ExternalLexer, []bool) bool
	Serialize(any, []byte) int
	Deserialize(any, []byte)
}

type diffResult struct {
	ok                          bool
	sym                         gotreesitter.Symbol
	start, end                  uint32
	hasTok                      bool
	pos                         int
	row, col                    uint32
	gotColumn                   bool
	frontier                    uint32
	readLookahead, readExamined uint32
	readNonlocal                bool
	stateLen                    int
	state                       string
}

func diffRun(sc diffScanner, prev []byte, src []byte, pos int, row, col uint32, valid []bool) diffResult {
	p := sc.Create()
	defer sc.Destroy(p)
	sc.Deserialize(p, prev)
	lexer := newNulLexer(src, pos, row, col)
	// The scan's read observer, as the parser installs it: every read is recorded.
	f := reflect.ValueOf(lexer).Elem().FieldByName("readFrontier")
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.New(f.Type().Elem()))
	r := diffResult{ok: sc.Scan(p, lexer, valid)}
	// Observations come from the lexer's fields: no method that records a read or a
	// column use is called before they are taken.
	rv := reflect.ValueOf(lexer).Elem()
	r.pos = int(rv.FieldByName("pos").Int())
	pt := rv.FieldByName("point")
	r.row, r.col = uint32(pt.FieldByName("Row").Uint()), uint32(pt.FieldByName("Column").Uint())
	r.gotColumn = rv.FieldByName("didGetColumn").Bool()
	r.frontier = uint32(rv.FieldByName("lookaheadEndByte").Uint())
	if f := diffLookaheadEnd(lexer); f > r.frontier {
		r.frontier = f
	}
	if o := rv.FieldByName("readFrontier").Elem(); o.IsValid() {
		r.readLookahead = uint32(o.FieldByName("lookahead").Uint())
		r.readExamined = uint32(o.FieldByName("examined").Uint())
		r.readNonlocal = o.FieldByName("nonlocal").Bool()
	}
	if tok, has := diffLexerToken(lexer); has {
		r.hasTok, r.sym, r.start, r.end = true, tok.Symbol, tok.StartByte, tok.EndByte
	}
	buf := make([]byte, 1024)
	r.stateLen = sc.Serialize(p, buf)
	if r.stateLen > 0 && r.stateLen <= len(buf) {
		r.state = string(buf[:r.stateLen])
	}
	return r
}

func diffEqual(a, b diffResult) bool { return a == b }

// TestDefaultModeEqualsMainForGeneratedCalls runs the scanner with the mode off
// and a copy of the scanner as it is on main (oracle_main_test.go, identifiers
// renamed) over generated calls: source text and offset, valid-symbol mask, and
// the saved state of the previous call. Return value, result symbol, token span,
// cursor and saved state bytes must be equal in every call.
func TestDefaultModeEqualsMainForGeneratedCalls(t *testing.T) {
	const calls = 300000
	rng := rand.New(rand.NewSource(1))
	pool := []string{
		"if x:\n", "    y = 1\n", "\tz\n", "def f(a):\n", "  return a\n", "# c\n", "\n", " ", "x = \"a\\nb\"\n", "s = 'q'\n",
		"f\"{x}\"", "f'{a!r:{w}}'", "rb\"x\"", "\"\"\"doc\"\"\"", "'''t", "(", ")", "[", "]", "{", "}", "{{", "}}", "\\\n", "\\", "\"", "'",
		"else:", "except", "lambda:", "`", "\x00", "\xff", "é", "\r\n", "\f", "\u2028", "1_000", "x[1:2]", "if", ":",
	}
	oracle := oraclePythonExternalScanner{}
	scanner := PythonExternalScanner{}
	var prev, prevO []byte
	diffs := 0
	for i := 0; i < calls; i++ {
		var b bytes.Buffer
		for n := 1 + rng.Intn(8); n > 0; n-- {
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
		valid := make([]bool, pyTokenCount)
		switch rng.Intn(4) {
		case 0:
			for j := range valid {
				valid[j] = true
			}
		case 1:
			valid[rng.Intn(pyTokenCount)] = true
		default:
			for j := range valid {
				valid[j] = rng.Intn(2) == 0
			}
		}
		switch rng.Intn(8) {
		case 0:
			prev, prevO = nil, nil
		case 1: // malformed: random bytes
			prev = make([]byte, rng.Intn(40))
			rng.Read(prev)
			prevO = prev
		case 2: // truncated: a prefix of the last state
			if len(prev) > 0 {
				prev = prev[:rng.Intn(len(prev))]
				prevO = prev
			}
		case 3: // boundary size: a header of one or two delimiters and indent words up to 1,030 bytes
			n := 1020 + rng.Intn(11)
			prev = make([]byte, n)
			prev[1] = byte(1 + rng.Intn(2))
			for k := 2 + int(prev[1]); k < n; k++ {
				prev[k] = byte(rng.Intn(256))
			}
			prevO = prev
		}
		a := diffRun(scanner, prev, src, pos, row, col, valid)
		o := diffRun(oracle, prevO, src, pos, row, col, valid)
		if !diffEqual(a, o) {
			diffs++
			if diffs <= 5 {
				t.Errorf("call %d: src %q pos %d valid %v prev %v: mode off %+v, main %+v", i, src, pos, valid, prev, a, o)
			}
		}
		if a.ok && o.ok {
			prev, prevO = []byte(a.state), []byte(o.state)
		}
	}
	t.Logf("DIFFCALLS port=python calls=%d differences=%d", calls, diffs)
	if diffs != 0 {
		t.Fatalf("%d of %d calls differ from main with the mode off", diffs, calls)
	}
}
