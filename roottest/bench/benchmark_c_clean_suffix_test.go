package bench_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCInitializerBoundsTransientRecoveryStacks(t *testing.T) {
	source := cleanSuffixInitializerSource(40)
	parser := gotreesitter.NewParser(grammars.CLanguage())
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse C initializer: %v", err)
	}
	t.Cleanup(tree.Release)
	requireCompleteCInitializerTree(t, source, tree)
	if got := tree.ParseRuntime().MaxStacksSeen; got > 8 {
		t.Fatalf("peak stack count = %d, want at most 8", got)
	}
}

func BenchmarkCInitializerAfterTransientRecovery(b *testing.B) {
	source := cleanSuffixInitializerSource(40)
	parser := gotreesitter.NewParser(grammars.CLanguage())
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	for range b.N {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatalf("parse C initializer: %v", err)
		}
		requireCompleteCInitializerTree(b, source, tree)
		tree.Release()
	}
}

func cleanSuffixInitializerSource(rows int) []byte {
	var source strings.Builder
	source.WriteString("static const element_t table[] = {\n")
	for range rows {
		source.WriteString("  {{ 0x1, 0x2, 0x3, 0x4, }},\n")
	}
	source.WriteString("};\n")
	return []byte(source.String())
}

func requireCompleteCInitializerTree(tb testing.TB, source []byte, tree *gotreesitter.Tree) {
	tb.Helper()
	if tree == nil || tree.RootNode() == nil {
		tb.Fatal("C initializer parse returned a nil tree")
	}
	root := tree.RootNode()
	if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
		tb.Fatalf("root span = %d:%d, want 0:%d", root.StartByte(), root.EndByte(), len(source))
	}
	if root.HasError() {
		tb.Fatal("C initializer parse returned an error tree")
	}
	runtime := tree.ParseRuntime()
	if got, want := runtime.StopReason, gotreesitter.ParseStopAccepted; got != want {
		tb.Fatalf("stop reason = %q, want %q", got, want)
	}
	if runtime.Truncated {
		tb.Fatal("C initializer parse returned a truncated tree")
	}
}
