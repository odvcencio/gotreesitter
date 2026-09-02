//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestStage5SoleChildHorizontalPaddingProbe(t *testing.T) {
	sources := [][]byte{
		[]byte("def classify(value):\n    if value:\n        return \"present\"\n    return \"missing\"\n"),
		[]byte("def nested(value):\n    if value:\n        for item in value:\n            if item:\n                return item\n    return None\n"),
		[]byte("class Box:\n    def value(self):\n        if True:\n            return 1\n        return 0\n"),
	}
	for sourceIndex, source := range sources {
		for lineStart := 0; lineStart < len(source); {
			if lineStart > 0 && source[lineStart-1] != '\n' {
				lineStart++
				continue
			}
			nextLine := bytes.IndexByte(source[lineStart:], '\n')
			if nextLine < 0 {
				nextLine = len(source) - lineStart
			}
			lineEnd := lineStart + nextLine
			indentEnd := lineStart
			for indentEnd < lineEnd && (source[indentEnd] == ' ' || source[indentEnd] == '\t') {
				indentEnd++
			}
			if indentEnd > lineStart {
				t.Run(fmt.Sprintf("source_%d_line_%d_insert", sourceIndex, lineStart), func(t *testing.T) {
					runSoleChildPaddingEdit(t, source, lineStart, lineStart, []byte(" "))
				})
				t.Run(fmt.Sprintf("source_%d_line_%d_delete", sourceIndex, lineStart), func(t *testing.T) {
					runSoleChildPaddingEdit(t, source, lineStart, lineStart+1, nil)
				})
			}
			lineStart = lineEnd + 1
		}
	}
}

func runSoleChildPaddingEdit(t *testing.T, source []byte, start, oldEnd int, replacement []byte) {
	t.Helper()
	edited := make([]byte, 0, len(source)-(oldEnd-start)+len(replacement))
	edited = append(edited, source[:start]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[oldEnd:]...)
	point := func(src []byte, offset int) gotreesitter.Point {
		var p gotreesitter.Point
		for _, b := range src[:offset] {
			if b == '\n' {
				p.Row++
				p.Column = 0
			} else {
				p.Column++
			}
		}
		return p
	}
	edit := gotreesitter.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(start + len(replacement)),
		StartPoint: point(source, start), OldEndPoint: point(source, oldEnd), NewEndPoint: point(edited, start+len(replacement)),
	}
	lang := grammars.PythonLanguage()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	defer incremental.Release()
	if profile.ReuseUnsupported {
		t.Fatalf("padding edit unexpectedly fell back: %+v", profile)
	}
	fresh, err := parser.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	goDigest, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if goDigest.SHA256 != freshDigest.SHA256 {
		t.Fatalf("incremental != fresh: %s != %s profile=%+v", goDigest.SHA256, freshDigest.SHA256, profile)
	}
	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parser returned nil tree")
	}
	defer cTree.Close()
	if diff := FirstDivergenceDumpV1(incremental.RootNode(), lang, cTree.RootNode()); diff != nil {
		t.Fatalf("incremental != C: %s profile=%+v", formatRealCorpusDivergence(diff), profile)
	}
}
