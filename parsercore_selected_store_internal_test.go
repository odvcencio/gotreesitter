//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreSelectedStoreCanonicalAdmissions(t *testing.T) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				t.Fatal(err)
			}
			scheduler, tokenSource, err := runner.executeSchedulerOpen(fixture.Source, runner.compact, false)
			if tokenSource != nil {
				defer tokenSource.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(scheduler.acceptedPayloads) == 0 {
				t.Fatal("accepted payloads were not carried from acceptance")
			}
			store, err := runner.compact.BuildAuthenticatedSelectedStore(scheduler.acceptedPayloads, fixture.Source, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			defer store.Release()
			if got := store.NodeCount(); got != row.selectedNodes {
				t.Fatalf("selected store nodes=%d want=%d", got, row.selectedNodes)
			}
			if bytesPerNode := store.RetainedBytes() / store.NodeCount(); bytesPerNode > 80 {
				t.Fatalf("selected store retained bytes/node=%d want<=80", bytesPerNode)
			}
			tree, materializeErr := runner.materialize(fixture.Source, runner.compact, scheduler.acceptedHead)
			if materializeErr != nil {
				t.Fatal(materializeErr)
			}
			defer tree.Release()
			requireDiagnosticParserCoreSelectedStoreMatchesTree(t, store, tree.root, runner.lang)
			if got := diagnosticParserCoreSelectedStoreDeepDigest(t, store, runner.lang, fixture.Source); got != row.deepTreeSHA256 {
				t.Fatalf("selected store digest=%s want=%s", got, row.deepTreeSHA256)
			}
			if work := runner.compact.Work(); work != row.work || work.Overflow {
				t.Fatalf("selected store work=%+v want=%+v", work, row.work)
			}
			t.Logf("SELECTED_STORE fixture=%s nodes=%d retained_bytes=%d", row.id, store.NodeCount(), store.RetainedBytes())
		})
	}
}

