//go:build cgo && treesitter_c_parity

package cgoharness

// Targeted C-oracle parity check for the Go new()/make() leading-argument
// residual (qualified_type and pointer_type forms). Each fixture is parsed by
// the pinned C reference and by gotreesitter; the full trees (kind, byte span,
// named flag, missing/extra, field name) must match node for node.
//
// Run: GTS_PARITY_ALLOW_HOST=1 go test ./cgo_harness -tags treesitter_c_parity \
//        -run TestGoNewMakeResidualParity -v

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func nmCSExpr(n *sitter.Node, src []byte, b *strings.Builder, depth int, field string) {
	b.WriteString(strings.Repeat("  ", depth))
	if field != "" {
		b.WriteString(field)
		b.WriteString(": ")
	}
	fmt.Fprintf(b, "%s [%d:%d] named=%v missing=%v extra=%v",
		n.Kind(), n.StartByte(), n.EndByte(), n.IsNamed(), n.IsMissing(), n.IsExtra())
	if n.ChildCount() == 0 {
		fmt.Fprintf(b, " %q", string(src[n.StartByte():n.EndByte()]))
	}
	b.WriteByte('\n')
	for i := 0; i < int(n.ChildCount()); i++ {
		fn := n.FieldNameForChild(uint32(i))
		nmCSExpr(n.Child(uint(i)), src, b, depth+1, fn)
	}
}

func nmGoSExpr(n *gts.Node, lang *gts.Language, src []byte, b *strings.Builder, depth int, field string) {
	b.WriteString(strings.Repeat("  ", depth))
	if field != "" {
		b.WriteString(field)
		b.WriteString(": ")
	}
	fmt.Fprintf(b, "%s [%d:%d] named=%v missing=%v extra=%v",
		n.Type(lang), n.StartByte(), n.EndByte(), n.IsNamed(), n.IsMissing(), n.IsExtra())
	if n.ChildCount() == 0 {
		fmt.Fprintf(b, " %q", string(src[n.StartByte():n.EndByte()]))
	}
	b.WriteByte('\n')
	for i := 0; i < n.ChildCount(); i++ {
		fn := n.FieldNameForChild(i, lang)
		nmGoSExpr(n.Child(i), lang, src, b, depth+1, fn)
	}
}

func TestGoNewMakeResidualParity(t *testing.T) {
	cLang := loadCanonicalGoCLanguage(t)
	goLang := grammars.GoLanguage()

	// Every form here is VALID Go whose leading new()/make() argument the C
	// oracle parses as a type. Invalid-Go inputs (`new(a.b.C)`, `new(&T)`) are
	// intentionally excluded: the C oracle drives them through strict
	// type-context ERROR recovery, a separate and pre-existing divergence that
	// this residual work does not touch.
	forms := []string{
		// Newly fixed residual forms.
		"new(pkg.Type)",
		"new(*T)",
		"new(*pkg.Type)",
		"new(**T)",
		"new(a.b)",
		"new((T))",
		"new((pkg.Type))",
		"new((*T))",
		"make(pkg.Type, 0)",
		"make(*T, 0)",
		// Already-correct forms (regression controls).
		"new(T)",
		"new([]T)",
		"new(*[]T)",
		"new([]*T)",
		"new(*map[K]V)",
		"make([]pkg.Type, 0)",
		"new(map[K]V)",
		"new(chan T)",
		"new(*chan T)",
		"new(struct{})",
		"new(pkg.Type[int])",
		// Composite-literal disambiguation guards.
		"Pattern{}",
		"&Pattern{}",
		"[]T{{X: 1}}",
		"map[K]V{k: v}",
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set C language: %v", err)
	}
	for _, candidate := range []bool{false, true} {
		t.Run(fmt.Sprintf("candidate=%t", candidate), func(t *testing.T) {
			goParser := gts.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(candidate)

			for _, form := range forms {
				t.Run(form, func(t *testing.T) {
					src := []byte("package p\n\nfunc f() {\n\t_ = " + form + "\n}\n")

					cTree := cParser.Parse(src, nil)
					defer cTree.Close()
					var cb strings.Builder
					nmCSExpr(cTree.RootNode(), src, &cb, 0, "")

					gts.ResetAdmissionCandidateCounters()
					goTree, err := goParser.Parse(src)
					if err != nil {
						t.Fatalf("go parse: %v", err)
					}
					defer goTree.Release()
					if candidate {
						routed, fallback := gts.AdmissionCandidateCounters()
						t.Logf("compact=%d fallback=%d reason=%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
					}
					var gb strings.Builder
					nmGoSExpr(goTree.RootNode(), goLang, src, &gb, 0, "")

					if cb.String() != gb.String() {
						t.Fatalf("PARITY MISMATCH for %q\n=== C ORACLE ===\n%s\n=== GOTREESITTER ===\n%s", form, cb.String(), gb.String())
					}
				})
			}
		})
	}
}
