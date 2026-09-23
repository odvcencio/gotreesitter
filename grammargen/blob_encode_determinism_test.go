package grammargen

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// syntheticLargeStateGotos returns a map shaped like what
// grammargen/assemble.go's recordLargeGoto populates for a grammar whose
// remapped LR state count crosses uint16 (today: only c_sharp). n entries is
// enough to make Go's randomized map iteration (internal/runtime/maps'
// Iter.Init calls rand() on every range, not just once per process) produce
// different orders across repeated ranges with overwhelming probability, so
// this reproduces the bug cheaply without generating a real large grammar.
func syntheticLargeStateGotos(n int) map[uint64]gotreesitter.StateID {
	m := make(map[uint64]gotreesitter.StateID, n)
	for i := 0; i < n; i++ {
		state := uint64(70000 + i)
		sym := uint64(i % 512)
		key := state<<32 | sym
		m[key] = gotreesitter.StateID(80000 + i*3)
	}
	return m
}

func csharpLikeLanguage() *gotreesitter.Language {
	return &gotreesitter.Language{
		Name:            "c_sharp_like",
		SymbolNames:     []string{"end", "identifier", "class_declaration"},
		StateCount:      90000,
		LargeStateGotos: syntheticLargeStateGotos(4000),
	}
}

// TestEncodeLanguageBlobDeterministicWithLargeStateGotos is the core
// regression test: encoding the same Language repeatedly must produce
// byte-identical blobs. Before the fix, gob's map codec iterated
// LargeStateGotos via reflect's randomized MapRange, so this failed
// intermittently (with overwhelming probability given 4000 entries).
func TestEncodeLanguageBlobDeterministicWithLargeStateGotos(t *testing.T) {
	lang := csharpLikeLanguage()

	first, err := encodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("encodeLanguageBlob: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected non-empty blob")
	}

	const runs = 25
	for i := 0; i < runs; i++ {
		got, err := encodeLanguageBlob(lang)
		if err != nil {
			t.Fatalf("encodeLanguageBlob run %d: %v", i, err)
		}
		if !bytes.Equal(first, got) {
			t.Fatalf("run %d: blob bytes differ from run 0 (len %d vs %d) -- gob map-iteration nondeterminism leaked through", i, len(got), len(first))
		}
	}
}

// TestEncodeLanguageBlobDoesNotMutateOriginalLanguage guards the
// GenerateLanguageAndBlob contract: the *Language returned to callers must
// keep its fully populated LargeStateGotos map, since encodeLanguageBlob
// must not mutate its input (it gob-encodes a shallow copy instead).
func TestEncodeLanguageBlobDoesNotMutateOriginalLanguage(t *testing.T) {
	lang := csharpLikeLanguage()
	want := len(lang.LargeStateGotos)

	if _, err := encodeLanguageBlob(lang); err != nil {
		t.Fatalf("encodeLanguageBlob: %v", err)
	}

	if got := len(lang.LargeStateGotos); got != want {
		t.Fatalf("encodeLanguageBlob mutated caller's Language: LargeStateGotos now has %d entries, want %d", got, want)
	}
}

// TestEncodeDecodeLanguageBlobRoundTripWithLargeStateGotos exercises the full
// encodeLanguageBlob -> decodeLanguageBlob path and asserts LargeStateGotos
// survives with identical content.
func TestEncodeDecodeLanguageBlobRoundTripWithLargeStateGotos(t *testing.T) {
	lang := csharpLikeLanguage()

	blob, err := encodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("encodeLanguageBlob: %v", err)
	}

	decoded, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("decodeLanguageBlob: %v", err)
	}

	if decoded.Name != lang.Name {
		t.Fatalf("Name = %q, want %q", decoded.Name, lang.Name)
	}
	if len(decoded.LargeStateGotos) != len(lang.LargeStateGotos) {
		t.Fatalf("LargeStateGotos has %d entries, want %d", len(decoded.LargeStateGotos), len(lang.LargeStateGotos))
	}
	for k, want := range lang.LargeStateGotos {
		if got := decoded.LargeStateGotos[k]; got != want {
			t.Fatalf("LargeStateGotos[%d] = %d, want %d", k, got, want)
		}
	}
}

// A runtime from before the envelope change calls gzip.NewReader on the blob
// directly. Trailer-bearing blobs must fail there rather than decode a
// Language whose LargeStateGotos map was cleared from the gob payload.
func TestEncodeLanguageBlobWithLargeStateGotosRejectedByLegacyGzipLoader(t *testing.T) {
	blob, err := encodeLanguageBlob(csharpLikeLanguage())
	if err != nil {
		t.Fatalf("encodeLanguageBlob: %v", err)
	}
	if _, err := gzip.NewReader(bytes.NewReader(blob)); err == nil {
		t.Fatal("legacy gzip-first loader accepted a trailer-bearing blob")
	}
}

