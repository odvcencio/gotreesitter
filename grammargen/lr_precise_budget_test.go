package grammargen

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func preciseBudgetGrammar(externalCount int) *Grammar {
	g := NewGrammar("precise_budget")
	g.Define("source_file", Choice(
		Seq(Str("a"), Sym("pair")),
		Seq(Str("c"), Sym("_external_0")),
	))
	g.Define("pair", Seq(Str("b"), Str("d")))
	externals := make([]*Rule, externalCount)
	for i := range externals {
		externals[i] = Sym(fmt.Sprintf("_external_%d", i))
	}
	g.SetExternals(externals...)
	return g
}

func setPreciseBudgetTestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("GOT_LR_FORCE_EXTERNAL_LALR", "")
	t.Setenv("GOT_LR_FORCE_PRECISE_EXTERNAL", "")
	t.Setenv("GOT_LALR_LR0_STATE_BUDGET", "0")
	t.Setenv("GOT_LALR_LR0_CORE_BUDGET", "0")
}

func parsePreciseBudgetGrammar(t *testing.T, g *Grammar) string {
	t.Helper()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	input := []byte("abd")
	tree, err := gotreesitter.NewParser(lang).Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root after generation")
	}
	if root.HasError() || root.EndByte() != uint32(len(input)) {
		t.Fatalf("incomplete parse after generation: tree=%s end=%d", root.SExpr(lang), root.EndByte())
	}
	return root.SExpr(lang)
}

func TestPreciseExternalStateBudgetFallsBackForSmallGrammar(t *testing.T) {
	for _, tc := range []struct {
		name          string
		externals     int
		preferPrecise bool
	}{
		{name: "many_externals_default", externals: 24},
		{name: "many_externals_prefer_precise", externals: 24, preferPrecise: true},
		{name: "few_externals", externals: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPreciseBudgetTestEnvironment(t)
			g := preciseBudgetGrammar(tc.externals)
			g.PreferPreciseExternalLexStates = tc.preferPrecise
			ng, err := Normalize(g)
			if err != nil {
				t.Fatal(err)
			}
			if len(ng.ExternalSymbols) != tc.externals || len(ng.Productions) > 400 {
				t.Fatalf("fixture misses small-grammar path: externals=%d productions=%d", len(ng.ExternalSymbols), len(ng.Productions))
			}

			t.Setenv("GOT_LR_PRECISE_EXTERNAL_STATE_BUDGET", "20000")
			want := parsePreciseBudgetGrammar(t, g)
			if want != "(source_file (pair))" {
				t.Fatalf("control tree = %s", want)
			}

			t.Setenv("GOT_LR_PRECISE_EXTERNAL_STATE_BUDGET", "1")
			if got := parsePreciseBudgetGrammar(t, g); got != want {
				t.Fatalf("fallback tree = %s, control = %s", got, want)
			}
			tables, ctx, err := buildLRTablesInternal(context.Background(), ng, false)
			if err != nil {
				t.Fatal(err)
			}
			// Materialization releases lalrLR0ItemSets after the fallback;
			// its retained work counter proves that LR(0) construction ran.
			if tables == nil || ctx.preciseStateBudgetExceeded || ctx.lalrLR0CoreEntries == 0 {
				t.Fatal("precise budget exhaustion did not finish the LALR fallback")
			}
		})
	}
}

func TestPreciseExternalStateBudgetPropagatesFallbackFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		want string
	}{
		{name: "state_budget", env: "GOT_LALR_LR0_STATE_BUDGET", want: "LALR LR0 state budget exceeded"},
		{name: "core_budget", env: "GOT_LALR_LR0_CORE_BUDGET", want: "LALR LR0 core budget exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPreciseBudgetTestEnvironment(t)
			t.Setenv("GOT_LR_PRECISE_EXTERNAL_STATE_BUDGET", "1")
			t.Setenv(tc.env, "1")
			g := preciseBudgetGrammar(24)
			ng, err := Normalize(g)
			if err != nil {
				t.Fatal(err)
			}
			tables, _, err := buildLRTablesInternal(context.Background(), ng, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) || tables != nil {
				t.Fatalf("buildLRTablesInternal: nil_tables=%t err=%v, want nil tables and %q", tables == nil, err, tc.want)
			}
			lang, err := GenerateLanguage(g)
			if err == nil || !strings.Contains(err.Error(), tc.want) || lang != nil {
				t.Fatalf("GenerateLanguage: nil_language=%t err=%v, want nil language and %q", lang == nil, err, tc.want)
			}
		})
	}
}

func TestPreciseExternalStateBudgetPropagatesCancellation(t *testing.T) {
	setPreciseBudgetTestEnvironment(t)
	t.Setenv("GOT_LR_PRECISE_EXTERNAL_STATE_BUDGET", "1")
	g := preciseBudgetGrammar(24)
	ng, err := Normalize(g)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tables, _, err := buildLRTablesInternal(ctx, ng, false)
	if tables != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("buildLRTablesInternal: nil_tables=%t err=%v, want nil tables and context.Canceled", tables == nil, err)
	}
	lang, err := GenerateLanguageWithContext(ctx, g)
	if lang != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("GenerateLanguageWithContext: nil_language=%t err=%v, want nil language and context.Canceled", lang == nil, err)
	}
}
