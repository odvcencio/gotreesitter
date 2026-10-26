//go:build gts_parsercorephase0

package gotreesitter_test

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func init() {
	gotreesitter.SetDiagnosticParserCoreWarmGoScannerForTest(grammars.GoExternalScanner{})
}

func init() { gotreesitter.SetQuiescentNativeGoLanguageForTest(grammars.GoLanguage) }

func init() {
	gotreesitter.SetQuiescentNativeFaustLanguageForTest(func() *gotreesitter.Language { return grammars.DetectLanguageByName("faust").Language() })
}

func init() {
	gotreesitter.SetQuiescentNativeINILanguageForTest(func() *gotreesitter.Language { return grammars.DetectLanguageByName("ini").Language() })
}

func init() {
	gotreesitter.SetQuiescentNativeAdaLanguageForTest(func() *gotreesitter.Language { return grammars.DetectLanguageByName("ada").Language() })
}

func init() {
	gotreesitter.SetQuiescentNativeReuseLanguageForTest(func(name string) *gotreesitter.Language { return grammars.DetectLanguageByName(name).Language() })
}
