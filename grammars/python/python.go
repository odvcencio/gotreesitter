// Package python provides the standalone Python grammar.
// Importing this package does not embed the aggregate grammar catalog.
package python

import (
	"crypto/sha256"
	"fmt"
	"sync"

	gotreesitter "github.com/odvcencio/gotreesitter"
	grammarblobs "github.com/odvcencio/gotreesitter/grammars/grammar_blobs"
	"github.com/odvcencio/gotreesitter/grammars/internal/pythonruntime"
)

// BlobSHA256 identifies the embedded Python grammar artifact.
const BlobSHA256 = "cde4a67dc6af6e1232dbbd1eab8618478d1d73727020e8a8002542390a452d37"

var (
	languageOnce sync.Once
	language     *gotreesitter.Language
	languageErr  error
)

// Language returns the cached standalone Python language.
func Language() *gotreesitter.Language {
	languageOnce.Do(loadLanguage)
	if languageErr != nil {
		panic(fmt.Sprintf("gotreesitter: load standalone Python grammar: %v", languageErr))
	}
	return language
}

func loadLanguage() {
	blob := grammarblobs.Python()
	if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != BlobSHA256 {
		languageErr = fmt.Errorf("Python grammar blob SHA-256 = %s, want %s", got, BlobSHA256)
		return
	}
	language, languageErr = gotreesitter.LoadLanguage(blob)
	if languageErr != nil {
		return
	}
	language.Name = "python"
	language.ExternalScanner = pythonruntime.PythonExternalScanner{}.ExternalScannerForLanguage(language)
	language.ExternalLexStates = pythonruntime.ExternalLexStates
	language.ExternalScannerFullParseRetryPolicy = gotreesitter.ExternalScannerFullParseRetrySkipRepeat
	language.CompactConvergedReductionSplitDropsCertified = true
	language.CompactPrimaryAcceptanceDerivationCertified = true
	language.CompactAcceptanceStructuralElectionCertified = true
	language.CompactMixedGSSMergeCertified = true
}
