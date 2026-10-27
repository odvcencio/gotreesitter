//go:build gts_parsercorephase0 || gts_workcount || gts_diag

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestLanguageCopyDoesNotAuthenticate(t *testing.T) {
	gts.RunLanguageCopyDoesNotAuthenticate(t, grammars.GoLanguage())
}

func TestScannerAdaptationRevokesProducer(t *testing.T) {
	language, err := grammars.LoadLanguage("go", grammars.BlobByName("go"))
	if err != nil {
		t.Fatal(err)
	}
	if language.SmallSourceReuseAuthority == nil {
		t.Fatal("decoded Go is missing its grant")
	}
	gts.AssertLanguageProducerKey(t, language, true)
	if !grammars.AdaptScannerForLanguage("go", language) {
		t.Fatal("Go scanner adaptation failed")
	}
	gts.AssertLanguageProducerKey(t, language, false)
}
