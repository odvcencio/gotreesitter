package grammargen

import (
	"bytes"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// calcPowerCustomization adds a right-associative "**" power rule to a calc
// grammar's expression choice. It is shared between the ExtendGrammar
// (closure) path and the ExtendGrammarJSON (data-driven delta) path in
// TestExtendGrammarJSONParityWithExtendGrammar so both apply byte-identical
// edits.
func calcPowerCustomization(g *Grammar) {
	g.Define("power", PrecRight(4, Seq(
		Field("left", Sym("expression")),
		Field("operator", Str("**")),
		Field("right", Sym("expression")),
	)))
	g.Define("expression", Choice(
		PrecLeft(1, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("+")),
			Field("right", Sym("expression")),
		)),
		PrecLeft(1, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("-")),
			Field("right", Sym("expression")),
		)),
		PrecLeft(2, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("*")),
			Field("right", Sym("expression")),
		)),
		PrecLeft(2, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("/")),
			Field("right", Sym("expression")),
		)),
		PrecRight(3, Seq(
			Field("operator", Str("-")),
			Field("operand", Sym("expression")),
		)),
		Seq(Str("("), Sym("expression"), Str(")")),
		Sym("number"),
		Sym("power"),
	))
}

// TestExtendGrammarJSONParityWithExtendGrammar is the core parity proof for
// Enabler 2: applying the same edits via ExtendGrammarJSON's data-driven
// delta and via ExtendGrammar's Go closure must produce Languages that are
// byte-identical when encoded, and the extended grammar must compile and
// parse a sample that exercises the added rule.
func TestExtendGrammarJSONParityWithExtendGrammar(t *testing.T) {
	const extName = "calc_power"

	// --- Closure path (existing ExtendGrammar API) ---
	closureExtended := ExtendGrammar(extName, CalcGrammar(), calcPowerCustomization)

	// --- Data-driven path (ExtendGrammarJSON) ---
	baseJSON, err := ExportGrammarJSON(CalcGrammar())
	if err != nil {
		t.Fatalf("ExportGrammarJSON(base): %v", err)
	}

	deltaGrammar := NewGrammar("delta")
	calcPowerCustomization(deltaGrammar)
	deltaJSON, err := ExportGrammarJSON(deltaGrammar)
	if err != nil {
		t.Fatalf("ExportGrammarJSON(delta): %v", err)
	}
	t.Logf("delta grammar.json:\n%s", deltaJSON)

	jsonExtended, err := ExtendGrammarJSON(extName, baseJSON, deltaJSON)
	if err != nil {
		t.Fatalf("ExtendGrammarJSON: %v", err)
	}

	t.Run("inherits base rules", func(t *testing.T) {
		if _, ok := jsonExtended.Rules["number"]; !ok {
			t.Error("missing inherited rule 'number'")
		}
	})

	t.Run("has new rule", func(t *testing.T) {
		if _, ok := jsonExtended.Rules["power"]; !ok {
			t.Error("missing new rule 'power'")
		}
	})

	t.Run("base grammar.json not mutated", func(t *testing.T) {
		base2, err := ImportGrammarJSON(baseJSON)
		if err != nil {
			t.Fatalf("re-import baseJSON: %v", err)
		}
		if _, ok := base2.Rules["power"]; ok {
			t.Error("base grammar.json was mutated to include 'power'")
		}
	})

	// --- Parity: generated Languages must be byte-identical. ---
	closureLang, err := GenerateLanguage(closureExtended)
	if err != nil {
		t.Fatalf("GenerateLanguage(closureExtended): %v", err)
	}
	jsonLang, err := GenerateLanguage(jsonExtended)
	if err != nil {
		t.Fatalf("GenerateLanguage(jsonExtended): %v", err)
	}

	closureBlob, err := encodeLanguageBlob(closureLang)
	if err != nil {
		t.Fatalf("encode closureLang: %v", err)
	}
	jsonBlob, err := encodeLanguageBlob(jsonLang)
	if err != nil {
		t.Fatalf("encode jsonLang: %v", err)
	}
	if !bytes.Equal(closureBlob, jsonBlob) {
		t.Fatalf("ExtendGrammarJSON output differs from ExtendGrammar closure output (encoded lengths %d vs %d)",
			len(closureBlob), len(jsonBlob))
	}

	// --- Both compile and parse a sample exercising the added rule. ---
	for _, tc := range []struct {
		name string
		lang *gotreesitter.Language
	}{
		{"closure", closureLang},
		{"json", jsonLang},
	} {
		t.Run(tc.name+" parses power expression", func(t *testing.T) {
			parser := gotreesitter.NewParser(tc.lang)
			tree, err := parser.Parse([]byte("2 ** 3 ** 2"))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			sexp := tree.RootNode().SExpr(tc.lang)
			t.Logf("parse: %s", sexp)
			if strings.Contains(sexp, "ERROR") {
				t.Fatalf("unexpected ERROR in tree: %s", sexp)
			}
			if !strings.Contains(sexp, "power") {
				t.Fatalf("expected 'power' node in tree: %s", sexp)
			}
		})
	}
}

