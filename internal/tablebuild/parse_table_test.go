package tablebuild

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// The Rust files that this stage ports (item.rs, item_set_builder.rs,
// build_parse_table.rs, build_tables.rs) have no unit tests at v0.27.1. The
// generator's conflict behaviour is tested through the grammars of
// test/fixtures/test_grammars, with the expected message in
// expected_error.txt; TestConflictFixtures builds those grammars (written
// here as grammar.json) and compares the messages.

// tablesDiff returns the first difference between two builds, or "".
// Lookahead sets are compared by content: their ids depend on the order in
// which sets were first interned.
func tablesDiff(a, b *RawTables) string {
	ta, tb := a.Table, b.Table
	if len(ta.States) != len(tb.States) {
		return fmt.Sprintf("state count %d, %d", len(ta.States), len(tb.States))
	}
	if fmt.Sprint(ta.Symbols) != fmt.Sprint(tb.Symbols) {
		return fmt.Sprintf("symbols %v, %v", ta.Symbols, tb.Symbols)
	}
	if ta.MaxAliasedProductionLength != tb.MaxAliasedProductionLength {
		return "max aliased production length"
	}
	if len(ta.ProductionInfos) != len(tb.ProductionInfos) {
		return fmt.Sprintf("production info count %d, %d", len(ta.ProductionInfos), len(tb.ProductionInfos))
	}
	for i := range ta.ProductionInfos {
		if !ta.ProductionInfos[i].equal(&tb.ProductionInfos[i]) {
			return fmt.Sprintf("production info %d", i)
		}
	}
	for i := range ta.States {
		sa, sb := &ta.States[i], &tb.States[i]
		if fmt.Sprint(sa.TerminalEntries) != fmt.Sprint(sb.TerminalEntries) {
			return fmt.Sprintf("state %d terminal entries:\n%v\n%v", i, sa.TerminalEntries, sb.TerminalEntries)
		}
		if fmt.Sprint(sa.NonterminalEntries) != fmt.Sprint(sb.NonterminalEntries) {
			return fmt.Sprintf("state %d non-terminal entries:\n%v\n%v", i, sa.NonterminalEntries, sb.NonterminalEntries)
		}
		if !sa.ReservedWords.Equal(&sb.ReservedWords) || sa.CoreID != sb.CoreID || sa.HasEOFGatedReduce != sb.HasEOFGatedReduce {
			return fmt.Sprintf("state %d reserved words, core id or eof-gated flag", i)
		}
		ka, kb := &a.StateInfo.ItemSets[i], &b.StateInfo.ItemSets[i]
		if len(ka.Entries) != len(kb.Entries) {
			return fmt.Sprintf("state %d kernel size %d, %d", i, len(ka.Entries), len(kb.Entries))
		}
		for k := range ka.Entries {
			ea, eb := &ka.Entries[k], &kb.Entries[k]
			if ea.Item.VariableIndex != eb.Item.VariableIndex || ea.Item.StepIndex != eb.Item.StepIndex || ea.Item.ProdID != eb.Item.ProdID ||
				ea.Item.HasPrecedingInheritedFields != eb.Item.HasPrecedingInheritedFields || ea.FollowingReservedWordSet != eb.FollowingReservedWordSet ||
				!a.StateInfo.Lookaheads(ea).Equal(b.StateInfo.Lookaheads(eb)) {
				return fmt.Sprintf("state %d kernel entry %d", i, k)
			}
		}
		if fmt.Sprint(a.StateInfo.PrecedingSymbolsByID[i]) != fmt.Sprint(b.StateInfo.PrecedingSymbolsByID[i]) {
			return fmt.Sprintf("state %d preceding symbols", i)
		}
	}
	return ""
}

