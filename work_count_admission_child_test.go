package gotreesitter_test

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestWorkCountAdmissionChild is the ordinary, untagged admission endpoint.
// The outer harness compiles it from the same private source snapshot as the
// tagged diagnostic child, then starts one fresh process for one public parse.
func TestWorkCountAdmissionChild(t *testing.T) {
	if os.Getenv(workCountSourcePathEnv) == "" || os.Getenv(workCountResultPathEnv) == "" {
		t.Skip("diagnostic work-count admission child is not configured")
	}
	input, err := loadWorkCountChildInput()
	if err != nil {
		t.Fatal(err)
	}
	parser := gotreesitter.NewParser(input.lang)
	parser.SetAdmissionCandidateRoute(false)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	tree, parseErr := parser.Parse(input.source)
	if parseErr != nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatalf("parse: %v", parseErr)
	}
	defer tree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
		t.Fatalf("production admission attempted compact parsing: routed=%d->%d fallback=%d->%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}
	result, err := authenticateWorkCountTree(input, tree)
	if err != nil {
		t.Fatal(err)
	}
	result.Engine = workCountAdmissionEngine
	if err := writeWorkCountChildResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestWorkCountAdmissionChildUsesProductionRoute(t *testing.T) {
	runWorkCountProductionRouteRegression(t, TestWorkCountAdmissionChild, workCountGoChildSchema, workCountAdmissionEngine)
}
