// Package standaloneregistry records opt-in grammar metadata without importing
// the aggregate grammar catalog.
package standaloneregistry

import (
	"sync"
	"sync/atomic"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Entry describes one opt-in standalone grammar.
type Entry struct {
	Name              string
	Extensions        []string
	Aliases           []string
	GenerateLanguage  func() (*gotreesitter.Language, error)
	GrammarSource     string
	HighlightQuery    string
	InheritHighlights string
	TagsQuery         string
}

var (
	registryMu sync.RWMutex
	registry   []Entry
	generation atomic.Uint64
)

// Register adds or replaces one standalone grammar entry.
func Register(entry Entry) {
	entry.Extensions = append([]string(nil), entry.Extensions...)
	entry.Aliases = append([]string(nil), entry.Aliases...)

	registryMu.Lock()
	defer registryMu.Unlock()
	for index := range registry {
		if registry[index].Name == entry.Name {
			registry[index] = entry
			generation.Add(1)
			return
		}
	}
	registry = append(registry, entry)
	generation.Add(1)
}

// Generation returns the current metadata revision without copying entries.
func Generation() uint64 { return generation.Load() }

// Snapshot returns independent metadata and its current generation.
func Snapshot() ([]Entry, uint64) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Entry, len(registry))
	for index := range registry {
		out[index] = registry[index]
		out[index].Extensions = append([]string(nil), registry[index].Extensions...)
		out[index].Aliases = append([]string(nil), registry[index].Aliases...)
	}
	return out, generation.Load()
}
