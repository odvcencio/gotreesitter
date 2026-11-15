//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	g "github.com/odvcencio/gotreesitter"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
)

// TestGraduationServed checks the generated certificate and the route.
func TestGraduationServed(t *testing.T) {
	if gradCensus() {
		defer ccoreConfigureViews(t)()
	} else if _, set := os.LookupEnv("CCORE_VIEWS"); set {
		t.Fatal("CCORE_VIEWS is for --report --census; the gate measures the default views switch")
	}
	gradRun(t)
	name := os.Getenv("GRAD_LANG")
	if name == "" {
		t.Skip("set GRAD_LANG")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("item 0 input=%q: no language", name)
	}
	lang := entry.Language()
	blob, ok := lang.GrammarBlobSHA256()
	if !ok {
		gradResult(0, "fail", "blob=missing", "input="+name+": grammar has no blob identity")
		return
	}
	need := certified.Fresh | certified.Recover | certified.Incr
	cert, found := certified.Blobs[blob]
	config, err := bind.ConfigHash(lang)
	nums := fmt.Sprintf("input=%s blob=%x modes=%d need=%d config=%x certificate_config=%x", name, blob, cert.Modes, need, config, cert.Config)
	if gradNeg(0) {
		gradResult(0, "fail", nums, "negative check: corrupted certificate expectation")
		return
	}
	if !found || cert.Modes&need != need || err != nil || cert.Config != config {
		gradResult(0, "fail", nums, fmt.Sprintf("certificate missing or mismatched: found=%t config_error=%v", found, err))
		return
	}
	src, input := gradSample(t, name)
	served, err := gradServedBy(g.NewParser(lang), src)
	if err != nil || !served {
		gradResult(0, "fail", nums, fmt.Sprintf("certified but not served by a default parser: input=%s served=%t parse_error=%v", input, served, err))
		return
	}
	gradResult(0, "pass", nums+" inputs=1 served=1", "")
}
