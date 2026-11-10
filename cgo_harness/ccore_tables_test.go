//go:build cgo && treesitter_c_parity

package cgoharness

// The ccore Tables of a language return what the root's lookupActionIndex and
// lookupGoto return, for every state and symbol.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func TestCcoreTables(t *testing.T) {
	names := []string{"json", "go", "python", "typescript", "bash", "javascript", "rust", "html"}
	if v := os.Getenv("CCORE_LANGS"); v != "" {
		names = strings.Split(v, ",")
	}
	for _, name := range names {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			t.Fatalf("unknown language %s", name)
		}
		lang := entry.Language()
		checked, differ, err := bind.CheckTables(lang)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		fmt.Printf("CCTABLES language=%s states=%d symbols=%d checked=%d differ=%d\n", name, lang.StateCount, lang.SymbolCount, checked, differ)
		if checked == 0 {
			t.Errorf("%s: no lookup was checked", name)
		}
		if differ != 0 {
			t.Errorf("%s: %d of %d lookups differ", name, differ, checked)
		}
		ccoreCount("TestCcoreTables", name, checked)
	}
}
