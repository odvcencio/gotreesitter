package lexbuild

import (
	"strings"
	"testing"
)

func gj(rules string, extra string) []byte {
	return []byte(`{"name":"t","rules":{` + rules + `}` + extra + `}`)
}

func TestClassLeadingBracketIsLiteral(t *testing.T) {
	h, err := parsePattern(`[]-a]`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := hirString(h); got != `[2D,5D,61,]` {
		t.Errorf("[]-a]: got %s, want the three characters ] - a", got)
	}
	h, err = parsePattern(`[^]a]`, false)
	if err != nil || hirString(h) != `[0-5C,5E-60,62-10FFFF,]` {
		t.Errorf("[^]a]: %v %v", h, err)
	}
}

func TestSetOperations(t *testing.T) {
	cases := map[string]string{
		`[+--a]`:          `L"+"`,
		`[--a]`:           `[2D,61,]`,
		`[--]`:            `L"-"`,
		`[-]]`:            `L"-]"`,
		`[a-z&&[^aeiou]]`: `[62-64,66-68,6A-6E,70-74,76-7A,]`,
		`[a-f~~d-k]`:      `[61-63,67-6B,]`,
		`[a-z--[b-y]]`:    `[61,7A,]`,
		`[a-e--b--d]`:     `[61,63,65,]`,
		`[^a-c&&b-d]`:     `[0-61,64-10FFFF,]`,
		`(?i)[a-c--b]`:    `[41,43,61,63,]`,
	}
	for pat, want := range cases {
		h, err := parsePattern(pat, false)
		if err != nil {
			t.Errorf("%s: %v", pat, err)
			continue
		}
		if got := hirString(h); got != want {
			t.Errorf("%s: got %s, want %s", pat, got, want)
		}
	}
	for _, bad := range []string{`[a-\p{L}]`} {
		if _, err := parsePattern(bad, false); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
	// Empty operands are valid: regex_syntax ast/parse.rs:868 and :934 (push_class_op takes
	// the union as it is, empty or not) and hir/translate.rs:616 (the empty class).
	for pat, want := range map[string]string{`[a--]`: `L"a"`, `[a&&]`: `[]`, `[&&a]`: `[]`, `[a&&[b--b]]`: `[]`} {
		h, err := parsePattern(pat, false)
		if err != nil || hirString(h) != want {
			t.Errorf("%s: got %v %v, want %s", pat, h, err, want)
		}
	}
}

func TestDuplicateRuleKeysKeepFirstPositionLastValue(t *testing.T) {
	p, err := Prepare(gj(`"source":{"type":"STRING","value":"a"},"x":{"type":"STRING","value":"q"},"source":{"type":"STRING","value":"b"}`, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Lex.Variables) != 1 || p.Lex.Variables[0].Ident != "b" {
		t.Errorf("tokens %+v: the later value of a repeated key must win", p.Lex.Variables)
	}
}

func TestLeadingCountedRepetitionRefused(t *testing.T) {
	for _, pat := range []string{`{2}`, `a|{2}`, `({2})`, `a{`, `a{x}`} {
		if _, err := parsePattern(pat, false); err == nil {
			t.Errorf("%s: want an error", pat)
		}
	}
	if _, err := parsePattern(`a{2}b{1,}c{1,2}`, false); err != nil {
		t.Error(err)
	}
}

func TestNestingBounded(t *testing.T) {
	deepClass := strings.Repeat("[", 300) + "a" + strings.Repeat("]", 300)
	deepGroup := strings.Repeat("(?:", 300) + "a" + strings.Repeat(")", 300)
	for name, pat := range map[string]string{"class": deepClass, "group": deepGroup} {
		if _, err := parsePattern(pat, false); err == nil || !strings.Contains(err.Error(), "nest limit") {
			t.Errorf("%s: want the nest limit error, got %v", name, err)
		}
	}
	if _, err := parsePattern(strings.Repeat("(?:", 100)+"a"+strings.Repeat(")", 100), false); err != nil {
		t.Error(err)
	}
}

func TestJSONStrictness(t *testing.T) {
	bad := map[string]string{
		"null pattern value": `"a":{"type":"PATTERN","value":null}`,
		"missing value":      `"a":{"type":"STRING"}`,
		"null prec value":    `"a":{"type":"PREC","value":null,"content":{"type":"STRING","value":"x"}}`,
		"missing content":    `"a":{"type":"TOKEN"}`,
		"unknown type":       `"a":{"type":"WOBBLE"}`,
		"non-string value":   `"a":{"type":"STRING","value":3}`,
		"non-bool named":     `"a":{"type":"ALIAS","named":"yes","value":"v","content":{"type":"STRING","value":"x"}}`,
		"lone surrogate":     `"a":{"type":"STRING","value":"\ud800"}`,
		"members not array":  `"a":{"type":"SEQ","members":{}}`,
		"float dynamic prec": `"a":{"type":"PREC_DYNAMIC","value":1.5,"content":{"type":"STRING","value":"x"}}`,
		"missing field name": `"a":{"type":"SYMBOL"}`,
		"missing context":    `"a":{"type":"RESERVED","content":{"type":"STRING","value":"x"}}`,
		"flags wrong type":   `"a":{"type":"PATTERN","value":"x","flags":3}`,
		"value out of range": `"a":{"type":"PREC","value":4294967296,"content":{"type":"STRING","value":"x"}}`,
	}
	for name, rules := range bad {
		if _, err := ParseGrammar(gj(rules, "")); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	for name, doc := range map[string]string{
		"no name":         `{"rules":{"a":{"type":"BLANK"}}}`,
		"no rules":        `{"name":"t"}`,
		"null extras":     `{"name":"t","rules":{"a":{"type":"STRING","value":"x"}},"extras":null}`,
		"inline not list": `{"name":"t","rules":{"a":{"type":"STRING","value":"x"}},"inline":"a"}`,
	} {
		if _, err := ParseGrammar([]byte(doc)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	ok := `"a":{"type":"STRING","value":"x😀","unknown":1},"b":{"type":"PATTERN","value":"y","flags":null}`
	if _, err := ParseGrammar(gj(ok, `,"word":null,"extra_field":1`)); err != nil {
		t.Errorf("valid input refused: %v", err)
	}
}

func TestValidationsRefuse(t *testing.T) {
	cases := map[string]string{
		"undeclared precedence": string(gj(`"a":{"type":"TOKEN","content":{"type":"PREC","value":"missing","content":{"type":"STRING","value":"x"}}}`, "")),
		"conflicting orderings": string(gj(`"a":{"type":"STRING","value":"x"}`, `,"precedences":[[{"type":"STRING","value":"p"},{"type":"STRING","value":"q"}],[{"type":"STRING","value":"q"},{"type":"STRING","value":"p"}]]`)),
		"indirect recursion":    string(gj(`"a":{"type":"SYMBOL","name":"b"},"b":{"type":"CHOICE","members":[{"type":"SYMBOL","name":"c"},{"type":"STRING","value":"x"}]},"c":{"type":"SYMBOL","name":"b"}`, "")),
		"inline first rule":     string(gj(`"a":{"type":"SEQ","members":[{"type":"STRING","value":"x"},{"type":"SYMBOL","name":"b"}]},"b":{"type":"STRING","value":"y"}`, `,"inline":["a"]`)),
		"inline token":          string(gj(`"a":{"type":"SEQ","members":[{"type":"STRING","value":"x"},{"type":"SYMBOL","name":"b"}]},"b":{"type":"STRING","value":"y"}`, `,"inline":["b"]`)),
	}
	for name, doc := range cases {
		if _, err := Prepare([]byte(doc)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	good := string(gj(`"a":{"type":"TOKEN","content":{"type":"PREC","value":"p","content":{"type":"STRING","value":"x"}}}`, `,"precedences":[[{"type":"STRING","value":"p"}]]`))
	if _, err := Prepare([]byte(good)); err != nil {
		t.Errorf("declared precedence refused: %v", err)
	}
}

// TestKeywordUsesGeneratorUnicode: U+1C89 is alphabetic in Unicode 16 and later
// (the generator's Rust release) and unassigned in Go's older tables, so the
// keyword set must hold the literal that starts with it.
func TestKeywordUsesGeneratorUnicode(t *testing.T) {
	doc := gj(`"source":{"type":"REPEAT1","content":{"type":"CHOICE","members":[{"type":"SYMBOL","name":"ident"},{"type":"STRING","value":"Ᲊa"}]}},`+
		`"ident":{"type":"PATTERN","value":"[a-z\\u1c89]+"}`, `,"word":"ident"`)
	p, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	var st TokenSet
	for i := range p.Lex.Variables {
		st.Insert(terminalSym(i))
	}
	a, err := p.Analyze([]TokenSet{st})
	if err != nil {
		t.Fatal(err)
	}
	kw := a.Keywords()
	found := false
	for _, s := range kw.symbols() {
		if p.Lex.Variables[s.Index].Ident == "Ᲊa" {
			found = true
		}
	}
	if !found {
		t.Error("the keyword that starts with U+1C89 is not in the keyword set")
	}
	if !isRustAlphabetic(0x1C89) {
		t.Error("embedded table")
	}
}

// TestPrecedenceOrderingSwapCarries: the generator swaps its first entry inside
// the inner loop and keeps the swap for the following entries, so the lists
// below (from a grammar that the generator accepts) are not a conflict even
// though two of them order two names in opposite ways.
func TestPrecedenceOrderingSwapCarries(t *testing.T) {
	lists := [][]string{
		{"getline_file", "getline_input", "field_ref", "binary_exponent", "binary_times", "binary_plus", "unary_exp", "binary_relation", "binary_match", "binary_in", "binary_and", "binary_or", "ternary_exp", "exp_list", "piped_io_exp", "range_pattern", "statement"},
		{"print_args", "grouping", "piped_io_exp", "binary_relation"},
	}
	seen := map[string]bool{}
	var members, rules, prec []string
	for _, l := range lists {
		var row []string
		for _, n := range l {
			row = append(row, `{"type":"SYMBOL","name":"`+n+`"}`)
			if !seen[n] {
				seen[n] = true
				members = append(members, `{"type":"SYMBOL","name":"`+n+`"}`)
				rules = append(rules, `"`+n+`":{"type":"STRING","value":"t_`+n+`"}`)
			}
		}
		prec = append(prec, "["+strings.Join(row, ",")+"]")
	}
	doc := gj(`"source":{"type":"CHOICE","members":[`+strings.Join(members, ",")+`]},`+strings.Join(rules, ","), `,"precedences":[`+strings.Join(prec, ",")+`]`)
	if _, err := Prepare(doc); err != nil {
		t.Errorf("refused: %v", err)
	}
}
