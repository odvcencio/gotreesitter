package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTypeScriptContextualFollowups(t *testing.T) {
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			lang := DetectLanguageByName(name).Language()
			for _, source := range []string{
				`class A { static get [x]() { return 1; } }`,
				`class A { static set [x](value: number) {} }`,
				`class A { get ["]"](): number { return 1; } }`,
				`class A { get ['['](): number { return 1; } }`,
				`type A = [keyof: string];`,
				`type A = [keyof?: string];`,
				`type Shape = { value: string }; type A = [keyof Shape];`,
				`type Shape = { value: string }; type A = [(keyof Shape)?];`,
			} {
				t.Run(source, func(t *testing.T) {
					tree, err := gotreesitter.NewParser(lang).ParseStrict([]byte(source))
					if err != nil {
						t.Fatal(err)
					}

					defer tree.Release()
					root := tree.RootNode()
					if root == nil {
						t.Fatal("nil root")
					}

					if root.HasErrorOrMissing() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(source)) {
						t.Fatalf("invalid tree: %s", root.SExpr(lang))
					}

					if root.NamedChild(0).Type(lang) == "class_declaration" {
						method := root.NamedChild(0).ChildByFieldName("body", lang).NamedChild(0)
						methodName := method.ChildByFieldName("name", lang)
						if methodName == nil || methodName.Type(lang) != "computed_property_name" {
							t.Fatalf("computed name field lost: %s", root.SExpr(lang))
						}
					}
				})
			}
		})
	}
}