// TestClosureOrderDoesNotMatter builds each grammar with the closure
// additions of every non-terminal made in ascending and in descending
// variable order (the generator iterates a hash map there; see
// newParseItemSetBuilder) and requires equal tables, kernels and paths.
func TestClosureOrderDoesNotMatter(t *testing.T) {
	for _, name := range l2Grammars {
		t.Run(name, func(t *testing.T) {
			grammarJSON := readTestdataGrammar(t, name)
			_, _, up := buildRaw(t, grammarJSON, ascendingVariables)
			_, _, down := buildRaw(t, grammarJSON, descendingVariables)
			if d := tablesDiff(up, down); d != "" {
				t.Fatalf("ascending and descending closure orders differ: %s", d)
			}
			t.Logf("%d raw states equal in both orders", len(up.Table.States))
		})
	}
}

// TestBuildIsDeterministic builds Go twice and requires equal results: the
// port iterates Go maps only where the comment at that place argues that the
// order reaches no result.
func TestBuildIsDeterministic(t *testing.T) {
	grammarJSON := readTestdataGrammar(t, "go")
	_, _, a := buildRaw(t, grammarJSON, ascendingVariables)
	_, _, b := buildRaw(t, grammarJSON, ascendingVariables)
	if d := tablesDiff(a, b); d != "" {
		t.Fatalf("two builds differ: %s", d)
	}
}

// --- grammar.json helpers for small test grammars ---

func jPrec(kind string, value any, content string) string {
	v := fmt.Sprint(value)
	if s, ok := value.(string); ok {
		v = fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf(`{"type":%q,"value":%s,"content":%s}`, kind, v, content)
}

func jRepeat0(content string) string { return `{"type":"REPEAT","content":` + content + `}` }

// tGrammar is a grammar.json with the fields the conflict fixtures use. The
// grammar DSL writes extras [/\s/] when a grammar gives none.
type tGrammar struct {
	rules       [][2]string
	extras      []string
	externals   []string
	conflicts   [][]string
	precedences [][]string
	word        string
}

func (g tGrammar) text() string {
	var rules []string
	for _, r := range g.rules {
		rules = append(rules, fmt.Sprintf("%q:%s", r[0], r[1]))
	}
	extras := g.extras
	if extras == nil {
		extras = []string{jPat(`\s`)}
	}
	var conflicts []string
	for _, c := range g.conflicts {
		var q []string
		for _, n := range c {
			q = append(q, fmt.Sprintf("%q", n))
		}
		conflicts = append(conflicts, "["+strings.Join(q, ",")+"]")
	}
	var precs []string
	for _, p := range g.precedences {
		precs = append(precs, "["+strings.Join(p, ",")+"]")
	}
	s := `{"name":"t","rules":{` + strings.Join(rules, ",") + `},"extras":[` + strings.Join(extras, ",") +
		`],"externals":[` + strings.Join(g.externals, ",") + `],"conflicts":[` + strings.Join(conflicts, ",") +
		`],"precedences":[` + strings.Join(precs, ",") + `],"inline":[],"supertypes":[]`
	if g.word != "" {
		s += fmt.Sprintf(`,"word":%q`, g.word)
	}
	return s + "}"
}

// buildText prepares a grammar.json and builds its raw tables.
func buildText(t *testing.T, text string) (*RawTables, error) {
	t.Helper()
	g, err := Prepare([]byte(text))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatalf("variable info: %v", err)
	}
	return BuildRawTables(g, info)
}

