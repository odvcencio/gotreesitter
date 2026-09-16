// Package grammarruntime supplies shared support for packaged grammars.
// It does not embed grammar blobs or import the aggregate grammar catalog.
package grammarruntime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

var blobSources = struct {
	sync.RWMutex
	readers   map[string]func() []byte
	catalog   func(string) ([]byte, func(), error)
	canonical func(string) string
}{readers: make(map[string]func() []byte)}

// RegisterBlob registers a blob provider during package initialization.
// The provider must return the same artifact on every call.
func RegisterBlob(name string, read func() []byte) {
	blobSources.Lock()
	defer blobSources.Unlock()
	blobSources.readers[strings.TrimSuffix(name, ".bin")+".bin"] = read
}

// RegisterCatalog connects an aggregate catalog without adding its dependency.
// The reader returns blob bytes and an optional release function.
// Its source takes precedence over individual providers, including read errors.
func RegisterCatalog(read func(string) ([]byte, func(), error), canonical func(string) string) {
	blobSources.Lock()
	defer blobSources.Unlock()
	blobSources.catalog = read
	blobSources.canonical = canonical
}

type grammarBlob struct {
	data    []byte
	release func()
}

func (b grammarBlob) close() {
	if b.release != nil {
		b.release()
	}
}

func readGrammarBlob(name string) (grammarBlob, error) {
	blobSources.RLock()
	read, catalog := blobSources.readers[name], blobSources.catalog
	blobSources.RUnlock()
	if catalog != nil {
		data, release, err := catalog(name)
		return grammarBlob{data: data, release: release}, err
	}
	if read != nil {
		return grammarBlob{data: read()}, nil
	}
	return grammarBlob{}, fmt.Errorf("grammar blob %q is not registered", name)
}

// Language returns a cached grammar with its scanner, repairs, and runtime profile.
func Language(name string) *gotreesitter.Language {
	return loadEmbeddedLanguage(strings.TrimSuffix(name, ".bin") + ".bin")
}
