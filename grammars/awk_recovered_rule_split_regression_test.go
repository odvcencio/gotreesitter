//go:build !grammar_subset

package grammars

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	awkRecoveredRuleSplitSourceBytes  = 7392
	awkRecoveredRuleSplitSourceSHA256 = "f3dd8c811b2ad06c865fb1ad59ac0098fef57bdbb89377ac96ddb4e845f6bfba"
	awkRecoveredRuleSplitTreeSHA256   = "a391657d3102410eaf6433dac5977913741a6fef0a00275b7f34973a0a1951cc"
)

// The fixture is testdir/T.gawk from onetrueawk/awk at commit
// 61a7c75e225e3035390be32d635545e40d8c5faf.
func TestAWKRecoveredRuleSplitLockedProductionCompletes(t *testing.T) {
	source := awkRecoveredRuleSplitFixture(t)
	parser := gotreesitter.NewParser(AwkLanguage())
	parser.SetAdmissionCandidateRoute(false)

	type parseResult struct {
		tree *gotreesitter.Tree
		err  error
	}
	done := make(chan parseResult, 1)
	start := time.Now()
	go func() {
		tree, err := parser.Parse(source)
		done <- parseResult{tree: tree, err: err}
	}()

	var result parseResult
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("locked AWK production parse did not complete in 5 seconds")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.tree == nil || result.tree.RootNode() == nil {
		t.Fatal("locked AWK production parse returned no tree")
	}
	t.Cleanup(result.tree.Release)

	root := result.tree.RootNode()
	if got, want := root.StartByte(), uint32(0); got != want {
		t.Fatalf("root start = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	inspection, err := benchfixtures.InspectGoTree(root, AwkLanguage())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != awkRecoveredRuleSplitTreeSHA256 {
		t.Fatalf(
			"tree digest = %s, want %s; parse elapsed = %s",
			inspection.SHA256,
			awkRecoveredRuleSplitTreeSHA256,
			time.Since(start),
		)
	}
}

func awkRecoveredRuleSplitFixture(t *testing.T) []byte {
	t.Helper()
	encoded, err := os.ReadFile(
		filepath.Join("..", "testdata", "awk_recovered_rule_split", "T.gawk.b64"),
	)
	if err != nil {
		t.Fatal(err)
	}
	source, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != awkRecoveredRuleSplitSourceBytes {
		t.Fatalf("fixture bytes = %d, want %d", len(source), awkRecoveredRuleSplitSourceBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != awkRecoveredRuleSplitSourceSHA256 {
		t.Fatalf("fixture SHA-256 = %s, want %s", got, awkRecoveredRuleSplitSourceSHA256)
	}
	return source
}
