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
