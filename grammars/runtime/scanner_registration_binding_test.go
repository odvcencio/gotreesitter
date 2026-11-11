//go:build !grammar_subset

package grammarruntime

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// TestRegisteredScannersGetTheirModeThroughTheRealRegistration: for each scanner
// that is registered with a mode (python, javascript, typescript, tsx) the scanner
// the core binds, behind zero, one and two order adapters, takes the mode from the
// registration and the real payload: the core accepts an input that the same
// scanner declines with the mode off. Nothing here sets a payload flag by hand.
func TestRegisteredScannersGetTheirModeThroughTheRealRegistration(t *testing.T) {
	cases := []struct {
		lang     string
		scanner  gotreesitter.ExternalScanner
		external string // the external symbol that is the only one valid
		src      string
		pos      int
	}{
		// scanner.c:133-135: an automatic semicolon is accepted at byte 0.
		{"javascript", JavaScriptExternalScanner{}, "_automatic_semicolon", "a", 0},
		// scanner.h:90-101: "}" returns at once, with no JSX veto.
		{"typescript", TypeScriptExternalScanner{}, "_automatic_semicolon", "c}</", 1},
		{"tsx", TsxExternalScanner{}, "_automatic_semicolon", "c}</", 1},
	}
	for _, c := range cases {
		reg, ok := scanmode.Lookup(c.scanner)
		if !ok || !reg.NeedsMode {
			t.Errorf("%s: registration %+v (found %v), want a scanner registered with a mode", c.lang, reg, ok)
		}
		payload := c.scanner.Create()
		if reg.Payload != reflect.TypeOf(payload) {
			t.Errorf("%s: registered payload type %v, Create returns %T", c.lang, reg.Payload, payload)
		}
		c.scanner.Destroy(payload)
		for layers := 0; layers <= 2; layers++ {
			core, lang := wrappedCoreScanner(t, c.lang, layers)
			valid := make([]bool, len(lang.ExternalSymbols))
			valid[externalIndex(t, lang, c.external)] = true
			var out ccore.ExtScan
			if !core.Scan([]byte(c.src), c.pos, 0, uint32(c.pos), nil, valid, make([]byte, 1024), &out) {
				t.Errorf("%s, %d layers: the core declined an input that C accepts: the mode did not reach the payload", c.lang, layers)
			}
			// The same scanner with the mode off declines it.
			sc := lang.ExternalScanner
			p := sc.Create()
			lexer := newPortParityLexer([]byte(c.src), c.pos, 0, uint32(c.pos))
			if sc.Scan(p, lexer, valid) {
				t.Errorf("%s, %d layers: the default scan accepted the input, so it does not tell the modes apart", c.lang, layers)
			}
			sc.Destroy(p)
		}
	}
	// python: the saved state has C's layout only with the mode on.
	reg, ok := scanmode.Lookup(PythonExternalScanner{})
	if !ok || !reg.NeedsMode {
		t.Errorf("python: registration %+v (found %v), want a scanner registered with a mode", reg, ok)
	}
	payload := PythonExternalScanner{}.Create()
	if reg.Payload != reflect.TypeOf(payload) {
		t.Errorf("python: registered payload type %v, Create returns %T", reg.Payload, payload)
	}
	PythonExternalScanner{}.Destroy(payload)
	for layers := 0; layers <= 2; layers++ {
		core, lang := wrappedCoreScanner(t, "python", layers)
		valid := make([]bool, len(lang.ExternalSymbols))
		valid[externalIndex(t, lang, "_indent")] = true
		var out ccore.ExtScan
		buf := make([]byte, 1024)
		if !core.Scan([]byte("\n    x"), 0, 0, 0, nil, valid, buf, &out) {
			t.Fatalf("python, %d layers: scan returned false", layers)
		}
		if got := string(buf[:out.StateLen]); got != string([]byte{0, 0, 4, 0}) {
			t.Errorf("python, %d layers: state %v, want C's {0 0 4 0}: the mode did not reach the payload", layers, []byte(got))
		}
	}
}
