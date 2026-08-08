package grammars

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func preserveRegistryState(t *testing.T) {
	t.Helper()
	ensureReady()

	registryMu.RLock()
	savedRegistry := append([]LangEntry(nil), registry...)
	savedResolved := highlightInheritanceResolved
	savedAliases := make(map[string]string, len(extensionAliases))
	for alias, name := range extensionAliases {
		savedAliases[alias] = name
	}
	registryMu.RUnlock()

	t.Cleanup(func() {
		registryMu.Lock()
		defer registryMu.Unlock()

		registry = savedRegistry
		highlightInheritanceResolved = savedResolved
		extensionAliases = savedAliases
		buildExtIndex()
	})
}

func preserveParserPool(t *testing.T, name string) {
	t.Helper()
	poolsMu.Lock()
	saved, existed := pools[name]
	delete(pools, name)
	poolsMu.Unlock()

	t.Cleanup(func() {
		poolsMu.Lock()
		defer poolsMu.Unlock()
		if existed {
			pools[name] = saved
		} else {
			delete(pools, name)
		}
	})
}

func TestRegisterExtensionGeneratesLanguageOnceConcurrently(t *testing.T) {
	preserveRegistryState(t)

	const workers = 32
	want := &gotreesitter.Language{Name: "zz_extension_once"}
	var calls atomic.Int32
	generatorStarted := make(chan struct{})
	releaseGenerator := make(chan struct{})

	RegisterExtension(ExtensionEntry{
		Name:       "zz_extension_once",
		Extensions: []string{".zz-extension-once"},
		GenerateLanguage: func() (*gotreesitter.Language, error) {
			calls.Add(1)
			close(generatorStarted)
			<-releaseGenerator
			return want, nil
		},
	})

	entry := DetectLanguage("sample.zz-extension-once")
	if entry == nil {
		t.Fatal("expected extension language to be registered")
	}

	start := make(chan struct{})
	results := make(chan *gotreesitter.Language, workers)
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(workers)
	done.Add(workers)
	for range workers {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			results <- entry.Language()
		}()
	}

	ready.Wait()
	close(start)
	<-generatorStarted
	close(releaseGenerator)
	done.Wait()
	close(results)

	for got := range results {
		if got != want {
			t.Errorf("Language() = %p, want %p", got, want)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("GenerateLanguage called %d times, want 1", got)
	}
}

func TestRegisterExtensionCachesGenerationError(t *testing.T) {
	preserveRegistryState(t)

	var calls atomic.Int32
	wantErr := errors.New("generate failed")
	RegisterExtension(ExtensionEntry{
		Name:       "zz_extension_error_once",
		Extensions: []string{".zz-extension-error"},
		GenerateLanguage: func() (*gotreesitter.Language, error) {
			calls.Add(1)
			return nil, wantErr
		},
	})

	entry := DetectLanguage("sample.zz-extension-error")
	if entry == nil {
		t.Fatal("expected extension language to be registered")
	}
	for i := 0; i < 3; i++ {
		if got := entry.Language(); got != nil {
			t.Fatalf("Language() call %d = %p, want nil after generation error", i+1, got)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("GenerateLanguage called %d times after cached error, want 1", got)
	}
}

func TestRegisterExtensionPreservesBlobSourceAndTags(t *testing.T) {
	preserveRegistryState(t)

	const tagsQuery = "(identifier) @name @definition.variable"
	RegisterExtension(ExtensionEntry{
		Name:          "zz_extension_blob_metadata",
		Extensions:    []string{".zz-extension-blob"},
		GrammarSource: GrammarSourceGrammargenBlob,
		TagsQuery:     tagsQuery,
		GenerateLanguage: func() (*gotreesitter.Language, error) {
			return nil, nil
		},
	})

	entry := DetectLanguage("sample.zz-extension-blob")
	if entry == nil {
		t.Fatal("expected extension language to be registered")
	}
	if entry.GrammarSource != GrammarSourceGrammargenBlob {
		t.Fatalf("GrammarSource = %q, want %q", entry.GrammarSource, GrammarSourceGrammargenBlob)
	}
	if entry.TagsQuery != tagsQuery {
		t.Fatalf("TagsQuery = %q, want %q", entry.TagsQuery, tagsQuery)
	}
}

func TestParseFilePooledReplacesPoolForReplacedLanguage(t *testing.T) {
	preserveRegistryState(t)
	const name = "zz_pool_language_identity"
	preserveParserPool(t, name)

	goLang := GoLanguage()
	pythonLang := PythonLanguage()
	Register(LangEntry{
		Name:       name,
		Extensions: []string{".zz-pool-identity"},
		Language: func() *gotreesitter.Language {
			return goLang
		},
	})

	first, err := ParseFilePooled("before.zz-pool-identity", []byte("package before\n"))
	if err != nil {
		t.Fatalf("first ParseFilePooled: %v", err)
	}
	if got := first.Language(); got != goLang {
		first.Release()
		t.Fatalf("first tree language = %p, want %p", got, goLang)
	}
	first.Release()

	poolsMu.RLock()
	firstPool := pools[name]
	poolsMu.RUnlock()
	if firstPool == nil {
		t.Fatal("first ParseFilePooled did not cache a parser pool")
	}

	Register(LangEntry{
		Name:       name,
		Extensions: []string{".zz-pool-identity"},
		Language: func() *gotreesitter.Language {
			return pythonLang
		},
	})

	second, err := ParseFilePooled("after.zz-pool-identity", []byte("value = 1\n"))
	if err != nil {
		t.Fatalf("second ParseFilePooled: %v", err)
	}
	if got := second.Language(); got != pythonLang {
		second.Release()
		t.Fatalf("second tree language = %p, want replacement %p", got, pythonLang)
	}
	second.Release()

	poolsMu.RLock()
	secondPool := pools[name]
	poolsMu.RUnlock()
	if secondPool == firstPool {
		t.Fatal("ParseFilePooled reused the pool built for the replaced Language")
	}
	if got := secondPool.Language(); got != pythonLang {
		t.Fatalf("replacement pool language = %p, want %p", got, pythonLang)
	}
}
