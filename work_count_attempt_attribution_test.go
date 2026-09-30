//go:build gts_workcount

package gotreesitter_test

import (
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	retryAttributionFixturePath   = "testdata/work_count/retry_go_query_kotlin_regression.go"
	retryAttributionFixtureBytes  = 3435
	retryAttributionFixtureSHA256 = "ada50bc6eef16b831e7ca0c44f11dffbcff7d9695515519cd5090b2aacf4789a"
	straightLRFixturePath         = "testdata/work_count/straight_lr_control.go"
	straightLRFixtureBytes        = 154
	straightLRFixtureSHA256       = "4c584e187fdd4fb62dbf3953ca771e868bd3037e44abe5349c1cc0b3faf6532b"
)

func TestDiagnosticWorkCountAttemptAttributionRetryWitness(t *testing.T) {
	source := loadWorkCountAttributionFixture(t, retryAttributionFixturePath, retryAttributionFixtureBytes, retryAttributionFixtureSHA256)
	baselineDigest := attributionBaselineTreeDigest(t, source)
	counts, tree := parseWithDiagnosticWorkCount(t, source)
	defer tree.Release()

	if len(counts.Attempts) != 2 {
		t.Fatalf("attempts=%d want=2", len(counts.Attempts))
	}
	initial, merge := counts.Attempts[0], counts.Attempts[1]
	if initial.LogicalRung != "initial_full" || initial.OperationCause != "fresh_dfa_full_parse" || initial.StopReason != gotreesitter.ParseStopAccepted || !initial.RootHasError {
		t.Fatalf("initial attempt=%+v", initial)
	}
	if merge.LogicalRung != "initial_merge" || merge.OperationCause != "initial_result_requires_merge_width" || merge.StopReason != gotreesitter.ParseStopAccepted || merge.RootHasError {
		t.Fatalf("merge attempt=%+v", merge)
	}
	if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
		t.Fatalf("returned tree error=%v end=%d want=%d", tree.RootNode().HasError(), tree.RootNode().EndByte(), len(source))
	}
	requireWorkCountAttributionSum(t, counts)
	requireAttributionTreeDigest(t, "retry_go_query_kotlin", tree, baselineDigest)
	logWorkCountConvergenceReceipt(t, "retry_go_query_kotlin", counts)
}

func TestDiagnosticWorkCountAttemptAttributionStraightLRControl(t *testing.T) {
	source := loadWorkCountAttributionFixture(t, straightLRFixturePath, straightLRFixtureBytes, straightLRFixtureSHA256)
	baselineDigest := attributionBaselineTreeDigest(t, source)
	counts, tree := parseWithDiagnosticWorkCount(t, source)
	defer tree.Release()

	if len(counts.Attempts) != 1 {
		t.Fatalf("attempts=%d want=1", len(counts.Attempts))
	}
	attempt := counts.Attempts[0]
	if attempt.LogicalRung != "initial_full" || attempt.OperationCause != "fresh_dfa_full_parse" || attempt.StopReason != gotreesitter.ParseStopAccepted || attempt.RootHasError {
		t.Fatalf("attempt=%+v", attempt)
	}
	if runtime := tree.ParseRuntime(); runtime.MaxStacksSeen != 1 || runtime.MultiStackIterations != 0 {
		t.Fatalf("straight-LR identity: max_stacks=%d multi_stack_iterations=%d", runtime.MaxStacksSeen, runtime.MultiStackIterations)
	}
	requireWorkCountAttributionSum(t, counts)
	requireAttributionTreeDigest(t, "straight_lr_control", tree, baselineDigest)
	logWorkCountConvergenceReceipt(t, "straight_lr_control", counts)
}

