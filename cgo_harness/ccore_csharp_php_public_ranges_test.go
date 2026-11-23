//go:build cgo && treesitter_c_parity && gts_ccore_test_certify

package cgoharness

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCcoreCSharpPHPPublicRanges(t *testing.T) {
	t.Run("c_sharp", func(t *testing.T) {
		const tail = "\nclass Removed {}\n"
		source, selected := selectedScannerChunks(" OMIT ", []string{
			"class C { string Text() => \"\"\"ab",
			"cd\"\"\"; }" + tail,
		})
		selected[1][1] -= len(tail)
		testCcoreIncludedRangesPublicRoutes(t, "c_sharp", grammars.CSharpLanguage(),
			source, [][2]int{{0, len(source)}}, selected, nil)
	})
	t.Run("php", func(t *testing.T) {
		const tail = "$tail = 1;\n"
		source, selected := selectedScannerChunks(" OMIT ", []string{
			"<?php\n$value = <<<LABEL\nab",
			"cd\nLABEL;\n" + tail,
		})
		selected[1][1] -= len(tail)
		testCcoreIncludedRangesPublicRoutes(t, "php", grammars.PhpLanguage(),
			source, [][2]int{{0, len(source)}}, selected, nil)
	})
}
