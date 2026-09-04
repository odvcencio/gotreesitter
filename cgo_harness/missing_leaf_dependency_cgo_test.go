//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const goMissingDependencyAssignmentTree = "(source_file (assignment_statement left: (expression_list (MISSING identifier)) right: (expression_list (identifier))))"

func TestCMissingSemicolonLookaheadIncrementalParity(t *testing.T) {
	source := []byte("int a int b;\n")
	goLanguage := grammars.CLanguage()
	gotreesitter.EnableArenaBreakdown(true)
	t.Cleanup(func() { gotreesitter.EnableArenaBreakdown(false) })
	cLanguage, err := COracleLanguage("c")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name        string
		offset      int
		replacement byte
	}{
		{name: "first_lookahead_byte", offset: 6, replacement: 'a'},
		{name: "middle_lookahead_byte", offset: 7, replacement: 'a'},
		{name: "last_lookahead_byte", offset: 8, replacement: 'x'},
	} {
		t.Run(test.name, func(t *testing.T) {
			edited := append([]byte(nil), source...)
			edited[test.offset] = test.replacement
			edit := gotreesitter.InputEdit{
				StartByte: uint32(test.offset), OldEndByte: uint32(test.offset + 1), NewEndByte: uint32(test.offset + 1),
				StartPoint: gotreesitter.Point{Column: uint32(test.offset)}, OldEndPoint: gotreesitter.Point{Column: uint32(test.offset + 1)}, NewEndPoint: gotreesitter.Point{Column: uint32(test.offset + 1)},
			}

			goParser := gotreesitter.NewParser(goLanguage)
			goOld, err := goParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer goOld.Release()
			goMissing := findFirstMissingDependencyGoNode(goOld.RootNode())
			if goMissing == nil || goMissing.Type(goLanguage) != ";" || goMissing.StartByte() != 5 || goMissing.EndByte() != 5 {
				t.Fatal("Go initial tree lacks the missing-semicolon witness")
			}
			breakdown, recorded := goOld.ArenaBreakdown()
			if !recorded || breakdown.MissingNodeDependencyCount == 0 || breakdown.MissingNodeDependencyBytesAllocated == 0 {
				t.Fatalf("Go parse did not record missing-dependency telemetry: recorded=%t breakdown=%+v", recorded, breakdown)
			}

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cOld := cParser.Parse(source, nil)
			if cOld == nil {
				t.Fatal("C initial parse returned nil")
			}
			defer cOld.Close()
			if diff := FirstDivergenceDumpV1(goOld.RootNode(), goLanguage, cOld.RootNode()); diff != nil {
				t.Fatalf("initial Go tree differs from C: %+v", *diff)
			}

			goOld.Edit(edit)
			if !goMissing.HasChanges() {
				t.Fatalf("edit at lookahead byte %d did not invalidate the Go missing semicolon", test.offset)
			}
			cEdit := realCorpusCInputEdit(edit)
			cOld.Edit(&cEdit)
			cMissing := findFirstMissingDependencyCNode(cOld.RootNode())
			if cMissing == nil || !cMissing.HasChanges() {
				t.Fatalf("edit at lookahead byte %d did not invalidate the C missing semicolon", test.offset)
			}

			goIncremental, profile, err := goParser.ParseIncrementalProfiled(edited, goOld)
			if err != nil {
				t.Fatal(err)
			}
			defer goIncremental.Release()
			goFresh, err := goParser.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer goFresh.Release()
			var goErrors []string
			compareGoNodes(goIncremental.RootNode(), goLanguage, goFresh.RootNode(), "root", &goErrors)
			if len(goErrors) != 0 {
				t.Fatalf("incremental Go tree differs from fresh Go: %s", goErrors[0])
			}

			cIncremental := cParser.Parse(edited, cOld)
			if cIncremental == nil {
				t.Fatal("C incremental parse returned nil")
			}
			defer cIncremental.Close()
			if diff := FirstDivergenceDumpV1(goIncremental.RootNode(), goLanguage, cIncremental.RootNode()); diff != nil {
				t.Fatalf("incremental Go tree differs from incremental C: %+v; profile=%+v", *diff, profile)
			}
		})
	}
}

