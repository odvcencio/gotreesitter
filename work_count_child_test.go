//go:build gts_workcount

package gotreesitter_test

import (
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

type workCountTaggedChildResult struct {
	workCountGoChildResult
	Counters    gotreesitter.DiagnosticWorkCount            `json:"counters"`
	BoardDirect gotreesitter.DiagnosticWorkCountBoardDirect `json:"board_direct"`
}

// TestDiagnosticWorkCountChild is an executable protocol endpoint, not a
// normal unit test. The authenticated parent harness compiles this test into a
// separate gts_workcount artifact and starts one fresh process for one parse.
func TestDiagnosticWorkCountChild(t *testing.T) {
	if os.Getenv(workCountSourcePathEnv) == "" || os.Getenv(workCountResultPathEnv) == "" {
		t.Skip("diagnostic work-count child protocol is not configured")
	}
	input, err := loadWorkCountChildInput()
	if err != nil {
		t.Fatal(err)
	}

	parser := gotreesitter.NewParser(input.lang)
	parser.SetAdmissionCandidateRoute(false)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	gotreesitter.BeginDiagnosticWorkCount()
	tree, parseErr := parser.Parse(input.source)
	counts := gotreesitter.EndDiagnosticWorkCount()
	if parseErr != nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatalf("parse: %v", parseErr)
	}
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatal("parse returned no root")
	}
	defer tree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
		t.Fatalf("production diagnostic attempted compact parsing: routed=%d->%d fallback=%d->%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}

	base, err := authenticateWorkCountTree(input, tree)
	if err != nil {
		t.Fatal(err)
	}
	if err := countSelectedGoNodes(tree.RootNode(), &counts); err != nil {
		t.Fatalf("selected tree census: %v", err)
	}
	base.Engine = workCountTaggedEngine
	base.Schema = workCountTaggedGoChildSchema
	result := workCountTaggedChildResult{workCountGoChildResult: base, Counters: counts, BoardDirect: counts.BoardDirect()}
	if err := writeWorkCountChildResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestDiagnosticWorkCountChildUsesProductionRoute(t *testing.T) {
	runWorkCountProductionRouteRegression(t, TestDiagnosticWorkCountChild, workCountTaggedGoChildSchema, workCountTaggedEngine)
}

func countSelectedGoNodes(root *gotreesitter.Node, counts *gotreesitter.DiagnosticWorkCount) error {
	if root == nil {
		return fmt.Errorf("nil root")
	}
	if counts == nil {
		return fmt.Errorf("nil counters")
	}
	stack := []*gotreesitter.Node{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			return fmt.Errorf("nil child")
		}
		childCount := node.ChildCount()
		counts.AddDiagnosticSelectedNode(childCount != 0)
		if childCount == 0 {
			continue
		}
		for i := childCount - 1; i >= 0; i-- {
			stack = append(stack, node.Child(i))
		}
	}
	return nil
}