// conflictFixtures are grammars of the generator's test/fixtures/test_grammars
// (v0.27.1), written as the grammar.json that the grammar DSL produces, with
// the expected_error.txt of each one that has an error.
var conflictFixtures = []struct {
	name    string
	grammar tGrammar
	want    string
}{
	{
		name: "conflicting_precedence",
		grammar: tGrammar{rules: [][2]string{
			{"expression", jChoice(jSym("sum"), jSym("product"), jSym("other_thing"))},
			{"sum", jPrec("PREC_LEFT", 0, jSeq(jSym("expression"), jStr("+"), jSym("expression")))},
			{"product", jPrec("PREC_LEFT", 1, jSeq(jSym("expression"), jStr("*"), jSym("expression")))},
			{"other_thing", jPrec("PREC_LEFT", -1, jSeq(jSym("expression"), jStr("*"), jStr("*")))},
			{"identifier", jPat("[a-zA-Z]+")},
		}},
		want: `Unresolved conflict for symbol sequence:

  expression  '+'  expression  •  '*'  …

Possible interpretations:

  1:  (sum  expression  '+'  expression)  •  '*'  …               (precedence: 0, associativity: Left)
  2:  expression  '+'  (other_thing  expression  •  '*'  '*')     (precedence: -1, associativity: Left)
  3:  expression  '+'  (product  expression  •  '*'  expression)  (precedence: 1, associativity: Left)

Possible resolutions:

  1:  Specify a higher precedence in ` + "`product` and `other_thing`" + ` than in the other rules.
  2:  Specify a higher precedence in ` + "`sum`" + ` than in the other rules.
  3:  Add a conflict for these rules: ` + "`sum`, `product`, `other_thing`",
	},
	{
		name: "partially_resolved_conflict",
		grammar: tGrammar{rules: [][2]string{
			{"expression", jChoice(jSym("binary"), jSym("identifier"))},
			{"unary_a", jPrec("PREC", 2, jSeq(jStr("!"), jSym("expression")))},
			{"unary_b", jPrec("PREC", 2, jSeq(jStr("!"), jSym("expression")))},
			{"binary", jSeq(jChoice(jSym("unary_a"), jSym("unary_b"), jSym("expression")), jStr("<"), jSym("expression"))},
			{"identifier", jPat("[a-z]+")},
		}},
		want: `Unresolved conflict for symbol sequence:

  '!'  expression  •  '<'  …

Possible interpretations:

  1:  (unary_a  '!'  expression)  •  '<'  …  (precedence: 2)
  2:  (unary_b  '!'  expression)  •  '<'  …  (precedence: 2)

Possible resolutions:

  1:  Specify a higher precedence in ` + "`unary_a`" + ` than in the other rules.
  2:  Specify a higher precedence in ` + "`unary_b`" + ` than in the other rules.
  3:  Add a conflict for these rules: ` + "`unary_a`, `unary_b`",
	},
	{
		name: "associativity_missing",
		grammar: tGrammar{rules: [][2]string{
			{"expression", jChoice(jSym("math_operation"), jSym("identifier"))},
			{"math_operation", jSeq(jSym("expression"), jStr("+"), jSym("expression"))},
			{"identifier", jPat("[a-z]+")},
		}},
		want: `Unresolved conflict for symbol sequence:

  expression  '+'  expression  •  '+'  …

Possible interpretations:

  1:  (math_operation  expression  '+'  expression)  •  '+'  …
  2:  expression  '+'  (math_operation  expression  •  '+'  expression)

Possible resolutions:

  1:  Specify a left or right associativity in ` + "`math_operation`" + `
  2:  Add a conflict for these rules: ` + "`math_operation`",
	},
	{
		name: "precedence_on_single_child_missing",
		grammar: tGrammar{rules: [][2]string{
			{"expression", jChoice(jSym("function_call"), jSym("identifier"))},
			{"function_call", jPrec("PREC_RIGHT", 0, jChoice(
				jSeq(jSym("identifier"), jSym("expression")),
				jSeq(jSym("identifier"), jSym("block")),
				jSeq(jSym("identifier"), jSym("expression"), jSym("block")),
			))},
			{"block", jSeq(jStr("{"), jSym("expression"), jStr("}"))},
			{"identifier", jPat("[a-zA-Z]+")},
		}},
		want: `Unresolved conflict for symbol sequence:

  identifier  identifier  •  '{'  …

Possible interpretations:

  1:  identifier  (expression  identifier)  •  '{'  …
  2:  identifier  (function_call  identifier  •  block)  (precedence: 0, associativity: Right)

Possible resolutions:

  1:  Specify a higher precedence in ` + "`function_call`" + ` than in the other rules.
  2:  Specify a higher precedence in ` + "`expression`" + ` than in the other rules.
  3:  Specify a left or right associativity in ` + "`expression`" + `
  4:  Add a conflict for these rules: ` + "`expression`, `function_call`",
	},
	{
		name: "conflict_in_repeat_rule",
		grammar: tGrammar{rules: [][2]string{
			{"statement", jChoice(jSeq(jSym("array"), jStr(";")), jSeq(jSym("array_type"), jSym("identifier"), jStr(";")))},
			{"array", jSeq(jStr("["), jRepeat0(jChoice(jSym("identifier"), jStr("0"))), jStr("]"))},
			{"array_type", jSeq(jStr("["), jRepeat0(jChoice(jSym("identifier"), jStr("void"))), jStr("]"))},
			{"identifier", jPat("[a-z]+")},
		}},
		want: `Unresolved conflict for symbol sequence:

  '['  identifier  •  ']'  …

Possible interpretations:

  1:  '['  (array_repeat1  identifier)  •  ']'  …
  2:  '['  (array_type_repeat1  identifier)  •  ']'  …

Possible resolutions:

  1:  Specify a higher precedence in ` + "`array_repeat1`" + ` than in the other rules.
  2:  Specify a higher precedence in ` + "`array_type_repeat1`" + ` than in the other rules.
  3:  Add a conflict for these rules: ` + "`array`, `array_type`",
	},
	{
		name: "conflict_in_repeat_rule_after_external_token",
		grammar: tGrammar{
			externals: []string{jSym("_program_start")},
			rules: [][2]string{
				{"statement", jChoice(
					jSeq(jSym("_program_start"), jSym("array"), jStr(";")),
					jSeq(jSym("_program_start"), jSym("array_type"), jSym("identifier"), jStr(";")),
				)},
				{"array", jSeq(jStr("["), jRepeat0(jChoice(jSym("identifier"), jStr("0"))), jStr("]"))},
				{"array_type", jSeq(jStr("["), jRepeat0(jChoice(jSym("identifier"), jStr("void"))), jStr("]"))},
				{"identifier", jPat("[a-z]+")},
			},
		},
		want: `Unresolved conflict for symbol sequence:

  _program_start  '['  identifier  •  ']'  …

Possible interpretations:

  1:  _program_start  '['  (array_repeat1  identifier)  •  ']'  …
  2:  _program_start  '['  (array_type_repeat1  identifier)  •  ']'  …

Possible resolutions:

  1:  Specify a higher precedence in ` + "`array_repeat1`" + ` than in the other rules.
  2:  Specify a higher precedence in ` + "`array_type_repeat1`" + ` than in the other rules.
  3:  Add a conflict for these rules: ` + "`array`, `array_type`",
	},
	{
		name: "associativity_left_with_lower_precedence_shift",
		grammar: tGrammar{rules: [][2]string{
			{"expression", jChoice(jSym("addition"), jSym("superaddition"), jSym("number"))},
			{"addition", jPrec("PREC_LEFT", 1, jSeq(jSym("expression"), jStr("+"), jSym("expression")))},
			{"superaddition", jPrec("PREC_RIGHT", 0, jSeq(jSym("expression"), jStr("+"), jStr("+"), jSym("expression")))},
			{"number", jStr("1")},
		}},
	},
	{
		name: "associativity_right_with_lower_precedence_shift",
		grammar: tGrammar{rules: [][2]string{
			{"expression", jChoice(jSym("addition"), jSym("superaddition"), jSym("number"))},
			{"addition", jPrec("PREC_RIGHT", 1, jSeq(jSym("expression"), jStr("+"), jSym("expression")))},
			{"superaddition", jPrec("PREC_RIGHT", 0, jSeq(jSym("expression"), jStr("+"), jStr("+"), jSym("expression")))},
			{"number", jStr("1")},
		}},
	},
	{
		name: "duplicate_reduction_precedence",
		grammar: tGrammar{rules: [][2]string{
			{"start", jChoice(jSym("a"), jSym("c"))},
			{"a", jChoice(jSym("x"), jPrec("PREC", 2, jSym("x")))},
			{"c", jSym("x")},
			{"x", jStr("x")},
		}},
	},
	{
		name: "named_precedences",
		grammar: tGrammar{
			conflicts: [][]string{{"expression", "type"}, {"expression", "nested_type"}},
			precedences: [][]string{
				{jSym("member_expression"), jStr("and"), jStr("or")},
				{jSym("nested_type"), jStr("type_intersection"), jStr("type_union")},
			},
			rules: [][2]string{
				{"program", jRepeat0(jChoice(jSym("expression_statement"), jSym("declaration_statement")))},
				{"expression_statement", jSeq(jSym("expression"), jStr(";"))},
				{"declaration_statement", jSeq(jSym("type"), jSym("expression"), jStr(";"))},
				{"expression", jChoice(jSym("member_expression"), jSym("binary_expression"), jSym("identifier"))},
				{"member_expression", jSeq(jSym("expression"), jStr("."), jSym("identifier"))},
				{"binary_expression", jChoice(
					jPrec("PREC_LEFT", "or", jSeq(jSym("expression"), jStr("||"), jSym("expression"))),
					jPrec("PREC_LEFT", "and", jSeq(jSym("expression"), jStr("&&"), jSym("expression"))),
				)},
				{"type", jChoice(jSym("nested_type"), jSym("binary_type"), jSym("identifier"))},
				{"nested_type", jSeq(jSym("identifier"), jStr("."), jSym("identifier"))},
				{"binary_type", jChoice(
					jPrec("PREC_LEFT", "type_union", jSeq(jSym("type"), jStr("||"), jSym("type"))),
					jPrec("PREC_LEFT", "type_intersection", jSeq(jSym("type"), jStr("&&"), jSym("type"))),
				)},
				{"identifier", jPat(`[a-z]\w+`)},
			},
		},
	},
}

