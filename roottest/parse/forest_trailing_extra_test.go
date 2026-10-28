package parse_test

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestForestTrailingExtraRetained locks the max-coverage accept fix. A file
// ending in a single MULTI-token extra (e.g. one lua `-- comment` at EOF, which
// is `--`+content reduced via a synthetic EOF) produces two accept candidates:
// the bare root, and a node ABOVE it that also consumed the trailing comment.
// Taking the last-seen accept dropped the comment; preferring the candidate that
// consumed the most input keeps it. (Two trailing comments already worked — the
// bug was specific to a single one.) Verified against the production parser.
func TestForestTrailingExtraRetained(t *testing.T) {
	lua := grammars.LuaLanguage

	t.Run("minimal_single_trailing_comment", func(t *testing.T) {
		src := []byte("x = 1\n-- c\n")
		ft, ok := gts.NewParser(lua()).ParseForestExperimental(src)
		if !ok {
			t.Fatal("forest declined a trivial statement + trailing comment")
		}
		defer ft.Release()
		pt, _ := gts.NewParser(lua()).Parse(src)
		defer pt.Release()
		if got, want := ft.RootNode().EndByte(), pt.RootNode().EndByte(); got != want {
			t.Fatalf("trailing comment dropped: forest end=%d production end=%d", got, want)
		}
		if got, want := ft.RootNode().ChildCount(), pt.RootNode().ChildCount(); got != want {
			t.Fatalf("forest root ChildCount=%d != production %d", got, want)
		}
	})

	t.Run("corpus_lua_functions_matches_production", func(t *testing.T) {
		src, err := os.ReadFile("../../cgo_harness/corpus_real/lua/small__functions.lua")
		if err != nil {
			t.Skip("corpus absent")
		}
		ft, ok := gts.NewParser(lua()).ParseForestExperimental(src)
		if !ok {
			t.Fatal("forest declined lua small__functions")
		}
		defer ft.Release()
		pt, _ := gts.NewParser(lua()).Parse(src)
		defer pt.Release()
		if ft.RootNode().EndByte() != pt.RootNode().EndByte() || ft.RootNode().ChildCount() != pt.RootNode().ChildCount() {
			t.Fatalf("forest (end=%d children=%d) != production (end=%d children=%d)",
				ft.RootNode().EndByte(), ft.RootNode().ChildCount(), pt.RootNode().EndByte(), pt.RootNode().ChildCount())
		}
	})
}

func TestForestOmitsZeroWidthRootExtraLikeProduction(t *testing.T) {
	source := []byte("echo \"hello\"\n")
	lang := grammars.NimLanguage()

	productionParser := gts.NewParser(lang)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer production.Release()

	forest, ok := gts.NewParser(lang).ParseForestExperimental(source)
	if !ok || forest == nil {
		t.Fatal("forest declined the zero-width external extra fixture")
	}
	defer forest.Release()

	if got, want := forest.RootNode().SExpr(lang), production.RootNode().SExpr(lang); got != want {
		t.Fatalf("forest shape=%s, want production %s", got, want)
	}
	if got, want := forest.RootNode().ChildCount(), production.RootNode().ChildCount(); got != want {
		t.Fatalf("forest root children=%d, want %d", got, want)
	}
}
