// Package bindall holds one cgo-free test: the core can serve every language
// that the repository registers. It is a package of its own because it links
// the root package, whose init registers the real adapter; the stub adapter of
// package bindtest's tests could not be registered next to it (Register may be
// called once).
package bindall

import (
	"testing"

	_ "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

// TestEveryRegisteredLanguageBinds builds a parser with bind.New for every
// registered language and fails for each one that is refused, with the language
// and the error. With -short it takes every 10th language (about 20) so that a
// race run stays short.
func TestEveryRegisteredLanguageBinds(t *testing.T) {
	entries := grammars.AllLanguages()
	if len(entries) == 0 {
		t.Fatal("no registered language")
	}
	step := 1
	if testing.Short() {
		step = 10
	}
	bound, refused := 0, 0
	for i := 0; i < len(entries); i += step {
		e := entries[i]
		lang := e.Language()
		p, err := bind.New(lang)
		if err != nil || p == nil {
			refused++
			t.Errorf("%s: bind.New refused the language: %v", e.Name, err)
			continue
		}
		bound++
	}
	t.Logf("languages tried %d of %d: bound %d, refused %d", bound+refused, len(entries), bound, refused)
	if bound == 0 {
		t.Fatal("no language bound")
	}
}
