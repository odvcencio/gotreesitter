package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseSmokeDegradedReason(report grammars.ParseSupport) string {
	if report.Reason != "" {
		return report.Reason
	}
	return "parser reported recoverable syntax errors on smoke sample"
}

type runStatus struct {
	name          string
	backend       grammars.ParseBackend
	quality       grammars.ParseQuality
	fieldMapCount int
	parseOK       bool
	degraded      bool
	reason        string
	genericHint   string
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
		sample := grammars.ParseSmokeSample(report.Name)

		entry := entryByName[report.Name]
		lang := entry.Language()
		src := []byte(sample)

		st := runStatus{name: report.Name, backend: report.Backend, quality: entry.Quality, fieldMapCount: len(lang.FieldMapEntries)}
		if report.Backend == grammars.ParseBackendDFAPartial {
			st.reason = report.Reason
		}
		if report.Backend == grammars.ParseBackendUnsupported {
			unsupported++
			st.reason = report.Reason
			st.genericHint = probeGeneric(src, lang)
			statuses = append(statuses, st)
			continue
		}

		parsed, hasError := runSmokeParse(report.Backend, src, lang, entry.TokenSourceFactory)
		switch report.Backend {
		case grammars.ParseBackendDFAPartial:
			if !parsed {
				st.reason = "smoke parse failed"
			} else if hasError {
				st.degraded = true
				st.reason = parseSmokeDegradedReason(report)
				parseable++
			} else {
				st.parseOK = true
				parseable++
			}
		default:
			if parsed && !hasError {
				st.parseOK = true
				parseable++
			} else if parsed && hasError {
				st.degraded = true
				st.reason = parseSmokeDegradedReason(report)
				parseable++
			} else {
				st.reason = "smoke parse failed"
			}
		}
		statuses = append(statuses, st)
	}

	fmt.Printf("coverage: parseable=%d total=%d unsupported=%d\n\n", parseable, len(reports), unsupported)
	fmt.Println("language\tbackend\tquality\tfields\tstatus\tnotes")
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
		} else if st.degraded {
			status = "degraded"
		} else if !st.parseOK {
			status = "fail"
		}
		fmt.Printf("%s\t%s\t%s\t%d\t%s\t%s\n", st.name, st.backend, st.quality, st.fieldMapCount, status, notes)
	}

	if *strict {
		allGood := unsupported == 0
		for _, st := range statuses {
			if st.backend != grammars.ParseBackendUnsupported && !st.parseOK && !st.degraded {
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
) (bool, bool) {
	p := gotreesitter.NewParser(lang)

	var tree *gotreesitter.Tree
	switch backend {
	case grammars.ParseBackendTokenSource:
		if factory == nil {
			return false, false
		}
		tree, _ = p.ParseWithTokenSource(src, factory(src, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, _ = p.Parse(src)
	default:
		return false, false
	}

	if tree == nil || tree.RootNode() == nil {
		return false, false
	}
	return true, tree.RootNode().HasError()
}

func probeGeneric(src []byte, lang *gotreesitter.Language) string {
	ts, err := grammars.NewGenericTokenSource(src, lang)
	if err != nil {
		return "generic init failed: " + err.Error()
	}
	p := gotreesitter.NewParser(lang)
	tree, parseErr := p.ParseWithTokenSource(src, ts)
	if parseErr != nil {
		return "generic parse error: " + parseErr.Error()
	}
	if tree == nil || tree.RootNode() == nil {
		return "generic parse nil root"
	}
	if tree.RootNode().HasError() {
		return "generic parse has errors"
	}
	return "generic smoke passes"
}
