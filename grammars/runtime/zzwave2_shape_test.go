package grammarruntime

// Wave-2 shape-neutrality harness: parses WAVE2_SHAPE_FILE with the language
// named by WAVE2_SHAPE_LANG and prints a full-S-expression FNV hash plus node
// and span stats, for byte-shape comparison across builds.

import (
	"fmt"
	"hash/fnv"
	"os"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

func w2DumpSexp(h interface{ Write([]byte) (int, error) }, lang *gotreesitter.Language, n *gotreesitter.Node, depth int) (nodes int) {
	if n == nil {
		return 0
	}
	fmt.Fprintf(h, "(%s %d %d %v %v", n.Type(lang), n.StartByte(), n.EndByte(), n.IsNamed(), n.IsMissing())
	nodes = 1
	for i := 0; i < n.ChildCount(); i++ {
		nodes += w2DumpSexp(h, lang, n.Child(i), depth+1)
	}
	fmt.Fprint(h, ")")
	return nodes
}

func TestZZWave2Shape(t *testing.T) {
	file := os.Getenv("WAVE2_SHAPE_FILE")
	if file == "" {
		t.Skip("set WAVE2_SHAPE_FILE")
	}
	langName := os.Getenv("WAVE2_SHAPE_LANG")
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lang := loadEmbeddedLanguage(langName + ".bin")
	if lang == nil {
		t.Fatalf("lang %q: not found", langName)
	}
	p := gotreesitter.NewParser(lang)
	start := time.Now()
	tree, err := p.Parse(src)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	h := fnv.New64a()
	nodes := w2DumpSexp(h, lang, root, 0)
	fmt.Printf("SHAPE %s lang=%s hash=%016x nodes=%d root=%s span=[%d,%d) hasErr=%v dur=%v\n",
		file, langName, h.Sum64(), nodes, root.Type(lang), root.StartByte(), root.EndByte(), root.HasError(), dur.Round(time.Millisecond))
}
