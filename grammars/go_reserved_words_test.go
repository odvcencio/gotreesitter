package grammars

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestGoLockedReservedWordTable(t *testing.T) {
	language := GoLanguage()
	if len(language.ExternalSymbols) != 0 || language.ExternalScanner != nil {
		t.Fatal("the locked Go grammar retained a legacy external scanner")
	}
	if language.LanguageVersion != 15 || language.MaxReservedWordSetSize != 25 || len(language.ReservedWords) != 200 {
		t.Fatalf("locked Go metadata: ABI=%d stride=%d entries=%d", language.LanguageVersion, language.MaxReservedWordSetSize, len(language.ReservedWords))
	}
	for state, mode := range language.LexModes {
		if int(mode.ReservedWordSetID+1)*25 > len(language.ReservedWords) {
			t.Fatalf("state %d reserved set exceeds the table", state)
		}
	}
	if attachBuiltinLanguageRuntimeProfile("go", mustRuntimeProfileSHA256("9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08"), &gts.Language{}) {
		t.Fatal("the stale Go blob retained its runtime profile")
	}
}
