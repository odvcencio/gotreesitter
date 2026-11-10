// Package ccoretest replays a ccore parse without the root package, so that
// internal/ccore can keep small golden traces as test inputs.
//
// A golden holds a source text, the trace C's logger gave for the same source,
// the visible-tree digest, and the hash and line count of C's subtree records
// (internal/ccore/records.go gives the line format). The language's Tables
// (with the lex automata) are in a shared file that TablesRef names, so that
// the goldens of one language do not repeat them. Replaying the parse must give
// C's trace line for line, C's digest, and C's records.
package ccoretest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// Golden is one recorded parse.
type Golden struct {
	Name string
	Src  []byte // bytes, so that invalid UTF-8 survives the JSON file
	// TablesRef names the file testdata/tables/<TablesRef>.json; Tables is nil
	// in the committed file and filled in by the loader.
	TablesRef   string        `json:",omitempty"`
	Tables      *ccore.Tables `json:",omitempty"`
	Trace       []string
	Digest      string
	Stop        string
	RecordHash  uint64 // C's FNV-1a 64 hash of the subtree records
	RecordCount uint64 // C's number of subtree records
}

// Replay parses g.Src with the golden's tables. It returns the trace, the
// visible-tree digest, the stop reason and the parser (for arena checks).
func Replay(g *Golden) (trace []string, digest, stop string, p *ccore.Parser) {
	p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
	if err != nil {
		panic("ccoretest: golden " + g.Name + ": " + err.Error())
	}
	p.SetLogger(func(s string) { trace = append(trace, s) })
	_, stop = p.Parse(g.Src)
	p.SetLogger(nil)
	return trace, p.Digest(), stop, p
}

// Read loads one golden file from dir and its shared tables from tablesDir,
// and builds the ASCII fast-path tables, as the root package does.
func Read(dir, tablesDir, name string) (*Golden, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	g := &Golden{}
	if err := json.Unmarshal(raw, g); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if g.Tables == nil {
		if g.TablesRef == "" {
			return nil, fmt.Errorf("%s: no tables and no TablesRef", name)
		}
		traw, err := os.ReadFile(filepath.Join(tablesDir, g.TablesRef+".json"))
		if err != nil {
			return nil, err
		}
		g.Tables = &ccore.Tables{}
		if err := json.Unmarshal(traw, g.Tables); err != nil {
			return nil, fmt.Errorf("%s: %w", g.TablesRef, err)
		}
	}
	g.Tables.BuildAscii()
	return g, nil
}
