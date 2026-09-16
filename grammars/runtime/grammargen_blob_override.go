package grammarruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

const grammargenBlobDirEnv = "GOTREESITTER_GRAMMARGEN_BLOB_DIR"

type preferredLanguageOverrideCacheEntry struct {
	path string
	once sync.Once
	lang *gotreesitter.Language
	err  error
}

var (
	preferredLanguageOverrideCacheMu sync.Mutex
	preferredLanguageOverrideCache   = map[string]*preferredLanguageOverrideCacheEntry{}
)

// loadPreferredLanguage loads the checked-in blob for a language while honoring
// GOTREESITTER_GRAMMARGEN_BLOB_DIR overrides. The override lookup is applied by
// loadEmbeddedLanguage for all built-in languages, so callers can use either
// entrypoint.
func loadPreferredLanguage(name string) *gotreesitter.Language {
	return loadEmbeddedLanguage(name + ".bin")
}

func loadPreferredLanguageOverride(name string) *gotreesitter.Language {
	path := preferredLanguageOverridePath(name)
	if path == "" {
		return nil
	}

	entry := getPreferredLanguageOverrideCacheEntry(path)
	entry.once.Do(func() {
		data, err := os.ReadFile(path)
		if err != nil {
			entry.err = fmt.Errorf("read grammar blob %q: %w", path, err)
			return
		}
		entry.lang, entry.err = decodeLanguageBlobData(path, data)
		if entry.err != nil {
			return
		}
		if entry.lang == nil {
			entry.err = fmt.Errorf("decoded nil language")
			return
		}
		if !entry.lang.CompatibleWithRuntime() {
			entry.err = fmt.Errorf("override blob %q uses incompatible ABI version %d", path, entry.lang.Version())
			return
		}
		if len(entry.lang.ExternalSymbols) > 0 && !AdaptScannerForLanguage(name, entry.lang) {
			entry.err = fmt.Errorf("override blob %q requires scanner adaptation for %q", path, name)
			return
		}
	})
	if entry.err != nil {
		preferredLanguageOverrideCacheMu.Lock()
		delete(preferredLanguageOverrideCache, path)
		preferredLanguageOverrideCacheMu.Unlock()
		return nil
	}
	return entry.lang
}

func preferredLanguageOverridePath(name string) string {
	root := strings.TrimSpace(os.Getenv(grammargenBlobDirEnv))
	if root == "" {
		return ""
	}
	return filepath.Join(root, name+".bin")
}

func getPreferredLanguageOverrideCacheEntry(path string) *preferredLanguageOverrideCacheEntry {
	preferredLanguageOverrideCacheMu.Lock()
	defer preferredLanguageOverrideCacheMu.Unlock()
	if entry, ok := preferredLanguageOverrideCache[path]; ok {
		return entry
	}
	entry := &preferredLanguageOverrideCacheEntry{path: path}
	preferredLanguageOverrideCache[path] = entry
	return entry
}

func purgePreferredLanguageOverrideCache() {
	preferredLanguageOverrideCacheMu.Lock()
	defer preferredLanguageOverrideCacheMu.Unlock()
	preferredLanguageOverrideCache = map[string]*preferredLanguageOverrideCacheEntry{}
}
