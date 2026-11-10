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
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
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

// TestCcoreScannerIsolation: two parsers made by bind.New for one language do
// not share scanner state. Two different inputs are parsed in turn on the two
// parsers (interleaved); every digest must equal the digest of a fresh parser.
// CCORE_LANG=<language>; a language without an external scanner passes with
// the same checks.
func TestCcoreScannerIsolation(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	gl := grammars.DetectLanguageByName(name).Language()
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
