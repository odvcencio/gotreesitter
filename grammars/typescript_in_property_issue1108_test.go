package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTypeScriptInPropertyIssue1108(t *testing.T) {
	languages := []struct {
		name     string
		language *gotreesitter.Language
	}{
		{"typescript", TypescriptLanguage()},
		{"tsx", TsxLanguage()},
	}
	tests := []struct {
		name   string
		source string
	}{
		{"default_then_in", "interface A {\n  easing: {\n    default: string\n    in: string\n  }\n}\n"},
		{"in_then_default", "interface A {\n  easing: {\n    in: string\n    default: string\n  }\n}\n"},
		{"in_then_in", "interface A {\n  easing: {\n    in: string\n    in: string\n  }\n}\n"},
		{"default_then_out", "interface A {\n  easing: {\n    default: string\n    out: string\n  }\n}\n"},
		{"binary_in_across_newline", "const found = key\n  in object\n"},
	}
	for _, language := range languages {
		for _, test := range tests {
			t.Run(language.name+"/"+test.name, func(t *testing.T) {
				parser := gotreesitter.NewParser(language.language)
				parser.SetTimeoutMicros(2_000_000)
				tree, err := parser.ParseStrict([]byte(test.source))
				if err != nil {
					t.Fatalf("ParseStrict: %v", err)
				}
				defer tree.Release()
				if tree.RootNode().HasErrorOrMissing() || tree.ParseStoppedEarly() {
					t.Fatalf("invalid tree: %s\n%s", tree.ParseRuntime().Summary(), tree.RootNode().SExpr(language.language))
				}
			})
		}
	}
}
