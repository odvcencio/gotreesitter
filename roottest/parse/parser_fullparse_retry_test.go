package parse_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGoFullParseBenchmarkSourceStaysWithinDefaultNodeBudget(t *testing.T) {
	gotreesitter.ResetArenaProfile()
	gotreesitter.EnableArenaProfile(true)
	defer gotreesitter.EnableArenaProfile(false)

	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(400)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	defer tree.Release()

	requireCompleteParse(t, tree, src, lang, "full dfa")

	profile := gotreesitter.ArenaProfileSnapshot()
	if got, want := profile.FullAcquire, uint64(1); got != want {
		t.Fatalf("ArenaProfileSnapshot().FullAcquire = %d, want %d for a single full-parse arena", got, want)
	}
}
