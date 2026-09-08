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
