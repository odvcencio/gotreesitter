package gotreesitter

import "testing"

func mustParse(t *testing.T, p *Parser, source []byte) *Tree {
	t.Helper()
	tree, err := p.Parse(source)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	return tree
}

func mustParseIncremental(t *testing.T, p *Parser, source []byte, oldTree *Tree) *Tree {
	t.Helper()
	tree, err := p.ParseIncremental(source, oldTree)
	if err != nil {
		t.Fatalf("ParseIncremental failed: %v", err)
	}
	return tree
}
