// Package frozengo holds the Go grammar blob that the repository shipped before
// the shipped blob was built from upstream's grammar text: the blob that
// declares the external token _automatic_semicolon, bound to GoExternalScanner.
//
// The compact scheduler was certified against that artifact, and tests of the
// older engine use it as a fixture, because the shipped blob is served by the
// certified core. Language returns it with the grants that the registry gave the
// shipped language (grammars/runtime: the "go" runtime profile before the
// swap). The package is a test fixture: no production code imports it.
package frozengo

import (
	"crypto/sha256"
	_ "embed"
	"fmt"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/incr"
)

//go:embed go.bin
var blob []byte

// SHA256 is the identity of the frozen blob.
const SHA256 = "67b873053c9917055e1b5aec94d3c0d9f7d26834f4340a39159e5970c0a0a598"

// Blob returns the frozen blob's bytes.
func Blob() []byte { return append([]byte(nil), blob...) }

// Language loads the frozen blob as the registry loaded it before the swap.
func Language() *gts.Language {
	if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != SHA256 {
		panic("frozengo: blob identity changed: " + got)
	}
	lang, err := gts.LoadLanguage(blob)
	if err != nil {
		panic("frozengo: " + err.Error())
	}
	lang.Name = "go"
	lang.ExternalScanner = grammars.GoExternalScanner{}.ExternalScannerForLanguage(lang)
	// The runtime profile of the frozen artifact.
	lang.SmallSourceReuseAuthority = incr.NewReuseAuthorityGrant(lang)
	sum := sha256.Sum256(blob)
	lang.SmallSourceReuseShadowArtifact = sum
	lang.CompactConvergedReductionSplitDropsCertified = true
	lang.CompactOwnedEOFRecoveryCertified = true
	lang.RecoveryMissingVersionTurnsCertified = true
	for symbol := 1; symbol < len(lang.SymbolNames) && uint32(symbol) < lang.TokenCount && symbol < len(lang.SymbolMetadata); symbol++ {
		var public string
		switch lang.SymbolNames[symbol] {
		case "_raw_string_literal_token1":
			public = "raw_string_literal_content"
		case "_interpreted_string_literal_token1":
			public = "interpreted_string_literal_content"
		default:
			continue
		}
		lang.SymbolNames[symbol] = public
		lang.SymbolMetadata[symbol].Name = public
		lang.SymbolMetadata[symbol].Visible = true
		lang.SymbolMetadata[symbol].Named = true
	}
	gts.CertifyCRecoveryCostCompetition(lang)
	return lang
}
