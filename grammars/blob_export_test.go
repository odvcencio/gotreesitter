package grammars

import "testing"

func TestBlobByName_Go(t *testing.T) {
	blob := BlobByName("go")
	if len(blob) == 0 {
		t.Fatal("expected non-empty blob for go")
	}
}

func TestBlobByName_Unknown(t *testing.T) {
	blob := BlobByName("nonexistent")
	if blob != nil {
		t.Fatal("expected nil for unknown language")
	}
}

func TestBlobByName_Alias(t *testing.T) {
	blob := BlobByName("golang")
	if len(blob) == 0 {
		t.Fatal("expected non-empty blob for golang alias")
	}
}

func TestBlobByName_CaseInsensitive(t *testing.T) {
	blob := BlobByName("Go")
	if len(blob) == 0 {
		t.Fatal("expected non-empty blob for Go (capitalized)")
	}
}

func TestBlobByName_ConsistentBytes(t *testing.T) {
	a := BlobByName("go")
	b := BlobByName("go")
	if len(a) != len(b) {
		t.Fatalf("expected same length, got %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("byte mismatch at offset %d", i)
			break
		}
	}
}

func TestLoadLanguageAttachesExternalScannerSupport(t *testing.T) {
	blob := BlobByName("python")
	if len(blob) == 0 {
		t.Fatal("expected python blob")
	}
	lang, err := LoadLanguage("python", blob)
	if err != nil {
		t.Fatalf("LoadLanguage(python) error = %v", err)
	}
	if lang.ExternalScanner == nil {
		t.Fatal("LoadLanguage(python) did not attach external scanner")
	}
	if len(lang.ExternalLexStates) == 0 {
		t.Fatal("LoadLanguage(python) did not attach external lex states")
	}
}

func TestLoadLanguageAcceptsAliases(t *testing.T) {
	blob := BlobByName("golang")
	if len(blob) == 0 {
		t.Fatal("expected go blob")
	}
	lang, err := LoadLanguage("golang", blob)
	if err != nil {
		t.Fatalf("LoadLanguage(golang) error = %v", err)
	}
	if lang.Name != "go" {
		t.Fatalf("LoadLanguage(golang).Name = %q, want %q", lang.Name, "go")
	}
}
