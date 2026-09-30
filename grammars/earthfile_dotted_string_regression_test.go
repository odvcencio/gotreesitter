package grammars_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestEarthfileDottedStringContinuation(t *testing.T) {
	lang := grammars.EarthfileLanguage()
	for _, tc := range []struct {
		source string
		want   string
	}{
		{"DO..Y\n", "(source_file (block (do_command (string (unquoted_string)))))"},
		{"SAVE ARTIFACT.s .s\n", "(source_file (block (save_artifact_command (string (unquoted_string)) (string (unquoted_string)))))"},
	} {
		for _, compact := range []bool{false, true} {
			route := "default"
			if compact {
				route = "compact"
			}
			t.Run(route+"/"+tc.source, func(t *testing.T) {
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(compact)
				tree, err := parser.Parse([]byte(tc.source))
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				root := tree.RootNode()
				if root.HasError() || root.EndByte() != uint32(len(tc.source)) {
					t.Fatalf("incomplete or erroneous parse: %s", root.SExpr(lang))
				}
				if got := root.SExpr(lang); got != tc.want {
					t.Fatalf("tree = %s; want %s", got, tc.want)
				}
			})
		}
	}
}

func TestEarthfileDottedStringEditMatchesFresh(t *testing.T) {
	lang := grammars.EarthfileLanguage()
	for _, tc := range []struct {
		name   string
		before string
		after  string
		at     uint32
	}{
		{"do", "DO.Y\n", "DO..Y\n", 3},
		{"save_artifact", "SAVE ARTIFACT.s s\n", "SAVE ARTIFACT.s .s\n", 16},
	} {
		for _, compact := range []bool{false, true} {
			route := "default"
			if compact {
				route = "compact"
			}
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(compact)
				old, err := parser.Parse([]byte(tc.before))
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				old.Edit(gotreesitter.InputEdit{
					StartByte: tc.at, OldEndByte: tc.at, NewEndByte: tc.at + 1,
					StartPoint:  gotreesitter.Point{Column: tc.at},
					OldEndPoint: gotreesitter.Point{Column: tc.at},
					NewEndPoint: gotreesitter.Point{Column: tc.at + 1},
				})
				incremental, err := parser.ParseIncremental([]byte(tc.after), old)
				if err != nil {
					t.Fatal(err)
				}
				defer incremental.Release()
				freshParser := gotreesitter.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(compact)
				fresh, err := freshParser.Parse([]byte(tc.after))
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				for _, tree := range []*gotreesitter.Tree{incremental, fresh} {
					if root := tree.RootNode(); root.HasError() || root.EndByte() != uint32(len(tc.after)) {
						t.Fatalf("incomplete or erroneous parse: %s", root.SExpr(lang))
					}
				}
				got, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 {
					t.Fatalf("incremental tree differs from fresh: %s != %s", got.SHA256, want.SHA256)
				}
			})
		}
	}
}
