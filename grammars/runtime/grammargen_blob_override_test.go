package grammarruntime

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestSQLLanguageOverridePropagatesIdentityAndReusesEqualBlob(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	purgePreferredLanguageOverrideCache()
	t.Cleanup(func() {
		PurgeEmbeddedLanguageCache()
		purgePreferredLanguageOverrideCache()
	})

	overrideDir := t.TempDir()
	override, original := writeSQLOverrideBlob(t, overrideDir, "sql-override")
	t.Setenv(grammargenBlobDirEnv, overrideDir)

	loaded := SqlLanguage()
	got, ok := loaded.GrammarBlobSHA256()
	if !ok || got != sha256.Sum256(override) {
		t.Fatalf("SQL override grammar identity = (%x, %t), want override bytes %x", got, ok, sha256.Sum256(override))
	}
	if got == sha256.Sum256(original) {
		t.Fatal("SQL override inherited the checked-in source grammar identity")
	}
	provider, ok := loaded.ExternalScanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("SQL override scanner is not identity-bearing")
	}
	identity, ok := provider.CheckpointIdentity()
	if !ok || string(identity.Grammar) != string(got[:]) {
		t.Fatal("SQL override scanner did not receive the override grammar identity")
	}

	source := []byte("SELECT $$x$$;\n")
	parser := gotreesitter.NewParser(loaded)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("SQL override base parse: %v", err)
	}
	defer oldTree.Release()
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   9,
		OldEndByte:  10,
		NewEndByte:  10,
		StartPoint:  gotreesitter.Point{Column: 9},
		OldEndPoint: gotreesitter.Point{Column: 10},
		NewEndPoint: gotreesitter.Point{Column: 10},
	})
	updated, profile, err := parser.ParseIncrementalProfiled([]byte("SELECT $$y$$;\n"), oldTree)
	if err != nil {
		t.Fatalf("SQL override incremental parse: %v", err)
	}
	defer updated.Release()
	if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("equal SQL override identity did not preserve reuse: %+v", profile)
	}
	fresh, err := gotreesitter.NewParser(loaded).Parse([]byte("SELECT $$y$$;\n"))
	if err != nil {
		t.Fatalf("SQL override fresh parse: %v", err)
	}
	defer fresh.Release()
	if got, want := updated.RootNode().SExpr(loaded), fresh.RootNode().SExpr(loaded); got != want {
		t.Fatalf("SQL override incremental tree differs from fresh parse:\n got %s\nwant %s", got, want)
	}
}

func TestSQLLanguageOverrideRejectsOldTreeAfterBlobDrift(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	purgePreferredLanguageOverrideCache()
	t.Cleanup(func() {
		PurgeEmbeddedLanguageCache()
		purgePreferredLanguageOverrideCache()
	})

	overrideDir := t.TempDir()
	first, _ := writeSQLOverrideBlob(t, overrideDir, "sql-override-first")
	t.Setenv(grammargenBlobDirEnv, overrideDir)
	oldLanguage := SqlLanguage()
	oldHash, ok := oldLanguage.GrammarBlobSHA256()
	if !ok || oldHash != sha256.Sum256(first) {
		t.Fatal("first SQL override did not record its own blob identity")
	}
	oldParser := gotreesitter.NewParser(oldLanguage)
	oldParser.SetAdmissionCandidateRoute(false)
	oldTree, err := oldParser.Parse([]byte("SELECT $$x$$;\n"))
	if err != nil {
		t.Fatalf("first SQL override parse: %v", err)
	}
	defer oldTree.Release()

	second, _ := writeSQLOverrideBlob(t, overrideDir, "sql-override-second")
	if sha256.Sum256(first) == sha256.Sum256(second) {
		t.Fatal("SQL override drift fixture did not change blob bytes")
	}
	PurgeEmbeddedLanguageCache()
	newLanguage := SqlLanguage()
	newHash, ok := newLanguage.GrammarBlobSHA256()
	if !ok || newHash != sha256.Sum256(second) || newHash == oldHash {
		t.Fatalf("SQL override drift identities = old %x, new (%x, %t)", oldHash, newHash, ok)
	}

	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   9,
		OldEndByte:  10,
		NewEndByte:  10,
		StartPoint:  gotreesitter.Point{Column: 9},
		OldEndPoint: gotreesitter.Point{Column: 10},
		NewEndPoint: gotreesitter.Point{Column: 10},
	})
	parser := gotreesitter.NewParser(newLanguage)
	parser.SetAdmissionCandidateRoute(false)
	updated, profile, err := parser.ParseIncrementalProfiled([]byte("SELECT $$y$$;\n"), oldTree)
	if err != nil {
		t.Fatalf("drifted SQL override incremental parse: %v", err)
	}
	defer updated.Release()
	if profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("drifted SQL override reused old checkpoints: %+v", profile)
	}
	fresh, err := gotreesitter.NewParser(newLanguage).Parse([]byte("SELECT $$y$$;\n"))
	if err != nil {
		t.Fatalf("drifted SQL override fresh parse: %v", err)
	}
	defer fresh.Release()
	if got, want := updated.RootNode().SExpr(newLanguage), fresh.RootNode().SExpr(newLanguage); got != want {
		t.Fatalf("drifted SQL override tree differs from fresh parse:\n got %s\nwant %s", got, want)
	}
}

