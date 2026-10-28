package gotreesitter

import (
	"sync"
	"testing"
)

func parseRuntimeHasNormalizationPass(rt ParseRuntime, name string) bool {
	if rt.NormalizationPasses == nil {
		return false
	}
	for _, pass := range *rt.NormalizationPasses {
		if pass.Name == name {
			return true
		}
	}
	return false
}

func seedStaleForestNormalizationStats(parser *Parser) {
	parser.recordNormalizationMetric("stale.before-forest-attempt", 1, 1, 7, 3)
}

func TestForestAttemptResetsNormalizationStatsOnParserReuse(t *testing.T) {
	previous := glrForestEnabled.Load()
	glrForestEnabled.Store(true)
	t.Cleanup(func() { glrForestEnabled.Store(previous) })

	t.Run("success", func(t *testing.T) {
		lang := loadBlobForDecode(t, "json")
		lang.WantsForest = true
		parser := NewParser(lang)
		seedStaleForestNormalizationStats(parser)

		tree, err := parser.Parse([]byte(`[1, 2, 3]`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		defer tree.Release()
		rt := tree.ParseRuntime()
		if !rt.ForestFastPath {
			t.Fatalf("parse did not use the forest path: %s", rt.Summary())
		}
		if parseRuntimeHasNormalizationPass(rt, "stale.before-forest-attempt") {
			t.Fatal("successful forest parse published stale normalization stats")
		}
	})

	t.Run("decline", func(t *testing.T) {
		lang := loadBlobForDecode(t, "json")
		parser := NewParser(lang)
		seedStaleForestNormalizationStats(parser)

		tree, ok := parser.ParseForestExperimental([]byte(`[`))
		if tree != nil {
			tree.Release()
		}
		if ok {
			t.Fatal("invalid JSON unexpectedly produced an experimental forest tree")
		}
		if len(parser.normalizationStats.namedPasses) != 0 {
			t.Fatalf("declined forest attempt retained normalization stats: %+v", parser.normalizationStats.namedPasses)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		lang := loadBlobForDecode(t, "json")
		lang.WantsForest = true
		parser := NewParser(lang)
		seedStaleForestNormalizationStats(parser)

		tree, err := parser.Parse([]byte(`[`))
		if err != nil {
			t.Fatalf("fallback parse: %v", err)
		}
		defer tree.Release()
		rt := tree.ParseRuntime()
		if rt.ForestFastPath {
			t.Fatalf("invalid JSON unexpectedly returned a forest tree: %s", rt.Summary())
		}
		if parseRuntimeHasNormalizationPass(rt, "stale.before-forest-attempt") {
			t.Fatal("forest fallback published stale normalization stats")
		}
	})
}

// Parses read the forest switch and the arena profile counters while tests and
// benchmarks toggle them. Run under -race: the plain globals this replaced
// reported a race on glrForestEnabled and on the arena counters.
func TestExportedTogglesRaceFree(t *testing.T) {
	previousForest := glrForestEnabled.Load()
	previousRecover := glrForestRecover.Load()
	t.Cleanup(func() {
		glrForestEnabled.Store(previousForest)
		glrForestRecover.Store(previousRecover)
		EnableArenaProfile(false)
		ResetArenaProfile()
	})

	lang := loadBlobForDecode(t, "json")
	source := []byte(`{"a": [1, 2, 3], "b": {"c": null}}`)

	stop := make(chan struct{})
	var toggler sync.WaitGroup
	toggler.Add(1)
	go func() {
		defer toggler.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			SetGLRForestEnabled(i%2 == 0)
			SetGLRForestRecover(i%3 == 0)
			EnableArenaProfile(i%2 == 0)
			if i%5 == 0 {
				ResetArenaProfile()
			}
			_ = ArenaProfileSnapshot()
		}
	}()

	var parsers sync.WaitGroup
	for g := 0; g < 2; g++ {
		parsers.Add(1)
		go func() {
			defer parsers.Done()
			parser := NewParser(lang)
			for i := 0; i < 20; i++ {
				tree, err := parser.Parse(source)
				if err != nil {
					t.Errorf("parse: %v", err)
					return
				}
				tree.Release()
			}
		}()
	}
	parsers.Wait()
	close(stop)
	toggler.Wait()
}
