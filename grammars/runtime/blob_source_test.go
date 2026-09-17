package grammarruntime

import (
	"errors"
	"testing"
)

func TestBlobSourcesPreserveCatalogSelectionAndErrors(t *testing.T) {
	blobSources.Lock()
	readers, catalog, canonical := blobSources.readers, blobSources.catalog, blobSources.canonical
	blobSources.readers = make(map[string]func() []byte)
	blobSources.catalog, blobSources.canonical = nil, nil
	blobSources.Unlock()
	t.Cleanup(func() {
		blobSources.Lock()
		defer blobSources.Unlock()
		blobSources.readers, blobSources.catalog, blobSources.canonical = readers, catalog, canonical
	})

	RegisterBlob("selected", func() []byte { return []byte("standalone") })
	blob, err := readGrammarBlob("selected.bin")
	if err != nil || string(blob.data) != "standalone" {
		t.Fatalf("standalone blob = %q, %v", blob.data, err)
	}
	if _, err := readGrammarBlob("absent.bin"); err == nil {
		t.Fatal("missing provider did not fail")
	}
	released := false
	RegisterCatalog(func(string) ([]byte, func(), error) {
		return []byte("catalog"), func() { released = true }, nil
	}, nil)
	blob, err = readGrammarBlob("selected.bin")
	if err != nil || string(blob.data) != "catalog" {
		t.Fatalf("catalog blob = %q, %v", blob.data, err)
	}
	blob.close()
	if !released {
		t.Fatal("catalog blob was not released")
	}
	want := errors.New("external blob is missing")
	RegisterCatalog(func(string) ([]byte, func(), error) { return nil, nil, want }, nil)
	if _, err := readGrammarBlob("selected.bin"); !errors.Is(err, want) {
		t.Fatalf("catalog error = %v, want %v", err, want)
	}
}
