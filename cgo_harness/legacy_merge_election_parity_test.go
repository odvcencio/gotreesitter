//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestLegacyMergeElectionLockedCParity(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{"perl", "{(p())if@;}"},
		{"c_sharp", "var x = [ y, ];\n"},
		{"python", "x, y, z = 1, 2, 3\nxyz = x, y, z\n"},
		{"python", "a = 1\nb = 2\npair = a, b\n"},
		{"python", "a = 1\nsingle = a,\n"},
		{"python", "x = 1\ny = 2\nz = f\"{x, y}\"\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(test.name)
			lang := entry.Language()
			cp := sitter.NewParser()
			defer cp.Close()
			cl, err := COracleLanguage(test.name)
			if err != nil {
				t.Fatal(err)
			}
			if err = cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			source := []byte(test.source)
			ct := cp.Parse(source, nil)
			defer ct.Close()
			p := gotreesitter.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			assertLockedCTreeExact(t, "fresh", old, lang, ct)
		})
	}
}

// The legacy unit election must preserve the incremental/fresh invariant on
// the minimal Meson edit that exposed the broader condensation experiment.
func TestLegacyMergeElectionMesonIncrementalConsistency(t *testing.T) {
	lang := grammars.MesonLanguage()
	p := gotreesitter.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	source := []byte("y('',e,d:e)")
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(gotreesitter.InputEdit{StartByte: 0, OldEndByte: 0, NewEndByte: 1, NewEndPoint: gotreesitter.Point{Column: 1}})
	after := append([]byte("x"), source...)
	inc, err := p.ParseIncremental(after, old)
	if err != nil {
		t.Fatal(err)
	}
	defer inc.Release()
	fresh, err := p.Parse(after)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	var differences []string
	compareGoNodes(inc.RootNode(), lang, fresh.RootNode(), "root", &differences)
	if len(differences) != 0 {
		t.Fatal(differences[0])
	}
	incDigest, err := benchfixtures.InspectGoTree(inc.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if incDigest.SHA256 != freshDigest.SHA256 {
		t.Fatal("incremental deep digest differs from fresh")
	}
	if inc.RootNode().HasError() != fresh.RootNode().HasError() {
		t.Fatal("error flags differ")
	}
	if inc.RootNode().EndByte() != uint32(len(after)) {
		t.Fatal("root does not cover the edit")
	}
	if allocations := testing.AllocsPerRun(10, func() {
		same, err := p.ParseIncremental(after, inc)
		if err != nil {
			t.Fatal(err)
		}
		same.Release()
	}); allocations != 0 {
		t.Fatalf("no-edit reparse allocates %g times", allocations)
	}
}
