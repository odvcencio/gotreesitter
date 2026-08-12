package bench_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func setupBench() ([]byte, *gotreesitter.InjectionParser, string) {
	source := []byte(`
<html>
  <body>
    <script>
      function hello() {
        console.log("Hello, world!");
        const a = 1; const b = 2; return a + b;
      }
    </script>
  </body>
</html>
`)
	ip := gotreesitter.NewInjectionParser()

	htmlLang := grammars.HtmlLanguage()
	jsLang := grammars.JavascriptLanguage()

	ip.RegisterLanguage("html", htmlLang)
	ip.RegisterLanguage("javascript", jsLang)

	query := `(script_element (raw_text) @injection.content (#set! injection.language "javascript"))`
	_ = ip.RegisterInjectionQuery("html", query)

	return source, ip, "html"
}

func BenchmarkInjectionParser_Parse(b *testing.B) {
	source, ip, langName := setupBench()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res, err := ip.Parse(source, langName)
		if err != nil {
			b.Fatal(err)
		}
		_ = res
	}
}

func BenchmarkInjectionParser_ParseIncremental(b *testing.B) {
	source, ip, langName := setupBench()

	oldResult, err := ip.Parse(source, langName)
	if err != nil {
		b.Fatalf("initial parse failed: %v", err)
	}

	if oldResult == nil || oldResult.Tree == nil {
		b.Fatal("oldResult or oldResult.Tree is nil")
	}

	newSource := []byte(string(source) + "\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res, err := ip.ParseIncremental(newSource, langName, oldResult)
		if err != nil {
			b.Fatal(err)
		}
		oldResult = res
	}
}

func BenchmarkInjectionParser_ParseReuse(b *testing.B) {
	source, ip, langName := setupBench()

	// Warmup - parse once before measuring.
	ip.Parse(source, langName)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip.Parse(source, langName) // Same source, same parser
	}
}

func TestInjectionParser_ArenaReuseBetweenParses(t *testing.T) {
	source, ip, langName := setupBench()

	gotreesitter.EnableArenaProfile(true)
	defer gotreesitter.EnableArenaProfile(false)

	// First parse
	ip.Parse(source, langName)

	// Reset profile after first parse (warmup)
	gotreesitter.ResetArenaProfile()

	// Do 100 parses with same source
	for i := 0; i < 100; i++ {
		ip.Parse(source, langName)
	}

	profile := gotreesitter.ArenaProfileSnapshot()
	t.Logf("Full acquire: %d, Full new: %d", profile.FullAcquire, profile.FullNew)
	t.Logf("Incremental acquire: %d, Incremental new: %d", profile.IncrementalAcquire, profile.IncrementalNew)

	// After fix: should reuse arenas. Full new should be 0 or very low.
	// Before fix: Full new was 100 (100% new).
	if profile.FullNew > 1 {
		t.Errorf("Expected arena reuse (FullNew <= 1), got FullNew=%d", profile.FullNew)
	}
}

func TestInjectionParser_ArenaReuseBetweenIncrementalParses(t *testing.T) {
	source, ip, langName := setupBench()

	// First parse
	firstResult, err := ip.Parse(source, langName)
	if err != nil {
		t.Fatal(err)
	}

	gotreesitter.EnableArenaProfile(true)
	defer gotreesitter.EnableArenaProfile(false)

	// Reset profile after warmup
	gotreesitter.ResetArenaProfile()

	// Do 100 incremental parses
	newSource := []byte(string(source) + "\n")
	result := firstResult
	for i := 0; i < 100; i++ {
		result, err = ip.ParseIncremental(newSource, langName, result)
		if err != nil {
			t.Fatal(err)
		}
	}

	profile := gotreesitter.ArenaProfileSnapshot()
	t.Logf("Full acquire: %d, Full new: %d", profile.FullAcquire, profile.FullNew)
	t.Logf("Incremental acquire: %d, Incremental new: %d", profile.IncrementalAcquire, profile.IncrementalNew)

	// After fix: should reuse arenas. New should be 0 or very low.
	if profile.IncrementalNew > 1 {
		t.Errorf("Expected arena reuse (IncrementalNew <= 1), got IncrementalNew=%d", profile.IncrementalNew)
	}
}
