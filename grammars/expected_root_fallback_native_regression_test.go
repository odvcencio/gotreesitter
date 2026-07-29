//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

var expectedRootFallbackCases = []struct {
	name     string
	language func() *gotreesitter.Language
	rootType string
	source   string
}{
	{
		name:     "hurl_recovered_entry",
		language: HurlLanguage,
		rootType: "hurl_file",
		source:   "GET http://localhost:8000/hello\n\nxxx",
	},
	{
		name:     "hurl_file_delimiter",
		language: HurlLanguage,
		rootType: "hurl_file",
		source:   "POST http://localhost:8000/data\nfile,",
	},
	{
		name:     "hurl_clean",
		language: HurlLanguage,
		rootType: "hurl_file",
		source:   "GET http://localhost:8000/hello",
	},
	{
		name:     "ini_malformed_files",
		language: IniLanguage,
		rootType: "document",
		source:   "[mypy]\nfiles =\n    one.py\n    two.py\nstrict = true\n",
	},
	{
		name:     "ini_error_code_continuation",
		language: IniLanguage,
		rootType: "document",
		source:   "[mypy]\nenable_error_code = \n    ignore-without-code,\n    unused-ignore\n",
	},
	{
		name:     "ini_clean",
		language: IniLanguage,
		rootType: "document",
		source:   "[mypy]\nstrict = true\n",
	},
}

func TestExpectedRootFallbackClassNeedsNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	for _, test := range expectedRootFallbackCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			language := test.language()

			normal, err := gotreesitter.NewParser(language).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(normal.Release)
			raw, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)

			if got := normal.RootNode().Type(language); got != test.rootType {
				t.Fatalf("root type = %q, want %q", got, test.rootType)
			}
			gotreesitter.Walk(normal.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
				if depth > 0 && node.Parent() == nil {
					t.Fatalf("node %q at depth %d has no parent", node.Type(language), depth)
				}
				return gotreesitter.WalkContinue
			})
			runtime := normal.ParseRuntime()
			if runtime.NormalizationNodesRewritten != 0 ||
				runtime.NormalizationNodesVisited != 0 {
				t.Fatalf(
					"normalization rewrote %d nodes and visited %d nodes",
					runtime.NormalizationNodesRewritten,
					runtime.NormalizationNodesVisited,
				)
			}
			if got, want := collapsedTokenTreeDigest(t, raw, language),
				collapsedTokenTreeDigest(t, normal, language); got != want {
				t.Fatalf("compatibility-free digest = %s, want %s", got, want)
			}
		})
	}
}

func TestExpectedRootFallbackClassRoutesStayExactOrFailClosed(t *testing.T) {
	for _, test := range expectedRootFallbackCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			language := test.language()
			baseSource := []byte(test.source)
			source := append(append([]byte(nil), baseSource...), '\n')

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			want := collapsedTokenTreeDigest(t, production, language)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			direct := routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore
			fallback := routedAfter == routedBefore && fallbackAfter == fallbackBefore+1
			if !direct && !fallback {
				t.Fatalf(
					"compact counters routed=%d/%d fallback=%d/%d",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
				)
			}
			if got := collapsedTokenTreeDigest(t, compact, language); got != want {
				t.Fatalf("compact digest = %s, want production %s", got, want)
			}

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(source)
			if !ok || forest == nil {
				if forest != nil {
					forest.Release()
					t.Fatal("forest returned a tree with a decline")
				}
				_, _, reason, _ := forestParser.ForestDeclineInfo()
				if reason == "" {
					t.Fatal("forest declined without a reason")
				}
			} else {
				t.Cleanup(forest.Release)
				if got := collapsedTokenTreeDigest(t, forest, language); got != want {
					t.Fatalf("forest digest = %s, want production %s", got, want)
				}
			}

			oldParser := gotreesitter.NewParser(language)
			oldParser.SetAdmissionCandidateRoute(false)
			oldTree, err := oldParser.Parse(baseSource)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			endPoint := retiredDispatchPointAtByte(baseSource, len(baseSource))
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(baseSource)),
				OldEndByte:  uint32(len(baseSource)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  endPoint,
				OldEndPoint: endPoint,
				NewEndPoint: gotreesitter.Point{Row: endPoint.Row + 1},
			})
			incremental, _, err := oldParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			if got := collapsedTokenTreeDigest(t, incremental, language); got != want {
				t.Fatalf("incremental digest = %s, want production %s", got, want)
			}
		})
	}
}
