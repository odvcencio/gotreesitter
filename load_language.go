package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
)

// LoadLanguage deserializes a compressed grammar blob into a Language.
// Blobs are produced by EncodeLanguageBlob, grammargen.Generate, or the
// grammar build toolchain. This is the only function needed at runtime to load
// pre-compiled grammars — no grammargen import required. It accepts legacy
// gzip blobs and version-enveloped trailer-bearing blobs.
func LoadLanguage(data []byte) (*Language, error) {
	enveloped, blobInfo, err := UnwrapLanguageBlobVersionHeader(data)
	if err != nil {
		return nil, fmt.Errorf("decode language: %w", err)
	}

	compressed, expectsTrailer, err := UnwrapLanguageBlobEnvelope(enveloped)
	if err != nil {
		return nil, fmt.Errorf("decode language: %w", err)
	}
	gzr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gzr.Close()

	raw, err := ReadAllGzipWithSizeHint(gzr, compressed)
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}

	var lang Language
	br := bytes.NewReader(raw)
	if err := gob.NewDecoder(br).Decode(&lang); err != nil {
		return nil, fmt.Errorf("decode language: %w", err)
	}
	// LargeStateGotos (when non-empty) is never gob-encoded directly -- see
	// large_state_gotos_trailer.go for why -- so restore it from the trailer
	// appended after the gob message, if the encoder wrote one. This is a
	// cheap no-op (returns immediately) for every blob without a trailer,
	// including all blobs encoded before this trailer mechanism existed.
	// Trailer restoration runs before validateDecodedLanguage below: the
	// validator bounds-checks LargeStateGotos (using StateCount/SymbolCount,
	// which gob decoding already populated), and validating before the
	// trailer is restored would let a malformed trailer entry through
	// unchecked.
	trailer, err := DecodeLargeStateGotosTrailer(br)
	if err != nil {
		return nil, fmt.Errorf("decode language: %w", err)
	}
	if expectsTrailer && len(trailer) == 0 {
		return nil, fmt.Errorf("decode language: envelope requires a non-empty large-state-gotos trailer")
	}
	if !expectsTrailer && len(trailer) != 0 {
		return nil, fmt.Errorf("decode language: large-state-gotos trailer requires a versioned envelope")
	}
	if trailer != nil {
		lang.LargeStateGotos = trailer
	}
	if err := validateDecodedLanguage(&lang); err != nil {
		return nil, fmt.Errorf("decode language: %w", err)
	}
	lang.grammarBlobSHA256 = sha256.Sum256(data)
	lang.grammarBlobSHA256Valid = true
	lang.blobInfo = blobInfo
	InferGeneratedRepeatAuxMetadata(&lang)

	return &lang, nil
}

// MaxDecompressedBlobSize is the hard ceiling ReadAllGzipWithSizeHint (and
// therefore LoadLanguage) enforces on the decompressed size of a gzip
// grammar blob, regardless of what the ISIZE trailer claims or how much data
// the gzip stream actually contains. It guards against a decompression bomb:
// a small compressed blob that decodes to an unbounded stream, whether
// because ISIZE lies (ISIZE is only the true size mod 2^32) or because the
// stream really is that large.
//
// The default is set to roughly 4x the largest legitimate shipped grammar
// blob's decompressed size (Swift, ~16.1 MiB as of v0.43.1, after
// grammargen's lex-state minimization brought its LexStates table down from
// 63,150 to 2,067 entries — see grammars/language_memory_ceiling_test.go for
// the retained-heap figure and grammars/dfa_minimize.go for the fix). That
// leaves headroom for grammar growth while still catching a bogus or
// adversarial stream long before it could exhaust available memory.
//
// This is a package-level variable, not a constant, so an embedder that
// legitimately ships a larger grammar can raise it before calling
// LoadLanguage. Lowering it below the largest blob actually loaded turns
// that blob's LoadLanguage call into an error.
var MaxDecompressedBlobSize int64 = 64 * 1024 * 1024 // 64 MiB

// gzipSizeHintCap bounds the ISIZE-derived pre-allocation in
// ReadAllGzipWithSizeHint: an ISIZE hint at or above this value is treated as
// untrustworthy for pre-sizing purposes (but the read itself is still capped
// by MaxDecompressedBlobSize either way, via the LimitReader below).
const gzipSizeHintCap = 512 * 1024 * 1024 // 512 MB

// ErrDecompressedBlobTooLarge is returned (wrapped, with the offending size
// and the active limit) by ReadAllGzipWithSizeHint, and so by LoadLanguage,
// when a gzip stream's decompressed content would exceed
// MaxDecompressedBlobSize. Test for this condition with errors.Is.
var ErrDecompressedBlobTooLarge = errors.New("gzip stream exceeds the decompressed-size limit")

// ReadAllGzipWithSizeHint reads all of r — an open gzip.Reader positioned at
// the start of the member whose raw (still-compressed) bytes are compressed
// — into memory, pre-sizing the destination buffer from the gzip ISIZE
// trailer (the last 4 bytes of compressed) instead of letting io.ReadAll grow
// the buffer by repeated doubling. ISIZE is the uncompressed size mod 2^32,
// which is exact for every blob under 4 GB; grammar blobs are always far
// smaller. Falls back to plain buffered reads when the hint is missing,
// zero, or exceeds gzipSizeHintCap.
//
// Every code path here is bounded by MaxDecompressedBlobSize: r is always
// wrapped in an io.LimitReader before any read happens, so neither the
// ISIZE-preallocated fast path nor the fallback path can be tricked into an
// unbounded read by a corrupt or adversarial ISIZE trailer or an oversized
// gzip stream. Reading more than MaxDecompressedBlobSize returns
// ErrDecompressedBlobTooLarge.
//
// This matters at grammar-load time: io.ReadAll's doubling growth roughly
// doubles peak transient allocation versus the final size, and for the
// largest shipped grammar blobs that transient churn is measured in tens of
// MB (observed via alloc-space pprof on Language() calls), which is what
// actually trips container memory limits even though the final retained
// Language is smaller.
func ReadAllGzipWithSizeHint(r io.Reader, compressed []byte) ([]byte, error) {
	limit := MaxDecompressedBlobSize
	if limit <= 0 {
		limit = 64 * 1024 * 1024
	}
	limited := &io.LimitedReader{R: r, N: limit + 1}

	if len(compressed) >= 4 {
		isize := int64(binary.LittleEndian.Uint32(compressed[len(compressed)-4:]))
		if isize > 0 && isize < gzipSizeHintCap && isize <= limit {
			raw := make([]byte, 0, isize)
			var buf [32 * 1024]byte
			for {
				n, readErr := limited.Read(buf[:])
				if n > 0 {
					raw = append(raw, buf[:n]...)
					if int64(len(raw)) > limit {
						return nil, fmt.Errorf("%w: exceeded %d bytes", ErrDecompressedBlobTooLarge, limit)
					}
				}
				if readErr == io.EOF {
					return raw, nil
				}
				if readErr != nil {
					return nil, readErr
				}
			}
		}
	}

	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%w: exceeded %d bytes", ErrDecompressedBlobTooLarge, limit)
	}
	return raw, nil
}
