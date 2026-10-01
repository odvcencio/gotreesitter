//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Bound only the untimed initial reader. Timed edits still use Parser.Parse,
// including the locked binding's source-copy cost.
func largeFileEditCInitial(p *sitter.Parser, source []byte) *sitter.Tree {
	return p.ParseWithOptions(func(offset int, _ sitter.Point) []byte {
		if offset >= len(source) {
			return nil
		}
		return source[offset:min(offset+8192, len(source))]
	}, nil, nil)
}

func TestLargeFileEditCInitialReader(t *testing.T) {
	for _, name := range []string{"c_sharp", "go", "java", "typescript", "python"} {
		for _, size := range []int{64 << 10, 1 << 20} {
			if size == 1<<20 && name != "go" {
				continue
			}
			t.Run(fmt.Sprintf("%s/%d", name, size), func(t *testing.T) {
				source, edited, edit, _ := cReuseFixtureAtSize(t, name, size)
				lang, err := COracleLanguage(name)
				if err != nil {
					t.Fatal(err)
				}
				p := sitter.NewParser()
				defer p.Close()
				if err := p.SetLanguage(lang); err != nil {
					t.Fatal(err)
				}
				ordinary := p.Parse(source, nil)
				bounded := largeFileEditCInitial(p, source)
				if ordinary == nil || bounded == nil {
					t.Fatal("C initial parse failed")
				}
				defer func() { ordinary.Close(); bounded.Close() }()
				check := func(step int) {
					a, err := COracleDeepDigest(ordinary)
					if err != nil {
						t.Fatal(err)
					}
					b, err := COracleDeepDigest(bounded)
					if err != nil {
						t.Fatal(err)
					}
					if a != b {
						t.Fatalf("step=%d ordinary=%s bounded=%s", step, a, b)
					}
				}
				check(-1)
				ce := realCorpusCInputEdit(edit)
				for step := 0; step < 4; step++ {
					to := edited
					if step%2 != 0 {
						to = source
					}
					ordinary.Edit(&ce)
					bounded.Edit(&ce)
					a, b := p.Parse(to, ordinary), p.Parse(to, bounded)
					if a == nil || b == nil {
						t.Fatal("C incremental parse failed")
					}
					ordinary.Close()
					bounded.Close()
					ordinary, bounded = a, b
					check(step)
				}
			})
		}
	}
}
