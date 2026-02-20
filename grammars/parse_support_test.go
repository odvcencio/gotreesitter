package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

var parseSmokeSamples = map[string]string{
	"bash":       "echo hi\n",
	"c":          "int main(void) { return 0; }\n",
	"cpp":        "int main() { return 0; }\n",
	"css":        "body { color: red; }\n",
	"go":         "package main\n\nfunc main() {\n\tprintln(1)\n}\n",
	"html":       "<html><body>Hello</body></html>\n",
	"java":       "class Main { int x; }\n",
	"javascript": "function f() { return 1; }\n",
	"json":       "{\"a\": 1}\n",
	"kotlin":     "fun main() { val x = 1 }\n",
	"lua":        "local x = 1\n",
	"php":        "<?php echo 1;\n",
	"python":     "def f():\n    return 1\n",
	"ruby":       "def f\n  1\nend\n",
	"rust":       "fn main() { let x = 1; }\n",
	"sql":        "SELECT 1;\n",
	"swift":      "func f() -> Int { return 1 }\n",
	"toml":       "a = 1\n",
	"tsx":        "const x = <div>Hello</div>;\n",
	"typescript": "function f(): number { return 1; }\n",
	"yaml":       "a: 1\n",
}

func TestSupportedLanguagesParseSmoke(t *testing.T) {
	entries := AllLanguages()
	entryByName := make(map[string]LangEntry, len(entries))
	for _, entry := range entries {
		entryByName[entry.Name] = entry
	}

	reports := AuditParseSupport()
	for _, report := range reports {
		sample, ok := parseSmokeSamples[report.Name]
		if !ok {
			t.Fatalf("missing parse smoke sample for language %q", report.Name)
		}

		if report.Backend == ParseBackendUnsupported {
			t.Logf("skip %s: %s", report.Name, report.Reason)
			continue
		}

		entry, ok := entryByName[report.Name]
		if !ok {
			t.Fatalf("missing registry entry for %q", report.Name)
		}
		lang := entry.Language()
		parser := gotreesitter.NewParser(lang)
		source := []byte(sample)

		var tree *gotreesitter.Tree
		switch report.Backend {
		case ParseBackendTokenSource:
			ts := entry.TokenSourceFactory(source, lang)
			tree = parser.ParseWithTokenSource(source, ts)
		case ParseBackendDFA:
			tree = parser.Parse(source)
		default:
			t.Fatalf("unknown backend %q for %q", report.Backend, report.Name)
		}

		if tree == nil || tree.RootNode() == nil {
			t.Fatalf("%s parse returned nil root using backend %q", report.Name, report.Backend)
		}
		if tree.RootNode().HasError() {
			t.Fatalf("%s parse smoke sample produced syntax errors", report.Name)
		}
	}
}
