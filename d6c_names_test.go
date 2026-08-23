//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"testing"
)

func TestTmpD6CGoNames(t *testing.T) {
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{6, 7, 9, 12, 86, 113, 121, 197, 229} {
		name := "<out-of-range>"
		if id >= 0 && id < len(lang.SymbolNames) {
			name = lang.SymbolNames[id]
		}
		fmt.Printf("symbol[%d]=%q\n", id, name)
		if id >= 0 && id < len(lang.SymbolMetadata) {
			fmt.Printf("symbol[%d].metadata=%+v\n", id, lang.SymbolMetadata[id])
		}
	}
	for _, id := range []int{3} {
		name := "<out-of-range>"
		if id >= 0 && id < len(lang.FieldNames) {
			name = lang.FieldNames[id]
		}
		fmt.Printf("field[%d]=%q\n", id, name)
	}
}
