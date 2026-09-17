package grammarruntime

import (
	"testing"
	"time"
)

// resetEmbeddedLanguageCacheForTest returns the cache to its default state
// (unlimited, no idle TTL, empty) and registers cleanup to restore it.
func resetEmbeddedLanguageCacheForTest(t *testing.T) {
	t.Helper()
	SetEmbeddedLanguageCacheLimit(-1)
	SetEmbeddedLanguageIdleTTL(0)
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() {
		SetEmbeddedLanguageCacheLimit(-1)
		SetEmbeddedLanguageIdleTTL(0)
		PurgeEmbeddedLanguageCache()
	})
}

func embeddedCacheHas(blob string) bool {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()
	_, ok := embeddedLanguageCache[blob]
	return ok
}

// TestEmbeddedLanguageCacheLimitEvictsPreloaded pins the contract that a cache
// limit set AFTER languages were already loaded (while the cache was unlimited)
// still evicts down to the limit. This is the load-while-eviction-inactive ->
// activate transition that any loader fast-path must preserve by rebuilding the
// LRU on activation.
func TestEmbeddedLanguageCacheLimitEvictsPreloaded(t *testing.T) {
	resetEmbeddedLanguageCacheForTest(t)

	// Preload several languages while the cache is unlimited (the default).
	_ = GoLanguage()
	_ = CssLanguage()
	_ = JsonLanguage()
	_ = PythonLanguage()
	if loaded, _ := EmbeddedLanguageCacheStats(); loaded < 4 {
		t.Fatalf("preload: loaded=%d want >=4", loaded)
	}

	// Now cap to 2 — the preloaded entries must be evicted down to the limit.
	SetEmbeddedLanguageCacheLimit(2)
	loaded, limit := EmbeddedLanguageCacheStats()
	if limit != 2 {
		t.Fatalf("limit=%d want 2", limit)
	}
	if loaded > 2 {
		t.Fatalf("after SetCacheLimit(2): loaded=%d want <=2", loaded)
	}
}

// TestEmbeddedLanguageCacheLimitEnforcedOnLoad pins that a limit set BEFORE
// loading caps the cache as languages are loaded.
func TestEmbeddedLanguageCacheLimitEnforcedOnLoad(t *testing.T) {
	resetEmbeddedLanguageCacheForTest(t)

	SetEmbeddedLanguageCacheLimit(2)
	_ = GoLanguage()
	_ = CssLanguage()
	_ = JsonLanguage()
	_ = PythonLanguage()
	if loaded, _ := EmbeddedLanguageCacheStats(); loaded > 2 {
		t.Fatalf("with limit 2: loaded=%d want <=2", loaded)
	}
}

// TestEmbeddedLanguageIdleEvicts pins that idle-TTL eviction removes a language
// that has not been accessed within the TTL when another load triggers a sweep.
func TestEmbeddedLanguageIdleEvicts(t *testing.T) {
	resetEmbeddedLanguageCacheForTest(t)

	SetEmbeddedLanguageIdleTTL(1 * time.Millisecond)
	_ = GoLanguage()
	if !embeddedCacheHas("go.bin") {
		t.Fatal("go.bin not cached after load")
	}
	time.Sleep(10 * time.Millisecond)

	// Loading another language triggers an idle sweep; go (idle > TTL) evicts.
	_ = CssLanguage()
	if embeddedCacheHas("go.bin") {
		t.Fatal("go.bin should have been idle-evicted")
	}
	if !embeddedCacheHas("css.bin") {
		t.Fatal("css.bin should still be cached (just loaded)")
	}
}
