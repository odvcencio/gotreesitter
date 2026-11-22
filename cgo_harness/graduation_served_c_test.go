//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
)

// TestGraduationServed checks the generated certificate and the route.
func TestGraduationServed(t *testing.T) {
	name := os.Getenv("GRAD_LANG")
	if name == "" {
		t.Skip("set GRAD_LANG")
	}
	if gradCensus() {
		defer ccoreConfigureViews(t)()
	} else if _, set := os.LookupEnv("CCORE_VIEWS"); set {
		t.Fatal("CCORE_VIEWS is for --report --census; the gate measures the default views switch")
	}
	gradRun(t)
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
	candidate := gradCandidate()
	if candidate {
		if !gradCensus() {
			gradResult(0, "fail", "input="+name, "GRAD_CANDIDATE is for --report --census: the gate itself reads the generated certificate")
			return
		}
		defer gradCandidateCertify(t)()
	}
	need := certified.Fresh | certified.Recover | certified.Incr
	cert, found := certified.Blobs[blob]
	config, err := bind.ConfigHash(lang)
	nums := fmt.Sprintf("input=%s blob=%x modes=%d need=%d config=%x certificate_config=%x", name, blob, cert.Modes, need, config, cert.Config)
	if gradNeg(0) {
		gradResult(0, "fail", nums, "negative check: corrupted certificate expectation")
		return
	}
	if candidate {
		nums += " candidate=1 providers=" + strings.Join(gradCandidateProviders(), ",")
	}
	if candidate && err == nil {
		found, cert.Modes, cert.Config = true, need, config // a candidate record is not in the generated list
	}
	if !found || cert.Modes&need != need || err != nil || cert.Config != config {
		gradResult(0, "fail", nums, fmt.Sprintf("certificate missing or mismatched: found=%t config_error=%v", found, err))
		return
	}
	src, input := gradSample(t, name)
	served, err := gradServedBy(gradRoute(lang), src)
	if err != nil || !served {
		gradResult(0, "fail", nums, fmt.Sprintf("certified but not served by a default parser: input=%s served=%t parse_error=%v", input, served, err))
		return
	}
	if entry.TokenSourceFactory == nil {
		gradResult(0, "pass", nums+" inputs=1 served=1", "")
		return
	}
	// The entry has a registered factory: LangEntry, the highlighter and the
	// token-source incremental entry point use it, so each must be served too.
	// A certificate that does not list the scan provider of the factory's source
	// fails here, and the first failure names the route.
	routes := gradFactoryRoutes(t, entry, lang, gradInputs(t, name), src)
	routeNums, first := gradRoutesVerdict(routes)
	nums += " factory=1 " + routeNums
	if first != "" {
		gradResult(0, "fail", nums, first)
		return
	}
	gradResult(0, "pass", nums+" inputs=1 served=1", "")
}
