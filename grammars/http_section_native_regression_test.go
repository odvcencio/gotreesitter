package grammars

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

var httpSectionExactFixtures = []struct {
	name       string
	source     string
	wantDigest string
}{
	{
		name:       "blank_section",
		source:     "# c\n\n### next\n",
		wantDigest: "12c900585dc7ecae3a5dc6c3a7072b927a56d0c5dd7f9f699eb8169acceb948b",
	},
	{
		name:       "content_sections",
		source:     "### a\n# c\nGET /\n### b\n",
		wantDigest: "e67faf42a5f1da1b558a4b40acc86c011ad900b6aa573713729ee1a7bcf43e7e",
	},
}

var httpLockedCorpusPaths = []string{
	"spec/examples/basic_get.http",
	"spec/examples/dotenv/with_dotenv.http",
	"spec/examples/dotenv/without_dotenv.http",
	"spec/examples/request_body/external_body.http",
	"spec/examples/script/post_request_script.http",
	"spec/examples/url_without_host.http",
	"spec/examples/variables/dynamic_vars.http",
	"spec/examples/variables/prompt_variables.http",
}

func TestHTTPSectionCoalescingNeedsNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	for _, fixture := range httpSectionExactFixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			assertHTTPSectionNativeOwnership(
				t,
				[]byte(fixture.source),
				fixture.wantDigest,
			)
		})
	}
}

func TestHTTPSectionCoalescingRoutesStayExactOrFailClosed(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	for _, fixture := range httpSectionExactFixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			source := []byte(fixture.source[:len(fixture.source)-1])
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			receipts := retiredDispatchRouteReceiptsAllowCompactFallback(
				t,
				HttpLanguage(),
				source,
			)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
				t.Fatalf(
					"compact route did not fail closed: routed=%d/%d fallback=%d/%d",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
				)
			}
			for _, receipt := range receipts {
				assertNoNormalizationPasses(t, receipt.tree)
				if receipt.name != "incremental" {
					continue
				}
				if !receipt.incrementalProfile.OldTreeReuseRoute {
					t.Fatalf("incremental route did not use the old tree: %+v", receipt.incrementalProfile)
				}
				if fixture.name == "content_sections" &&
					receipt.incrementalProfile.ReusedSubtrees == 0 {
					t.Fatalf("incremental route reused no subtree: %+v", receipt.incrementalProfile)
				}
			}
		})
	}
}

func TestHTTPSectionCoalescingLockedCorpusNativeOwnership(t *testing.T) {
	root := os.Getenv("GTS_HTTP_LOCKED_CORPUS")
	if root == "" {
		t.Skip("set GTS_HTTP_LOCKED_CORPUS to the rest.nvim corpus root")
	}
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	for _, path := range httpLockedCorpusPaths {
		path := path
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				t.Fatal(err)
			}
			assertHTTPSectionNativeOwnership(t, source, "")
		})
	}
}

func assertHTTPSectionNativeOwnership(
	t *testing.T,
	source []byte,
	wantDigest string,
) {
	t.Helper()
	language := HttpLanguage()
	rawParser := gotreesitter.NewParser(language)
	rawParser.SetAdmissionCandidateRoute(false)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(raw.Release)
	assertNoNormalizationPasses(t, raw)

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	assertNoNormalizationPasses(t, production)

	rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	productionInspection, err := benchfixtures.InspectGoTree(
		production.RootNode(),
		language,
	)
	if err != nil {
		t.Fatal(err)
	}
	if rawInspection.SHA256 != productionInspection.SHA256 {
		t.Fatalf(
			"compatibility-free digest=%s production=%s",
			rawInspection.SHA256,
			productionInspection.SHA256,
		)
	}
	if wantDigest != "" && rawInspection.SHA256 != wantDigest {
		t.Fatalf("native digest=%s, want %s", rawInspection.SHA256, wantDigest)
	}
}
