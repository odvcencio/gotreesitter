//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestNorgScannerLockedCParity(t *testing.T) {
	const name = "norg"
	source, fixture := ccoreRouteSource(t, name)
	if fixture != "r4" {
		t.Fatal("missing locked Norg fixture")
	}
	cTree := parseLockedC(t, name, source)
	lang := parityEntriesByName[name].Language()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	if tree.ParseStopReason() != gotreesitter.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(source)) {
		t.Fatalf("Norg parse ended at %d of %d bytes: %s", tree.RootNode().EndByte(), len(source), tree.ParseStopReason())
	}
	assertLockedCTreeExact(t, "locked Norg fixture", tree, lang, cTree)

	step := benchfixtures.EditingSession(source)[0]
	tree.Edit(step.Edit)
	edited, err := parser.ParseIncremental(step.Source, tree)
	if err != nil {
		t.Fatal(err)
	}
	if edited != tree {
		defer edited.Release()
	}
	cEdited := parseLockedC(t, name, step.Source)
	assertLockedCTreeExact(t, "first Norg edit", edited, lang, cEdited)
}
