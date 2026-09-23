package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// gobGzipBlob gob-encodes v and gzip-compresses the result, mirroring the
// legacy (unenveloped) blob format LoadLanguage must keep accepting.
func gobGzipBlob(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gzw).Encode(v); err != nil {
		t.Fatalf("gob encode: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func TestValidateDecodedLanguageAcceptsAllShippedBlobs(t *testing.T) {
	matches, err := filepath.Glob("grammars/grammar_blobs/*.bin")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no shipped grammar blobs found at grammars/grammar_blobs -- run from the repo root")
	}
	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			lang, err := LoadLanguage(data)
			if err != nil {
				t.Fatalf("LoadLanguage(%s): %v", path, err)
			}
			if lang == nil {
				t.Fatalf("LoadLanguage(%s) returned a nil Language with no error", path)
			}
		})
	}
}

func TestValidateDecodedLanguageRejectsOutOfRangeReduceSymbol(t *testing.T) {
	lang := &Language{
		Name:        "evil_reduce_symbol",
		SymbolCount: 3,
		TokenCount:  1,
		SymbolNames: []string{"end", "a", "b"},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 99}}},
		},
	}
	blob := gobGzipBlob(t, lang)
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted a reduce action whose symbol is outside SymbolCount")
	}
}

func TestValidateDecodedLanguageRejectsOutOfRangeShiftState(t *testing.T) {
	lang := &Language{
		Name:        "evil_shift_state",
		SymbolCount: 2,
		TokenCount:  1,
		StateCount:  2,
		SymbolNames: []string{"end", "a"},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionShift, State: 999}}},
		},
	}
	blob := gobGzipBlob(t, lang)
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted a shift action whose target state is outside StateCount")
	}
}

func TestValidateDecodedLanguageRejectsOutOfRangeParseTableCell(t *testing.T) {
	lang := &Language{
		Name:        "evil_parse_table_cell",
		SymbolCount: 2,
		TokenCount:  1,
		StateCount:  1,
		SymbolNames: []string{"end", "a"},
		// Column 0 is the terminal "end"; the value 1 must index into
		// ParseActions, which is empty here.
		ParseTable: [][]uint16{{1}},
	}
	blob := gobGzipBlob(t, lang)
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted a parse table cell whose action index is outside ParseActions")
	}
}

func TestValidateDecodedLanguageRejectsTruncatedSmallParseTable(t *testing.T) {
	lang := &Language{
		Name:               "evil_small_parse_table",
		SymbolCount:        2,
		TokenCount:         1,
		StateCount:         1,
		SymbolNames:        []string{"end", "a"},
		SmallParseTableMap: []uint32{0},
		// A group header claims 5 group entries but the table is empty
		// after the header, so decoding it would run off the end.
		SmallParseTable: []uint16{5},
	}
	blob := gobGzipBlob(t, lang)
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted a truncated SmallParseTable group")
	}
}

func TestValidateDecodedLanguageRejectsOutOfRangeLexTransition(t *testing.T) {
	lang := &Language{
		Name:        "evil_lex_transition",
		SymbolCount: 1,
		SymbolNames: []string{"end"},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 42}}},
		},
	}
	blob := gobGzipBlob(t, lang)
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted a lex transition whose NextState is outside LexStates")
	}
}

func TestValidateDecodedLanguageRejectsInsaneCount(t *testing.T) {
	lang := &Language{
		Name:        "evil_symbol_count",
		SymbolCount: maxSaneLanguageTableCount + 1,
	}
	blob := gobGzipBlob(t, lang)
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted a SymbolCount far beyond any real grammar")
	}
}

func TestValidateDecodedLanguageRejectsShortSymbolNames(t *testing.T) {
	lang := &Language{
		Name:        "evil_short_symbol_names",
		SymbolCount: 10,
		SymbolNames: []string{"end", "a"},
	}
	blob := gobGzipBlob(t, lang)
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted SymbolNames shorter than SymbolCount")
	}
}

