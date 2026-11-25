package grammargen

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExportGrammarJSONRoundTrip(t *testing.T) {
	// Build a grammar that exercises every rule kind.
	g := NewGrammar("roundtrip_test")
	g.SetWord("identifier")

	g.Define("source_file", Repeat(Sym("_statement")))

	g.Define("_statement", Choice(
		Sym("assignment"),
		Sym("call_expression"),
	))

	g.Define("assignment", Seq(
		Field("name", Sym("identifier")),
		Str("="),
		Field("value", Sym("_expression")),
	))

	g.Define("_expression", Choice(
		Sym("identifier"),
		Sym("number"),
		Sym("string_literal"),
		Sym("call_expression"),
		Sym("binary_expression"),
		Sym("unary_expression"),
	))

	g.Define("binary_expression", PrecLeft(1, Seq(
		Field("left", Sym("_expression")),
		Field("op", Choice(Str("+"), Str("-"), Str("*"), Str("/"))),
		Field("right", Sym("_expression")),
	)))

	g.Define("unary_expression", PrecRight(2, Seq(
		Str("-"),
		Field("operand", Sym("_expression")),
	)))

	g.Define("call_expression", PrecDynamic(5, Seq(
		Field("function", Sym("identifier")),
		Str("("),
		Optional(CommaSep1(Sym("_expression"))),
		Str(")"),
	)))

	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))
	g.Define("number", Pat(`[0-9]+`))
	g.Define("string_literal", Token(Seq(
		Str("\""),
		Pat(`[^"]*`),
		Str("\""),
	)))

	g.Define("comment", Token(Seq(
		Str("//"),
		Pat(`[^\n]*`),
	)))

	// Alias: number aliased as "int_literal"
	g.Define("aliased_number", Alias(Sym("number"), "int_literal", true))

	// ImmToken
	g.Define("immediate_dot", ImmToken(Str(".")))

	// Repeat1
	g.Define("identifier_list", Repeat1(Sym("identifier")))

	// Prec (non-left, non-right)
	g.Define("prec_test", Prec(3, Sym("identifier")))

	g.SetExtras(Pat(`\s`), Sym("comment"))
	g.SetConflicts(
		[]string{"_statement", "call_expression"},
	)
	g.SetExternals(Sym("indent"), Sym("dedent"))
	g.SetInline("_statement")
	g.SetSupertypes("_expression")

	// Export.
	data, err := ExportGrammarJSON(g)
	if err != nil {
		t.Fatalf("ExportGrammarJSON: %v", err)
	}

	t.Logf("Exported JSON (%d bytes):\n%s", len(data), string(data))

	// Validate it's valid JSON.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("exported JSON is not valid: %v", err)
	}

	// Re-import.
	g2, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON: %v", err)
	}

	// Verify top-level fields.
	if g2.Name != g.Name {
		t.Errorf("Name: got %q, want %q", g2.Name, g.Name)
	}
	if g2.Word != g.Word {
		t.Errorf("Word: got %q, want %q", g2.Word, g.Word)
	}

	// Verify rule count and order.
	if len(g2.RuleOrder) != len(g.RuleOrder) {
		t.Errorf("RuleOrder length: got %d, want %d", len(g2.RuleOrder), len(g.RuleOrder))
	}
	for i, name := range g.RuleOrder {
		if i >= len(g2.RuleOrder) {
			break
		}
		if g2.RuleOrder[i] != name {
			t.Errorf("RuleOrder[%d]: got %q, want %q", i, g2.RuleOrder[i], name)
		}
	}

	// Verify all rules exist.
	for name := range g.Rules {
		if _, ok := g2.Rules[name]; !ok {
			t.Errorf("missing rule %q after round-trip", name)
		}
	}

	// Verify extras count.
	if len(g2.Extras) != len(g.Extras) {
		t.Errorf("Extras length: got %d, want %d", len(g2.Extras), len(g.Extras))
	}

	// Verify conflicts.
	if len(g2.Conflicts) != len(g.Conflicts) {
		t.Errorf("Conflicts length: got %d, want %d", len(g2.Conflicts), len(g.Conflicts))
	} else {
		for i, conflict := range g.Conflicts {
			if len(g2.Conflicts[i]) != len(conflict) {
				t.Errorf("Conflicts[%d] length: got %d, want %d", i, len(g2.Conflicts[i]), len(conflict))
			}
		}
	}

	// Verify externals count.
	if len(g2.Externals) != len(g.Externals) {
		t.Errorf("Externals length: got %d, want %d", len(g2.Externals), len(g.Externals))
	}

	// Verify inline.
	if len(g2.Inline) != len(g.Inline) {
		t.Errorf("Inline length: got %d, want %d", len(g2.Inline), len(g.Inline))
	}

	// Verify supertypes.
	if len(g2.Supertypes) != len(g.Supertypes) {
		t.Errorf("Supertypes length: got %d, want %d", len(g2.Supertypes), len(g.Supertypes))
	}

	// Deep verify specific rules survive the round-trip.
	// Check that binary_expression has PrecLeft wrapping.
	if r, ok := g2.Rules["binary_expression"]; ok {
		if r.Kind != RulePrecLeft {
			t.Errorf("binary_expression kind: got %v, want RulePrecLeft", r.Kind)
		}
		if r.Prec != 1 {
			t.Errorf("binary_expression prec: got %d, want 1", r.Prec)
		}
	}

	// Check that unary_expression has PrecRight wrapping.
	if r, ok := g2.Rules["unary_expression"]; ok {
		if r.Kind != RulePrecRight {
			t.Errorf("unary_expression kind: got %v, want RulePrecRight", r.Kind)
		}
		if r.Prec != 2 {
			t.Errorf("unary_expression prec: got %d, want 2", r.Prec)
		}
	}

	// Check that call_expression has PrecDynamic wrapping.
	if r, ok := g2.Rules["call_expression"]; ok {
		if r.Kind != RulePrecDynamic {
			t.Errorf("call_expression kind: got %v, want RulePrecDynamic", r.Kind)
		}
		if r.Prec != 5 {
			t.Errorf("call_expression prec: got %d, want 5", r.Prec)
		}
	}

	// Check alias round-trip.
	if r, ok := g2.Rules["aliased_number"]; ok {
		if r.Kind != RuleAlias {
			t.Errorf("aliased_number kind: got %v, want RuleAlias", r.Kind)
		}
		if r.Value != "int_literal" {
			t.Errorf("aliased_number value: got %q, want %q", r.Value, "int_literal")
		}
		if !r.Named {
			t.Error("aliased_number named: got false, want true")
		}
	}

	// Check prec_test round-trip.
	if r, ok := g2.Rules["prec_test"]; ok {
		if r.Kind != RulePrec {
			t.Errorf("prec_test kind: got %v, want RulePrec", r.Kind)
		}
		if r.Prec != 3 {
			t.Errorf("prec_test prec: got %d, want 3", r.Prec)
		}
	}
}

