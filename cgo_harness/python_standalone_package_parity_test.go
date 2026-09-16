//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	standalonepython "github.com/odvcencio/gotreesitter/grammars/python"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestStandalonePythonPackageLockedCRoutes(t *testing.T) {
	language := standalonepython.Language()
	cLanguage, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}

	witnesses := map[string][]byte{
		"assignment tuple": []byte("x, y, z = 1, 2, 3\nxyz = x, y, z\n"),
		"f-string tuple":   []byte("x = 1\ny = 2\nz = f\"{x, y}\"\n"),
		"f-string splat":   []byte("xs = [1, 2]\nz = f\"{*xs,}\"\n"),
	}
	for name, source := range witnesses {
		t.Run(name, func(t *testing.T) {
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C returned no tree")
			}
			defer cTree.Close()

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer production.Release()
			assertLockedCTreeExact(t, "standalone Python production", production, language, cTree)

			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer compact.Release()
			assertLockedCTreeExact(t, "standalone Python compact", compact, language, cTree)

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(source)
			if !ok || forest == nil {
				t.Fatal("standalone Python forest route declined")
			}
			defer forest.Release()
			assertLockedCTreeExact(t, "standalone Python forest", forest, language, cTree)

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			defer oldTree.Release()
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  pythonDispatchPoint(base),
				OldEndPoint: pythonDispatchPoint(base),
				NewEndPoint: pythonDispatchPoint(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			assertLockedCTreeExact(t, "standalone Python incremental", incremental, language, cTree)
			if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" {
				t.Fatalf("standalone Python scanner reuse declined: %q", profile.ReuseUnsupportedReason)
			}
		})
	}
}
