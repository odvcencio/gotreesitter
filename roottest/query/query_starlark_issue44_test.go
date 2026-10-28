package query_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const issue44StarlarkQuery = `
(expression_statement
 (assignment (identifier) @a (#eq? @a "a")
             (dictionary (pair (string (string_content) @b (#eq? @b "b"))
                           (dictionary
                            (pair (string (string_content) @c_key (#eq? @c_key "c"))
                                  (string) @c)
                            (pair (string (string_content) @d_key (#eq? @d_key "d"))
                                  (string) @d))))))
`

func TestQueryStarlarkNestedDictionaryPredicates(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "first_matching_pair",
			source: `a = {
    "b": {
        "c": "a1",
        "d": "a2",
    },
    "m": {
        "c": "a1",
        "d": "a2",
    },
}
`,
		},
		{
			name: "matching_pair_after_nonmatching_sibling",
			source: `a = {
    "b": {
        "n": "p",
        "c": "a1",
        "d": "a2",
    },
    "m": {
        "c": "a1",
        "d": "a2",
    },
}
`,
		},
	}

	lang := grammars.StarlarkLanguage()
	query, err := gotreesitter.NewQuery(issue44StarlarkQuery, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			defer tree.Release()

			matches := query.Execute(tree)
			if len(matches) != 1 {
				t.Fatalf("matches: got %d, want 1", len(matches))
			}

			got := make([]string, 0, len(matches[0].Captures))
			for _, capture := range matches[0].Captures {
				got = append(got, capture.Name+"="+capture.Node.Text(source))
			}
			want := []string{
				"a=a",
				"b=b",
				"c_key=c",
				"c=\"a1\"",
				"d_key=d",
				"d=\"a2\"",
			}
			if len(got) != len(want) {
				t.Fatalf("captures: got %d %v, want %d %v", len(got), got, len(want), want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("capture[%d]: got %q, want %q; all captures %v", i, got[i], want[i], got)
				}
			}
		})
	}
}