func TestConflictFixtures(t *testing.T) {
	for _, fx := range conflictFixtures {
		t.Run(fx.name, func(t *testing.T) {
			raw, err := buildText(t, fx.grammar.text())
			if fx.want == "" {
				if err != nil {
					t.Fatalf("want no error, got:\n%v", err)
				}
				t.Logf("%d raw states, diagnostics %+v", len(raw.Table.States), raw.Diagnostics)
				return
			}
			var ce *ConflictError
			if !errors.As(err, &ce) {
				t.Fatalf("want a conflict error, got %v", err)
			}
			got := strings.TrimRight(err.Error(), "\n")
			if got != fx.want {
				t.Fatalf("message differs from expected_error.txt:\n--- got\n%s\n--- want\n%s", got, fx.want)
			}
		})
	}
}

// TestDeclaredConflictKeepsActions declares the conflict of
// associativity_missing: the table keeps both actions, and the build has no
// unnecessary-conflict diagnostic.
func TestDeclaredConflictKeepsActions(t *testing.T) {
	g := tGrammar{
		conflicts: [][]string{{"math_operation"}},
		rules: [][2]string{
			{"expression", jChoice(jSym("math_operation"), jSym("identifier"))},
			{"math_operation", jSeq(jSym("expression"), jStr("+"), jSym("expression"))},
			{"identifier", jPat("[a-z]+")},
		},
	}
	raw, err := buildText(t, g.text())
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Diagnostics.UnnecessaryConflicts) != 0 {
		t.Fatalf("unexpected diagnostics: %v", raw.Diagnostics.UnnecessaryConflicts)
	}
	double := 0
	for i := range raw.Table.States {
		for _, e := range raw.Table.States[i].TerminalEntries {
			if len(e.Entry.Actions) == 2 && e.Entry.Actions[0].Kind == ActionReduce && e.Entry.Actions[1].Kind == ActionShift {
				double++
			}
		}
	}
	if double == 0 {
		t.Fatal("no entry keeps both the Reduce and the Shift action")
	}

	// A declared conflict that no state needs is reported.
	g.conflicts = append(g.conflicts, []string{"math_operation", "expression"})
	raw, err = buildText(t, g.text())
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(raw.Diagnostics.UnnecessaryConflicts); got != "[[expression math_operation]]" {
		t.Fatalf("unnecessary conflicts %s", got)
	}
}

