package highlight_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTaggerIntegrationGo(t *testing.T) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		t.Skip("Go grammar not available")
	}

	lang := entry.Language()
	source := []byte(`package main

func Add(a, b int) int {
	return a + b
}

func main() {
	Add(1, 2)
}
`)

	var opts []gotreesitter.TaggerOption
	if entry.TokenSourceFactory != nil {
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTaggerTokenSourceFactory(func(src []byte) gotreesitter.TokenSource {
			return factory(src, lang)
		}))
	}

	// Use positional matching for this test — field-based matching also works
	// now that grammar blobs include FieldMapEntries.
	tagger, err := gotreesitter.NewTagger(lang, `
(function_declaration (identifier) @name) @definition.function
(method_declaration (field_identifier) @name) @definition.method
(call_expression (identifier) @name) @reference.call
`, opts...)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tags := tagger.Tag(source)
	if len(tags) == 0 {
		t.Fatal("expected tags from Go source")
	}

	defs := 0
	refs := 0
	for _, tag := range tags {
		switch {
		case tag.Kind == "definition.function":
			defs++
			t.Logf("def: %s at %d:%d", tag.Name, tag.NameRange.StartPoint.Row, tag.NameRange.StartPoint.Column)
		case tag.Kind == "reference.call":
			refs++
			t.Logf("ref: %s at %d:%d", tag.Name, tag.NameRange.StartPoint.Row, tag.NameRange.StartPoint.Column)
		}
	}

	if defs < 2 {
		t.Errorf("expected >= 2 function definitions, got %d", defs)
	}
	if refs < 1 {
		t.Errorf("expected >= 1 call reference, got %d", refs)
	}
}

func TestParseFileAndWalkIntegration(t *testing.T) {
	bt, err := grammars.ParseFile("main.go", []byte(`package main

func hello() {}
`))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	var funcNames []string
	gotreesitter.Walk(bt.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if bt.NodeType(node) == "function_declaration" {
			// Walk children by type (field-based lookup via ChildByFieldName also works).
			for i := 0; i < node.ChildCount(); i++ {
				child := node.Child(i)
				if bt.NodeType(child) == "identifier" {
					funcNames = append(funcNames, bt.NodeText(child))
					break
				}
			}
		}
		return gotreesitter.WalkContinue
	})

	if len(funcNames) != 1 || funcNames[0] != "hello" {
		t.Errorf("expected [hello], got %v", funcNames)
	}
}