func findFirstMissingDependencyGoNode(node *gotreesitter.Node) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.IsMissing() {
		return node
	}
	for index := 0; index < node.ChildCount(); index++ {
		if missing := findFirstMissingDependencyGoNode(node.Child(index)); missing != nil {
			return missing
		}
	}
	return nil
}

type missingDependencyOracleCase struct {
	name             string
	source           []byte
	edited           []byte
	oldRanges        []gotreesitter.Range
	newRanges        []gotreesitter.Range
	edit             gotreesitter.InputEdit
	wantEdited       string
	wantChanged      []gotreesitter.Range
	wantRightChanged bool
}

// TestGoMissingLeafDependencyLockedCOracle pins the C behavior that package
// five must preserve. Separate Go unit tests apply these dependency bounds to
// sparse missing-node metadata. The current Go parser has an earlier recovery
// topology difference for this source, so this test must not hide that gap by
// comparing unlike initial trees.
func TestGoMissingLeafDependencyLockedCOracle(t *testing.T) {
	includedRange := gotreesitter.Range{
		StartByte: 1, EndByte: 3,
		StartPoint: gotreesitter.Point{Row: 1}, EndPoint: gotreesitter.Point{Row: 1, Column: 2},
	}
	cases := []missingDependencyOracleCase{
		{
			name: "included_padding_replacement", source: []byte("\n=i"), edited: []byte("x=i"),
			oldRanges:  []gotreesitter.Range{includedRange},
			newRanges:  []gotreesitter.Range{{StartByte: 1, EndByte: 3, StartPoint: gotreesitter.Point{Column: 1}, EndPoint: gotreesitter.Point{Column: 3}}},
			edit:       gotreesitter.InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: gotreesitter.Point{Row: 1}, NewEndPoint: gotreesitter.Point{Column: 1}},
			wantEdited: goMissingDependencyAssignmentTree,
		},
		{
			name: "included_padding_insertion", source: []byte("\n=i"), edited: []byte("x\n=i"),
			oldRanges:  []gotreesitter.Range{includedRange},
			newRanges:  []gotreesitter.Range{{StartByte: 2, EndByte: 4, StartPoint: gotreesitter.Point{Row: 1}, EndPoint: gotreesitter.Point{Row: 1, Column: 2}}},
			edit:       gotreesitter.InputEdit{StartByte: 0, OldEndByte: 0, NewEndByte: 1, NewEndPoint: gotreesitter.Point{Column: 1}},
			wantEdited: goMissingDependencyAssignmentTree,
		},
		{
			name: "included_padding_deletion", source: []byte("\n=i"), edited: []byte("=i"),
			oldRanges:  []gotreesitter.Range{includedRange},
			newRanges:  []gotreesitter.Range{{StartByte: 0, EndByte: 2, EndPoint: gotreesitter.Point{Column: 2}}},
			edit:       gotreesitter.InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 0, OldEndPoint: gotreesitter.Point{Row: 1}},
			wantEdited: goMissingDependencyAssignmentTree,
		},
		{
			name: "included_lookahead_operator", source: []byte("\n=i"), edited: []byte("\n+i"),
			oldRanges: []gotreesitter.Range{includedRange}, newRanges: []gotreesitter.Range{includedRange},
			edit:        gotreesitter.InputEdit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartPoint: gotreesitter.Point{Row: 1}, OldEndPoint: gotreesitter.Point{Row: 1, Column: 1}, NewEndPoint: gotreesitter.Point{Row: 1, Column: 1}},
			wantEdited:  "(source_file (ERROR (unary_expression operand: (identifier))))",
			wantChanged: []gotreesitter.Range{includedRange},
		},
		{
			name: "included_lookahead_identifier", source: []byte("\n=i"), edited: []byte("\n=j"),
			oldRanges: []gotreesitter.Range{includedRange}, newRanges: []gotreesitter.Range{includedRange},
			edit:             gotreesitter.InputEdit{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartPoint: gotreesitter.Point{Row: 1, Column: 1}, OldEndPoint: gotreesitter.Point{Row: 1, Column: 2}, NewEndPoint: gotreesitter.Point{Row: 1, Column: 2}},
			wantEdited:       goMissingDependencyAssignmentTree,
			wantRightChanged: true,
		},
		{
			name: "plain_lookahead_operator", source: []byte("=i"), edited: []byte("+i"),
			edit:        gotreesitter.InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: gotreesitter.Point{Column: 1}, NewEndPoint: gotreesitter.Point{Column: 1}},
			wantEdited:  "(source_file (ERROR (unary_expression operand: (identifier))))",
			wantChanged: []gotreesitter.Range{{StartByte: 0, EndByte: 2, EndPoint: gotreesitter.Point{Column: 2}}},
		},
		{
			name: "plain_lookahead_identifier", source: []byte("=i"), edited: []byte("=j"),
			edit:             gotreesitter.InputEdit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartPoint: gotreesitter.Point{Column: 1}, OldEndPoint: gotreesitter.Point{Column: 2}, NewEndPoint: gotreesitter.Point{Column: 2}},
			wantEdited:       goMissingDependencyAssignmentTree,
			wantRightChanged: true,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			runGoMissingDependencyOracleCase(t, test)
		})
	}
}