func TestExportGrammarJSONCalcRoundTrip(t *testing.T) {
	// Test with the built-in CalcGrammar.
	g := CalcGrammar()

	data, err := ExportGrammarJSON(g)
	if err != nil {
		t.Fatalf("ExportGrammarJSON: %v", err)
	}

	g2, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON: %v", err)
	}

	if g2.Name != g.Name {
		t.Errorf("Name: got %q, want %q", g2.Name, g.Name)
	}

	if len(g2.RuleOrder) != len(g.RuleOrder) {
		t.Errorf("RuleOrder length: got %d, want %d", len(g2.RuleOrder), len(g.RuleOrder))
	}

	for name := range g.Rules {
		if _, ok := g2.Rules[name]; !ok {
			t.Errorf("missing rule %q after round-trip", name)
		}
	}
}

func TestExportGrammarJSONRuleOrder(t *testing.T) {
	// Verify that rule order is preserved in the JSON output.
	g := NewGrammar("order_test")
	g.Define("zebra", Str("z"))
	g.Define("alpha", Str("a"))
	g.Define("mango", Str("m"))

	data, err := ExportGrammarJSON(g)
	if err != nil {
		t.Fatalf("ExportGrammarJSON: %v", err)
	}

	g2, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON: %v", err)
	}

	expected := []string{"zebra", "alpha", "mango"}
	if len(g2.RuleOrder) != len(expected) {
		t.Fatalf("RuleOrder length: got %d, want %d", len(g2.RuleOrder), len(expected))
	}
	for i, name := range expected {
		if g2.RuleOrder[i] != name {
			t.Errorf("RuleOrder[%d]: got %q, want %q", i, g2.RuleOrder[i], name)
		}
	}
}