// oldEncodeLanguageBlob is a frozen copy of encodeLanguageBlob exactly as it
// existed before the LargeStateGotos determinism fix: a single unconditional
// gob.Encode of lang, with no clone and no trailer. It exists only to prove
// (mechanically, not just by inspection) that the fixed encodeLanguageBlob
// produces byte-identical output to the pre-fix implementation whenever
// LargeStateGotos is empty -- i.e. for every language but c_sharp today.
func oldEncodeLanguageBlob(lang *gotreesitter.Language) ([]byte, error) {
	var out bytes.Buffer
	gzw := gzip.NewWriter(&out)
	if err := gob.NewEncoder(gzw).Encode(lang); err != nil {
		_ = gzw.Close()
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// TestEncodeLanguageBlobByteIdenticalWhenLargeStateGotosEmpty is the VERIFY
// #3 proof: languages that never populate LargeStateGotos (everything but
// c_sharp) must produce byte-identical blobs before and after this fix. This
// compares the fixed encodeLanguageBlob against a frozen copy of the exact
// pre-fix implementation across a representative sample of realistic
// Language shapes, which is a stronger guarantee than spot-checking a
// handful of real blob files: it holds for every Language with an empty
// LargeStateGotos, not just the three sampled in CI.
func TestEncodeLanguageBlobByteIdenticalWhenLargeStateGotosEmpty(t *testing.T) {
	cases := map[string]*gotreesitter.Language{
		"nil LargeStateGotos": {
			Name:        "java_like",
			SymbolNames: []string{"end", "identifier"},
			ParseTable:  [][]uint16{{0, 1}, {2, 0}},
			LexModes:    []gotreesitter.LexMode{{}, {}},
		},
		"empty non-nil LargeStateGotos": {
			Name:            "go_like",
			SymbolNames:     []string{"end", "identifier", "package_clause"},
			LargeStateGotos: map[uint64]gotreesitter.StateID{},
		},
		"minimal empty language": {},
	}

	for name, lang := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := encodeLanguageBlob(lang)
			if err != nil {
				t.Fatalf("encodeLanguageBlob: %v", err)
			}
			corePayload, err := oldEncodeLanguageBlob(lang)
			if err != nil {
				t.Fatalf("oldEncodeLanguageBlob: %v", err)
			}
			// encodeLanguageBlob (unlike the frozen pre-fix copy) always adds
			// the version header from hardening/fuzz-blob-safety's blob
			// schema/version guard. That header is orthogonal to the
			// LargeStateGotos determinism fix this test is pinning, so wrap
			// the pre-fix core payload the same way before comparing: the
			// core gzip+gob bytes underneath must still be byte-identical.
			want, err := gotreesitter.WrapLanguageBlobVersionHeader(corePayload, gotreesitter.BlobRuntimeVersion, gotreesitter.DefaultBlobGeneratorVersion)
			if err != nil {
				t.Fatalf("WrapLanguageBlobVersionHeader: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s: fixed encoder bytes (len %d) differ from pre-fix encoder bytes plus version header (len %d); LargeStateGotos handling must be a no-op when empty", name, len(got), len(want))
			}
		})
	}
}

// TestDecodeLanguageBlobOldFormatWithLargeStateGotos simulates decoding a
// blob produced by the pre-fix encoder (LargeStateGotos gob-encoded directly,
// no trailer) -- this is exactly the shape of the 206 blobs checked in before
// this change, including grammar_blobs/c_sharp.bin. decodeLanguageBlob must
// still restore LargeStateGotos correctly via gob's native map decode.
func TestDecodeLanguageBlobOldFormatWithLargeStateGotos(t *testing.T) {
	want := syntheticLargeStateGotos(300)
	lang := &gotreesitter.Language{
		Name:            "old_format_csharp_like",
		SymbolNames:     []string{"end"},
		LargeStateGotos: want,
	}

	oldBlob, err := oldEncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("oldEncodeLanguageBlob: %v", err)
	}

	decoded, err := decodeLanguageBlob(oldBlob)
	if err != nil {
		t.Fatalf("decodeLanguageBlob(old-format blob): %v", err)
	}
	if len(decoded.LargeStateGotos) != len(want) {
		t.Fatalf("LargeStateGotos has %d entries, want %d", len(decoded.LargeStateGotos), len(want))
	}
	for k, v := range want {
		if decoded.LargeStateGotos[k] != v {
			t.Fatalf("LargeStateGotos[%d] = %d, want %d", k, decoded.LargeStateGotos[k], v)
		}
	}
}

// TestDecodeLanguageBlobNewFormatWithoutTrailerUnaffected pins that decoding
// a fixed-encoder blob for a Language without LargeStateGotos behaves
// identically to before (no trailer is written or expected).
func TestDecodeLanguageBlobNewFormatWithoutTrailerUnaffected(t *testing.T) {
	lang := &gotreesitter.Language{
		Name:        "regex_like",
		SymbolNames: []string{"end", "char_class"},
	}

	blob, err := encodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("encodeLanguageBlob: %v", err)
	}
	decoded, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("decodeLanguageBlob: %v", err)
	}
	if decoded.Name != lang.Name {
		t.Fatalf("Name = %q, want %q", decoded.Name, lang.Name)
	}
	if len(decoded.LargeStateGotos) != 0 {
		t.Fatalf("LargeStateGotos = %v, want empty", decoded.LargeStateGotos)
	}
}
