//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"encoding/json"
	"flag"
	"math/rand"
	"strconv"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/wasmfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const wasmIdentifierQuery = `(identifier) @id`

// Both front ends return the same UTF-16 highlight payload. The timed boundary
// includes input copying, edit discovery, Tree.Edit, parsing, all captures,
// JSON encoding and release. Parser/query setup stays outside the operation.
// B/op and allocs/op describe the Go heap; C allocations require process RSS.
type wasmOperationResult struct {
	HasError   bool                      `json:"hasError"`
	Highlights []gts.UTF16HighlightRange `json:"highlights"`
}

func wasmOperationCResult(tree *sitter.Tree, query *sitter.Query, source []byte) wasmOperationResult {
	root := tree.RootNode()
	result := wasmOperationResult{HasError: root.HasError(), Highlights: make([]gts.UTF16HighlightRange, 0)}
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(query, root, source)
	for match := matches.Next(); match != nil; match = matches.Next() {
		for _, capture := range match.Captures {
			n := capture.Node
			start, end := n.StartPosition(), n.EndPosition()
			result.Highlights = append(result.Highlights, gts.UTF16HighlightRange{StartCodeUnit: uint32(n.StartByte() / 2), EndCodeUnit: uint32(n.EndByte() / 2), StartPoint: gts.Point{Row: uint32(start.Row), Column: uint32(start.Column / 2)}, EndPoint: gts.Point{Row: uint32(end.Row), Column: uint32(end.Column / 2)}, Capture: "id", PatternIndex: int(match.PatternIndex)})
		}
	}
	return result
}

func wasmOperationSetup(tb testing.TB) (*gts.Parser, *gts.Highlighter, *sitter.Parser, *sitter.Query) {
	tb.Helper()
	lang := grammars.GoLanguage()
	h, err := gts.NewHighlighter(lang, wasmIdentifierQuery)
	if err != nil {
		tb.Fatal(err)
	}
	cLang, err := COracleLanguage("go")
	if err != nil {
		tb.Fatal(err)
	}
	identity, err := COracleIdentity("go")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Logf("oracle=%s@%s grammar=%s artifact=%s", identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarCommit, identity.GrammarArtifactSHA256)
	c := sitter.NewParser()
	if err := c.SetLanguage(cLang); err != nil {
		tb.Fatal(err)
	}
	q, qErr := sitter.NewQuery(cLang, wasmIdentifierQuery)
	if qErr != nil {
		tb.Fatal(qErr)
	}
	return gts.NewParser(lang), h, c, q
}