// precedenceLevelsJSON returns the "precedences" field of a grammar.json
// document, re-encoded so that two documents compare by content.
func precedenceLevelsJSON(t *testing.T, data []byte) string {
	t.Helper()
	var doc struct {
		Precedences json.RawMessage `json:"precedences"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode grammar.json: %v", err)
	}
	if len(doc.Precedences) == 0 {
		return ""
	}
	var levels any
	if err := json.Unmarshal(doc.Precedences, &levels); err != nil {
		t.Fatalf("decode precedences: %v", err)
	}
	out, err := json.Marshal(levels)
	if err != nil {
		t.Fatalf("encode precedences: %v", err)
	}
	return string(out)
}

// TestExportGrammarJSONPrecedencesRoundTrip imports a grammar.json whose
// precedences hold named (STRING) and rule (SYMBOL) entries, exports it, and
// imports the export again. The precedence levels must survive both steps.
func TestExportGrammarJSONPrecedencesRoundTrip(t *testing.T) {
	src := []byte(`{
  "name": "prec_levels",
  "rules": {
    "expression": {"type": "CHOICE", "members": [
      {"type": "SYMBOL", "name": "binary"},
      {"type": "SYMBOL", "name": "unary"},
      {"type": "SYMBOL", "name": "number"}]},
    "binary": {"type": "CHOICE", "members": [
      {"type": "PREC_LEFT", "value": "add", "content": {"type": "SEQ", "members": [
        {"type": "SYMBOL", "name": "expression"}, {"type": "STRING", "value": "+"}, {"type": "SYMBOL", "name": "expression"}]}},
      {"type": "PREC_LEFT", "value": "mul", "content": {"type": "SEQ", "members": [
        {"type": "SYMBOL", "name": "expression"}, {"type": "STRING", "value": "*"}, {"type": "SYMBOL", "name": "expression"}]}}]},
    "unary": {"type": "PREC", "value": "neg", "content": {"type": "SEQ", "members": [
      {"type": "STRING", "value": "-"}, {"type": "SYMBOL", "name": "expression"}]}},
    "number": {"type": "PATTERN", "value": "[0-9]+"}
  },
  "extras": [{"type": "PATTERN", "value": "\\s"}],
  "conflicts": [],
  "precedences": [
    [{"type": "STRING", "value": "neg"}, {"type": "STRING", "value": "mul"}, {"type": "STRING", "value": "add"}],
    [{"type": "SYMBOL", "name": "unary"}, {"type": "SYMBOL", "name": "binary"}]
  ],
  "externals": [],
  "inline": [],
  "supertypes": []
}`)
	want := [][]PrecEntry{
		{{Name: "neg"}, {Name: "mul"}, {Name: "add"}},
		{{IsSymbol: true, Name: "unary"}, {IsSymbol: true, Name: "binary"}},
	}

	g1, err := ImportGrammarJSON(src)
	if err != nil {
		t.Fatalf("ImportGrammarJSON(source): %v", err)
	}
	if !reflect.DeepEqual(g1.Precedences, want) {
		t.Fatalf("import read precedences %v, want %v", g1.Precedences, want)
	}

	data, err := ExportGrammarJSON(g1)
	if err != nil {
		t.Fatalf("ExportGrammarJSON: %v", err)
	}
	if got, want := precedenceLevelsJSON(t, data), precedenceLevelsJSON(t, src); got != want {
		t.Fatalf("exported precedences = %s, want %s", got, want)
	}

	g2, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON(export): %v", err)
	}
	if !reflect.DeepEqual(g2.Precedences, want) {
		t.Fatalf("precedences after export and import = %v, want %v", g2.Precedences, want)
	}
	again, err := ExportGrammarJSON(g2)
	if err != nil {
		t.Fatalf("ExportGrammarJSON(second): %v", err)
	}
	if string(again) != string(data) {
		t.Fatalf("export is not stable across a second round trip:\nfirst:  %s\nsecond: %s", data, again)
	}
}

// TestExportGrammarJSONMarkdownPrecedences checks the three symbol
// precedence levels that MarkdownGrammar declares (setext headings and
// indented code over _block) through an export and import.
func TestExportGrammarJSONMarkdownPrecedences(t *testing.T) {
	g := MarkdownGrammar()
	if len(g.Precedences) != 3 {
		t.Fatalf("MarkdownGrammar declares %d precedence levels, want 3", len(g.Precedences))
	}
	data, err := ExportGrammarJSON(g)
	if err != nil {
		t.Fatalf("ExportGrammarJSON: %v", err)
	}
	g2, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON: %v", err)
	}
	if !reflect.DeepEqual(g2.Precedences, g.Precedences) {
		t.Fatalf("precedences after export and import = %v, want %v", g2.Precedences, g.Precedences)
	}
}