func TestDiagnosticParserCoreSelectedStoreCompletenessFailureReleasesBacking(t *testing.T) {
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	store, err := runner.parseSelectedStore(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	if adopted, err := requireParserCoreSelectedStoreCompleteness(store, len(fixture.Source)+1); err == nil || adopted != nil {
		t.Fatalf("incomplete selected store adopted=%v err=%v", adopted, err)
	}
	if store.NodeCount() != 0 || store.RetainedBytes() != 0 {
		t.Fatalf("failed completeness retained nodes=%d bytes=%d", store.NodeCount(), store.RetainedBytes())
	}
	fresh, err := runner.parseSelectedStore(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Release()
}

func requireDiagnosticParserCoreSelectedStoreMatchesTree(t testing.TB, store *core.SelectedStore, root *Node, lang *Language) {
	t.Helper()
	type pair struct {
		id    core.SelectedNodeID
		node  *Node
		field core.FieldID
	}
	stack := []pair{{id: store.Root(), node: root}}
	for visited := 0; len(stack) != 0; visited++ {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		record, ok := store.Record(current.id)
		if !ok || current.node == nil {
			t.Fatalf("selected store/tree missing node at visit %d", visited)
		}
		treeField := FieldID(0)
		if current.node.parent != nil && current.node.childIndex >= 0 {
			treeField = nodeFieldIDAt(current.node.parent, int(current.node.childIndex))
		}
		terminal := current.node.ChildCount() == 0
		if Symbol(record.Symbol) != current.node.symbol || record.StartByte != current.node.startByte || record.EndByte != current.node.endByte ||
			record.Named() != current.node.IsNamed() || record.Extra() != current.node.IsExtra() || record.External() != current.node.hasFlag(nodeFlagExternalScannerToken) ||
			record.Terminal() != terminal || record.Missing() != current.node.IsMissing() || record.HasError() != current.node.HasError() ||
			int(record.ChildCount) != current.node.ChildCount() || FieldID(current.field) != treeField ||
			record.ProductionID != current.node.productionID || record.DynamicPrecedence != current.node.dynamicPrecedence {
			t.Fatalf("selected store metadata mismatch visit=%d store=%+v tree={sym:%d span:%d..%d named:%t extra:%t external:%t terminal:%t missing:%t has_error:%t children:%d field:%d production:%d precedence:%d type:%s}", visited, record, current.node.symbol, current.node.startByte, current.node.endByte, current.node.IsNamed(), current.node.IsExtra(), current.node.hasFlag(nodeFlagExternalScannerToken), terminal, current.node.IsMissing(), current.node.HasError(), current.node.ChildCount(), treeField, current.node.productionID, current.node.dynamicPrecedence, current.node.Type(lang))
		}
		for index := int(record.ChildCount) - 1; index >= 0; index-- {
			childID, _ := store.Child(record, uint32(index))
			childRecord, _ := store.Record(childID)
			stack = append(stack, pair{id: childID, node: current.node.Child(index), field: childRecord.Field})
		}
	}
}

type selectedStoreDigest struct {
	h     hash.Hash
	open  uint64
	nodes uint64
}

func diagnosticParserCoreSelectedStoreDeepDigest(t testing.TB, store *core.SelectedStore, lang *Language, source []byte) string {
	t.Helper()
	if store == nil || lang == nil {
		t.Fatal("selected store digest requires store and language")
	}
	d := selectedStoreDigest{h: sha256.New(), open: 1}
	_, _ = d.h.Write([]byte("gts-deep-tree-v1\x00"))
	points, err := newDiagnosticParserCorePointIndex(source, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	type item struct {
		id    core.SelectedNodeID
		field core.FieldID
	}
	stack := []item{{id: store.Root()}}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		record, ok := store.Record(current.id)
		if !ok {
			t.Fatalf("selected store omitted node %d", current.id)
		}
		typeName := ""
		if int(record.Symbol) < len(lang.SymbolNames) {
			typeName = unescapePunctuationSymbolName(lang.SymbolNames[record.Symbol])
		}
		fieldName := ""
		if current.field != 0 && int(current.field) < len(lang.FieldNames) {
			fieldName = lang.FieldNames[current.field]
		}
		start, end := points.point(record.StartByte), points.point(record.EndByte)
		d.addString(typeName)
		d.addString(fieldName)
		for _, value := range [...]uint32{record.StartByte, record.EndByte, start.Row, start.Column, end.Row, end.Column} {
			d.add32(value)
		}
		var flags byte
		if record.Named() {
			flags |= 1 << 0
		}
		if record.Extra() {
			flags |= 1 << 1
		}
		_, _ = d.h.Write([]byte{flags})
		d.add32(uint32(record.ChildCount))
		if d.open == 0 {
			t.Fatal("selected store digest closed before traversal ended")
		}
		d.open--
		d.open += uint64(record.ChildCount)
		d.nodes++
		for index := int(record.ChildCount) - 1; index >= 0; index-- {
			childID, ok := store.Child(record, uint32(index))
			if !ok {
				t.Fatalf("selected store child window drifted at node %d child %d", current.id, index)
			}
			child, ok := store.Record(childID)
			if !ok || child.Parent != current.id {
				t.Fatalf("selected store parent linkage drifted at child %d", childID)
			}
			stack = append(stack, item{id: childID, field: child.Field})
		}
	}
	if d.open != 0 || d.nodes == 0 {
		t.Fatalf("selected store digest incomplete: open=%d nodes=%d", d.open, d.nodes)
	}
	return fmt.Sprintf("%x", d.h.Sum(nil))
}

func (d *selectedStoreDigest) addString(value string) {
	d.add32(uint32(len(value)))
	_, _ = d.h.Write([]byte(value))
}

func (d *selectedStoreDigest) add32(value uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], value)
	_, _ = d.h.Write(buf[:])
}
