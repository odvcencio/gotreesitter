package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotRejectsNewRootFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "root_files.txt")
	if err := snapshot(path, []string{"parser.go"}, true); err != nil {
		t.Fatal(err)
	}
	if err := snapshot(path, []string{"parser.go", "new_engine.go"}, false); err == nil || !strings.Contains(err.Error(), "new_engine.go") {
		t.Fatalf("new file escaped allowlist: %v", err)
	}
}

func TestAPISnapshotIncludesTagOnlyExport(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":    "module example.com/layouttest\n\ngo 1.22\n",
		"base.go":   "package layouttest\nfunc Public() {}\n",
		"tagged.go": "//go:build special\n\npackage layouttest\nfunc Tagged() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	base, err := apiSurface(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := apiSurface(root, "special")
	if err != nil {
		t.Fatal(err)
	}
	added, removed := difference(base, tagged)
	if len(added) != 1 || added[0] != "func Tagged()" || len(removed) != 0 {
		t.Fatalf("tagged API delta: added=%v removed=%v", added, removed)
	}
}

func TestMoveInventoryRejectsRenamedTest(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.name", "Layout Test")
	git("config", "user.email", "layout@example.invalid")
	oldPath := filepath.Join(root, "old_test.go")
	old := "package old\nimport \"testing\"\n" + strings.Repeat("// Keep this move recognizable to git.\n", 20) + "func TestPinned(t *testing.T) {}\n"
	if err := os.WriteFile(oldPath, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "old_test.go")
	git("commit", "-qm", "baseline")
	base := git("rev-parse", "HEAD")
	git("mv", "old_test.go", "new_test.go")
	newer := strings.Replace(old, "package old", "package new", 1)
	if err := os.WriteFile(filepath.Join(root, "new_test.go"), []byte(newer), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "new_test.go")
	git("commit", "-qm", "move")
	if moves, err := checkMoveInventory(root, base); err != nil || moves != 1 {
		t.Fatalf("preserved move: moves=%d err=%v", moves, err)
	}
	newer = strings.Replace(newer, "TestPinned", "TestMissing", 1)
	if err := os.WriteFile(filepath.Join(root, "new_test.go"), []byte(newer), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "new_test.go")
	git("commit", "-qm", "rename test")
	if _, err := checkMoveInventory(root, base); err == nil || !strings.Contains(err.Error(), "TestPinned") {
		t.Fatalf("renamed test escaped move inventory: %v", err)
	}
}

func TestAdapterFilesAreTheNamedList(t *testing.T) {
	want := []string{
		"ccore_bind.go", "ccore_scanner.go", "ccore_route.go", "ccore_tree.go", "ccore_incremental.go",
		"ccore_views.go", "ccore_cursor.go", "ccore_query_reader.go", "ccore_certified_gen.go",
	}
	if strings.Join(adapterFiles, " ") != strings.Join(want, " ") {
		t.Fatalf("adapter file list changed: %v", adapterFiles)
	}
	for _, name := range adapterFiles {
		if !strings.HasPrefix(name, "ccore_") || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			t.Errorf("%s is not a ccore_*.go non-test file", name)
		}
	}
}

func TestRootAllowlistGrowthAllowsOnlyNamedAdapters(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.name", "Layout Test")
	git("config", "user.email", "layout@example.invalid")
	if err := os.MkdirAll(filepath.Join(root, "cmd/v1layout"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd/v1layout/root_files.txt"), []byte("parser.go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "baseline")
	base := git("rev-parse", "HEAD")
	if err := checkRootAllowlistGrowth(root, base, []string{"parser.go", "ccore_bind.go", "ccore_scanner.go"}); err != nil {
		t.Fatalf("named adapter files refused: %v", err)
	}
	for _, name := range []string{"ccore_other.go", "ccore_bind_test.go", "new_engine.go"} {
		err := checkRootAllowlistGrowth(root, base, []string{"parser.go", name})
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("%s escaped the allowlist: %v", name, err)
		}
	}
}

// TestAdapterFilesBudgetAndTags: the budget is 648 plus the number of files of
// the named list that exist in the root, and none of them has a //go:build
// line (the tag-atom check only rejects atoms it does not know).
func TestAdapterFilesBudgetAndTags(t *testing.T) {
	root := filepath.Join("..", "..")
	exist := 0
	for _, name := range adapterFiles {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		exist++
		for _, line := range strings.Split(string(raw), "\n") {
			// Both the current and the legacy form of a build constraint.
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//go:build") || strings.HasPrefix(trimmed, "// +build") {
				t.Errorf("%s has a build line: %s", name, line)
			}
		}
	}
	if rootBudget != 648+exist {
		t.Errorf("budget %d, want 648 plus %d existing adapter files = %d", rootBudget, exist, 648+exist)
	}
}
