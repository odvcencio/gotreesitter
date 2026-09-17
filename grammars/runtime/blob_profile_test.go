package grammarruntime

import (
	"github.com/odvcencio/gotreesitter"
	"testing"
)

func TestJavaScriptForestAllowanceRequiresExactBuiltinBlobIdentity(t *testing.T) {
	wrongSHA := [32]byte{1}
	wrongIdentity := &gotreesitter.Language{Name: "javascript"}
	if attachBuiltinLanguageRuntimeProfile("javascript", wrongSHA, wrongIdentity) {
		t.Fatal("wrong blob identity unexpectedly attached a runtime profile")
	}
	if got := wrongIdentity.AutomaticForestMemoryAllowanceBytes; got != 0 {
		t.Fatalf("wrong-identity allowance = %d, want zero", got)
	}

	blob := BlobByName("javascript")
	custom, err := gotreesitter.LoadLanguage(blob)
	if err != nil {
		t.Fatalf("decode caller-owned javascript blob: %v", err)
	}
	custom.WantsForest = true
	if got := custom.AutomaticForestMemoryAllowanceBytes; got != 0 {
		t.Fatalf("caller-owned WantsForest allowance = %d, want zero/full-budget behavior", got)
	}
}