// TestExtendGrammarJSONFieldMerges exercises each documented merge rule
// (add rule, override rule, union extras/conflicts/externals/supertypes/
// inline, word override, reserved override) against a small synthetic base
// so each behavior is checked in isolation rather than only through the
// calc/power parity scenario.
func TestExtendGrammarJSONFieldMerges(t *testing.T) {
	base := NewGrammar("base")
	base.Define("program", Repeat(Sym("statement")))
	base.Define("statement", Sym("expression_statement"))
	base.Define("expression_statement", Seq(Sym("expression"), Str(";")))
	base.Define("expression", Choice(Sym("number"), Sym("identifier")))
	base.Define("identifier", Pat(`[a-z]+`))
	base.Define("number", Pat(`[0-9]+`))
	base.SetExtras(Pat(`\s`))
	base.SetConflicts([]string{"expression", "statement"})
	base.SetExternals(Sym("_external_token"))
	base.Inline = []string{"expression_statement"}
	base.Supertypes = []string{"expression"}
	base.Word = "identifier"

	baseJSON, err := ExportGrammarJSON(base)
	if err != nil {
		t.Fatalf("ExportGrammarJSON(base): %v", err)
	}

	delta := NewGrammar("delta")
	// New rule.
	delta.Define("print_statement", Seq(Str("print"), Sym("expression"), Str(";")))
	// Override an existing rule.
	delta.Define("statement", Choice(
		Sym("expression_statement"),
		Sym("print_statement"),
	))
	// Union: new extra.
	delta.SetExtras(Pat(`//[^\n]*`))
	// Union: new conflict group.
	delta.SetConflicts([]string{"print_statement", "statement"})
	// Union: new external.
	delta.SetExternals(Sym("_other_external_token"))
	// Union with de-dup: inline repeats an existing entry plus a new one.
	delta.Inline = []string{"expression_statement", "print_statement"}
	// Union with de-dup: supertypes repeats an existing entry plus a new one.
	delta.Supertypes = []string{"expression", "statement"}
	// Word is intentionally left unset here (empty): the "word overridden"
	// merge behavior is exercised in isolation by
	// TestExtendGrammarJSONWordOverride instead of on this grammar, since
	// swapping the word token away from the identifier pattern this
	// grammar's keyword extraction depends on would make the "compiles and
	// parses" check below exercise keyword-extraction fallout instead of
	// the merge plumbing it's meant to check.

	deltaJSON, err := ExportGrammarJSON(delta)
	if err != nil {
		t.Fatalf("ExportGrammarJSON(delta): %v", err)
	}

	g, err := ExtendGrammarJSON("extended", baseJSON, deltaJSON)
	if err != nil {
		t.Fatalf("ExtendGrammarJSON: %v", err)
	}

	if g.Name != "extended" {
		t.Errorf("Name = %q, want %q", g.Name, "extended")
	}

	t.Run("inherits base rule", func(t *testing.T) {
		if _, ok := g.Rules["number"]; !ok {
			t.Error("missing inherited rule 'number'")
		}
	})

	t.Run("adds new rule", func(t *testing.T) {
		if _, ok := g.Rules["print_statement"]; !ok {
			t.Error("missing new rule 'print_statement'")
		}
	})

	t.Run("overrides existing rule", func(t *testing.T) {
		stmt := g.Rules["statement"]
		if stmt.Kind != RuleChoice || len(stmt.Children) != 2 {
			t.Errorf("expected statement to be a choice of 2 after override, got kind=%d children=%d", stmt.Kind, len(stmt.Children))
		}
	})

	t.Run("base rule order preserved, new rule appended", func(t *testing.T) {
		want := []string{"program", "statement", "expression_statement", "expression", "identifier", "number", "print_statement"}
		if len(g.RuleOrder) != len(want) {
			t.Fatalf("RuleOrder = %v, want %v", g.RuleOrder, want)
		}
		for i, name := range want {
			if g.RuleOrder[i] != name {
				t.Errorf("RuleOrder[%d] = %q, want %q (full: %v)", i, g.RuleOrder[i], name, g.RuleOrder)
			}
		}
	})

	t.Run("extras union", func(t *testing.T) {
		if len(g.Extras) != 2 {
			t.Fatalf("Extras = %d entries, want 2 (base + delta): %+v", len(g.Extras), g.Extras)
		}
	})

	t.Run("conflicts union", func(t *testing.T) {
		if len(g.Conflicts) != 2 {
			t.Fatalf("Conflicts = %d groups, want 2 (base + delta): %+v", len(g.Conflicts), g.Conflicts)
		}
	})

	t.Run("externals union", func(t *testing.T) {
		if len(g.Externals) != 2 {
			t.Fatalf("Externals = %d entries, want 2 (base + delta): %+v", len(g.Externals), g.Externals)
		}
	})

	t.Run("inline union de-duplicated", func(t *testing.T) {
		want := []string{"expression_statement", "print_statement"}
		if len(g.Inline) != len(want) {
			t.Fatalf("Inline = %v, want %v", g.Inline, want)
		}
		for i, name := range want {
			if g.Inline[i] != name {
				t.Errorf("Inline[%d] = %q, want %q (full: %v)", i, g.Inline[i], name, g.Inline)
			}
		}
	})

	t.Run("supertypes union de-duplicated", func(t *testing.T) {
		want := []string{"expression", "statement"}
		if len(g.Supertypes) != len(want) {
			t.Fatalf("Supertypes = %v, want %v", g.Supertypes, want)
		}
		for i, name := range want {
			if g.Supertypes[i] != name {
				t.Errorf("Supertypes[%d] = %q, want %q (full: %v)", i, g.Supertypes[i], name, g.Supertypes)
			}
		}
	})

	t.Run("word inherited when delta doesn't specify one", func(t *testing.T) {
		if g.Word != "identifier" {
			t.Errorf("Word = %q, want inherited %q", g.Word, "identifier")
		}
	})

	t.Run("base grammar not mutated", func(t *testing.T) {
		if _, ok := base.Rules["print_statement"]; ok {
			t.Error("base grammar was mutated to include 'print_statement'")
		}
		if len(base.Extras) != 1 {
			t.Errorf("base grammar Extras mutated: %+v", base.Extras)
		}
	})

	t.Run("compiles and parses", func(t *testing.T) {
		lang, err := GenerateLanguage(g)
		if err != nil {
			t.Fatalf("GenerateLanguage: %v", err)
		}
		parser := gotreesitter.NewParser(lang)
		tree, err := parser.Parse([]byte("print 42;"))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		sexp := tree.RootNode().SExpr(lang)
		t.Logf("parse: %s", sexp)
		// print_statement (unioned into Inline by the delta, same as
		// expression_statement already was in the base) is expanded at its
		// use site and removed as a nonterminal per Grammar.Inline's
		// documented semantics -- so it will not appear as a named node
		// here. What this checks is that the merged grammar (new rule +
		// unioned extras/conflicts/externals/inline/supertypes) still
		// compiles and parses the new "print ...;" surface cleanly.
		if strings.Contains(sexp, "ERROR") {
			t.Fatalf("unexpected ERROR in tree: %s", sexp)
		}
	})
}

