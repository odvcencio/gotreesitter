// Package bindall holds one cgo-free test: the core's answer for every language
// that the repository registers. It is a package of its own because it links
// the root package, whose init registers the real adapter; the stub adapter of
// package bindtest's tests could not be registered next to it (Register may be
// called once).
package bindall

import (
	"strings"
	"testing"

	_ "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// registeredScanners are the languages whose external scanner is registered with
// scanmode.RegisterScanner (python, javascript, typescript, tsx, css, scss, racket, mojo, starlark, teal, templ, hcl). The core serves
// them; every other language that has an external scanner is refused with the
// named reason.
var registeredScanners = map[string]bool{"python": true, "javascript": true, "typescript": true, "tsx": true, "css": true, "scss": true, "racket": true, "mojo": true, "starlark": true, "teal": true, "templ": true, "hcl": true}

// TestEveryRegisteredLanguageBinds builds a parser with bind.New for every
// registered language and checks the exact outcome for each: a language without an
// external scanner, or with a registered one, binds; a language whose external
// scanner is not registered is refused with scanmode.ReasonScannerNotRegistered.
// Any other outcome fails with the language and the error. With -short it takes
// every 10th language (about 20) so that a race run stays short.
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
	registeredTypes := 0
	refusedNames := map[string]bool{}
	for i := 0; i < len(entries); i += step {
		e := entries[i]
		lang := e.Language()
		if lang.ExternalScanner != nil {
			if _, ok := scanmode.Lookup(lang.ExternalScanner); ok {
				registeredTypes++
				if !registeredScanners[e.Name] {
					t.Errorf("%s: its scanner is registered but the test does not list the language as checked", e.Name)
				}
			}
		}
		p, err := bind.New(lang)
		if lang.ExternalScanner == nil || registeredScanners[e.Name] {
			if err != nil || p == nil {
				t.Errorf("%s: bind.New refused a language the core serves: %v", e.Name, err)
				continue
			}
			bound++
			continue
		}
		if err == nil || p != nil || !strings.Contains(err.Error(), scanmode.ReasonScannerNotRegistered) {
			t.Errorf("%s: want a refusal with the reason %q, got parser %v, error %v", e.Name, scanmode.ReasonScannerNotRegistered, p != nil, err)
			continue
		}
		refused++
		refusedNames[e.Name] = true
	}
	t.Logf("languages tried %d of %d: bound %d, refused by name %d", bound+refused, len(entries), bound, refused)
	if bound == 0 {
		t.Fatal("no language bound")
	}
	if step == 1 && registeredTypes != len(registeredScanners) {
		t.Fatalf("%d languages have a registered scanner in a normal build, want exactly the %d checked ones", registeredTypes, len(registeredScanners))
	}
	if step == 1 && !refusedNames["go"] {
		t.Fatal("the go scanner is not registered in a normal build and must be refused; it was not")
	}
	if step == 1 && refused == 0 {
		t.Fatal("no language was refused: the policy is not applied")
	}
}