func TestSQLAdaptedScannerFailsClosedWithoutGrammarIdentity(t *testing.T) {
	lang := &gotreesitter.Language{ExternalSymbols: append([]gotreesitter.Symbol(nil), SqlLanguage().ExternalSymbols...)}
	if !AdaptScannerForLanguage("sql", lang) {
		t.Fatal("SQL scanner adaptation failed for a structurally valid target")
	}
	provider, ok := lang.ExternalScanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("adapted SQL scanner does not expose checkpoint identity")
	}
	if _, ok := provider.CheckpointIdentity(); ok {
		t.Fatal("adapted SQL scanner exposed identity without a target grammar blob")
	}
}

func writeSQLOverrideBlob(t *testing.T, dir, name string) ([]byte, []byte) {
	t.Helper()
	original, err := os.ReadFile(filepath.Join("..", "grammar_blobs", "sql.bin"))
	if err != nil {
		t.Fatalf("read checked-in SQL blob: %v", err)
	}
	lang, err := decodeLanguageBlobData("../grammar_blobs/sql.bin", original)
	if err != nil {
		t.Fatalf("decode checked-in SQL blob: %v", err)
	}
	lang.Name = name
	override := encodeLanguageBlobForTest(t, lang)
	if err := os.WriteFile(filepath.Join(dir, "sql.bin"), override, 0o644); err != nil {
		t.Fatalf("write SQL override blob: %v", err)
	}
	return override, original
}

func TestFortranLanguageUsesPreferredGrammargenBlobOverride(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	purgePreferredLanguageOverrideCache()
	t.Cleanup(func() {
		PurgeEmbeddedLanguageCache()
		purgePreferredLanguageOverrideCache()
	})

	original, err := os.ReadFile(filepath.Join("..", "grammar_blobs", "fortran.bin"))
	if err != nil {
		t.Fatalf("read checked-in fortran blob: %v", err)
	}
	lang, err := decodeLanguageBlobData("../grammar_blobs/fortran.bin", original)
	if err != nil {
		t.Fatalf("decode checked-in fortran blob: %v", err)
	}
	lang.Name = "fortran-override"

	overrideDir := t.TempDir()
	overridePath := filepath.Join(overrideDir, "fortran.bin")
	if err := os.WriteFile(overridePath, encodeLanguageBlobForTest(t, lang), 0o644); err != nil {
		t.Fatalf("write override blob: %v", err)
	}
	t.Setenv(grammargenBlobDirEnv, overrideDir)

	loaded := FortranLanguage()
	if loaded == nil {
		t.Fatal("FortranLanguage() returned nil with override present")
	}
	if loaded.Name != "fortran-override" {
		t.Fatalf("FortranLanguage().Name = %q, want %q", loaded.Name, "fortran-override")
	}
	if loaded.ExternalScanner == nil {
		t.Fatal("FortranLanguage() override did not receive adapted external scanner")
	}
	if again := FortranLanguage(); again != loaded {
		t.Fatal("FortranLanguage() did not reuse cached override language")
	}
}

