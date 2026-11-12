package recordscheck

import (
	_ "github.com/odvcencio/gotreesitter/grammars"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// TestLexExactRecordsMatchList compares the records that this build registers
// with lex_exact_list.txt (written by cmd/ts2go -lexexact-only). For every
// language of the build that is on the list, a record must be registered under
// the SHA-256 that the language reports, decode, name that same hash, and have
// the lex state counts of the blob. For every language that is not on the
// list, no record may be registered. A build that holds no listed language
// passes with zero records.
func TestLexExactRecordsMatchList(t *testing.T) {
	text, err := os.ReadFile("../../../grammars/lex_exact_list.txt")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			listed[line] = true
		}
	}
	if len(listed) == 0 {
		t.Fatal("lex_exact_list.txt names no grammar")
	}
	var found, bytes, languages int
	for _, entry := range grammars.AllLanguages() {
		lang := entry.Language()
		if lang == nil {
			continue
		}
		languages++
		sha, ok := lang.GrammarBlobSHA256()
		if !ok {
			t.Errorf("%s: the language reports no blob hash", entry.Name)
			continue
		}
		enc, have := lexexact.Lookup(sha)
		switch {
		case listed[entry.Name] && !have:
			t.Errorf("%s: on the list, but no record is registered under blob hash %x", entry.Name, sha)
		case !listed[entry.Name] && have:
			t.Errorf("%s: a record is registered under blob hash %x, but the grammar is not on the list", entry.Name, sha)
		case have:
			rec, err := lexexact.Decode(enc)
			if err != nil {
				t.Errorf("%s: record does not decode: %v", entry.Name, err)
				break
			}
			if rec.BlobSHA256 != sha {
				t.Errorf("%s: record names blob %x, the language reports %x", entry.Name, rec.BlobSHA256, sha)
			}
			if err := rec.Validate(len(lang.LexStates), len(lang.KeywordLexStates)); err != nil {
				t.Errorf("%s: %v", entry.Name, err)
			}
			found++
			bytes += len(enc)
		}
		lang, enc = nil, nil
		runtime.GC()
	}
	t.Logf("%d languages in this build, %d records, %d bytes", languages, found, bytes)
}
