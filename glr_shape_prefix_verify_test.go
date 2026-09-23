package gotreesitter_test

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestGSSShapePrefixCacheNeverStaleAcrossGrammars proves the conditional
// shape-prefix invalidation (gssShapePrefixLink0Rewrites) with an oracle:
// every cache hit is recomputed from the full link-0 chain and must agree.
// The workload covers clean smoke samples plus malformed variants that drive
// GLR forking, merging, and error recovery across twenty grammars, which is
// where a stale prefix would change a merge decision.
func TestGSSShapePrefixCacheNeverStaleAcrossGrammars(t *testing.T) {
	restore := gotreesitter.SetGSSShapePrefixVerifyForTest()
	defer restore()

	names := []string{"c_sharp", "python", "javascript", "typescript", "tsx", "rust", "go", "java", "yaml", "bash", "c", "cpp", "ruby", "kotlin", "swift", "scala", "perl", "cobol", "erlang", "haskell", "php", "html", "json", "toml"}
	seeds := [][]byte{[]byte(""), []byte("\x00"), []byte("\n"), []byte("((((((((((((((((\n"), []byte("\"\"\"\"\"\"\"\"\n"), {0xE2, 0x82}, []byte("\xff\xfe\xfd"), []byte("{"), []byte("[a"), []byte("from a import b, .c\n")}
	parsed := 0
	for _, name := range names {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			continue
		}
		lang := entry.Language()
		if lang == nil {
			continue
		}
		inputs := append([][]byte{}, seeds...)
		if sample, ok := grammars.ParseSmokeSamples[entry.Name]; ok {
			src := []byte(sample)
			inputs = append(inputs, src)
			if len(src) > 8 {
				mid := len(src) / 2
				inputs = append(inputs,
					src[:mid],
					append(append(append([]byte{}, src[:mid]...), 0), src[mid:]...),
					append([]byte("(((( "), src...),
					bytes.ReplaceAll(src, []byte("\n"), []byte(" ) \n")),
				)
			}
		}
		for _, src := range inputs {
			p := gotreesitter.NewParser(lang)
			tree, err := p.Parse(src)
			if err == nil && tree != nil {
				tree.Release()
			}
			parsed++
		}
		if got := gotreesitter.GSSShapePrefixVerifyMismatchesForTest(); got != 0 {
			t.Fatalf("%s: the shape-prefix cache served %d stale prefixes", name, got)
		}
	}
	if parsed < 100 {
		t.Fatalf("workload too small: %d parses", parsed)
	}
	t.Logf("%d parses across %d grammars, 0 stale shape prefixes", parsed, len(names))
}