// TestValidateDecodedLanguageRejectsOutOfRangeLargeStateGoto covers a blob
// whose LargeStateGotos trailer (see large_state_gotos_trailer.go) restores
// an entry outside StateCount. Unlike the other validation tests in this
// file, this one must go through EncodeLanguageBlob rather than
// gobGzipBlob: LargeStateGotos is only ever carried through the trailer
// mechanism when non-empty (see EncodeLanguageBlobWithGenerator), so a plain
// gob-encoded blob would never exercise the code path where LoadLanguage
// restores the trailer before validating it.
func TestValidateDecodedLanguageRejectsOutOfRangeLargeStateGoto(t *testing.T) {
	lang := &Language{
		Name:        "evil_large_state_goto",
		SymbolCount: 2,
		TokenCount:  1,
		StateCount:  2,
		SymbolNames: []string{"end", "a"},
		LargeStateGotos: map[uint64]StateID{
			(uint64(99) << 32): 0, // source state 99 is outside StateCount (2)
		},
	}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob: %v", err)
	}
	if _, err := LoadLanguage(blob); err == nil {
		t.Fatal("LoadLanguage accepted a LargeStateGotos trailer entry whose source state is outside StateCount")
	}
}

func TestReadAllGzipWithSizeHintEnforcesMaxDecompressedBlobSize(t *testing.T) {
	original := MaxDecompressedBlobSize
	t.Cleanup(func() { MaxDecompressedBlobSize = original })
	MaxDecompressedBlobSize = 64

	payload := bytes.Repeat([]byte{'x'}, 4096)
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if _, err := gzw.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	compressed := buf.Bytes()

	gzr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gzr.Close()

	if _, err := ReadAllGzipWithSizeHint(gzr, compressed); !errors.Is(err, ErrDecompressedBlobTooLarge) {
		t.Fatalf("ReadAllGzipWithSizeHint error = %v, want ErrDecompressedBlobTooLarge", err)
	}
}

func TestReadAllGzipWithSizeHintEnforcesLimitWithoutTrustworthyHint(t *testing.T) {
	// A payload whose ISIZE trailer undercounts the real decompressed size
	// (mod 2^32 wraparound, or a hand-crafted lie) must still be bounded by
	// MaxDecompressedBlobSize via the fallback io.ReadAll path's LimitReader,
	// not just the ISIZE-preallocated fast path.
	original := MaxDecompressedBlobSize
	t.Cleanup(func() { MaxDecompressedBlobSize = original })
	MaxDecompressedBlobSize = 64

	payload := bytes.Repeat([]byte{'y'}, 4096)
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if _, err := gzw.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	compressed := buf.Bytes()
	// Corrupt the last 4 bytes (ISIZE) so it looks huge, forcing the
	// preallocated fast path to bail out of ISIZE-trust and fall through to
	// io.ReadAll -- which must still be limited.
	compressed[len(compressed)-1] = 0x7f
	compressed[len(compressed)-2] = 0xff
	compressed[len(compressed)-3] = 0xff
	compressed[len(compressed)-4] = 0xff

	gzr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gzr.Close()

	if _, err := ReadAllGzipWithSizeHint(gzr, compressed); !errors.Is(err, ErrDecompressedBlobTooLarge) {
		t.Fatalf("ReadAllGzipWithSizeHint error = %v, want ErrDecompressedBlobTooLarge", err)
	}
}

func TestReadAllGzipWithSizeHintAllowsUnderLimit(t *testing.T) {
	original := MaxDecompressedBlobSize
	t.Cleanup(func() { MaxDecompressedBlobSize = original })
	MaxDecompressedBlobSize = 1 << 20

	payload := []byte("small payload well under the limit")
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if _, err := gzw.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	compressed := buf.Bytes()

	gzr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gzr.Close()

	got, err := ReadAllGzipWithSizeHint(gzr, compressed)
	if err != nil {
		t.Fatalf("ReadAllGzipWithSizeHint: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("ReadAllGzipWithSizeHint = %q, want %q", got, payload)
	}
}
