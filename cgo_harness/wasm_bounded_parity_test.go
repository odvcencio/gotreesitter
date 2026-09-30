//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/wasmfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func wasmASCIIUnits(source []byte) []uint16 {
	units := make([]uint16, len(source))
	for i, b := range source {
		units[i] = uint16(b)
	}
	return units
}

func wasmFixtureEdit(source, edited []byte) gts.InputEdit {
	i := 0
	for i < len(source) && source[i] == edited[i] {
		i++
	}
	if i == len(source) {
		panic("WASM fixture has no edit")
	}
	row := bytes.Count(source[:i], []byte{'\n'})
	column := i - bytes.LastIndexByte(source[:i], '\n') - 1
	return gts.InputEdit{StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i + 1), StartPoint: gts.Point{Row: uint32(row), Column: uint32(column)}, OldEndPoint: gts.Point{Row: uint32(row), Column: uint32(column + 1)}, NewEndPoint: gts.Point{Row: uint32(row), Column: uint32(column + 1)}}
}

func wasmAssertCParity(t testing.TB, lang *gts.Language, goRoot *gts.Node, cRoot *sitter.Node) {
	t.Helper()
	type pair struct {
		goNode *gts.Node
		cNode  *sitter.Node
	}
	type frame struct {
		Node      pair
		NextChild int
	}
	var inline [32]frame
	frames := append(inline[:0], frame{Node: pair{goRoot, cRoot}, NextChild: -1})
	nodes := 0
	for len(frames) > 0 {
		f := &frames[len(frames)-1]
		g, c := f.Node.goNode, f.Node.cNode
		if f.NextChild == -1 {
			nodes++
			if g == nil || c == nil {
				t.Fatalf("WASM node %d is nil: Go=%t C=%t", nodes, g == nil, c == nil)
			}
			if g.Type(lang) != c.Kind() || g.StartByte() != uint32(c.StartByte()) || g.EndByte() != uint32(c.EndByte()) || g.IsNamed() != c.IsNamed() || g.IsMissing() != c.IsMissing() || g.IsError() != c.IsError() || g.HasError() != c.HasError() || g.ChildCount() != int(c.ChildCount()) {
				t.Fatalf("WASM node %d differs: Go=%s %d..%d named=%t missing=%t error=%t hasError=%t children=%d; C=%s %d..%d named=%t missing=%t error=%t hasError=%t children=%d", nodes, g.Type(lang), g.StartByte(), g.EndByte(), g.IsNamed(), g.IsMissing(), g.IsError(), g.HasError(), g.ChildCount(), c.Kind(), c.StartByte(), c.EndByte(), c.IsNamed(), c.IsMissing(), c.IsError(), c.HasError(), c.ChildCount())
			}
			start, end := c.StartPosition(), c.EndPosition()
			if g.StartPoint() != (gts.Point{Row: uint32(start.Row), Column: uint32(start.Column)}) || g.EndPoint() != (gts.Point{Row: uint32(end.Row), Column: uint32(end.Column)}) {
				t.Fatalf("WASM node %d point coordinates differ", nodes)
			}
			f.NextChild = 0
		}
		if f.NextChild == g.ChildCount() {
			frames = frames[:len(frames)-1]
			continue
		}
		i := f.NextChild
		f.NextChild++
		if g.FieldNameForChild(i, lang) != c.FieldNameForChild(uint32(i)) {
			t.Fatalf("WASM node %d child %d field differs: Go=%q C=%q", nodes, i, g.FieldNameForChild(i, lang), c.FieldNameForChild(uint32(i)))
		}
		frames = append(frames, frame{Node: pair{g.Child(i), c.Child(uint(i))}, NextChild: -1})
	}
	t.Logf("deep parity: %d nodes", nodes)
}

func TestWASMBoundedGoParity(t *testing.T) {
	cLang, err := COracleLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("go")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("oracle=%s@%s grammar=%s artifact=%s", identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarCommit, identity.GrammarArtifactSHA256)
	lang := grammars.GoLanguage()
	for _, fixture := range wasmfixtures.Cases() {
		t.Run(fixture.Name, func(t *testing.T) {
			t.Logf("source_bytes=%d sha256=%s", len(fixture.Source), fixture.SHA256)
			parser := gts.NewParser(lang)
			units, nextUnits := wasmASCIIUnits(fixture.Source), wasmASCIIUnits(fixture.Edited)
			old, err := parser.ParseUTF16(units)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cOld := cParser.Parse(fixture.Source, nil)
			defer cOld.Close()
			if old.ParseStopReason() != gts.ParseStopAccepted || old.RootNode().EndByte() != uint32(len(fixture.Source)) || old.RootNode().HasError() {
				t.Fatalf("clean WASM fixture stopped: %s", old.ParseStopReason())
			}
			wasmAssertCParity(t, lang, old.RootNode(), cOld.RootNode())
			edit := wasmFixtureEdit(fixture.Source, fixture.Edited)
			if !old.EditUTF16(gts.UTF16Edit{StartCodeUnit: edit.StartByte, OldEndCodeUnit: edit.OldEndByte, NewEndCodeUnit: edit.NewEndByte}, nextUnits) {
				t.Fatal("UTF-16 edit rejected")
			}
			cOld.Edit(&sitter.InputEdit{StartByte: uint(edit.StartByte), OldEndByte: uint(edit.OldEndByte), NewEndByte: uint(edit.NewEndByte), StartPosition: sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)}})
			next, err := parser.ParseIncrementalUTF16(nextUnits, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			cNext := cParser.Parse(fixture.Edited, cOld)
			defer cNext.Close()
			wasmAssertCParity(t, lang, next.RootNode(), cNext.RootNode())
			fresh, err := gts.NewParser(lang).ParseUTF16(nextUnits)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			wasmAssertCParity(t, lang, fresh.RootNode(), cNext.RootNode())
			r := next.ParseRuntime()
			t.Logf("work: tokens=%d iterations=%d nodes=%d arena_bytes=%d scratch_bytes=%d stop=%s root_end=%d", r.TokensConsumed, r.Iterations, r.NodesAllocated, r.ArenaBytesAllocated, r.ScratchBytesAllocated, r.StopReason, r.RootEndByte)
		})
	}
}
