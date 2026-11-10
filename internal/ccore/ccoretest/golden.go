// Package ccoretest replays a ccore parse without the root package, so that
// internal/ccore can keep small golden traces as test inputs.
//
// A golden holds a language's Tables (with the lex automata), a source text, the
// trace C's logger gave for the same source, and the visible-tree digest.
// Replaying the parse must give C's trace line for line.
package ccoretest

import (
	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// Golden is one recorded parse.
type Golden struct {
	Name   string
	Src    []byte // bytes, so that invalid UTF-8 survives the JSON file
	Tables *ccore.Tables
	Trace  []string
	Digest string
	Stop   string
}

// Replay parses g.Src with the golden's tables. It returns the trace, the
// visible-tree digest, the stop reason and the parser (for arena checks).
func Replay(g *Golden) (trace []string, digest, stop string, p *ccore.Parser) {
	p = ccore.NewParser(ccore.Config{Tables: g.Tables})
	p.SetLogger(func(s string) { trace = append(trace, s) })
	_, stop = p.Parse(g.Src)
	p.SetLogger(nil)
	return trace, p.Digest(), stop, p
}