func TestFortranLanguageFallsBackWhenPreferredOverrideIsInvalid(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	purgePreferredLanguageOverrideCache()
	t.Cleanup(func() {
		PurgeEmbeddedLanguageCache()
		purgePreferredLanguageOverrideCache()
	})

	overrideDir := t.TempDir()
	overridePath := filepath.Join(overrideDir, "fortran.bin")
	if err := os.WriteFile(overridePath, []byte("not-a-valid-grammar-blob"), 0o644); err != nil {
		t.Fatalf("write invalid override blob: %v", err)
	}
	t.Setenv(grammargenBlobDirEnv, overrideDir)

	loaded := FortranLanguage()
	if loaded == nil {
		t.Fatal("FortranLanguage() returned nil when override blob was invalid")
	}
	if loaded.Name == "fortran-override" {
		t.Fatal("FortranLanguage() should have fallen back to the checked-in blob")
	}
	if loaded.ExternalScanner == nil {
		t.Fatal("FortranLanguage() fallback did not attach external scanner")
	}
}

func TestJsonLanguageUsesPreferredGrammargenBlobOverride(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	purgePreferredLanguageOverrideCache()
	t.Cleanup(func() {
		PurgeEmbeddedLanguageCache()
		purgePreferredLanguageOverrideCache()
	})

	original, err := os.ReadFile(filepath.Join("..", "grammar_blobs", "json.bin"))
	if err != nil {
		t.Fatalf("read checked-in json blob: %v", err)
	}
	lang, err := decodeLanguageBlobData("../grammar_blobs/json.bin", original)
	if err != nil {
		t.Fatalf("decode checked-in json blob: %v", err)
	}
	lang.Name = "json-override"

	overrideDir := t.TempDir()
	overridePath := filepath.Join(overrideDir, "json.bin")
	if err := os.WriteFile(overridePath, encodeLanguageBlobForTest(t, lang), 0o644); err != nil {
		t.Fatalf("write override blob: %v", err)
	}
	t.Setenv(grammargenBlobDirEnv, overrideDir)

	loaded := JsonLanguage()
	if loaded == nil {
		t.Fatal("JsonLanguage() returned nil with override present")
	}
	if loaded.Name != "json-override" {
		t.Fatalf("JsonLanguage().Name = %q, want %q", loaded.Name, "json-override")
	}
}

func TestJavaLanguageOverrideDoesNotInheritBuiltinRetryProfile(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	purgePreferredLanguageOverrideCache()
	t.Cleanup(func() {
		PurgeEmbeddedLanguageCache()
		purgePreferredLanguageOverrideCache()
	})

	original, err := os.ReadFile(filepath.Join("..", "grammar_blobs", "java.bin"))
	if err != nil {
		t.Fatalf("read checked-in java blob: %v", err)
	}
	lang, err := decodeLanguageBlobData("../grammar_blobs/java.bin", original)
	if err != nil {
		t.Fatalf("decode checked-in java blob: %v", err)
	}
	lang.Name = "java-override"

	overrideDir := t.TempDir()
	overridePath := filepath.Join(overrideDir, "java.bin")
	if err := os.WriteFile(overridePath, encodeLanguageBlobForTest(t, lang), 0o644); err != nil {
		t.Fatalf("write override blob: %v", err)
	}
	t.Setenv(grammargenBlobDirEnv, overrideDir)

	loaded := JavaLanguage()
	if loaded == nil {
		t.Fatal("JavaLanguage() returned nil with override present")
	}
	if loaded.Name != "java-override" {
		t.Fatalf("JavaLanguage().Name = %q, want %q", loaded.Name, "java-override")
	}
	if got := loaded.FullParseAcceptedErrorRetryProfile; got.MinSourceBytes != 0 || got.InitialStackCeiling != 0 {
		t.Fatalf("Java override inherited built-in retry profile: %+v", got)
	}
}

func encodeLanguageBlobForTest(t *testing.T, lang interface{}) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gzw).Encode(lang); err != nil {
		_ = gzw.Close()
		t.Fatalf("encode override grammar blob: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close override grammar gzip writer: %v", err)
	}
	return buf.Bytes()
}
