//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestCOracleCounterBuildDigest prints, for a few fixtures and a chain of
// pinned edits, a SHA-256 of the C parse log and the tree (S-expression) of
// each parse. Run it once with the tag gts_subtree_counters and once without:
// the COUNTER_DIGEST lines must be identical, which shows that the counters
// change neither a decision nor a log line. COUNTER_TOTAL lines (the
// creations) differ by design: zero without the tag. GRAD_DIGEST_LANGS
// (comma separated) picks the fixtures.
func TestCOracleCounterBuildDigest(t *testing.T) {
	names := os.Getenv("GRAD_DIGEST_LANGS")
	if names == "" {
		t.Skip("set GRAD_DIGEST_LANGS")
	}
	for _, name := range strings.Split(names, ",") {
		_, cl := ccoreLanguages(t, name)
		src, _ := gradSample(t, name)
		chain := gradChain(src)
		var log []string
		cp := gradCParser(t, &gradCtx{cLang: cl}, &log)
		var prev *sitter.Tree
		for i, st := range chain {
			if i > 4 {
				break
			}
			log = nil
			if prev != nil && st.edit != nil {
				prev.Edit(gradSitterEdit(*st.edit))
			}
			sitter.SubtreeCountersReset()
			tree := cp.Parse(st.src, prev)
			made := sitter.SubtreeCountersRead()
			h := sha256.New()
			fmt.Fprintf(h, "%s\n%s", strings.Join(log, "\n"), tree.RootNode().ToSexp())
			fmt.Printf("COUNTER_DIGEST %s %s log_lines=%d %x\n", name, st.id, len(log), h.Sum(nil)[:12])
			fmt.Printf("COUNTER_TOTAL %s %s enabled=%t leaves=%d nodes=%d clones=%d scratch=%d\n", name, st.id, sitter.SubtreeCountersEnabled(), made.Leaves, made.Nodes, made.Clones, made.Scratch)
			if prev != nil {
				prev.Close()
			}
			prev = tree
		}
		if prev != nil {
			prev.Close()
		}
		cp.Close()
	}
}
