//go:build cgo && treesitter_c_parity

package cgoharness

// DECISIVE diag for the `dispatch.julia` retirement decision (campaign wave 3,
// campaign/julia-decision.md). Dumps THREE full trees for one env-gated
// witness so raw-Go vs production-Go vs locked-C oracle output can be compared
// line by line:
//
//   - locked C oracle: grammars/languages.lock pinned tree-sitter-julia,
//     compiled -O2 and dlopen'd via ParityCLanguage
//   - raw Go: ParseNoResultCompatibilityBenchmarkOnly (no compat arms)
//   - production Go: NewParser(lang).Parse (dispatch.julia arm live)
//
//	REPRO_LANG=julia REPRO_FILE=/path/to/witness.jl \
//	  GTS_PARITY_ALLOW_HOST=1 \
//	  go test ./cgo_harness -tags "cgo,treesitter_c_parity" \
//	    -run TestJuliaDecisionThreeWayDump -v
//
// If REPRO_FILE is unset, the 38-byte ACTIVE julia witness from the wave-2
// probe (inline-recovered-return-range) is used.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const juliaDecisionWitness = "function f()\n    return 1:(2 + 3)\nend\n"

func jddDumpC(n *sitter.Node, src []byte, depth int, b *strings.Builder) {
	txt := ""
	s, e := n.StartByte(), n.EndByte()
	if int(e) <= len(src) {
		raw := string(src[s:e])
		if len(raw) > 60 {
			raw = raw[:60]
		}
		txt = fmt.Sprintf("%q", raw)
	}
	fmt.Fprintf(b, "C %s%s [%d:%d] named=%v missing=%v extra=%v cc=%d %s\n",
		strings.Repeat("  ", depth), n.Kind(), s, e, n.IsNamed(), n.IsMissing(), n.IsExtra(), int(n.ChildCount()), txt)
	for i := 0; i < int(n.ChildCount()); i++ {
		jddDumpC(n.Child(uint(i)), src, depth+1, b)
	}
}

func jddDumpGo(n *gts.Node, lang *gts.Language, src []byte, depth int, b *strings.Builder) {
	txt := ""
	s, e := n.StartByte(), n.EndByte()
	if int(e) <= len(src) {
		raw := string(src[s:e])
		if len(raw) > 60 {
			raw = raw[:60]
		}
		txt = fmt.Sprintf("%q", raw)
	}
	fmt.Fprintf(b, "G %s%s [%d:%d] named=%v missing=%v extra=%v cc=%d %s\n",
		strings.Repeat("  ", depth), n.Type(lang), s, e, n.IsNamed(), n.IsMissing(), n.IsExtra(), n.ChildCount(), txt)
	for i := 0; i < n.ChildCount(); i++ {
		jddDumpGo(n.Child(i), lang, src, depth+1, b)
	}
}

func jddDigest(s string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(s)))
}

func TestJuliaDecisionThreeWayDump(t *testing.T) {
	name := os.Getenv("REPRO_LANG")
	if name == "" {
		name = "julia"
	}
	src := []byte(juliaDecisionWitness)
	if file := os.Getenv("REPRO_FILE"); file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		src = b
	}

	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Fatalf("%s: no locked C reference: %v", name, err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatalf("set C language: %v", err)
	}
	ct := cp.Parse(src, nil)
	defer ct.Close()
	cRoot := ct.RootNode()
	var cB strings.Builder
	jddDumpC(cRoot, src, 0, &cB)

	goLang := grammars.JuliaLanguage()
	gp := gts.NewParser(goLang)
	prod, err := gp.Parse(src)
	if err != nil {
		t.Fatalf("production parse failed: %v", err)
	}
	defer prod.Release()
	raw, err := gts.NewParser(goLang).ParseNoResultCompatibilityBenchmarkOnly(src)
	if err != nil {
		t.Fatalf("raw parse failed: %v", err)
	}
	defer raw.Release()
	var prodB, rawB strings.Builder
	jddDumpGo(prod.RootNode(), goLang, src, 0, &prodB)
	jddDumpGo(raw.RootNode(), goLang, src, 0, &rawB)

	cDigest, err := COracleDeepDigest(ct)
	if err != nil {
		t.Logf("C deep digest unavailable: %v", err)
	}

	t.Logf("witness bytes=%d", len(src))
	t.Logf("=== locked C oracle tree ===\n%s", cB.String())
	t.Logf("=== raw Go tree (no compat) ===\n%s", rawB.String())
	t.Logf("=== production Go tree (arm live) ===\n%s", prodB.String())
	t.Logf("root error flags: C=%v rawGo=%v prodGo=%v",
		cRoot.HasError(), raw.RootNode().HasError(), prod.RootNode().HasError())
	t.Logf("structural digests: C(normalized-dump)=%s rawGo=%s prodGo=%s",
		jddDigest(strings.ReplaceAll(cB.String(), "C ", "X")),
		jddDigest(strings.ReplaceAll(rawB.String(), "G ", "X")),
		jddDigest(strings.ReplaceAll(prodB.String(), "G ", "X")))
	if cDigest != "" {
		t.Logf("COracleDeepDigest(C)=%s", cDigest)
	}

	// Machine-checkable verdict lines.
	cEqRaw := strings.ReplaceAll(cB.String(), "C ", "X") == strings.ReplaceAll(rawB.String(), "G ", "X")
	cEqProd := strings.ReplaceAll(cB.String(), "C ", "X") == strings.ReplaceAll(prodB.String(), "G ", "X")
	t.Logf("VERDUMP C==rawGo: %v", cEqRaw)
	t.Logf("VERDUMP C==prodGo: %v", cEqProd)
}
