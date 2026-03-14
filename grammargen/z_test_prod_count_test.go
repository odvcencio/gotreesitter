package grammargen

import (
	"testing"
	"time"
)

func TestGLRCountComparison(t *testing.T) {
	grammars := []string{"javascript", "python", "scala", "bash", "haskell", "yaml", "ini", "diff", "c_lang", "jsdoc"}
	for _, name := range grammars {
		var g importParityGrammar
		for _, pg := range importParityGrammars {
			if pg.name == name {
				g = pg
				break
			}
		}
		if g.name == "" {
			continue
		}
		gram, err := importParityGrammarSource(g)
		if err != nil {
			t.Logf("%s: import error: %v", name, err)
			continue
		}
		timeout := g.genTimeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		genLang, err := generateWithTimeout(gram, timeout)
		if err != nil {
			t.Logf("%s: generate error: %v", name, err)
			continue
		}
		refLang := g.blobFunc()

		genGLR, refGLR := 0, 0
		for _, entry := range genLang.ParseActions {
			if len(entry.Actions) > 1 {
				genGLR++
			}
		}
		for _, entry := range refLang.ParseActions {
			if len(entry.Actions) > 1 {
				refGLR++
			}
		}
		t.Logf("%-12s states=%d/%d actions=%d/%d GLR=%d/%d",
			name, genLang.StateCount, refLang.StateCount,
			len(genLang.ParseActions), len(refLang.ParseActions),
			genGLR, refGLR)
	}
}
