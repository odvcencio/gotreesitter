package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexrun"
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
	if info.MinRuntimeVersion != baseBlobRuntimeVersion {
		t.Fatalf("BlobInfo().MinRuntimeVersion = %d, want %d", info.MinRuntimeVersion, baseBlobRuntimeVersion)
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

func TestLegacyReductionContinuationGrantRequiresExactRuntimeArtifact(t *testing.T) {
	lang := &Language{Name: "continuation_certificate"}
	if lang.CertifyLegacyReductionContinuations([32]byte{}) {
		t.Fatal("unloaded language certified")
	}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := loaded.GrammarBlobSHA256()
	if !ok {
		t.Fatal("missing loaded identity")
	}
	wrong := identity
	wrong[0] ^= 1
	if loaded.CertifyLegacyReductionContinuations(wrong) || loaded.legacyReductionContinuationsCertified {
		t.Fatal("wrong artifact certified")
	}
	before, err := EncodeLanguageBlob(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.CertifyLegacyReductionContinuations(identity) || !loaded.legacyReductionContinuationsCertified {
		t.Fatal("exact artifact not certified")
	}
	if loaded.CertifyLegacyReductionContinuations(identity) {
		t.Fatal("duplicate grant reported a change")
	}
	after, err := EncodeLanguageBlob(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("runtime grant changed serialized grammar bytes")
	}
	roundtrip, err := LoadLanguage(after)
	if err != nil {
		t.Fatal(err)
	}
	if roundtrip.legacyReductionContinuationsCertified {
		t.Fatal("runtime grant survived encoding")
	}
}

func TestBlobUnconditionalLexerEndRoundTrip(t *testing.T) {
	lang := &Language{Name: "early_end", LexStates: []LexState{{Default: -1, EOF: -1}, {Default: -1, EOF: -1}}}
	base, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatal(err)
	}
	oldPayload, oldInfo, err := UnwrapLanguageBlobVersionHeader(base)
	if err != nil || oldInfo.MinRuntimeVersion != 1 {
		t.Fatalf("old payload version %+v err=%v", oldInfo, err)
	}
	if err := lang.SetLexerEndStates([]uint32{1}); err != nil {
		t.Fatal(err)
	}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatal(err)
	}
	payload, info, err := UnwrapLanguageBlobVersionHeader(blob)
	if err != nil || info.MinRuntimeVersion != BlobRuntimeVersion || info.MinRuntimeVersion <= oldInfo.MinRuntimeVersion {
		t.Fatalf("new payload version %+v err=%v", info, err)
	}
	gobPayload, marked, err := lexrun.UnwrapBuiltinEndStates(payload)
	if err != nil || !bytes.Equal(oldPayload, gobPayload) || len(marked) != 1 || marked[0] != 1 {
		t.Fatalf("new encoding changed original payload or missed states: %v %v", marked, err)
	}
	got, err := LoadLanguage(blob)
	if err != nil || got.LexerStateAcceptsEndBeforeEOF(0) || !got.LexerStateAcceptsEndBeforeEOF(1) {
		t.Fatalf("marker did not round-trip: %+v err=%v", got, err)
	}
	roundTrip, err := EncodeLanguageBlob(got)
	if err != nil || !bytes.Equal(blob, roundTrip) {
		t.Fatalf("re-encoding changed bytes: %v", err)
	}
	if _, err := LoadLanguage(payload); err == nil {
		t.Fatal("accepted end metadata without a version requirement")
	}

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"truncated", payload[:10]},
		{"duplicate", lexrun.WrapBuiltinEndStates(oldPayload, []uint32{1, 1})},
		{"out-of-range", lexrun.WrapBuiltinEndStates(oldPayload, []uint32{2})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := WrapLanguageBlobVersionHeader(tc.data, BlobRuntimeVersion, DefaultBlobGeneratorVersion)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := LoadLanguage(blob); err == nil {
				t.Fatal("accepted invalid end metadata")
			}
		})
	}
}

func TestLexerUnconditionalEndBeforeUnreadText(t *testing.T) {
	state := LexState{Default: -1, EOF: -1, AcceptEOF: true}
	src := []byte(" !")
	orig := NewLexer([]LexState{state}, src).NextWithErrorRuns(0)
	if orig.EndByte != 2 {
		t.Fatalf("ordinary EOF-only lexer stopped early: %+v", orig)
	}
	state.AcceptEOF = false
	lang := &Language{LexStates: []LexState{state}}
	if err := lang.SetLexerEndStates([]uint32{0}); err != nil {
		t.Fatal(err)
	}
	got := lang.newLexer(src).NextWithErrorRuns(0)
	if got.Symbol != 0 || got.StartByte != 0 || got.EndByte != 0 || got.lexerLookaheadEndByte != 1 {
		t.Fatalf("end marker consumed text: %+v", got)
	}
}
