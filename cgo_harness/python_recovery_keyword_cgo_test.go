//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPythonRecoveryKeywordLockedC(t *testing.T) {
	source, err := os.ReadFile("../testdata/incremental_gate/python_setup.py")
	if err != nil {
		t.Fatal(err)
	}
	language := grammars.PythonLanguage()
	cLanguage, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	originalC := cParser.Parse(source, nil)
	if originalC == nil {
		t.Fatal("C returned no original tree")
	}
	defer originalC.Close()
	if originalC.RootNode().HasError() {
		t.Fatal("C original fixture has errors")
	}
	original, err := gts.NewParser(language).Parse(source)
	if original != nil {
		defer original.Release()
	}
	if err != nil {
		t.Fatal(err)
	}
	if original == nil {
		t.Fatal("Go returned no original tree")
	}
	assertG18LockedCExact(t, "original", original, language, originalC)

	// Change spaces before keywords. Recovery must re-lex each keyword.
	for _, editCase := range []struct {
		position    int
		replacement string
	}{
		{593, "z"}, {594, "z"}, {595, "z"}, {596, "z"},
		{1241, ""}, {1241, "z"},
	} {
		position := editCase.position
		t.Run(fmt.Sprintf("%d-%q", position, editCase.replacement), func(t *testing.T) {
			if source[position] != ' ' {
				t.Fatal("edit position is no longer a space")
			}
			edited := append([]byte(nil), source[:position]...)
			edited = append(edited, editCase.replacement...)
			edited = append(edited, source[position+1:]...)
			oracle := cParser.Parse(edited, nil)
			if oracle == nil {
				t.Fatal("C returned no edited tree")
			}
			defer oracle.Close()
			if !oracle.RootNode().HasError() {
				t.Fatal("C must report an error after the edit")
			}
			fresh, err := gts.NewParser(language).Parse(edited)
			if fresh != nil {
				defer fresh.Release()
			}
			if err != nil {
				t.Fatal(err)
			}
			if fresh == nil {
				t.Fatal("Go returned no fresh tree")
			}
			assertG18LockedCExact(t, "fresh", fresh, language, oracle)

			previous := original.Copy()
			defer previous.Release()
			previous.Edit(gts.InputEdit{
				StartByte: uint32(position), OldEndByte: uint32(position + 1), NewEndByte: uint32(position + len(editCase.replacement)),
				StartPoint: pointAtOffset(source, position), OldEndPoint: pointAtOffset(source, position+1),
				NewEndPoint: pointAtOffset(edited, position+len(editCase.replacement)),
			})
			incremental, err := gts.NewParser(language).ParseIncremental(edited, previous)
			if incremental != nil {
				defer incremental.Release()
			}
			if err != nil {
				t.Fatal(err)
			}
			if incremental == nil {
				t.Fatal("Go returned no incremental tree")
			}
			assertG18LockedCExact(t, "incremental", incremental, language, oracle)
		})
	}
}
