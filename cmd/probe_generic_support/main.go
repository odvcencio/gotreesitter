package main

import (
	"fmt"
	"sort"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
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
	"toml":       "a = 1\n",
	"tsx":        "const x = <div>Hello</div>;\n",
	"typescript": "function f(): number { return 1; }\n",
	"yaml":       "a: 1\n",
}

func main() {
	entries := grammars.AllLanguages()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	fmt.Println("language\tresult\tnotes")
	for _, entry := range entries {
		lang := entry.Language()
		report := grammars.EvaluateParseSupport(entry, lang)
		if report.Backend != grammars.ParseBackendUnsupported {
			continue
		}

		sample, ok := parseSmokeSamples[entry.Name]
		if !ok {
			fmt.Printf("%s\tSKIP\tmissing sample\n", entry.Name)
			continue
		}

		src := []byte(sample)
		ts, err := grammars.NewGenericTokenSource(src, lang)
		if err != nil {
			fmt.Printf("%s\tINIT_FAIL\t%v\n", entry.Name, err)
			continue
		}

		p := gotreesitter.NewParser(lang)
		tree := p.ParseWithTokenSource(src, ts)
		if tree != nil && tree.RootNode() != nil && !tree.RootNode().HasError() {
			fmt.Printf("%s\tPASS\tgeneric token source parses smoke sample\n", entry.Name)
		} else {
			fmt.Printf("%s\tFAIL\tparse has errors with generic token source\n", entry.Name)
		}
	}
}
