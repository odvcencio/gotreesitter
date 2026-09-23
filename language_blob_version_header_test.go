package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"strings"
	"testing"
)

func gzipGobBlobForVersionHeaderTest(t *testing.T, v any) []byte {
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

func TestLanguageBlobVersionHeaderRoundTrip(t *testing.T) {
	payload := gzipGobBlobForVersionHeaderTest(t, &Language{Name: "header_round_trip"})

	wrapped, err := WrapLanguageBlobVersionHeader(payload, BlobRuntimeVersion, "unit-test-generator")
	if err != nil {
		t.Fatalf("WrapLanguageBlobVersionHeader: %v", err)
	}
	if bytes.Equal(wrapped, payload) {
		t.Fatal("WrapLanguageBlobVersionHeader did not change the payload")
	}
	if bytes.HasPrefix(wrapped, []byte{0x1f, 0x8b}) {
		t.Fatal("wrapped blob still begins with the gzip magic; a legacy gzip-first loader would misparse it")
	}
	if _, err := gzip.NewReader(bytes.NewReader(wrapped)); err == nil {
		t.Fatal("legacy gzip-first loader accepted a version-header-wrapped blob")
	}

	gotPayload, info, err := UnwrapLanguageBlobVersionHeader(wrapped)
	if err != nil {
		t.Fatalf("UnwrapLanguageBlobVersionHeader: %v", err)
	}
	if !info.HasHeader {
		t.Fatal("UnwrapLanguageBlobVersionHeader did not report a header")
	}
	if info.MinRuntimeVersion != BlobRuntimeVersion {
		t.Fatalf("MinRuntimeVersion = %d, want %d", info.MinRuntimeVersion, BlobRuntimeVersion)
	}
	if info.GeneratorVersion != "unit-test-generator" {
		t.Fatalf("GeneratorVersion = %q, want %q", info.GeneratorVersion, "unit-test-generator")
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatal("unwrapped payload differs from the original")
	}
}

func TestUnwrapLanguageBlobVersionHeaderLeavesLegacyBlobUnchanged(t *testing.T) {
	legacy := gzipGobBlobForVersionHeaderTest(t, &Language{Name: "legacy"})
	got, info, err := UnwrapLanguageBlobVersionHeader(legacy)
	if err != nil {
		t.Fatalf("UnwrapLanguageBlobVersionHeader: %v", err)
	}
	if info.HasHeader {
		t.Fatal("legacy blob unexpectedly reported a version header")
	}
	if !bytes.Equal(got, legacy) {
		t.Fatal("legacy blob bytes changed while unwrapping the (absent) version header")
	}
}

func TestUnwrapLanguageBlobVersionHeaderRejectsNewerMinRuntimeVersion(t *testing.T) {
	payload := gzipGobBlobForVersionHeaderTest(t, &Language{Name: "future_blob"})
	wrapped, err := WrapLanguageBlobVersionHeader(payload, BlobRuntimeVersion+1, "future-generator")
	if err != nil {
		t.Fatalf("WrapLanguageBlobVersionHeader: %v", err)
	}
	_, _, err = UnwrapLanguageBlobVersionHeader(wrapped)
	if err == nil {
		t.Fatal("expected a blob requiring a newer runtime version to be rejected")
	}
	if !strings.Contains(err.Error(), "requires runtime version") {
		t.Fatalf("error = %v, want a message naming the required runtime version", err)
	}
}

func TestUnwrapLanguageBlobVersionHeaderRejectsTruncatedHeader(t *testing.T) {
	if _, _, err := UnwrapLanguageBlobVersionHeader([]byte("GTSVER1\x00")); err == nil {
		t.Fatal("expected a truncated header to be rejected")
	}
}

func TestUnwrapLanguageBlobVersionHeaderRejectsUnsupportedSchemaVersion(t *testing.T) {
	payload := gzipGobBlobForVersionHeaderTest(t, &Language{Name: "bad_schema"})
	wrapped, err := WrapLanguageBlobVersionHeader(payload, BlobRuntimeVersion, "gen")
	if err != nil {
		t.Fatalf("WrapLanguageBlobVersionHeader: %v", err)
	}
	// Byte 8 onward is the big-endian schema version (uint16); corrupt it.
	corrupted := append([]byte(nil), wrapped...)
	corrupted[8] = 0xFF
	corrupted[9] = 0xFF
	if _, _, err := UnwrapLanguageBlobVersionHeader(corrupted); err == nil {
		t.Fatal("expected an unsupported header schema version to be rejected")
	}
}

func TestLoadLanguageRejectsBlobRequiringNewerRuntime(t *testing.T) {
	payload := gzipGobBlobForVersionHeaderTest(t, &Language{Name: "future_runtime"})
	wrapped, err := WrapLanguageBlobVersionHeader(payload, BlobRuntimeVersion+1, "future-generator")
	if err != nil {
		t.Fatalf("WrapLanguageBlobVersionHeader: %v", err)
	}
	if _, err := LoadLanguage(wrapped); err == nil {
		t.Fatal("LoadLanguage accepted a blob whose MinRuntimeVersion is newer than BlobRuntimeVersion")
	}
}

func TestLoadLanguageRecordsBlobInfoForHeaderedBlob(t *testing.T) {
	lang := &Language{Name: "headered_blob_info"}
	blob, err := EncodeLanguageBlobWithGenerator(lang, "test-generator/1.0")
	if err != nil {
		t.Fatalf("EncodeLanguageBlobWithGenerator: %v", err)
	}
	loaded, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	info := loaded.BlobInfo()
	if !info.HasHeader {
		t.Fatal("BlobInfo().HasHeader = false, want true")
	}
	if info.GeneratorVersion != "test-generator/1.0" {
		t.Fatalf("BlobInfo().GeneratorVersion = %q, want %q", info.GeneratorVersion, "test-generator/1.0")
	}
	if info.MinRuntimeVersion != BlobRuntimeVersion {
		t.Fatalf("BlobInfo().MinRuntimeVersion = %d, want %d", info.MinRuntimeVersion, BlobRuntimeVersion)
	}
}

func TestLoadLanguageRecordsLegacyBlobInfoForHeaderlessBlob(t *testing.T) {
	legacy := gzipGobBlobForVersionHeaderTest(t, &Language{Name: "legacy_blob_info"})
	loaded, err := LoadLanguage(legacy)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if loaded.BlobInfo().HasHeader {
		t.Fatal("BlobInfo().HasHeader = true for a blob with no version header")
	}
}

func TestEncodeLanguageBlobDefaultGeneratorVersion(t *testing.T) {
	lang := &Language{Name: "default_generator"}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob: %v", err)
	}
	loaded, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if got := loaded.BlobInfo().GeneratorVersion; got != DefaultBlobGeneratorVersion {
		t.Fatalf("GeneratorVersion = %q, want %q", got, DefaultBlobGeneratorVersion)
	}
}

