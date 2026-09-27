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
