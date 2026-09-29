//go:build !gts_no_parsercorephase0

package gotreesitter

import "testing"

// TestParserPoolSharesWarmCompactRunner checks that parser-pool defaults leave
// the Language's warm runner available to another parser.
func TestParserPoolSharesWarmCompactRunner(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	p := NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	tree, err := p.Parse([]byte("1+2+3"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree.Release()
	if p.admissionCandidateRunner != nil {
		t.Fatal("parse retained the language pool's runner")
	}
	runner := lang.admissionRunnerPool().Get()
	if runner == nil {
		t.Fatal("compact parse did not return a warm runner")
	}
	lang.admissionRunnerPool().Put(runner)
	pool.applyDefaults(p)
	if got := lang.admissionRunnerPool().Get(); got != runner {
		t.Fatal("applyDefaults dropped the language pool's warm runner")
	}
	lang.admissionRunnerPool().Put(runner)
	p.SetParseWorkLimits(ParseWorkLimits{NodeLimit: 10})
	if p.admissionCandidateRunner != nil {
		t.Fatal("changed work limits must drop the runner")
	}
}
