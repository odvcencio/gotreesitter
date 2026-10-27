package grammars

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTypeScriptGenericTaggedTemplates(t *testing.T) {
	cases := []struct {
		source   string
		comments []string
	}{
		{source: "const value = tag<string>`text`;"},
		{source: "const value = tag<string[]>`text ${item}`;"},
		{source: "const value = tag<Array<string[]>>`text`;"},
		{source: "const value = sql.query<Row, Params>`select 1`;"},
		{source: "const value = { result: tag<string[]>`text` };"},
		{source: "const value = tag<string[]>`\n// template text\n/* template text */\n`;\n// actual comment", comments: []string{"// actual comment"}},
		{source: "const value = tag<string[]>`${flag ? `/*` : /{/.test(text)}`;\n// actual comment", comments: []string{"// actual comment"}},
		{source: "const value = tag<string[]>`${\n// interpolation comment\nitem\n}`;", comments: []string{"// interpolation comment"}},
	}
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			lang := DetectLanguageByName(name).Language()
			for _, tc := range cases {
				t.Run(tc.source, func(t *testing.T) {
					tree, err := gotreesitter.NewParser(lang).ParseStrict([]byte(tc.source))
					if err != nil {
						t.Fatal(err)
					}

					defer tree.Release()
					root := tree.RootNode()
					if root == nil {
						t.Fatal("nil root")
					}

					if root.HasErrorOrMissing() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(tc.source)) {
						t.Fatalf("invalid tree: %s", root.SExpr(lang))
					}

					var comments []string
					genericTemplateCall := false
					var visit func(*gotreesitter.Node)
					visit = func(node *gotreesitter.Node) {
						switch node.Type(lang) {
						case "call_expression":
							args := node.ChildByFieldName("arguments", lang)
							if node.ChildByFieldName("type_arguments", lang) != nil && args != nil && args.Type(lang) == "template_string" {
								genericTemplateCall = true
							}
						case "comment":
							comments = append(comments, tc.source[node.StartByte():node.EndByte()])
						}
						for i := range node.ChildCount() {
							visit(node.Child(i))
						}
					}
					visit(root)
					if !genericTemplateCall {
						t.Fatalf("missing generic tagged template call: %s", root.SExpr(lang))
					}

					if !reflect.DeepEqual(comments, tc.comments) {
						t.Fatalf("comments=%q want=%q", comments, tc.comments)
					}
				})
			}
		})
	}
}
