package certgen

import (
	"strings"
	"testing"
)

// Two spellings of one SHA-256 (upper and lower case) are one blob: Render
// refuses them instead of writing a Go map literal with a duplicate key.
func TestRenderRefusesTheSameHashInTwoSpellings(t *testing.T) {
	h := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	cfg := strings.Repeat("ab", 32)
	rec := func(s string) Record { return Record{BlobSHA256: s, Modes: Fresh | Recover, ConfigSHA256: cfg} }
	if _, err := Render([]Record{rec(h), rec(strings.ToUpper(h))}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("a hash listed in two spellings was accepted: %v", err)
	}
	if _, err := Render([]Record{rec(h)}); err != nil {
		t.Fatalf("one record refused: %v", err)
	}
}

// A record without the effective-configuration hash is refused: a certificate must say
// which configuration of the blob it certifies.
func TestRenderRefusesARecordWithoutAConfigurationHash(t *testing.T) {
	h := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	if _, err := Render([]Record{{BlobSHA256: h, Modes: Fresh | Recover}}); err == nil || !strings.Contains(err.Error(), "configuration hash") {
		t.Fatalf("a record without a configuration hash was accepted: %v", err)
	}
}

// Parse reads the list form of the certificate file; a value that is not a
// list is an error.
func TestParseReadsTheRecordList(t *testing.T) {
	recs, err := Parse([]byte(`[{"BlobSHA256":"ab","Modes":3,"Scanner":"T"}]`))
	if err != nil || len(recs) != 1 || recs[0].BlobSHA256 != "ab" || recs[0].Modes != 3 || recs[0].Scanner != "T" {
		t.Fatalf("records %+v, error %v", recs, err)
	}
	if recs, err := Parse([]byte(`[]`)); err != nil || len(recs) != 0 {
		t.Fatalf("empty list: %+v, %v", recs, err)
	}
	if _, err := Parse([]byte(`{"BlobSHA256":"ab"}`)); err == nil {
		t.Fatal("an object in place of the list was accepted")
	}
}

// A record lists scan providers by identity. Render writes the list when there
// is one, writes nothing when there is none, and refuses an identity it does
// not know.
func TestRenderWritesAndChecksProviders(t *testing.T) {
	h := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	cfg := strings.Repeat("ab", 32)
	plain, err := Render([]Record{{BlobSHA256: h, Modes: Fresh | Recover, ConfigSHA256: cfg}})
	if err != nil || strings.Contains(string(plain), "Providers") {
		t.Fatalf("a record with no providers: %v\n%s", err, plain)
	}
	granted, err := Render([]Record{{BlobSHA256: h, Modes: Fresh | Recover, ConfigSHA256: cfg, Providers: []string{"table-v1"}}})
	if err != nil || !strings.Contains(string(granted), `Providers: []string{"table-v1"}`) {
		t.Fatalf("a record with a provider: %v\n%s", err, granted)
	}
	if _, err := Render([]Record{{BlobSHA256: h, Modes: Fresh | Recover, ConfigSHA256: cfg, Providers: []string{"other"}}}); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("an unknown provider was accepted: %v", err)
	}
	recs, err := Parse([]byte(`[{"BlobSHA256":"ab","Modes":3,"Providers":["table-v1"]}]`))
	if err != nil || len(recs) != 1 || len(recs[0].Providers) != 1 || recs[0].Providers[0] != "table-v1" {
		t.Fatalf("records %+v, error %v", recs, err)
	}
}
