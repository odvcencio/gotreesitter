//go:build cgo && treesitter_c_parity

package cgoharness

// C-oracle coverage for the included-ranges route. No test covered
// SetIncludedRanges for any language before this one, which is why a proposed
// deletion of the Go arm's normalizeGoSourceFileRoot member passed a full
// green suite while it turned this route's root from `source_file` into
// `ERROR`.
//
// Production reaches the route through injection: injection.go calls
// childParser.SetIncludedRanges for every injected child, and
// grammars/markdown_injection_register.go maps the `go` and `golang` fence
// languages to the Go grammar.
//
// READ THIS BEFORE CITING THIS TEST AS EVIDENCE. gotreesitter and the C
// oracle do NOT agree on this route in general. They agree on the root symbol
// for three of the four geometries pinned here, and they agree on the root
// span and child count for exactly one: the geometry whose first range starts
// at byte 0 and whose last range ends at end of file. Every other measured
// geometry diverges on span, and one diverges on the root symbol itself. This
// test pins each observation per geometry. It is a change detector for the
// route, not a parity certificate for it.
//
// Run: GTS_PARITY_ALLOW_HOST=1 go test ./cgo_harness -tags treesitter_c_parity \
//        -run TestIncludedRangesGo -v

import (
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func includedRangesPointAt(src []byte, off int) (uint, uint) {
	var row, col uint
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			row++
			col = 0
			continue
		}
		col++
	}
	return row, col
}

// includedRangesRootObservation is one measured root, pinned exactly.
type includedRangesRootObservation struct {
	Kind       string
	StartByte  uint32
	EndByte    uint32
	ChildCount int
	HasError   bool
}

type includedRangesGeometry struct {
	name  string
	spans [2][2]int
	// armRewrites is the node count the Go arm's root member rewrote. A
	// nonzero value proves the member repaired an ERROR root on this
	// geometry, because normalizeGoSourceFileRoot returns early for any other
	// root symbol.
	armRewrites uint64
	c           includedRangesRootObservation
	gts         includedRangesRootObservation
	// note records what this geometry proves and what it does not.
	note string
}

// includedRangesGoGeometries pins every measured geometry, including the ones
// where gotreesitter diverges from C. Divergence is recorded, never asserted
// away.
//
// The padding_kills_stacks case is a known open defect, not a Go arm problem:
// bytesAreParserPadding scans raw bytes between the stack offset and the next
// token without clipping to the included ranges, so bytes that lie between two
// ranges kill every GLR stack and force recovery where C never recovers. A
// separate lane owns that fix. When it lands these pins move, and the Go arm's
// root member should be re-censused on this route: it may become genuinely
// dead.
var includedRangesGoGeometries = []includedRangesGeometry{
	{
		name:        "interior_anchors_arm_live",
		spans:       [2][2]int{{26, 150}, {203, 276}},
		armRewrites: 96,
		c:           includedRangesRootObservation{"source_file", 26, 276, 7, true},
		gts:         includedRangesRootObservation{"source_file", 0, 276, 7, false},
		note: "The realistic injection shape: both ranges start at a positive " +
			"offset. The root symbol matches C and the Go arm member is live. " +
			"The root span diverges: gotreesitter starts at byte 0 while C " +
			"starts at the first included range.",
	},
	{
		name:        "anchored_at_zero_and_eof",
		spans:       [2][2]int{{0, 150}, {203, 276}},
		armRewrites: 96,
		c:           includedRangesRootObservation{"source_file", 0, 276, 7, true},
		gts:         includedRangesRootObservation{"source_file", 0, 276, 7, false},
		note: "The ONLY geometry where the root span and child count reach C " +
			"parity, and it is a shape an injection child never receives: a " +
			"Markdown fence cannot start at byte 0, because the opening fence " +
			"line always precedes the content. Kept to document that the " +
			"parity is an artifact of the anchors, not a property of the route.",
	},
	{
		name:        "trimmed_tail_child_count_diverges",
		spans:       [2][2]int{{0, 150}, {203, 250}},
		armRewrites: 96,
		c:           includedRangesRootObservation{"source_file", 0, 250, 6, true},
		gts:         includedRangesRootObservation{"source_file", 0, 276, 7, false},
		note: "Moving the last range off end of file diverges the span and the " +
			"child count even with the first range anchored at byte 0.",
	},
	{
		name:        "padding_kills_stacks",
		spans:       [2][2]int{{40, 150}, {203, 260}},
		armRewrites: 0,
		c:           includedRangesRootObservation{"source_file", 40, 260, 5, true},
		gts:         includedRangesRootObservation{"ERROR", 0, 153, 4, true},
		note: "The root symbol itself diverges here and the Go arm member does " +
			"NOT fire, so the member cannot be credited with root repair in " +
			"general on this route. Pinned as an open defect; see the note on " +
			"bytesAreParserPadding above.",
	},
}

