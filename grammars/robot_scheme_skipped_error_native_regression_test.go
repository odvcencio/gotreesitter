//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

var robotSchemeSkippedErrorCases = []struct {
	name     string
	language func() *gotreesitter.Language
	source   string
}{
	{
		name:     "robot_expression",
		language: RobotLanguage,
		source:   `${tc['\${i}']}`,
	},
	{
		name:     "robot_test_case",
		language: RobotLanguage,
		source:   "*** Test Cases ***\nTest\n    Log    ${tc['\\${i}']}",
	},
	{
		name:     "robot_variable_definition",
		language: RobotLanguage,
		source:   "*** Variables ***\n${x}    ${tc['\\${i}']}",
	},
	{
		name:     "scheme_quote_adjacent_space",
		language: SchemeLanguage,
		source:   "'| x",
	},
	{
		name:     "scheme_quote_separated_space",
		language: SchemeLanguage,
		source:   "' | x",
	},
	{
		name:     "scheme_quote_adjacent",
		language: SchemeLanguage,
		source:   "'|x",
	},
	{
		name:     "scheme_quote_separated",
		language: SchemeLanguage,
		source:   "' |x",
	},
	{
		name:     "scheme_quasiquote",
		language: SchemeLanguage,
		source:   "`| x",
	},
	{
		name:     "scheme_unquote",
		language: SchemeLanguage,
		source:   ",| x",
	},
	{
		name:     "scheme_unquote_splicing",
		language: SchemeLanguage,
		source:   ",@| x",
	},
	{
		name:     "scheme_syntax",
		language: SchemeLanguage,
		source:   "#'| x",
	},
	{
		name:     "scheme_quasisyntax",
		language: SchemeLanguage,
		source:   "#`| x",
	},
	{
		name:     "scheme_unsyntax",
		language: SchemeLanguage,
		source:   "#,| x",
	},
	{
		name:     "scheme_unsyntax_splicing",
		language: SchemeLanguage,
		source:   "#,@| x",
	},
	{
		name:     "scheme_list",
		language: SchemeLanguage,
		source:   "('| x)",
	},
}

func TestRobotSchemeSkippedErrorsNeedNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	for _, test := range robotSchemeSkippedErrorCases {
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

func TestRobotSchemeSkippedErrorRoutesStayExactOrFailClosed(t *testing.T) {
	for _, test := range robotSchemeSkippedErrorCases {
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
