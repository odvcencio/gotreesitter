//go:build !gts_no_parsercorephase0

package gotreesitter

import "testing"

// TestParserPoolKeepsWarmCompactRunner checks that a pooled parser keeps its
// compact runner across checkouts. A cold runner allocates its arenas again
// on the next parse.
func TestParserPoolKeepsWarmCompactRunner(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	p := NewParser(lang)
	tree, err := p.Parse([]byte("1+2+3"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree.Release()
	if p.admissionCandidateRunner == nil {
		t.Skip("the compact route did not run for this grammar")
	}
	runner := p.admissionCandidateRunner
	pool.applyDefaults(p)
	if p.admissionCandidateRunner != runner {
		t.Fatal("applyDefaults dropped the warm compact runner")
	}
	p.SetParseWorkLimits(ParseWorkLimits{NodeLimit: 10})
	if p.admissionCandidateRunner != nil {
		t.Fatal("changed work limits must drop the runner")
	}
}