func TestBlobInfoZeroValueForUnloadedLanguage(t *testing.T) {
	lang := &Language{Name: "never_loaded"}
	if lang.BlobInfo().HasHeader {
		t.Fatal("a Language never passed through LoadLanguage reported HasHeader = true")
	}
	var nilLang *Language
	if nilLang.BlobInfo().HasHeader {
		t.Fatal("(*Language)(nil).BlobInfo() reported HasHeader = true")
	}
}

func TestGrammarBlobSHA256PinsExactBytesAcrossHeaderWrapping(t *testing.T) {
	lang := &Language{Name: "pin_by_sha256"}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob: %v", err)
	}
	loaded, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	sha, ok := loaded.GrammarBlobSHA256()
	if !ok {
		t.Fatal("GrammarBlobSHA256() ok = false for a language loaded via LoadLanguage")
	}
	// Flipping a single byte anywhere in the blob -- including inside the new
	// version header -- must change the pinned identity: GrammarBlobSHA256
	// pins the exact bytes LoadLanguage was called with, not just the core
	// gob payload.
	mutated := append([]byte(nil), blob...)
	mutated[0] ^= 0xFF
	if _, err := LoadLanguage(mutated); err == nil {
		t.Fatal("mutating the version header magic byte did not break loading")
	}

	rewrapped, err := WrapLanguageBlobVersionHeader(mustCorePayload(t, blob), BlobRuntimeVersion, DefaultBlobGeneratorVersion+"x")
	if err != nil {
		t.Fatalf("WrapLanguageBlobVersionHeader: %v", err)
	}
	reloaded, err := LoadLanguage(rewrapped)
	if err != nil {
		t.Fatalf("LoadLanguage(rewrapped): %v", err)
	}
	reSHA, ok := reloaded.GrammarBlobSHA256()
	if !ok {
		t.Fatal("GrammarBlobSHA256() ok = false for the rewrapped blob")
	}
	if reSHA == sha {
		t.Fatal("GrammarBlobSHA256 did not change after the generator identity (part of the pinned bytes) changed")
	}
}

func mustCorePayload(t *testing.T, blob []byte) []byte {
	t.Helper()
	payload, _, err := UnwrapLanguageBlobVersionHeader(blob)
	if err != nil {
		t.Fatalf("UnwrapLanguageBlobVersionHeader: %v", err)
	}
	return payload
}

func TestWrapLanguageBlobVersionHeaderRejectsOversizedGeneratorVersion(t *testing.T) {
	huge := strings.Repeat("x", 1<<17)
	if _, err := WrapLanguageBlobVersionHeader([]byte("payload"), BlobRuntimeVersion, huge); err == nil {
		t.Fatal("expected an oversized generator version string to be rejected")
	}
}
