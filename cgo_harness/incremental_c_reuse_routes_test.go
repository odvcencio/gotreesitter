//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// These pinned edits require the caller's fresh admission route. In particular,
// suppressing admission used to introduce a false Djot syntax error.
func TestIncrementalCReuseFreshRouteCorpus(t *testing.T) {
	data, err := os.ReadFile("../internal/benchfixtures/real_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language, Role string
			Path           string `json:"committed_path"`
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"djot", "toml"} {
		t.Run(name, func(t *testing.T) {
			var source []byte
			for _, entry := range manifest.Entries {
				if entry.Language == name && entry.Role == "sample" {
					source, err = os.ReadFile(filepath.Join("../internal/benchfixtures", entry.Path))
					if err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			if len(source) == 0 {
				t.Fatal("missing source")
			}
			lang := grammars.DetectLanguageByName(name).Language()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			step := benchfixtures.EditingSession(source)[0]
			old.Edit(step.Edit)
			next, _, err := p.ParseIncrementalProfiled(step.Source, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			fresh, err := p.Parse(step.Source)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			cl, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(step.Source, nil)
			if ct == nil {
				t.Fatal("no C tree")
			}
			defer ct.Close()
			oracle, err := COracleDeepDigest(ct)
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != want.SHA256 || got.SHA256 != oracle || next.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(step.Source)) {
				t.Fatalf("incremental=%s fresh=%s C=%s error=%t", got.SHA256, want.SHA256, oracle, next.RootNode().HasError())
			}
		})
	}
}