// TestExtendGrammarJSONWordOverride checks the "word" merge rule (delta
// replaces the base's word token when non-empty) in isolation, since
// swapping the word token interacts with keyword extraction and is easiest
// to verify as a plain field check rather than through a compiled grammar.
func TestExtendGrammarJSONWordOverride(t *testing.T) {
	base := NewGrammar("base")
	base.Define("start", Sym("identifier"))
	base.Define("identifier", Pat(`[a-z]+`))
	base.Word = "identifier"

	baseJSON, err := ExportGrammarJSON(base)
	if err != nil {
		t.Fatalf("ExportGrammarJSON(base): %v", err)
	}

	delta := NewGrammar("delta")
	delta.Define("keyword", Pat(`[A-Z]+`))
	delta.Word = "keyword"

	deltaJSON, err := ExportGrammarJSON(delta)
	if err != nil {
		t.Fatalf("ExportGrammarJSON(delta): %v", err)
	}

	g, err := ExtendGrammarJSON("extended", baseJSON, deltaJSON)
	if err != nil {
		t.Fatalf("ExtendGrammarJSON: %v", err)
	}
	if g.Word != "keyword" {
		t.Errorf("Word = %q, want delta override %q", g.Word, "keyword")
	}
	if base.Word != "identifier" {
		t.Errorf("base grammar Word was mutated to %q", base.Word)
	}
}

