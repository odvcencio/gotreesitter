//go:build !grammar_subset

package grammarruntime

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// cFaithfulPayload is a new payload of sc with the C-faithful mode on, when the payload has a mode:
// the tests that compare a port with its C scanner run the mode the core turns on.
func cFaithfulPayload(sc gotreesitter.ExternalScanner) any {
	p := sc.Create()
	if m, ok := p.(interface{ SetCFaithful(bool) }); ok {
		m.SetCFaithful(true)
	}
	return p
}

// checkRegisteredScanner checks one scanner that a scanner package registered after
// the differential check against its pinned C scanner (cgo_harness/scandiff_test.go):
// the registration exists and says what the port declares; the registered payload type
// is what Create returns; a port with a mode starts with the mode off, takes it on and
// off again; and the core binds the language through the real registration behind zero,
// one and two order adapters and runs a scan. modeOn reads the mode of a payload (nil
// for a port without a mode).
func checkRegisteredScanner(t *testing.T, lang string, scanner gotreesitter.ExternalScanner, wantMode bool, modeOn func(payload any) bool) {
	t.Helper()
	reg, ok := scanmode.Lookup(scanner)
	if !ok || reg.NeedsMode != wantMode {
		t.Fatalf("%s: registration %+v (found %v), want a registered scanner with needsMode=%v", lang, reg, ok, wantMode)
	}
	payload := scanner.Create()
	if reg.Payload != reflect.TypeOf(payload) {
		t.Errorf("%s: registered payload type %v, Create returns %T", lang, reg.Payload, payload)
	}
	if wantMode {
		m, ok := payload.(scanmode.CFaithful)
		if !ok {
			t.Fatalf("%s: the payload %T has no SetCFaithful although the scanner is registered with a mode", lang, payload)
		}
		if modeOn(payload) {
			t.Errorf("%s: a new payload has the mode on", lang)
		}
		m.SetCFaithful(true)
		if !modeOn(payload) {
			t.Errorf("%s: SetCFaithful(true) did not turn the mode on", lang)
		}
		m.SetCFaithful(false)
		if modeOn(payload) {
			t.Errorf("%s: SetCFaithful(false) did not turn the mode off (the core's release depends on it)", lang)
		}
	} else if _, has := payload.(scanmode.CFaithful); has {
		t.Errorf("%s: the payload has a mode but the scanner is registered without one", lang)
	}
	scanner.Destroy(payload)
	for layers := 0; layers <= 2; layers++ {
		core, l := wrappedCoreScanner(t, lang, layers)
		valid := make([]bool, len(l.ExternalSymbols))
		for i := range valid {
			valid[i] = true
		}
		var out ccore.ExtScan
		core.Scan([]byte("a b\n"), 0, 0, 0, nil, valid, make([]byte, 1024), &out) // must not panic
	}
}
