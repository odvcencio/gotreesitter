// Package ccoretest records and replays a ccore parse without the root
// package, so that internal/ccore can keep small golden traces as test inputs.
//
// A golden holds a language's Tables, a source text, the answers the lexer
// kernels gave during one parse (recorded by the harness against the root
// kernel), the trace C's logger gave for the same source, and the visible-tree
// digest. Replaying the kernel answers must give C's trace line for line.
package ccoretest

import (
	"fmt"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// Call is one recorded kernel answer.
type Call struct {
	State uint32
	Pos   int
	Row   uint32
	Col   uint32
	OK    bool
	Tok   ccore.Token
	FailS [3]uint32 // Fail() start: bytes, row, col
	FailE [3]uint32 // Fail() end
}

// Golden is one recorded parse.
type Golden struct {
	Name    string
	Src     string
	Tables  *ccore.Tables
	Main    []Call
	Keyword []Call // empty when the language has no keyword automaton
	Trace   []string
	Digest  string
	Stop    string
}

// Recorder wraps a kernel and records its answers.
type Recorder struct {
	Inner ccore.LexKernel
	Calls []Call
}

func (r *Recorder) Scan(lexState uint32, src []byte, pos int, row, col uint32, out *ccore.Token) bool {
	ok := r.Inner.Scan(lexState, src, pos, row, col, out)
	c := Call{State: lexState, Pos: pos, Row: row, Col: col, OK: ok, Tok: *out}
	if !ok {
		s, e := r.Inner.Fail()
		c.FailS = [3]uint32{s.Bytes(), s.Row(), s.Col()}
		c.FailE = [3]uint32{e.Bytes(), e.Row(), e.Col()}
	}
	r.Calls = append(r.Calls, c)
	return ok
}

func (r *Recorder) Fail() (ccore.Length, ccore.Length) { return r.Inner.Fail() }

type key struct {
	state    uint32
	pos      int
	row, col uint32
}

// Replayer answers kernel calls from recorded Calls.
type Replayer struct {
	m    map[key]Call
	last Call
}

// NewReplayer indexes calls by (state, position, row, column).
func NewReplayer(calls []Call) *Replayer {
	r := &Replayer{m: make(map[key]Call, len(calls))}
	for _, c := range calls {
		r.m[key{c.State, c.Pos, c.Row, c.Col}] = c
	}
	return r
}

func (r *Replayer) Scan(lexState uint32, src []byte, pos int, row, col uint32, out *ccore.Token) bool {
	c, ok := r.m[key{lexState, pos, row, col}]
	if !ok {
		panic(fmt.Sprintf("ccoretest: no recorded kernel answer for state %d at byte %d", lexState, pos))
	}
	r.last = c
	*out = c.Tok
	return c.OK
}

func (r *Replayer) Fail() (ccore.Length, ccore.Length) {
	return ccore.NewLength(r.last.FailS[0], r.last.FailS[1], r.last.FailS[2]),
		ccore.NewLength(r.last.FailE[0], r.last.FailE[1], r.last.FailE[2])
}

// Replay parses g.Src with the recorded answers. It returns the trace, the
// visible-tree digest, the stop reason and the parser (for arena checks).
func Replay(g *Golden) (trace []string, digest, stop string, p *ccore.Parser) {
	cfg := ccore.Config{Tables: g.Tables, Lex: NewReplayer(g.Main)}
	if len(g.Keyword) != 0 {
		cfg.Keyword = NewReplayer(g.Keyword)
	}
	p = ccore.NewParser(cfg)
	p.SetLogger(func(s string) { trace = append(trace, s) })
	_, stop = p.Parse([]byte(g.Src))
	p.SetLogger(nil)
	return trace, p.Digest(), stop, p
}