// TestApplyGrammarJSONDeltaReservedWordSets checks the reserved-word-set
// merge rule directly against applyGrammarJSONDelta: a delta set replaces a
// base set of the same name; sets the delta doesn't mention are left
// untouched; new set names are appended. This bypasses
// ExportGrammarJSON/ImportGrammarJSON (which doesn't round-trip "reserved"
// today) and ImportGrammarJSON's unrelated sawReservedNode pruning, both of
// which are orthogonal to the delta-merge logic being verified here.
func TestApplyGrammarJSONDeltaReservedWordSets(t *testing.T) {
	g := NewGrammar("base")
	g.Define("start", Sym("identifier"))
	g.Define("identifier", Pat(`[a-z]+`))
	g.ReservedWordSets = []ReservedWordSet{
		{Name: "global", Rules: []*Rule{Str("if")}},
		{Name: "loop", Rules: []*Rule{Str("break")}},
	}

	deltaJSON := []byte(`{
		"reserved": {
			"global": [
				{"type": "STRING", "value": "if"},
				{"type": "STRING", "value": "print"}
			],
			"extra_set": [
				{"type": "STRING", "value": "new"}
			]
		}
	}`)

	if err := applyGrammarJSONDelta(g, deltaJSON); err != nil {
		t.Fatalf("applyGrammarJSONDelta: %v", err)
	}

	byName := make(map[string]ReservedWordSet, len(g.ReservedWordSets))
	for _, set := range g.ReservedWordSets {
		byName[set.Name] = set
	}

	if len(g.ReservedWordSets) != 3 {
		t.Fatalf("ReservedWordSets = %d sets, want 3: %+v", len(g.ReservedWordSets), g.ReservedWordSets)
	}
	if set, ok := byName["global"]; !ok || len(set.Rules) != 2 {
		t.Errorf(`ReservedWordSets["global"] = %+v, want 2 rules (replaced by delta)`, set)
	}
	if set, ok := byName["loop"]; !ok || len(set.Rules) != 1 {
		t.Errorf(`ReservedWordSets["loop"] = %+v, want 1 rule (untouched by delta)`, set)
	}
	if set, ok := byName["extra_set"]; !ok || len(set.Rules) != 1 {
		t.Errorf(`ReservedWordSets["extra_set"] = %+v, want 1 rule (appended by delta)`, set)
	}
}

// TestExtendGrammarJSONInvalidJSON confirms errors are surfaced (not
// panics) for malformed base/delta JSON.
func TestExtendGrammarJSONInvalidJSON(t *testing.T) {
	validBase, err := ExportGrammarJSON(CalcGrammar())
	if err != nil {
		t.Fatalf("ExportGrammarJSON: %v", err)
	}

	t.Run("invalid base JSON", func(t *testing.T) {
		if _, err := ExtendGrammarJSON("x", []byte("{not json"), []byte(`{}`)); err == nil {
			t.Fatal("expected error for invalid base JSON, got nil")
		}
	})

	t.Run("invalid delta JSON", func(t *testing.T) {
		if _, err := ExtendGrammarJSON("x", validBase, []byte("{not json")); err == nil {
			t.Fatal("expected error for invalid delta JSON, got nil")
		}
	})

	t.Run("empty delta is a no-op clone", func(t *testing.T) {
		g, err := ExtendGrammarJSON("x", validBase, []byte(`{}`))
		if err != nil {
			t.Fatalf("ExtendGrammarJSON with empty delta: %v", err)
		}
		if len(g.Rules) == 0 {
			t.Fatal("expected inherited rules from base with an empty delta")
		}
		if _, err := GenerateLanguage(g); err != nil {
			t.Fatalf("GenerateLanguage: %v", err)
		}
	})
}
