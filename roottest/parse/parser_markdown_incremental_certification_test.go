package parse_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestMarkdownIncrementalScannerCertification(t *testing.T) {
	plans := []struct {
		name      string
		size      int
		positions []string
		classes   []markdownCertificationEditClass
	}{
		{
			name:      "8KiB",
			size:      8 << 10,
			positions: []string{"start", "middle", "end"},
			classes: []markdownCertificationEditClass{
				markdownCertificationInsert,
				markdownCertificationDelete,
				markdownCertificationReplace,
			},
		},
		{
			name:      "256KiB",
			size:      256 << 10,
			positions: []string{"middle"},
			classes:   []markdownCertificationEditClass{markdownCertificationReplace},
		},
	}

	for _, plan := range plans {
		source, sites := makeMarkdownCertificationSource(plan.size)
		for _, position := range plan.positions {
			var site int
			switch position {
			case "start":
				site = sites[1]
			case "middle":
				site = sites[len(sites)/2]
			case "end":
				site = sites[len(sites)-2]
			default:
				t.Fatalf("unknown Markdown certification position %q", position)
			}
			for _, class := range plan.classes {
				t.Run(fmt.Sprintf("%s/%s/%s", plan.name, class, position), func(t *testing.T) {
					edited, edit := applyMarkdownCertificationEdit(source, site, class)
					profile := runMarkdownCertificationSample(t, source, edited, edit)
					if plan.size >= 256<<10 && profile.ReusedBytes*100 < uint64(len(edited))*25 {
						t.Fatalf("Markdown certification reused only %d/%d bytes (<25%%): %+v", profile.ReusedBytes, len(edited), profile)
					}
					if plan.size >= 256<<10 {
						t.Logf("Markdown reuse: %d/%d bytes, %d subtrees", profile.ReusedBytes, len(edited), profile.ReusedSubtrees)
					}
				})
			}
		}
	}
}

type markdownCertificationEditClass uint8

const (
	markdownCertificationInsert markdownCertificationEditClass = iota
	markdownCertificationDelete
	markdownCertificationReplace
)

func (c markdownCertificationEditClass) String() string {
	switch c {
	case markdownCertificationInsert:
		return "insert"
	case markdownCertificationDelete:
		return "delete"
	case markdownCertificationReplace:
		return "replace"
	default:
		return "unknown"
	}
}

func makeMarkdownCertificationSource(target int) ([]byte, []int) {
	var source strings.Builder
	source.Grow(target + 1024)
	source.WriteString("# Incremental Markdown editing\n\n")
	sites := make([]int, 0, target/320)
	for i := 0; source.Len() < target || len(sites) < 4; i++ {
		fmt.Fprintf(&source, "## Section %06d\n\n", i)
		fmt.Fprintf(&source, "> Quoted context for section %06d.\n", i)
		fmt.Fprintf(&source, "> - nested item %06d\n", i)
		fmt.Fprintf(&source, "> - second nested item %06d\n\n", i)
		source.WriteString("Paragraph with **emphasis**, a [link](https://example.com), and edit token_")
		sites = append(sites, source.Len())
		fmt.Fprintf(&source, "%06d for incremental reuse.\n\n", i)
		fmt.Fprintf(&source, "```go\nfmt.Printf(\"section %%d\", %d)\n```\n\n", i)
	}
	return []byte(source.String()), sites
}

func applyMarkdownCertificationEdit(source []byte, offset int, class markdownCertificationEditClass) ([]byte, gts.InputEdit) {
	oldEnd := offset
	replacement := []byte{'x'}
	switch class {
	case markdownCertificationDelete:
		oldEnd = offset + 1
		replacement = nil
	case markdownCertificationReplace:
		oldEnd = offset + 1
		replacement = []byte("xy")
	}
	edited := make([]byte, 0, len(source)-(oldEnd-offset)+len(replacement))
	edited = append(edited, source[:offset]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[oldEnd:]...)
	return edited, gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(offset + len(replacement)),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, oldEnd),
		NewEndPoint: pointForOffset(edited, offset+len(replacement)),
	}
}

func runMarkdownCertificationSample(t *testing.T, source, edited []byte, edit gts.InputEdit) gts.IncrementalParseProfile {
	t.Helper()
	lang := grammars.MarkdownLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial Markdown parse: %v", err)
	}
	defer oldTree.Release()
	requireMarkdownAcceptedFullSource(t, oldTree, source, "initial Markdown certification")
	if oldTree.RootNode().HasError() {
		t.Fatalf("initial Markdown parse produced ERROR: %s", oldTree.RootNode().SExpr(lang))
	}
	initialRuntime := oldTree.ParseRuntime()
	if initialRuntime.ExternalScannerCheckpointRecords == 0 ||
		initialRuntime.ExternalScannerCheckpointBytesAllocated == 0 {
		t.Fatalf("initial Markdown parse did not record scanner checkpoints: %s", initialRuntime.Summary())
	}

	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental Markdown parse: %v", err)
	}
	defer incremental.Release()
	requireMarkdownAcceptedFullSource(t, incremental, edited, "incremental Markdown certification")
	if profile.ReuseUnsupported {
		t.Fatalf("certified Markdown scanner fell back: reason=%q", profile.ReuseUnsupportedReason)
	}
	if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("certified Markdown scanner did not reuse old syntax: %+v", profile)
	}
	if incremental.RootNode().HasError() {
		t.Fatalf("incremental Markdown parse produced ERROR: %s", incremental.RootNode().SExpr(lang))
	}

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh Markdown parse: %v", err)
	}
	defer fresh.Release()
	requireMarkdownAcceptedFullSource(t, fresh, edited, "fresh Markdown certification")
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
	return profile
}

func requireMarkdownAcceptedFullSource(t *testing.T, tree *gts.Tree, source []byte, phase string) {
	t.Helper()
	rt := tree.ParseRuntime()
	if tree.ParseStoppedEarly() || rt.StopReason != gts.ParseStopAccepted || rt.Truncated ||
		rt.ExpectedEOFByte != uint32(len(source)) {
		t.Fatalf("%s: parse was not accepted, non-truncated, and full-source: %s", phase, rt.Summary())
	}
}

func BenchmarkMarkdownParseFull256KiB(b *testing.B) {
	source, _ := makeMarkdownCertificationSource(256 << 10)
	lang := grammars.MarkdownLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}

func BenchmarkMarkdownParseIncrementalInsertDelete256KiB(b *testing.B) {
	sourceA, sites := makeMarkdownCertificationSource(256 << 10)
	offset := sites[len(sites)/2]
	sourceB, insert := applyMarkdownCertificationEdit(sourceA, offset, markdownCertificationInsert)
	deleteEdit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset),
		StartPoint:  pointForOffset(sourceB, offset),
		OldEndPoint: pointForOffset(sourceB, offset+1),
		NewEndPoint: pointForOffset(sourceA, offset),
	}

	lang := grammars.MarkdownLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(sourceA)
	if err != nil {
		b.Fatal(err)
	}
	currentA := true
	b.Cleanup(func() {
		if tree != nil {
			tree.Release()
		}
	})
	b.ReportAllocs()
	b.SetBytes(int64(len(sourceA)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var next []byte
		edit := insert
		if currentA {
			next = sourceB
		} else {
			next = sourceA
			edit = deleteEdit
		}
		tree.Edit(edit)
		nextTree, parseErr := parser.ParseIncremental(next, tree)
		if parseErr != nil {
			b.Fatal(parseErr)
		}
		tree.Release()
		tree = nextTree
		currentA = !currentA
	}
}
