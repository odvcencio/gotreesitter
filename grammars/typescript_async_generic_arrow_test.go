package grammars

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// +spec=`An async generic arrow remains one arrow_function, including its async keyword and type parameters.`
func TestTypeScriptAsyncGenericArrow(t *testing.T) {
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			lang := DetectLanguageByName(name).Language()
			for _, candidate := range []bool{false, true} {
				t.Run(fmt.Sprintf("candidate=%t", candidate), func(t *testing.T) {
					for _, source := range []string{
						`const invoke = async <T>(value: T): Promise<T> => value;`,
						`const invoke = async <T>(value: T) => value;`,
						`const invoke = async <T,>(value: T): Promise<T> => value;`,
						`const invoke = async <T extends object>(value: T): Promise<T> => value;`,
						`const invoke = async <T, U>(value: T, other: U): Promise<T> => value;`,
						`const invoke = async /* generic */ <T>(value: T): Promise<T> => { return value; };`,
					} {
						t.Run(source, func(t *testing.T) {
							parser := gotreesitter.NewParser(lang)
							parser.SetAdmissionCandidateRoute(candidate)
							tree, err := parser.ParseStrict([]byte(source))
							if err != nil {
								t.Fatal(err)
							}
							defer tree.Release()
							root := tree.RootNode()
							if root == nil || root.HasErrorOrMissing() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(source)) {
								t.Fatalf("incomplete tree: %v", root)
							}
							value := root.NamedChild(0).NamedChild(0).ChildByFieldName("value", lang)
							if value == nil || value.Type(lang) != "arrow_function" {
								t.Fatalf("async generic arrow lost: %s", root.SExpr(lang))
							}
							if value.StartByte() != uint32(strings.Index(source, "async")) {
								t.Fatalf("async keyword excluded: %s", root.SExpr(lang))
							}
							for _, field := range []string{"type_parameters", "parameters", "return_type", "body"} {
								if field == "return_type" && !strings.Contains(source, "Promise") {
									continue
								}
								if value.ChildByFieldName(field, lang) == nil {
									t.Fatalf("missing %s: %s", field, root.SExpr(lang))
								}
							}
						})
					}
				})
			}
		})
	}
}
