package query_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func FuzzQueryCompile(f *testing.F) {
	f.Add("(identifier) @name")
	f.Add("(function_declaration name: (identifier) @name) @definition.function")
	f.Add("(call_expression function: (identifier) @name) @reference.call")
	f.Add("[\"func\" \"return\"] @keyword")
	f.Add("((identifier) @x (#eq? @x \"main\"))")
	f.Add("")
	f.Add("(((")
	f.Add("@@@")
	f.Add("(unknown_node_type_xyz)")
	f.Add("(identifier) @a @b @c")
	f.Add("(identifier)?")
	f.Add("(identifier)*")
	f.Add("(identifier)+")

	lang := grammars.GoLanguage()

	f.Fuzz(func(t *testing.T, pattern string) {
		if len(pattern) > 1<<16 {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic while compiling query pattern (%d bytes): %v", len(pattern), r)
			}
		}()

		q, err := gotreesitter.NewQuery(pattern, lang)
		if err != nil {
			return // expected for most fuzz inputs
		}
		_ = q.PatternCount()
		_ = q.CaptureNames()
	})
}
