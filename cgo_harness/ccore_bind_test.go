//go:build cgo && treesitter_c_parity

package cgoharness

// How the equality tests reach internal/ccore. The root package registers its
// adapter with internal/ccore/bind in an init function (ccore_bind.go,
// ccore_scanner.go); importing the root package is enough. These helpers ask
// bind for a configured parser for a *Language. The root exports nothing for
// this.

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
)

// ccoreNewE returns a ccore parser for lang, or the reason the core cannot
// serve it. CCORE_DERIVED_ROWS=1 derives the external valid-symbol rows from
// the action table even when the blob has them.
func ccoreNewE(lang *g.Language) (*bindtest.Parser, error) {
	p, err := bindtest.NewParser(lang)
	if err != nil {
		return nil, err
	}
	if os.Getenv("CCORE_DERIVED_ROWS") != "" {
		p.ForceDerivedRows(true)
	}
	return p, nil
}

// ccoreNew is ccoreNewE for tests that cannot continue without a parser.
func ccoreNew(lang *g.Language) *bindtest.Parser {
	p, err := ccoreNewE(lang)
	if err != nil {
		panic(fmt.Sprintf("ccore: no parser for the language: %v", err))
	}
	return p
}

// ccoreExpect fails the test when an equality does not hold. The reporting
// campaigns, which count differences over many grammars on purpose, set
// CCORE_REPORT_ONLY=1: the difference is then logged and the test passes.
func ccoreExpect(t testing.TB, ok bool, format string, args ...any) {
	t.Helper()
	if ok {
		return
	}
	if os.Getenv("CCORE_REPORT_ONLY") != "" {
		t.Logf("differs (report only): "+format, args...)
		return
	}
	t.Errorf(format, args...)
}

// ccoreCount prints the count line that docker/ccore_equality.sh requires of
// every test it runs: the number of inputs (or lookups, or edit steps) that
// the test compared with C. A test that skips or compares nothing prints no
// line, or a zero, and the script fails.
func ccoreCount(test, set string, checked int) {
	fmt.Printf("CCCOUNT test=%s set=%s checked=%d\n", test, set, checked)
}

// TestCcoreScannerIsolation checks two things for one language
// (CCORE_LANG=<language>).
//
// 1. Directly: two bind.Config(gl) values hold a scanner of their own. When the
// language has an external scanner, both Scanner fields must be non-nil,
// comparable and distinct. The payload lives in the adapter object its two
// functions are bound to; hook identity is the strongest direct check available
// outside the root package. A language without an external scanner has nil
// Scanner in both configs.
//
// 2. By parsing: two parsers made by bind.New are used in turn on two different
// inputs (interleaved, three rounds); every digest must equal the digest of a
// fresh parser. This does not prove isolation by itself (every scan starts with
// a deserialize of the previous state), which is why check 1 exists.
func TestCcoreScannerIsolation(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	gl := grammars.DetectLanguageByName(name).Language()
	cfg1, err1 := bind.Config(gl)
	cfg2, err2 := bind.Config(gl)
	if err1 != nil || err2 != nil {
		t.Fatalf("%s: bind.Config: %v, %v", name, err1, err2)
	}
	if gl.ExternalScanner != nil {
		if cfg1.Scanner == nil || cfg2.Scanner == nil {
			t.Fatalf("%s: the language has an external scanner but a Config has none (%v, %v)", name, cfg1.Scanner == nil, cfg2.Scanner == nil)
		}
		v1, v2 := reflect.ValueOf(cfg1.Scanner), reflect.ValueOf(cfg2.Scanner)
		if !v1.Comparable() || !v2.Comparable() {
			t.Fatalf("%s: scanner hook identity is not comparable", name)
		}
		ccoreExpect(t, cfg1.Scanner != cfg2.Scanner, "%s: two Config values hold the same scanner", name)
	} else if cfg1.Scanner != nil || cfg2.Scanner != nil {
		t.Fatalf("%s: the language has no external scanner but a Config has one", name)
	}
	ins := ccoreInputs(t, name, false)
	if len(ins) < 2 {
		t.Fatalf("%s: need two inputs, have %d", name, len(ins))
	}
	a, b := ins[0].Src, ins[len(ins)-1].Src
	digest := func(p *bindtest.Parser, src []byte) string {
		if stop := p.Parse(src, nil); stop != "" {
			t.Fatalf("%s: stop %q", name, stop)
		}
		return p.Digest()
	}
	wantA, wantB := digest(ccoreNew(gl), a), digest(ccoreNew(gl), b)
	p1, p2 := ccoreNew(gl), ccoreNew(gl)
	for round := 0; round < 3; round++ {
		ccoreExpect(t, digest(p1, a) == wantA, "%s: parser 1 gives a different digest for input A in round %d", name, round)
		ccoreExpect(t, digest(p2, b) == wantB, "%s: parser 2 gives a different digest for input B in round %d", name, round)
		ccoreExpect(t, digest(p2, a) == wantA, "%s: parser 2 gives a different digest for input A in round %d", name, round)
		ccoreExpect(t, digest(p1, b) == wantB, "%s: parser 1 gives a different digest for input B in round %d", name, round)
	}
	ccoreCount("TestCcoreScannerIsolation", name, 12)
}

// ccoreExpectStore fails the test when one of the store's integrity counters
// (sharedInTree, doubleFree, badRetain) of the parser is not 0. The counters
// are process-wide, so a raised counter anywhere earlier in the test run also
// fails here. where names the test and the input set for the message.
func ccoreExpectStore(t testing.TB, where string, p *bindtest.Parser) {
	t.Helper()
	shared, double, bad := p.SharedInTree(), p.DoubleFree(), p.BadRetain()
	ccoreExpect(t, shared == 0 && double == 0 && bad == 0,
		"%s: store counters must be 0: sharedInTree %d, doubleFree %d, badRetain %d", where, shared, double, bad)
}