func runGoMissingDependencyOracleCase(t *testing.T, test missingDependencyOracleCase) {
	t.Helper()
	cLanguage, err := COracleLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	if err := cParser.SetIncludedRanges(toCMissingDependencyRanges(test.oldRanges)); err != nil {
		t.Fatal(err)
	}
	oldTree := cParser.Parse(test.source, nil)
	if oldTree == nil {
		t.Fatal("locked C initial parse returned nil")
	}
	defer oldTree.Close()
	assertGoMissingDependencyOracleTree(t, oldTree.RootNode(), test.oldRanges)
	oldMissing := findFirstMissingDependencyCNode(oldTree.RootNode())
	oldRightIdentifier := findFirstCleanDependencyCIdentifier(oldTree.RootNode())
	if oldMissing == nil || oldRightIdentifier == nil {
		t.Fatal("locked C initial tree lacks the dependency witness nodes")
	}

	cEdit := sitter.InputEdit{
		StartByte: uint(test.edit.StartByte), OldEndByte: uint(test.edit.OldEndByte), NewEndByte: uint(test.edit.NewEndByte),
		StartPosition: toCMissingDependencyPoint(test.edit.StartPoint), OldEndPosition: toCMissingDependencyPoint(test.edit.OldEndPoint), NewEndPosition: toCMissingDependencyPoint(test.edit.NewEndPoint),
	}
	oldTree.Edit(&cEdit)
	if !oldMissing.HasChanges() {
		t.Fatal("locked C edit did not invalidate the missing-node dependency")
	}
	if oldRightIdentifier.HasChanges() != test.wantRightChanged {
		t.Fatalf("locked C right identifier has_changes=%t, want %t", oldRightIdentifier.HasChanges(), test.wantRightChanged)
	}
	if err := cParser.SetIncludedRanges(toCMissingDependencyRanges(test.newRanges)); err != nil {
		t.Fatal(err)
	}
	newTree := cParser.Parse(test.edited, oldTree)
	if newTree == nil {
		t.Fatal("locked C incremental parse returned nil")
	}
	defer newTree.Close()
	if got := newTree.RootNode().ToSexp(); got != test.wantEdited {
		t.Fatalf("edited C tree=%s, want %s", got, test.wantEdited)
	}
	if test.wantEdited == goMissingDependencyAssignmentTree {
		assertGoMissingDependencyOracleTree(t, newTree.RootNode(), test.newRanges)
	}
	assertMissingDependencyChangedRanges(t, oldTree.ChangedRanges(newTree), test.wantChanged)
}

