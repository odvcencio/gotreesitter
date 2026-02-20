package main

import (
	"flag"
	"fmt"
	"os"
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
	"swift":      "func f() -> Int { return 1 }\n",
	"toml":       "a = 1\n",
	"tsx":        "const x = <div>Hello</div>;\n",
	"typescript": "function f(): number { return 1; }\n",
	"yaml":       "a: 1\n",
}

type runStatus struct {
	name        string
	backend     grammars.ParseBackend
	parseOK     bool
	reason      string
	genericHint string
}

func main() {
	strict := flag.Bool("strict", false, "exit non-zero unless every manifest grammar parses smoke sample")
	flag.Parse()

	entries := grammars.AllLanguages()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	entryByName := make(map[string]grammars.LangEntry, len(entries))
	for _, e := range entries {
		entryByName[e.Name] = e
	}

	reports := grammars.AuditParseSupport()
	sort.Slice(reports, func(i, j int) bool { return reports[i].Name < reports[j].Name })

	statuses := make([]runStatus, 0, len(reports))
	var parseable int
	var unsupported int

	for _, report := range reports {
		sample, ok := parseSmokeSamples[report.Name]
		if !ok {
			statuses = append(statuses, runStatus{
				name:    report.Name,
				backend: report.Backend,
				parseOK: false,
				reason:  "missing smoke sample",
			})
			continue
		}

		entry := entryByName[report.Name]
		lang := entry.Language()
		src := []byte(sample)

		st := runStatus{name: report.Name, backend: report.Backend}
		if report.Backend == grammars.ParseBackendUnsupported {
			unsupported++
			st.reason = report.Reason
			st.genericHint = probeGeneric(src, lang)
			statuses = append(statuses, st)
			continue
		}

		if runSmokeParse(report.Backend, src, lang, entry.TokenSourceFactory) {
			st.parseOK = true
			parseable++
		} else {
			st.reason = "smoke parse failed"
		}
		statuses = append(statuses, st)
	}

	fmt.Printf("coverage: parseable=%d total=%d unsupported=%d\n\n", parseable, len(reports), unsupported)
	fmt.Println("language\tbackend\tstatus\tnotes")
	for _, st := range statuses {
		status := "ok"
		notes := st.reason
		if st.backend == grammars.ParseBackendUnsupported {
			status = "unsupported"
			if st.genericHint != "" {
				if notes != "" {
					notes += "; "
				}
				notes += st.genericHint
			}
		} else if !st.parseOK {
			status = "fail"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", st.name, st.backend, status, notes)
	}

	if *strict {
		allGood := unsupported == 0
		for _, st := range statuses {
			if st.backend != grammars.ParseBackendUnsupported && !st.parseOK {
				allGood = false
				break
			}
		}
		if !allGood {
			os.Exit(1)
		}
	}
}

func runSmokeParse(
	backend grammars.ParseBackend,
	src []byte,
	lang *gotreesitter.Language,
	factory func([]byte, *gotreesitter.Language) gotreesitter.TokenSource,
) bool {
	p := gotreesitter.NewParser(lang)

	var tree *gotreesitter.Tree
	switch backend {
	case grammars.ParseBackendTokenSource:
		if factory == nil {
			return false
		}
		tree = p.ParseWithTokenSource(src, factory(src, lang))
	case grammars.ParseBackendDFA:
		tree = p.Parse(src)
	default:
		return false
	}

	if tree == nil || tree.RootNode() == nil {
		return false
	}
	return !tree.RootNode().HasError()
}

func probeGeneric(src []byte, lang *gotreesitter.Language) string {
	ts, err := grammars.NewGenericTokenSource(src, lang)
	if err != nil {
		return "generic init failed: " + err.Error()
	}
	p := gotreesitter.NewParser(lang)
	tree := p.ParseWithTokenSource(src, ts)
	if tree == nil || tree.RootNode() == nil {
		return "generic parse nil root"
	}
	if tree.RootNode().HasError() {
		return "generic parse has errors"
	}
	return "generic smoke passes"
}
