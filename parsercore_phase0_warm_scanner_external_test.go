//go:build gts_parsercorephase0

package gotreesitter_test

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func init() {
	gotreesitter.SetDiagnosticParserCoreWarmGoScannerForTest(grammars.GoExternalScanner{})
}
