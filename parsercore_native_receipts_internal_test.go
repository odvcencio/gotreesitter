//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/incr"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestNativeProjectionReceiptPassMatchesReference(t *testing.T) {
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	scheduler, tokens, err := runner.executeSchedulerOpen(fixture.Source, runner.compact, false)
	if tokens != nil {
		defer tokens.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	id := scheduler.acceptedPayloads[0]
	g, err := runner.compact.SubtreeGeometry(id)
	if err != nil {
		t.Fatal(err)
	}
	points, err := newDiagnosticParserCorePointIndexInto(fixture.Source, func() error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"none", "point", "state", "symbol", "source", "duplicate", "reorder", "foreign", "cancel"} {
		t.Run(mutation, func(t *testing.T) {
			// Three primary slots force the remaining records into an overflow
			// slab and exercise direct receipt addressing across the boundary.
			arena := &nodeArena{nodes: make([]Node, 3)}
			var workspace nativeProjectionWorkspace
			workspace.records.Append(nativeStreamRecord{})
			reads := incr.NewReads(len(fixture.Source))
			reads.Record(0, uint32(len(fixture.Source))+1)
			s := &diagnosticParserCoreGenericScheduler{compact: runner.compact}
			s.reuseDependencies.reads = reads
			s.reuseDependencies.leafWords = make([]uint32, (uint32(id)+16)/16)
			incr.SetLeafReceipt(s.reuseDependencies.leafWords, uint32(id), legacyReuseLeafKnown|legacyReuseKeyword)
			var nodes []*Node
			for i := 0; i < 9; i++ {
				n := newLeafNodeInArena(arena, Symbol(g.Symbol), true, g.StartByte, g.EndByte, points.point(g.StartByte), points.point(g.EndByte))
				n.setCompactMaterialized(true)
				n.setCompactParseStateProof(true)
				n.setCompactPreGotoStateProof(true)
				nodes = append(nodes, n)
				workspace.records.Append(nativeStreamRecord{node: n, metadata: nativeStreamMetadata{source: id, field: core.FieldID(g.Symbol)}})
			}
			switch mutation {
			case "point":
				nodes[4].endPoint.Column++
			case "state":
				nodes[4].setCompactParseStateProof(false)
			case "symbol":
				nodes[4].symbol++
			case "source":
				workspace.records.At(5).metadata.source = 0
			case "duplicate":
				workspace.records.At(5).node = nodes[0]
			case "reorder":
				workspace.records.At(5).node, workspace.records.At(6).node = nodes[5], nodes[4]
			case "foreign":
				nodes[4].ownerArena = &nodeArena{}
			}
			stop := errors.New("cancel publication")
			err := s.publishNativeProjectionReads(arena, &workspace, &points, func() error {
				if mutation == "cancel" {
					return stop
				}
				return nil
			})
			mustFail := mutation == "duplicate" || mutation == "reorder" || mutation == "foreign" || mutation == "cancel"
			if (err != nil) != mustFail {
				t.Fatalf("publication error=%v, want failure=%t", err, mustFail)
			}
			if mustFail {
				return
			}
			for i, n := range nodes {
				word := legacyReuseWord(n, false)
				rejected := i == 4 && mutation != "none"
				if rejected {
					if word != nil && *word != 0 {
						t.Fatal("changed projection retained a receipt")
					}
					continue
				}
				count, ok := reads.LeafLookahead(n.EndByte())
				want := incr.Encode(count) | legacyReuseLeafKnown | legacyReuseKeyword
				if !ok || word == nil || *word != want {
					t.Fatalf("slot %d receipt=%v want=%d", i, word, want)
				}
			}
		})
	}
}