func measureIncludedRangesRoots(
	t *testing.T,
	src []byte,
	cLang *sitter.Language,
	goLang *gts.Language,
	spans [2][2]int,
) (includedRangesRootObservation, includedRangesRootObservation, uint64) {
	t.Helper()
	cRanges := make([]sitter.Range, 0, len(spans))
	goRanges := make([]gts.Range, 0, len(spans))
	for _, span := range spans {
		start, end := span[0], span[1]
		if end > len(src) {
			end = len(src)
		}
		sr, sc := includedRangesPointAt(src, start)
		er, ec := includedRangesPointAt(src, end)
		cRanges = append(cRanges, sitter.Range{
			StartByte:  uint(start),
			EndByte:    uint(end),
			StartPoint: sitter.Point{Row: sr, Column: sc},
			EndPoint:   sitter.Point{Row: er, Column: ec},
		})
		goRanges = append(goRanges, gts.Range{
			StartByte:  uint32(start),
			EndByte:    uint32(end),
			StartPoint: gts.Point{Row: uint32(sr), Column: uint32(sc)},
			EndPoint:   gts.Point{Row: uint32(er), Column: uint32(ec)},
		})
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set C language: %v", err)
	}
	if err := cParser.SetIncludedRanges(cRanges); err != nil {
		t.Fatalf("C SetIncludedRanges: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil {
		t.Fatal("C parse returned nil")
	}
	defer cTree.Close()
	cRoot := cTree.RootNode()

	goParser := gts.NewParser(goLang)
	goParser.SetIncludedRanges(goRanges)
	goTree, err := goParser.Parse(src)
	if err != nil {
		t.Fatalf("go parse: %v", err)
	}
	if goTree == nil || goTree.RootNode() == nil {
		t.Fatal("go parse returned no tree")
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()

	var armRewrites uint64
	if passes := goTree.ParseRuntime().NormalizationPasses; passes != nil {
		for _, pass := range *passes {
			if pass.Name == "dispatch.go.source-file-root" {
				armRewrites = pass.NodesRewritten
			}
		}
	}

	return includedRangesRootObservation{
			Kind:       cRoot.Kind(),
			StartByte:  uint32(cRoot.StartByte()),
			EndByte:    uint32(cRoot.EndByte()),
			ChildCount: int(cRoot.ChildCount()),
			HasError:   cRoot.HasError(),
		}, includedRangesRootObservation{
			Kind:       goRoot.Type(goLang),
			StartByte:  goRoot.StartByte(),
			EndByte:    goRoot.EndByte(),
			ChildCount: goRoot.ChildCount(),
			HasError:   goRoot.HasError(),
		}, armRewrites
}

func loadIncludedRangesGoFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "testdata", "included_ranges", "go_two_fences.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read included-ranges fixture: %v", err)
	}
	return src
}

// TestIncludedRangesGoRootParity pins the measured root of both parsers on the
// included-ranges route, per geometry. Every field is a snapshot: a change in
// either parser, in either direction, fails here.
//
// This test asserts parity nowhere. It records what each parser produces. Read
// the per-geometry note before citing any row as evidence.
func TestIncludedRangesGoRootParity(t *testing.T) {
	cLang := loadCanonicalGoCLanguage(t)
	goLang := grammars.GoLanguage()
	if goLang == nil {
		t.Skip("go grammar unavailable")
	}
	src := loadIncludedRangesGoFixture(t)

	for _, geometry := range includedRangesGoGeometries {
		geometry := geometry
		t.Run(geometry.name, func(t *testing.T) {
			t.Setenv("GTS_DISPATCHER_CENSUS", "1")
			gotC, gotGo, armRewrites := measureIncludedRangesRoots(t, src, cLang, goLang, geometry.spans)

			t.Logf("note: %s", geometry.note)
			t.Logf("C   %+v", gotC)
			t.Logf("GTS %+v", gotGo)
			t.Logf("dispatch.go.source-file-root rewrote %d nodes", armRewrites)

			if gotC != geometry.c {
				t.Errorf("C oracle root moved: got %+v, pinned %+v", gotC, geometry.c)
			}
			if gotGo != geometry.gts {
				t.Errorf("gotreesitter root moved: got %+v, pinned %+v", gotGo, geometry.gts)
			}
			if armRewrites != geometry.armRewrites {
				t.Errorf("dispatch.go.source-file-root rewrites = %d, pinned %d", armRewrites, geometry.armRewrites)
			}
		})
	}
}

// TestIncludedRangesGoArmGuardsRootSymbol is the deletion guard. It is
// separate from the pin table above so its intent cannot be misread: on the
// geometries where the Go arm's root member fires, deleting the member turns
// the root from `source_file` into `ERROR`, which moves gotreesitter away from
// C. Nothing here claims the route is at parity.
func TestIncludedRangesGoArmGuardsRootSymbol(t *testing.T) {
	cLang := loadCanonicalGoCLanguage(t)
	goLang := grammars.GoLanguage()
	if goLang == nil {
		t.Skip("go grammar unavailable")
	}
	src := loadIncludedRangesGoFixture(t)

	guarded := 0
	for _, geometry := range includedRangesGoGeometries {
		if geometry.armRewrites == 0 {
			continue
		}
		geometry := geometry
		guarded++
		t.Run(geometry.name, func(t *testing.T) {
			t.Setenv("GTS_DISPATCHER_CENSUS", "1")
			gotC, gotGo, armRewrites := measureIncludedRangesRoots(t, src, cLang, goLang, geometry.spans)
			if armRewrites == 0 {
				t.Fatal("the Go arm root member no longer fires here; the deletion guard is now vacuous")
			}
			if gotGo.Kind != gotC.Kind {
				t.Fatalf("root symbol diverged from C: gotreesitter=%q C=%q", gotGo.Kind, gotC.Kind)
			}
			if gotGo.Kind == "ERROR" {
				t.Fatalf("root is an ERROR node while C publishes %q", gotC.Kind)
			}
		})
	}
	if guarded == 0 {
		t.Fatal("no geometry exercises the Go arm root member; the deletion guard is vacuous")
	}
}
