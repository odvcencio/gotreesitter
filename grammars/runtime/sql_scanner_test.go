//go:build !grammar_subset || grammar_subset_sql

package grammarruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestSQLScannerCheckpointRoundTrip(t *testing.T) {
	scanner := SqlExternalScanner{}.ExternalScannerForLanguage(SqlLanguage()).(SqlExternalScanner)
	buf := make([]byte, 4096)

	for _, tag := range []string{"", "$$", "$named_42$", "$" + strings.Repeat("x", sqlMaxDollarTagBytes-2) + "$"} {
		t.Run(checkpointTagName(tag), func(t *testing.T) {
			original := &sqlScannerState{tag: tag}
			n := scanner.Serialize(original, buf)
			if n == 0 {
				t.Fatalf("reachable scanner state %q was not checkpointable", tag)
			}
			restored := &sqlScannerState{tag: "$stale$"}
			scanner.Deserialize(restored, buf[:n])
			if restored.tag != tag {
				t.Fatalf("checkpoint round trip tag = %q, want %q", restored.tag, tag)
			}
		})
	}
}

func TestSQLScannerAcceptsUncheckpointableDollarTagForFullParse(t *testing.T) {
	maxTag := "$" + strings.Repeat("x", sqlMaxDollarTagBytes-2) + "$"
	tooLongTag := "$" + strings.Repeat("x", sqlMaxDollarTagBytes-1) + "$"

	if tag, ok := scanSqlDollarTag(newSQLExternalLexer([]byte(maxTag), 0, 0, 0), false); !ok || tag != maxTag {
		t.Fatalf("maximum checkpointable tag was rejected: len=%d ok=%v", len(tag), ok)
	}
	if tag, ok := scanSqlDollarTag(newSQLExternalLexer([]byte(tooLongTag), 0, 0, 0), false); !ok || tag != tooLongTag {
		t.Fatalf("valid uncheckpointable tag was rejected: len=%d ok=%v", len(tag), ok)
	}
	buf := make([]byte, 4096)
	if n := (SqlExternalScanner{}).Serialize(&sqlScannerState{tag: tooLongTag}, buf); n != 0 {
		t.Fatalf("uncheckpointable scanner state serialized as %d bytes, want absent checkpoint", n)
	}
}

func TestSQLScannerFailedClosePreservesCheckpointState(t *testing.T) {
	scanner := SqlExternalScanner{}
	state := &sqlScannerState{tag: "$wanted$"}
	valid := make([]bool, sqlTokDollarTagEnd+1)
	valid[sqlTokDollarTagEnd] = true
	if scanner.Scan(state, newSQLExternalLexer([]byte("$other$"), 0, 0, 0), valid) {
		t.Fatal("mismatched close tag scanned successfully")
	}
	if state.tag != "$wanted$" {
		t.Fatalf("failed scan mutated tag to %q", state.tag)
	}
}

func TestSQLScannerCheckpointIdentityBindsExactGrammarBlob(t *testing.T) {
	lang := SqlLanguage()
	grammar, ok := lang.GrammarBlobSHA256()
	if !ok {
		t.Fatal("native SQL language has no grammar identity")
	}
	bound, ok := SqlExternalScanner{}.ExternalScannerForLanguage(lang).(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("bound SQL scanner does not expose checkpoint identity")
	}
	first, ok := bound.CheckpointIdentity()
	if !ok || len(first.Scanner) != sha256.Size || len(first.Grammar) != sha256.Size {
		t.Fatalf("SQL checkpoint identity = (%d, %d, %t), want two 32-byte values", len(first.Scanner), len(first.Grammar), ok)
	}
	if string(first.Grammar) != string(grammar[:]) {
		t.Fatalf("SQL grammar identity = %x, want %x", first.Grammar, grammar)
	}
	if got, want := hex.EncodeToString(first.Scanner), "7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d"; got != want {
		t.Fatalf("SQL scanner identity = %s, want %s", got, want)
	}
	second, ok := bound.CheckpointIdentity()
	if !ok || string(first.Scanner) != string(second.Scanner) || string(first.Grammar) != string(second.Grammar) {
		t.Fatal("SQL checkpoint identity was not stable")
	}
	first.Scanner[0]++
	first.Grammar[0]++
	third, ok := bound.CheckpointIdentity()
	if !ok || third.Scanner[0] == first.Scanner[0] || third.Grammar[0] == first.Grammar[0] {
		t.Fatal("SQL checkpoint identity returned aliased mutable bytes")
	}

	identityScanner := SqlExternalScanner{}.ExternalScannerForLanguage(&gotreesitter.Language{}).(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if _, ok := identityScanner.CheckpointIdentity(); ok {
		t.Fatal("SQL scanner exposed identity without a bound grammar blob")
	}
}

func TestSQLScannerIdentityBindsLocalPortSource(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "sql_scanner.go"))
	if err != nil {
		t.Fatalf("read SQL scanner source: %v", err)
	}
	begin := []byte("// C26Q_SQL_SCANNER_LOCAL_PORT_BEGIN\n")
	end := []byte("// C26Q_SQL_SCANNER_LOCAL_PORT_END\n")
	start := bytes.Index(source, begin)
	if start < 0 {
		t.Fatal("SQL scanner local-port begin marker is missing")
	}
	start += len(begin)
	finish := bytes.Index(source[start:], end)
	if finish < 0 {
		t.Fatal("SQL scanner local-port end marker is missing")
	}
	got := sha256.Sum256(source[start : start+finish])
	if gotHex := hex.EncodeToString(got[:]); gotHex != sqlExternalScannerLocalPortSHA256 {
		t.Fatalf("SQL local scanner port hash = %s, want %s; update the identity when the marked port changes", gotHex, sqlExternalScannerLocalPortSHA256)
	}
}

func TestSQLScannerSpecAndNativeBlobIdentity(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("sql")
	if !ok {
		t.Fatal("missing SQL external scanner spec")
	}
	if got, want := spec.UpstreamCommit, sqlExternalScannerUpstreamCommit; got != want {
		t.Fatalf("SQL scanner commit = %q, want %q", got, want)
	}
	if got, want := len(spec.SourceFiles), 2; got != want {
		t.Fatalf("SQL scanner source count = %d, want %d", got, want)
	}
	lang := SqlLanguage()
	grammarSum, ok := lang.GrammarBlobSHA256()
	if !ok {
		t.Fatal("native SQL language has no compressed blob identity")
	}
	wantBytes, err := hex.DecodeString("e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268")
	if err != nil {
		t.Fatalf("decode SQL blob digest: %v", err)
	}
	if string(grammarSum[:]) != string(wantBytes) {
		t.Fatalf("native SQL grammar identity = %x, want %x", grammarSum, wantBytes)
	}
	provider, ok := lang.ExternalScanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("native SQL scanner is not identity-bearing")
	}
	identity, ok := provider.CheckpointIdentity()
	if !ok || string(identity.Grammar) != string(grammarSum[:]) {
		t.Fatal("native SQL scanner did not bind the native grammar identity")
	}
}

func checkpointTagName(tag string) string {
	if tag == "" {
		return "empty"
	}
	if len(tag) == sqlMaxDollarTagBytes {
		return "maximum"
	}
	return tag
}

//go:linkname newSQLExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newSQLExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer
