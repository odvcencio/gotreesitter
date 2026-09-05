//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"testing"
)

func TestCompactBorrowedFirstNavigationUsesNewTree(t *testing.T) {
	parser := newAdmissionCandidateGoParser(t)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	borrowed := compactExecutionNode(old.RootNode(), parser.language, "function_declaration", uint32(bytes.Index(source, []byte("func b"))))
	if borrowed == nil {
		t.Fatal("initial tree has no function b")
	}
	offset := bytes.IndexByte(source, '1')
	edited, edit := compactExecutionEdit(source, offset, offset+1, "1+3")
	old.Edit(edit)
	next, err := parser.ParseIncremental(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if !next.compactMaterialized {
		t.Fatal("incremental execution did not return a compact tree")
	}
	// Navigate the borrowed pointer before any accessor visits the new root.
	if got := borrowed.Parent(); got != next.root {
		t.Fatalf("first borrowed Parent call returned %p; want new root %p", got, next.root)
	}
	if sibling := borrowed.PrevSibling(); sibling == nil || sibling.ownerArena != next.arena || sibling.Text(edited) != "func a() { _ = 1+3 }" {
		t.Fatal("first borrowed sibling call did not use the reparsed function")
	}
}

func TestCompactBorrowedMaterializationRecordsRejectedAllocation(t *testing.T) {
	parser := newAdmissionCandidateGoParser(t)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() { _ = 1 }\n")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	runner, ok := parser.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	if !ok || !old.compactMaterialized {
		t.Fatal("initial parse did not retain an accepted compact derivation")
	}
	for _, reject := range []bool{false, true} {
		materializationSource := source
		if reject {
			materializationSource = append(append([]byte(nil), source...), '@')
		}
		var timing incrementalParseTiming
		session := &compactIncrementalReuseSession{oldTree: old, timing: &timing}
		scratch := parserCoreRunnerScratch{incrementalReuse: session}
		tree, err := materializeDiagnosticParserCoreAcceptedTree(runner.compact, runner.scheduler.acceptedHead, runner.parser, materializationSource, &scratch, true, false)
		if reject {
			if tree != nil {
				tree.Release()
			}
			if err == nil {
				t.Fatal("materialization accepted an uncovered source suffix")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if timing.newNodes != uint64(tree.rawParseRuntime().NodesAllocated) {
				t.Fatal("successful materialization allocation disagrees with its runtime")
			}
			tree.Release()
		}
		if timing.newNodes == 0 {
			t.Fatalf("materialization discarded its allocated-node count: rejected=%t", reject)
		}
		if scratch.incrementalReuse != nil {
			t.Fatal("materialization retained the allocation timing session")
		}
	}
}
