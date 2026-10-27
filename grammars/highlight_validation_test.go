package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// highlightNoRangesExpected tracks languages whose highlight query compiles
// but the smoke sample is too simple to produce any highlight ranges.
var highlightNoRangesExpected = map[string]bool{
	"cpp":     true, // C++ highlight query requires predicate support beyond current Highlighter
	"angular": true, // upstream highlights.scm (synced at 38a8014) targets bindings, pipes, and control-flow syntax the plain-tag smoke sample does not exercise
}

// highlightSampleOverrides replaces the parse smoke sample of a language whose
// sample reaches no capture that colors text, so the zero-ranges check below can
// still fail for it. The shared smoke sample stays as it is: the parser,
// admission and parity suites read it for every language, and the admission
// scorecard ratchet (admissionScorecardRequiredCompactPasses) lists djot against it.
var highlightSampleOverrides = map[string]string{
	// The plain-paragraph smoke sample matches only @spell, which colors nothing
	// and is not returned. A heading is the smallest djot construct with a color.
	"djot": "# Heading\n\nHello world\n",
}

func TestAllHighlightQueriesCompile(t *testing.T) {
	entries := AllLanguages()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	var withQuery int
	var compileErrs int

	for _, entry := range entries {
		if strings.TrimSpace(entry.HighlightQuery) == "" {
			continue
		}
		withQuery++
		lang := entry.Language()
		if _, err := gotreesitter.NewQuery(entry.HighlightQuery, lang); err != nil {
			compileErrs++
			t.Errorf("%s: highlight query compile error: %v", entry.Name, err)
		}
		UnloadEmbeddedLanguage(entry.Name + ".bin")
	}

	t.Logf("highlight compile audit: with_query=%d compile_errors=%d", withQuery, compileErrs)
}

func TestAllTagsQueriesCompile(t *testing.T) {
	entries := AllLanguages()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	var withQuery int
	var compileErrs int

	for _, entry := range entries {
		tagsQ := ResolveTagsQuery(entry)
		if strings.TrimSpace(tagsQ) == "" {
			continue
		}
		withQuery++
		lang := entry.Language()
		if _, err := gotreesitter.NewTagger(lang, tagsQ); err != nil {
			compileErrs++
			t.Errorf("%s: tags query compile error: %v", entry.Name, err)
		}
		UnloadEmbeddedLanguage(entry.Name + ".bin")
	}

	t.Logf("tags compile audit: with_query=%d compile_errors=%d", withQuery, compileErrs)
}

func TestHighlightQueriesProduceResults(t *testing.T) {
	entries := AllLanguages()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	var tested, skippedNoQuery, skippedNoSample, skippedUnsupported int
	for _, entry := range entries {
		name := entry.Name
		if strings.TrimSpace(entry.HighlightQuery) == "" {
			skippedNoQuery++
			continue
		}

		lang := entry.Language()
		report := EvaluateParseSupport(entry, lang)
		if report.Backend == ParseBackendUnsupported {
			skippedUnsupported++
			UnloadEmbeddedLanguage(entry.Name + ".bin")
			continue
		}

		sample := ParseSmokeSample(name)
		if override, ok := highlightSampleOverrides[name]; ok {
			sample = override
		}
		if sample == "x\n" {
			skippedNoSample++
			UnloadEmbeddedLanguage(entry.Name + ".bin")
			continue
		}

		tested++
		t.Run(name, func(t *testing.T) {
			// Build highlighter options.
			var opts []gotreesitter.HighlighterOption
			if entry.TokenSourceFactory != nil {
				factory := entry.TokenSourceFactory
				opts = append(opts, gotreesitter.WithTokenSourceFactory(
					func(src []byte) gotreesitter.TokenSource {
						return factory(src, lang)
					},
				))
			}

			h, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
			if err != nil {
				t.Fatalf("compile highlight query: %v", err)
			}

			ranges := h.Highlight([]byte(sample))
			if len(ranges) == 0 && !highlightNoRangesExpected[name] {
				t.Errorf("highlight query compiled but produced 0 ranges for sample %q", sample)
			}
		})
		UnloadEmbeddedLanguage(entry.Name + ".bin")
	}

	t.Logf("highlight validation: tested=%d skipped(no_query=%d no_sample=%d unsupported=%d)",
		tested, skippedNoQuery, skippedNoSample, skippedUnsupported)

}