func TestWASMCompleteOperationParity(t *testing.T) {
	p, h, c, q := wasmOperationSetup(t)
	defer c.Close()
	defer q.Close()
	for _, f := range wasmfixtures.Cases() {
		t.Run(f.Name, func(t *testing.T) {
			tree, err := p.ParseUTF16(wasmASCIIUnits(f.Source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			ct := c.ParseUTF16LE(wasmASCIIUnits(f.Source), nil)
			if ct == nil {
				t.Fatal("C parse returned nil")
			}
			defer ct.Close()
			goWire, err := json.Marshal(wasmOperationResult{tree.RootNode().HasError(), h.HighlightTreeUTF16(tree)})
			if err != nil {
				t.Fatal(err)
			}
			cWire, err := json.Marshal(wasmOperationCResult(ct, q, f.Source))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(goWire, cWire) {
				t.Fatalf("complete-operation payloads differ: Go=%d bytes C=%d bytes", len(goWire), len(cWire))
			}
			t.Logf("matched UTF-16 wire payload: %d bytes", len(goWire))
		})
	}
}

var wasmOperationSink []byte

func BenchmarkWASMCompleteOperation(b *testing.B) {
	fixtures := wasmfixtures.Cases()
	seed, _ := strconv.ParseInt(flag.Lookup("test.shuffle").Value.String(), 10, 64)
	random := rand.New(rand.NewSource(seed))
	random.Shuffle(len(fixtures), func(i, j int) { fixtures[i], fixtures[j] = fixtures[j], fixtures[i] })
	for _, f := range fixtures {
		if f.Name != "functions-64k" && f.Name != "functions-1m" {
			continue
		}
		operations := []bool{false, true}
		random.Shuffle(len(operations), func(i, j int) { operations[i], operations[j] = operations[j], operations[i] })
		for _, incremental := range operations {
			operation := "full"
			if incremental {
				operation = "edit"
			}
			for _, engine := range []string{"Go", "C", "C", "Go"} {
				b.Run(f.Name+"/"+operation+"/"+engine, func(b *testing.B) {
					p, h, c, q := wasmOperationSetup(b)
					defer c.Close()
					defer q.Close()
					original, next := wasmASCIIUnits(f.Source), wasmASCIIUnits(f.Edited)
					var tree *gts.Tree
					var ct *sitter.Tree
					var err error
					if incremental {
						if engine == "Go" {
							tree, err = p.ParseUTF16(original)
							if err != nil {
								b.Fatal(err)
							}
							defer func() { tree.Release() }()
						} else {
							ct = c.ParseUTF16LE(original, nil)
							if ct == nil {
								b.Fatal("C parse returned nil")
							}
							defer func() { ct.Close() }()
						}
					}
					current := original
					b.ReportAllocs()
					b.SetBytes(int64(len(f.Source)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						source := original
						raw := f.Source
						if incremental && i%2 == 0 {
							source = next
							raw = f.Edited
						}
						units := append([]uint16(nil), source...)
						var edit gts.InputEdit
						if incremental {
							offset := 0
							for offset < len(units) && units[offset] == current[offset] {
								offset++
							}
							row, column := uint32(0), uint32(0)
							for _, u := range current[:offset] {
								if u == '\n' {
									row++
									column = 0
								} else {
									column++
								}
							}
							edit = gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1), StartPoint: gts.Point{Row: row, Column: column}, OldEndPoint: gts.Point{Row: row, Column: column + 1}, NewEndPoint: gts.Point{Row: row, Column: column + 1}}
						}
						var result wasmOperationResult
						if engine == "Go" {
							var parsed *gts.Tree
							if incremental {
								if !tree.EditUTF16(gts.UTF16Edit{StartCodeUnit: edit.StartByte, OldEndCodeUnit: edit.OldEndByte, NewEndCodeUnit: edit.NewEndByte}, units) {
									b.Fatal("UTF-16 edit rejected")
								}
								parsed, err = p.ParseIncrementalUTF16(units, tree)
							} else {
								parsed, err = p.ParseUTF16(units)
							}
							if err != nil {
								b.Fatal(err)
							}
							if parsed.RootNode().EndByte() != uint32(len(raw)) || parsed.RootNode().HasError() {
								b.Fatal("Go operation did not cover clean input")
							}
							result = wasmOperationResult{parsed.RootNode().HasError(), h.HighlightTreeUTF16(parsed)}
							wasmOperationSink, err = json.Marshal(result)
							if incremental {
								if parsed != tree {
									tree.Release()
								}
								tree = parsed
							} else {
								parsed.Release()
							}
						} else {
							if incremental {
								ct.Edit(&sitter.InputEdit{StartByte: uint(edit.StartByte * 2), OldEndByte: uint(edit.OldEndByte * 2), NewEndByte: uint(edit.NewEndByte * 2), StartPosition: sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column * 2)}, OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column * 2)}, NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column * 2)}})
							}
							old := ct
							parsed := c.ParseUTF16LE(units, old)
							if parsed == nil || parsed.RootNode().EndByte() != uint(len(raw)*2) || parsed.RootNode().HasError() {
								b.Fatal("C operation did not cover clean input")
							}
							result = wasmOperationCResult(parsed, q, raw)
							wasmOperationSink, err = json.Marshal(result)
							if incremental {
								old.Close()
								ct = parsed
							} else {
								parsed.Close()
							}
						}
						if err != nil {
							b.Fatal(err)
						}
						current = source
					}
				})
			}
		}
	}
}