// TestRepetitionShift checks the repeat-rule branch of handle_conflict: a
// Shift/Reduce conflict inside one auxiliary rule keeps both actions and
// marks the Shift as a repetition.
func TestRepetitionShift(t *testing.T) {
	g := tGrammar{rules: [][2]string{
		{"list", jRepeat0(jSym("item"))},
		{"item", jPat("[a-z]+")},
	}}
	raw, err := buildText(t, g.text())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range raw.Table.States {
		for _, e := range raw.Table.States[i].TerminalEntries {
			for _, a := range e.Entry.Actions {
				if a.Kind == ActionShift && a.IsRepetition {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("no Shift action is marked as a repetition")
	}
}

// TestImproperNonTerminalExtra is the ImproperNonTerminalExtra error of
// build: a non-terminal extra whose production starts with a non-terminal.
func TestImproperNonTerminalExtra(t *testing.T) {
	g := tGrammar{
		extras: []string{jPat(`\s`), jSym("comment")},
		rules: [][2]string{
			{"program", jRepeat0(jSym("word"))},
			{"word", jPat("[a-z]+")},
			{"comment", jSeq(jSym("marker"), jStr("#"))},
			{"marker", jSeq(jStr("%"), jStr("!"))},
		},
	}
	_, err := buildText(t, g.text())
	var e *ImproperNonTerminalExtraError
	if !errors.As(err, &e) || e.Name != "marker" {
		t.Fatalf("want ImproperNonTerminalExtra for marker, got %v", err)
	}
}

// TestNonTerminalExtraStates checks the states of non-terminal extras: one
// per distinct kernel of the productions grouped by first terminal, created
// right after the start state, with a Shift to it in every other state. Two
// productions whose kernels are equal items (equal after the dot; the symbols
// before it do not count) share one state, as in the generator.
func TestNonTerminalExtraStates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		second  string
		targets []uint32
	}{
		{"distinct kernels", jSeq(jStr("%"), jStr("%")), []uint32{2, 3}},
		{"equal kernels", jSeq(jStr("%"), jPat("[0-9]+")), []uint32{2, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := tGrammar{
				extras: []string{jPat(`\s`), jSym("comment")},
				rules: [][2]string{
					{"program", jRepeat0(jSym("word"))},
					{"word", jPat("[a-z]+")},
					{"comment", jChoice(jSeq(jStr("#"), jPat("[0-9]+")), tc.second)},
				},
			}
			raw, err := buildText(t, g.text())
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range tc.targets {
				k := raw.StateInfo.ItemSets[id]
				if len(k.Entries) != 1 || k.Entries[0].Item.StepIndex != 1 || !raw.StateInfo.Lookaheads(&k.Entries[0]).Contains(lexbuild.EndOfNonTerminalExtraSym()) {
					t.Fatalf("state %d is not a non-terminal extra start state: %+v", id, k)
				}
			}
			prepared, err := Prepare([]byte(g.text()))
			if err != nil {
				t.Fatal(err)
			}
			var got []uint32
			for _, e := range raw.Table.States[1].TerminalEntries {
				name := prepared.Lex.Variables[e.Symbol.Index].Ident
				if e.Symbol.IsTerminal() && (name == "#" || name == "%") && len(e.Entry.Actions) == 1 && e.Entry.Actions[0].Kind == ActionShift && e.Entry.Reusable {
					got = append(got, e.Entry.Actions[0].State)
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.targets) {
				t.Fatalf("state 1 shifts to the extra states %v, want %v: %v", got, tc.targets, raw.Table.States[1].TerminalEntries)
			}
		})
	}
}

// TestWordTokenSymbolSlot checks populate_used_symbols: the word token takes
// symbol slot 1.
func TestWordTokenSymbolSlot(t *testing.T) {
	g := tGrammar{
		word: "identifier",
		rules: [][2]string{
			{"program", jRepeat0(jChoice(jSym("statement"), jSym("identifier")))},
			{"statement", jSeq(jStr("if"), jSym("identifier"))},
			{"identifier", jPat("[a-z]+")},
		},
	}
	raw, err := buildText(t, g.text())
	if err != nil {
		t.Fatal(err)
	}
	s := raw.Table.Symbols
	if len(s) < 2 || s[0] != lexbuild.EndSym() || !s[1].IsTerminal() {
		t.Fatalf("symbols %v", s)
	}
	var lexNames []string
	g2, _ := Prepare([]byte(g.text()))
	for _, v := range g2.Lex.Variables {
		lexNames = append(lexNames, v.Ident)
	}
	if lexNames[s[1].Index] != "identifier" {
		t.Fatalf("symbol 1 is %q, want the word token (lexical variables %v)", lexNames[s[1].Index], lexNames)
	}
	// populate_error_state gives the word token a Recover entry whatever it
	// conflicts with.
	for _, e := range raw.Table.States[0].TerminalEntries {
		if e.Symbol == s[1] && (len(e.Entry.Actions) != 1 || e.Entry.Actions[0].Kind != ActionRecover) {
			t.Fatalf("word token entry in the error state: %+v", e)
		}
	}
}

// TestBudgetStopsBuild lowers the parse-state budget: the build stops with a
// *lexbuild.SizeError.
func TestBudgetStopsBuild(t *testing.T) {
	old := bParseStates.limit
	bParseStates.limit = 10
	defer func() { bParseStates.limit = old }()
	_, err := buildText(t, string(readTestdataGrammar(t, "json")))
	var se *lexbuild.SizeError
	if !errors.As(err, &se) || se.What != bParseStates.what {
		t.Fatalf("want a size error for %q, got %v", bParseStates.what, err)
	}
}

// TestComparePrecedence covers build_parse_table.rs: fn compare_precedence.
func TestComparePrecedence(t *testing.T) {
	sg := &lexbuild.SyntaxGrammar{
		Variables: []lexbuild.SyntaxVariable{{Ident: "a"}, {Ident: "b"}},
		PrecedenceOrderings: [][]lexbuild.PrecedenceEntry{
			{{IsSymbol: true, Ident: "a"}, {Ident: "x"}, {Ident: "y"}},
		},
	}
	none := lexbuild.Precedence{}
	i := func(v int32) lexbuild.Precedence { return lexbuild.Precedence{Kind: lexbuild.PrecInteger, Int: v} }
	n := func(s string) lexbuild.Precedence { return lexbuild.Precedence{Kind: lexbuild.PrecName, Ident: s} }
	a, b := []Symbol{lexbuild.NonTerminalSym(0)}, []Symbol{lexbuild.NonTerminalSym(1)}
	cases := []struct {
		l    lexbuild.Precedence
		ls   []Symbol
		r    lexbuild.Precedence
		rs   []Symbol
		want int
	}{
		{i(1), b, i(2), b, -1},
		{i(0), b, i(0), b, 0},
		{i(3), b, none, b, 1},
		{none, b, i(-1), b, 1},
		{n("x"), b, n("y"), b, 1},  // earlier in the list is higher
		{n("y"), b, n("x"), b, -1}, // later is lower
		{none, a, n("x"), b, 1},    // symbol entry a comes first
		{n("z"), b, n("x"), b, 0},  // z is in no list
		{i(1), b, n("x"), b, 0},    // integer against name: no ordering
	}
	for k, c := range cases {
		if got := comparePrecedence(sg, c.l, c.ls, c.r, c.rs); got != c.want {
			t.Errorf("case %d: got %d, want %d", k, got, c.want)
		}
	}
}

// TestLookaheadSetPool checks that ids are canonical and the memos agree.
func TestLookaheadSetPool(t *testing.T) {
	p := newLookaheadSetPool()
	x := p.singleton(lexbuild.TerminalSym(3))
	y := p.singleton(lexbuild.EndSym())
	if p.union(x, emptyLookaheads) != x || p.union(emptyLookaheads, y) != y || p.union(x, x) != x {
		t.Fatal("union with empty or self")
	}
	xy := p.union(x, y)
	if p.union(y, x) != xy {
		t.Fatal("union is not symmetric")
	}
	if p.insert(x, lexbuild.EndSym()) != xy {
		t.Fatal("insert gives a different id than union for the same set")
	}
	var s lexbuild.TokenSet
	s.Insert(lexbuild.EndSym())
	s.Insert(lexbuild.TerminalSym(3))
	if p.internRef(&s) != xy {
		t.Fatal("intern of an equal set gives a new id")
	}
	if !p.get(xy).Contains(lexbuild.TerminalSym(3)) || !p.get(xy).Contains(lexbuild.EndSym()) || p.get(xy).Len() != 2 {
		t.Fatal("union content")
	}
}

// TestItemKeyMapRanks checks ItemKeyMap::new on Go: equal content shares a
// cmp rank, ranks follow the content order, and eqWithSyms refines cmp.
func TestItemKeyMapRanks(t *testing.T) {
	g, err := Prepare(readTestdataGrammar(t, "go"))
	if err != nil {
		t.Fatal(err)
	}
	sg := g.Syntax
	m := NewItemKeyMap(sg)
	type pd struct {
		id  uint32
		dot int
	}
	var all []pd
	for id := range sg.Productions {
		for dot := 0; dot <= len(sg.Productions[id].Steps); dot++ {
			all = append(all, pd{uint32(id), dot})
		}
	}
	content := func(x pd) itemContent { return itemContent{production(sg, x.id), x.dot} }
	byCmp := map[uint32]uint32{}
	for k := 0; k < len(all); k += 7 {
		for j := k + 1; j < len(all) && j < k+200; j += 3 {
			a, b := all[k], all[j]
			ka, kb := m.keysFor(a.id)[a.dot], m.keysFor(b.id)[b.dot]
			c := content(a).cmp(content(b))
			if (c < 0) != (ka.cmp < kb.cmp) || (c == 0) != (ka.cmp == kb.cmp) {
				t.Fatalf("rank order differs from content order at %v %v", a, b)
			}
			if ka.eqWithSyms == kb.eqWithSyms && ka.cmp != kb.cmp {
				t.Fatalf("eqWithSyms does not refine cmp at %v %v", a, b)
			}
		}
	}
	for _, x := range all {
		k := m.keysFor(x.id)[x.dot]
		if c, ok := byCmp[k.eqWithSyms]; ok && c != k.cmp {
			t.Fatalf("eqWithSyms %d spans two cmp classes", k.eqWithSyms)
		}
		byCmp[k.eqWithSyms] = k.cmp
	}
	if len(m.startKeys()) != 2 {
		t.Fatalf("start production keys %d, want 2", len(m.startKeys()))
	}
}

// BenchmarkBuildRawTables measures BuildRawTables alone (the grammar is
// prepared once) and reports the raw state count and the closure work.
func BenchmarkBuildRawTables(b *testing.B) {
	for _, name := range l2Grammars {
		b.Run(name, func(b *testing.B) {
			g, err := Prepare(readTestdataGrammar(b, name))
			if err != nil {
				b.Fatal(err)
			}
			info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			var raw *RawTables
			for i := 0; i < b.N; i++ {
				// BuildRawTables only reads the prepared grammar, so one
				// preparation serves every iteration.
				if raw, err = BuildRawTables(g, info); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(raw.Table.States)), "states")
			b.ReportMetric(float64(raw.ClosureEntries), "closure-insertions")
		})
	}
}
