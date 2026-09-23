package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"reflect"
)

// DefaultBlobGeneratorVersion is the generator identity EncodeLanguageBlob
// stamps on every blob it writes. Callers that want their own tool identity
// recorded (e.g. "ts2go" or "grammargen", each with its own version string)
// should use EncodeLanguageBlobWithGenerator instead.
const DefaultBlobGeneratorVersion = "gotreesitter"

// EncodeLanguageBlob serializes lang in the runtime's stable grammar-blob
// format, stamped with DefaultBlobGeneratorVersion. See
// EncodeLanguageBlobWithGenerator for the full format description and for
// attaching a caller-specific generator identity.
func EncodeLanguageBlob(lang *Language) ([]byte, error) {
	return EncodeLanguageBlobWithGenerator(lang, DefaultBlobGeneratorVersion)
}

// EncodeLanguageBlobWithGenerator serializes lang in the runtime's stable
// grammar-blob format. Languages without LargeStateGotos retain the legacy
// gzip+gob wire representation byte-for-byte, aside from the version header
// below. When LargeStateGotos is populated, the map is removed from a
// shallow exported-field copy, encoded as a sorted trailer, and wrapped in
// the versioned fail-closed GTSBLOB envelope understood by LoadLanguage.
//
// The result is then wrapped in a version header (see
// language_blob_version_header.go) recording BlobRuntimeVersion as the
// blob's MinRuntimeVersion and generatorVersion as its generator identity.
// LoadLanguage rejects a blob whose MinRuntimeVersion is newer than this
// runtime's BlobRuntimeVersion, and exposes the header's contents (or its
// absence, for every blob written before this header existed) via
// Language.BlobInfo. This function always writes the header; existing
// shipped blobs are not retroactively rewritten by this change -- they keep
// loading as headerless ("legacy") blobs until something re-encodes them.
//
// Keeping this encoder in the root package makes the format invariant shared
// by every producer, including grammargen and ts2go. The input Language is
// never mutated and can remain in concurrent use while it is encoded.
func EncodeLanguageBlobWithGenerator(lang *Language, generatorVersion string) ([]byte, error) {
	toEncode := lang
	var trailer []byte
	if lang != nil && len(lang.LargeStateGotos) > 0 {
		var err error
		trailer, err = EncodeLargeStateGotosTrailer(lang.LargeStateGotos)
		if err != nil {
			return nil, fmt.Errorf("encode language blob: %w", err)
		}
		clone := shallowCopyExportedLanguageFields(lang)
		clone.LargeStateGotos = nil
		toEncode = clone
	}

	var out bytes.Buffer
	gzw := gzip.NewWriter(&out)
	if err := gob.NewEncoder(gzw).Encode(toEncode); err != nil {
		_ = gzw.Close()
		return nil, fmt.Errorf("encode language blob: %w", err)
	}
	if len(trailer) > 0 {
		if _, err := gzw.Write(trailer); err != nil {
			_ = gzw.Close()
			return nil, fmt.Errorf("encode language blob: %w", err)
		}
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("finalize language blob: %w", err)
	}

	result := out.Bytes()
	if len(trailer) > 0 {
		enveloped, err := WrapLanguageBlobEnvelope(result)
		if err != nil {
			return nil, fmt.Errorf("finalize language blob: %w", err)
		}
		result = enveloped
	}

	headered, err := WrapLanguageBlobVersionHeader(result, BlobRuntimeVersion, generatorVersion)
	if err != nil {
		return nil, fmt.Errorf("finalize language blob: %w", err)
	}
	return headered, nil
}

// shallowCopyExportedLanguageFields returns a new Language containing exactly
// the fields gob can observe. A direct struct copy would also copy sync.Once
// and atomic cache state; mutating the original map in place would race with
// parsers already using the Language.
func shallowCopyExportedLanguageFields(lang *Language) *Language {
	src := reflect.ValueOf(lang).Elem()
	t := src.Type()
	dstPtr := reflect.New(t)
	dst := dstPtr.Elem()
	for i := 0; i < t.NumField(); i++ {
		if !t.Field(i).IsExported() {
			continue
		}
		dst.Field(i).Set(src.Field(i))
	}
	return dstPtr.Interface().(*Language)
}
