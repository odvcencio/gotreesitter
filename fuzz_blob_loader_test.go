package gotreesitter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// fuzzBlobLoaderSeedFiles are a handful of the smallest real shipped grammar
// blobs, used as a starting corpus for FuzzLoadLanguage so the fuzzer spends
// its time mutating genuine gzip+gob+envelope structure instead of
// rediscovering gzip's magic bytes from nothing.
var fuzzBlobLoaderSeedFiles = []string{
	"grammars/grammar_blobs/eds.bin",
	"grammars/grammar_blobs/pem.bin",
	"grammars/grammar_blobs/todotxt.bin",
	"grammars/grammar_blobs/ebnf.bin",
	"grammars/grammar_blobs/ini.bin",
}

// FuzzLoadLanguage exercises the blob loader (LoadLanguage, and transitively
// UnwrapLanguageBlobVersionHeader, UnwrapLanguageBlobEnvelope,
// ReadAllGzipWithSizeHint, and validateDecodedLanguage) against arbitrary
// bytes. It is the fuzz target for hardening/fuzz-blob-safety finding #1
// (decompression bomb / unvalidated table indices): a malformed or
// adversarial blob must return an error, never panic, and never allocate
// past MaxDecompressedBlobSize.
func FuzzLoadLanguage(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte{0x1f, 0x8b})
	f.Add([]byte("GTSBLOB\x00"))
	f.Add([]byte("GTSVER1\x00"))

	for _, path := range fuzzBlobLoaderSeedFiles {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			// Best-effort: the fuzz corpus is still useful without these
			// (the synthetic seeds above still cover the magic bytes), and a
			// missing file here should not fail the whole build.
			continue
		}
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4<<20 {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic while loading fuzz input (%d bytes): %v", len(data), r)
			}
		}()

		lang, err := gotreesitter.LoadLanguage(data)
		if err != nil {
			if lang != nil {
				t.Fatalf("LoadLanguage returned a non-nil Language alongside an error: %v", err)
			}
			return
		}
		if lang == nil {
			t.Fatal("LoadLanguage returned a nil Language with no error")
		}
	})
}