func attributionBaselineTreeDigest(t *testing.T, source []byte) string {
	t.Helper()
	lang := grammars.GoLanguage()
	baseline, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer baseline.Release()
	baselineInspection, err := benchfixtures.InspectGoTree(baseline.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	return baselineInspection.SHA256
}

func requireAttributionTreeDigest(t *testing.T, label string, observed *gotreesitter.Tree, baselineDigest string) {
	t.Helper()
	observedInspection, err := benchfixtures.InspectGoTree(observed.RootNode(), grammars.GoLanguage())
	if err != nil {
		t.Fatal(err)
	}
	if observedInspection.SHA256 != baselineDigest {
		t.Fatalf("%s diagnostic digest=%s baseline=%s", label, observedInspection.SHA256, baselineDigest)
	}
	t.Logf("deep_tree_receipt fixture=%s format=%s sha256=%s", label, benchfixtures.DeepTreeDigestVersion, observedInspection.SHA256)
}

func loadWorkCountAttributionFixture(t *testing.T, path string, wantBytes int, wantSHA string) []byte {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != wantBytes {
		t.Fatalf("fixture %s bytes=%d want=%d", path, len(source), wantBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != wantSHA {
		t.Fatalf("fixture %s sha256=%s want=%s", path, got, wantSHA)
	}
	return source
}

func parseWithDiagnosticWorkCount(t *testing.T, source []byte) (gotreesitter.DiagnosticWorkCount, *gotreesitter.Tree) {
	t.Helper()
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	gotreesitter.BeginDiagnosticWorkCount()
	tree, err := parser.Parse(source)
	counts := gotreesitter.EndDiagnosticWorkCount()
	if err != nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatal(err)
	}
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatal("parse returned no tree")
	}
	return counts, tree
}

func requireWorkCountAttributionSum(t *testing.T, counts gotreesitter.DiagnosticWorkCount) {
	t.Helper()
	total := reflect.ValueOf(counts.DiagnosticWorkCountValues)
	sums := reflect.New(total.Type()).Elem()
	add := func(values gotreesitter.DiagnosticWorkCountValues) {
		value := reflect.ValueOf(values)
		for i := 0; i < value.NumField(); i++ {
			current := sums.Field(i).Uint()
			delta := value.Field(i).Uint()
			if ^uint64(0)-current < delta {
				t.Fatalf("counter %s sum overflow", value.Type().Field(i).Name)
			}
			sums.Field(i).SetUint(current + delta)
		}
	}
	for _, attempt := range counts.Attempts {
		add(attempt.Counters)
		var phases gotreesitter.DiagnosticWorkCountValues
		phaseValue := reflect.ValueOf(&phases).Elem()
		for _, phase := range []gotreesitter.DiagnosticWorkCountValues{attempt.EntryToCaps, attempt.CapsToFinalize, attempt.Finalize} {
			value := reflect.ValueOf(phase)
			for i := 0; i < value.NumField(); i++ {
				phaseValue.Field(i).SetUint(phaseValue.Field(i).Uint() + value.Field(i).Uint())
			}
		}
		if !reflect.DeepEqual(phases, attempt.Counters) {
			t.Fatalf("attempt %d phase sum does not match counters", attempt.Index)
		}
	}
	add(counts.OutsideAttempt)
	if !reflect.DeepEqual(sums.Interface(), counts.DiagnosticWorkCountValues) {
		t.Fatalf("aggregate counters do not equal attempts plus outside-attempt residual")
	}
}

func TestDiagnosticRetryBudgetCliffCounters(t *testing.T) {
	archive, err := os.Open("internal/benchfixtures/testdata/cliffs/go_parser.go.gz")
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
	var before gotreesitter.DiagnosticWorkCount
	var digest string
	for _, budget := range []bool{false, true} {
		lang := *grammars.GoLanguage()
		lang.FullParseRetryWorkBudgetEnabled = budget
		parser := gotreesitter.NewParser(&lang)
		gotreesitter.BeginDiagnosticWorkCount()
		tree, err := parser.Parse(source)
		counts := gotreesitter.EndDiagnosticWorkCount()
		if err != nil {
			t.Fatal(err)
		}
		inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), &lang)
		tree.Release()
		if err != nil {
			t.Fatal(err)
		}
		requireWorkCountAttributionSum(t, counts)
		if !budget {
			before, digest = counts, inspection.SHA256
		} else {
			if inspection.SHA256 != digest {
				t.Fatal("budget changed the selected tree")
			}
			if len(counts.Attempts) != 2 || len(before.Attempts) != 6 {
				t.Fatalf("attempts=%d->%d, want 6->2", len(before.Attempts), len(counts.Attempts))
			}
			if counts.Shifts >= before.Shifts || counts.Reductions >= before.Reductions || counts.LexerFrontDoorCallsProxy >= before.LexerFrontDoorCallsProxy {
				t.Fatal("retry work did not decrease")
			}
		}
		t.Logf("budget=%t attempts=%d shifts=%d reductions=%d lexer_calls=%d parents=%d leaves=%d digest=%s", budget, len(counts.Attempts), counts.Shifts, counts.Reductions, counts.LexerFrontDoorCallsProxy, counts.ParentConstructionsProxy, counts.LeafConstructionsProxy, inspection.SHA256)
	}
}
