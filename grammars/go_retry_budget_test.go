package grammars_test

import (
	"compress/gzip"
	"io"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestGoRetryBudgetPreservesSelectedTree(t *testing.T) {
	archive, err := os.Open("../internal/benchfixtures/testdata/cliffs/go_parser.go.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	source, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	checkGoRetryBudgetTree(t, source, 5)
}

func TestGoRetryBudgetShrunkWitness(t *testing.T) {
	// Reduced from the pinned Go parser cliff. The old ladder runs five
	// retries even though none fixes this malformed top-level statement.
	source := []byte("\tif t := ast.Unparen(x); t != x {\n\t}\n\tif _, isBad := x.(*ast.BadExpr); !isBad {\n\t\tp.error(p.safePos(x.End()), fmt.Sprintf(\"expression in %s must be function call\", callType))\n\t}\n}")
	checkGoRetryBudgetTree(t, source, 5)
}

func checkGoRetryBudgetTree(t *testing.T, source []byte, baselineRetries uint64) {
	t.Helper()
	gts.EnableRecoveryRuntimeTelemetry(true)
	defer gts.EnableRecoveryRuntimeTelemetry(false)
	var digest string
	var before uint64
	for _, enabled := range []bool{false, true} {
		lang := *grammars.GoLanguage()
		lang.FullParseRetryWorkBudgetEnabled = enabled
		parser := gts.NewParser(&lang)
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), &lang)
		if err != nil {
			tree.Release()
			t.Fatal(err)
		}
		passes := parser.DebugRecoveryRuntimeStats().RetryPassCount
		if !enabled {
			digest, before = inspection.SHA256, passes
			if passes != baselineRetries {
				t.Errorf("baseline retries=%d, want %d", passes, baselineRetries)
			}
		} else {
			if inspection.SHA256 != digest {
				t.Error("work budget changed the selected tree")
			}
			if passes >= before || passes > 2 {
				t.Errorf("budget retries=%d, baseline=%d; want at most two retries", passes, before)
			}
			if tree.RootNode().IsError() && !tree.RootNode().HasError() {
				t.Error("ERROR root lacks HasError")
			}
			if tree.RootNode().EndByte() != uint32(len(source)) && tree.ParseRuntime().StopReason == gts.ParseStopAccepted {
				t.Error("accepted root omitted input")
			}
		}
		t.Logf("budget=%t bytes=%d retries=%d digest=%s", enabled, len(source), passes, inspection.SHA256)
		tree.Release()
	}
}

func TestGoRetryBudgetPreservesCleanMergeRetry(t *testing.T) {
	source, err := os.ReadFile("../testdata/work_count/retry_go_query_kotlin_regression.go")
	if err != nil {
		t.Fatal(err)
	}
	parser := gts.NewParser(grammars.GoLanguage())
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
		t.Fatalf("required merge retry lost: %s", tree.ParseRuntime().Summary())
	}
}
