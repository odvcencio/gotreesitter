package main

import "testing"

func TestExtractReservedWordDimensions(t *testing.T) {
	for _, tc := range []struct {
		name, defines, rows, width, body string
		valid                            bool
	}{
		{"numeric", "", "2", "2", "[1] = {1, 0}", true},
		{"macro", "#define COUNT 2\n#define WIDTH 2\n", "COUNT", "WIDTH", "[1] = {1, 0}", true},
		{"undefined", "", "COUNT", "2", "", false},
		{"overflow", "", "999999999999999999999999999", "2", "", false},
		{"product", "", "65536", "65535", "", false},
		{"index", "", "2", "2", "[2] = {1}", false},
		{"row_width", "", "2", "2", "[1] = {1, 2, 3}", false},
		{"negative_symbol", "", "2", "2", "[1] = {-1, 0}", false},
		{"symbol_expression", "", "2", "2", "[1] = {1+2}", false},
		{"negative_index", "", "2", "2", "[-1] = {1}", false},
		{"index_expression", "", "2", "2", "[0+1] = {1}", false},
		{"positional_row", "", "2", "2", "{0}, {1}", false},
		{"mixed_positional_row", "", "2", "2", "[0] = {0}, {1}", false},
		{"duplicate_row", "", "2", "2", "[1] = {1}, [1] = {0}", false},
		{"missing_row_comma", "", "2", "2", "[0] = {0} [1] = {1}", false},
		{"comment_not_concatenation", "", "2", "2", "[1] = {1/**/2}", false},
		{"trailing_junk", "", "2", "2", "[1] = {1};", false},
		{"comments", "", "2", "2", "/* before */ [/* index */1] /* equals */ = {1, // next value\n 0,}, /* after */", true},
		{"comment_braces", "", "2", "2", "[1] = {1, /* } [bad] = { */ 0}", true},
		{"named_row", "#define ROW 1\n", "2", "2", "[ROW] = {1, 0}", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.defines + "static const TSSymbol ts_reserved_words[" + tc.rows + "][" + tc.width + "] = {" + tc.body + "};"
			g := &ExtractedGrammar{enumValues: map[string]int{}}
			err := extractReservedWords(source, g)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && (g.MaxReservedWordSetSize != 2 || len(g.ReservedWords) != 4 || g.ReservedWords[2] != 1) {
				t.Fatalf("table=%+v", g.ReservedWords)
			}
		})
	}
}
