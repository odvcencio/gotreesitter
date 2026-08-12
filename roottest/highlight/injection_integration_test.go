package highlight_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestInjectionParserMarkdownChildTreeCoordinatesStayDocumentRelative(t *testing.T) {
	mdEntry := grammars.DetectLanguage("sample.md")
	if mdEntry == nil {
		t.Skip("Markdown grammar not available")
	}
	goEntry := grammars.DetectLanguage("main.go")
	if goEntry == nil {
		t.Skip("Go grammar not available")
	}

	ip := gotreesitter.NewInjectionParser()
	ip.RegisterLanguage("markdown", mdEntry.Language())
	ip.RegisterLanguage("go", goEntry.Language())

	const mdInjectionQuery = `
(fenced_code_block
  (info_string (language) @injection.language)
  (code_fence_content) @injection.content)
`
	if err := ip.RegisterInjectionQuery("markdown", mdInjectionQuery); err != nil {
		t.Fatalf("RegisterInjectionQuery: %v", err)
	}

	source := []byte("# Doc\n\n```go\nfunc f0() int { return 0 }\n```\n")
	result, err := ip.Parse(source, "markdown")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}

	inj := result.Injections[0]
	if inj.Tree == nil {
		t.Fatal("injection tree is nil")
	}
	if len(inj.Ranges) != 1 {
		t.Fatalf("expected 1 injection range, got %d", len(inj.Ranges))
	}

	root := inj.Tree.RootNode()
	if root == nil {
		t.Fatal("child root is nil")
	}
	if root.StartByte() != inj.Ranges[0].StartByte || root.EndByte() != inj.Ranges[0].EndByte {
		t.Fatalf("child root bytes = [%d,%d), want [%d,%d)",
			root.StartByte(), root.EndByte(), inj.Ranges[0].StartByte, inj.Ranges[0].EndByte)
	}
	if root.StartPoint() != inj.Ranges[0].StartPoint || root.EndPoint() != inj.Ranges[0].EndPoint {
		t.Fatalf("child root points = [%v,%v), want [%v,%v)",
			root.StartPoint(), root.EndPoint(), inj.Ranges[0].StartPoint, inj.Ranges[0].EndPoint)
	}
	if got := root.Text(source); got != "func f0() int { return 0 }\n" {
		t.Fatalf("child root text from document source = %q", got)
	}
}