func assertGoMissingDependencyOracleTree(t *testing.T, root *sitter.Node, ranges []gotreesitter.Range) {
	t.Helper()
	if got := root.ToSexp(); got != goMissingDependencyAssignmentTree {
		t.Fatalf("initial C tree=%s, want %s", got, goMissingDependencyAssignmentTree)
	}
	wantByte := uint(0)
	wantPoint := sitter.Point{}
	if len(ranges) != 0 {
		wantByte = uint(ranges[0].StartByte)
		wantPoint = toCMissingDependencyPoint(ranges[0].StartPoint)
	}
	missing := findFirstMissingDependencyCNode(root)
	if missing == nil {
		t.Fatal("initial C tree has no missing node")
	}
	if missing.Kind() != "identifier" || !missing.IsNamed() || !missing.IsMissing() || missing.IsExtra() || missing.IsError() || !missing.HasError() ||
		missing.StartByte() != wantByte || missing.EndByte() != wantByte || missing.StartPosition() != wantPoint || missing.EndPosition() != wantPoint {
		t.Fatalf("missing C node kind=%q named=%t missing=%t extra=%t error=%t has_error=%t bytes=%d..%d points=%+v..%+v", missing.Kind(), missing.IsNamed(), missing.IsMissing(), missing.IsExtra(), missing.IsError(), missing.HasError(), missing.StartByte(), missing.EndByte(), missing.StartPosition(), missing.EndPosition())
	}
}

func findFirstMissingDependencyCNode(node *sitter.Node) *sitter.Node {
	if node == nil {
		return nil
	}
	if node.IsMissing() {
		return node
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if missing := findFirstMissingDependencyCNode(node.Child(index)); missing != nil {
			return missing
		}
	}
	return nil
}

func findFirstCleanDependencyCIdentifier(node *sitter.Node) *sitter.Node {
	if node == nil {
		return nil
	}
	if node.Kind() == "identifier" && !node.IsMissing() {
		return node
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if identifier := findFirstCleanDependencyCIdentifier(node.Child(index)); identifier != nil {
			return identifier
		}
	}
	return nil
}

func assertMissingDependencyChangedRanges(t *testing.T, got []sitter.Range, want []gotreesitter.Range) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("changed range count=%d, want %d: got=%+v want=%+v", len(got), len(want), got, want)
	}
	for index := range want {
		item := got[index]
		converted := gotreesitter.Range{
			StartByte: uint32(item.StartByte), EndByte: uint32(item.EndByte),
			StartPoint: gotreesitter.Point{Row: uint32(item.StartPoint.Row), Column: uint32(item.StartPoint.Column)},
			EndPoint:   gotreesitter.Point{Row: uint32(item.EndPoint.Row), Column: uint32(item.EndPoint.Column)},
		}
		if converted != want[index] {
			t.Fatalf("changed range %d=%+v, want %+v", index, converted, want[index])
		}
	}
}

func toCMissingDependencyPoint(point gotreesitter.Point) sitter.Point {
	return sitter.Point{Row: uint(point.Row), Column: uint(point.Column)}
}

func toCMissingDependencyRanges(ranges []gotreesitter.Range) []sitter.Range {
	out := make([]sitter.Range, len(ranges))
	for index, item := range ranges {
		out[index] = sitter.Range{
			StartByte: uint(item.StartByte), EndByte: uint(item.EndByte),
			StartPoint: toCMissingDependencyPoint(item.StartPoint), EndPoint: toCMissingDependencyPoint(item.EndPoint),
		}
	}
	return out
}
