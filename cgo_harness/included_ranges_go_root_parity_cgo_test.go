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
// Update: the parser's padding scan now clips to the included ranges, so the
// route no longer forces recovery between two included ranges. All four
// geometries pinned below now agree with the C oracle on the root symbol
// (before the fix, one of the four produced an ERROR root here where C
// produced source_file), and the Go arm's root member no longer fires on any
// of them. The root span and child count still diverge from C on every
// geometry.
//
// READ THIS BEFORE CITING THIS TEST AS EVIDENCE. gotreesitter and the C
// oracle do NOT agree on this route in general. They agree on the root symbol
// for all four geometries pinned here, and on the root span for exactly one:
// the geometry whose first range starts at byte 0 and whose last range ends
// at end of file. No geometry reaches child-count parity. This test pins
// each observation per geometry. It is a change detector for the route, not
// a parity certificate for it.
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
// The padding scan now clips to the included ranges: it no longer scans the
// excluded bytes between two ranges as if they had to be whitespace, so a
// non-whitespace gap between ranges no longer kills every GLR stack and
// forces recovery. That is what padding_kills_stacks used to pin. Every
// geometry below now reaches a `source_file` root, and the Go arm's root
// member no longer fires on any of them, because none of them hands it an
// ERROR root to repair. The root span and child count still diverge from C
// on every geometry — a separate, pre-existing set of divergences this fix
// does not address.
var includedRangesGoGeometries = []includedRangesGeometry{
	{
		name:        "interior_anchors_arm_live",
		spans:       [2][2]int{{26, 150}, {203, 276}},
		armRewrites: 0,
		c:           includedRangesRootObservation{"source_file", 26, 276, 7, true},
		gts:         includedRangesRootObservation{"source_file", 0, 276, 10, true},
		note: "The realistic injection shape: both ranges start at a positive " +
			"offset. The root symbol matches C; the Go arm member does not " +
			"fire, because the root already carries the right symbol before " +
			"the member inspects it. The root span still diverges: " +
			"gotreesitter starts at byte 0 while C starts at the first " +
			"included range. The child count now diverges too (10 vs. 7): the " +
			"parser folds both ranges into one pass instead of the recovery " +
			"reparse the old, unclipped scan used to force.",
	},
	{
		name:        "anchored_at_zero_and_eof",
		spans:       [2][2]int{{0, 150}, {203, 276}},
		armRewrites: 0,
		c:           includedRangesRootObservation{"source_file", 0, 276, 7, true},
		gts:         includedRangesRootObservation{"source_file", 0, 276, 10, true},
		note: "A shape an injection child never receives: a Markdown fence " +
			"cannot start at byte 0, because the opening fence line always " +
			"precedes the content. The root span still reaches C parity (both " +
			"ranges anchor the root at document start and end), but the child " +
			"count no longer does: this used to be the one geometry with full " +
			"span-and-child-count parity, and clipping the padding scan traded " +
			"that for a root symbol that matches C on every geometry instead.",
	},
	{
		name:        "trimmed_tail_child_count_diverges",
		spans:       [2][2]int{{0, 150}, {203, 250}},
		armRewrites: 0,
		c:           includedRangesRootObservation{"source_file", 0, 250, 6, true},
		gts:         includedRangesRootObservation{"source_file", 0, 251, 10, true},
		note: "Moving the last range off end of file diverges the span and the " +
			"child count even with the first range anchored at byte 0.",
	},
	{
		name:        "padding_kills_stacks",
		spans:       [2][2]int{{40, 150}, {203, 260}},
		armRewrites: 0,
		c:           includedRangesRootObservation{"source_file", 40, 260, 5, true},
		gts:         includedRangesRootObservation{"source_file", 40, 264, 9, true},
		note: "Before the padding-scan fix, the root symbol itself diverged " +
			"here (gotreesitter published ERROR where C published source_file) " +
			"and the Go arm member did not fire, because the recovery that " +
			"produced the ERROR root happened by a different path than the one " +
			"the member repairs. The root symbol now matches C. The span and " +
			"child count still diverge.",
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

// TestIncludedRangesGoArmGuardsRootSymbol is the root-symbol regression
// guard for this route. It is separate from the pin table above so its
// intent cannot be misread: on every pinned geometry, the root symbol must
// keep matching C, and the Go arm's root member must stay inert, because the
// padding-scan fix keeps this route out of recovery and the member no longer
// has an ERROR root to repair. If a future change reopens the route to
// recovery on any pinned geometry, the member fires again, this guard's
// armRewrites check fails, and — if the member's repair does not fully
// recover the root symbol — the Kind check fails too. Nothing here claims
// the route is at span or child-count parity.
func TestIncludedRangesGoArmGuardsRootSymbol(t *testing.T) {
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
			if armRewrites != 0 {
				t.Fatalf("dispatch.go.source-file-root rewrote %d nodes; want 0 now that the padding scan clips to included ranges", armRewrites)
			}
			if gotGo.Kind != gotC.Kind {
				t.Fatalf("root symbol diverged from C: gotreesitter=%q C=%q", gotGo.Kind, gotC.Kind)
			}
			if gotGo.Kind == "ERROR" {
				t.Fatalf("root is an ERROR node while C publishes %q", gotC.Kind)
			}
		})
	}
}
