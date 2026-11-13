//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"reflect"
	"sort"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// Scanners that are bound for comparison with C but have NOT been checked against C.
//
// In a normal build the core serves only the scanners that their packages register
// with scanmode.RegisterScanner after checking them against the pinned C scanner
// (python, javascript, typescript, tsx; see grammars/runtime). The harness compares
// the core with C for many more languages (go, bash, rust, html, ...). For those it
// needs a scanner hook even though the scanner's port has no checked mode. This
// file, which exists only in the harness test binary, registers every shipped
// external scanner type that production code has not registered, with no mode:
// scanmode.RegisterScanner(<scanner>, <payload>, false). Nothing here says that the
// port equals its C scanner; the comparison tests are what test that.
//
// Order: the registry freezes at the first binding. This initializer runs when the
// test binary starts, before any test binds, and after the production registrations
// (the packages it imports have been initialized by then), so it neither collides
// with them nor comes late.

// scannersBoundForComparisonWithC lists the languages whose scanner this file registered.
var scannersBoundForComparisonWithC []string

func init() { registerScannersBoundForComparisonWithC() }

func registerScannersBoundForComparisonWithC() {
	for _, e := range grammars.AllLanguages() {
		sc := e.Language().ExternalScanner
		if sc == nil {
			continue
		}
		if _, checked := scanmode.Lookup(sc); checked {
			continue // registered by its package (checked against C), or by an earlier language
		}
		payload := sc.Create()
		sc.Destroy(payload)
		scanmode.RegisterScanner(sc, payload, false)
		scannersBoundForComparisonWithC = append(scannersBoundForComparisonWithC, e.Name)
	}
}

// TestCcoreEveryLanguageBindsInTheHarness: in the harness binary every registered
// language binds (206 of 206 on this base): the languages without an external scanner,
// the four whose scanners are checked against C, and the others, whose scanners are
// bound for comparison with C by the initializer above and are not checked against C.
func TestCcoreEveryLanguageBindsInTheHarness(t *testing.T) {
	entries := grammars.AllLanguages()
	checked := map[string]bool{"python": true, "javascript": true, "typescript": true, "tsx": true}
	bound := 0
	for _, e := range entries {
		if _, err := bind.New(e.Language()); err != nil {
			t.Errorf("%s: bind.New refused the language in the harness binary: %v", e.Name, err)
			continue
		}
		bound++
	}
	for _, name := range scannersBoundForComparisonWithC {
		if checked[name] {
			t.Errorf("%s: its scanner is checked against C but was registered by the harness initializer", name)
		}
	}
	sorted := append([]string(nil), scannersBoundForComparisonWithC...)
	sort.Strings(sorted)
	t.Logf("CCBINDALL languages bound %d of %d; scanner types registered for comparison only: %d (%d languages)", bound, len(entries), countTypes(), len(sorted))
	if bound != len(entries) {
		t.Fatalf("bound %d of %d", bound, len(entries))
	}
}

func countTypes() int {
	seen := map[reflect.Type]bool{}
	for _, e := range grammars.AllLanguages() {
		if sc := e.Language().ExternalScanner; sc != nil {
			seen[reflect.TypeOf(sc)] = true
		}
	}
	return len(seen)
}
