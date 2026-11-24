//go:build cgo && (treesitter_c_parity || treesitter_c_bench)

package cgoharness

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyPinnedRepoAcceptsCleanExactHead(t *testing.T) {
	repoDir, head := newPinnedTestRepo(t)
	if err := verifyPinnedRepo(repoDir, head); err != nil {
		t.Fatalf("verify clean pinned repository: %v", err)
	}
}

func TestVerifyPinnedRepoAcceptsAttributeFilterFalseDirty(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runPinnedTestGit(t, repoDir, "init")
	runPinnedTestGit(t, repoDir, "config", "user.name", "Gotreesitter Test")
	runPinnedTestGit(t, repoDir, "config", "user.email", "gotreesitter@example.invalid")
	writePinnedTestFile(t, filepath.Join(repoDir, ".gitattributes"), "*.txt text eol=lf\n")
	writePinnedTestFile(t, filepath.Join(repoDir, "crlf.txt"), "line\r\n")
	runPinnedTestGit(t, repoDir, "add", ".gitattributes")
	blob := strings.TrimSpace(runPinnedTestGit(t, repoDir, "hash-object", "-w", "--no-filters", "crlf.txt"))
	runPinnedTestGit(t, repoDir, "update-index", "--add", "--cacheinfo", "100644", blob, "crlf.txt")
	runPinnedTestGit(t, repoDir, "commit", "-m", "inconsistent attributes fixture")
	head := strings.TrimSpace(runPinnedTestGit(t, repoDir, "rev-parse", "HEAD"))

	status := runPinnedTestGit(t, repoDir, "status", "--porcelain=v1")
	if !strings.Contains(status, "crlf.txt") {
		t.Fatalf("fixture status = %q, want Git filter false-dirty path", status)
	}
	if err := verifyPinnedRepo(repoDir, head); err != nil {
		t.Fatalf("verify exact raw bytes despite attribute filter: %v", err)
	}
}

func TestVerifyPinnedRepoRejectsWrongHead(t *testing.T) {
	repoDir, pinnedHead := newPinnedTestRepo(t)
	writePinnedTestFile(t, filepath.Join(repoDir, "tracked.txt"), "second\n")
	runPinnedTestGit(t, repoDir, "add", "tracked.txt")
	runPinnedTestGit(t, repoDir, "commit", "-m", "second")

	err := verifyPinnedRepo(repoDir, pinnedHead)
	if err == nil || !strings.Contains(err.Error(), "HEAD mismatch") {
		t.Fatalf("verify wrong HEAD error=%v, want actionable HEAD mismatch", err)
	}
}

func TestVerifyPinnedRepoRejectsDirtyTrackedFile(t *testing.T) {
	repoDir, head := newPinnedTestRepo(t)
	writePinnedTestFile(t, filepath.Join(repoDir, "tracked.txt"), "dirty\n")

	err := verifyPinnedRepo(repoDir, head)
	if err == nil || !strings.Contains(err.Error(), "working tree is not clean") || !strings.Contains(err.Error(), "tracked.txt") {
		t.Fatalf("verify dirty tracked file error=%v, want path and clean-tree guidance", err)
	}
}

func TestVerifyPinnedRepoRejectsUntrackedFile(t *testing.T) {
	repoDir, head := newPinnedTestRepo(t)
	writePinnedTestFile(t, filepath.Join(repoDir, "untracked.txt"), "untracked\n")

	err := verifyPinnedRepo(repoDir, head)
	if err == nil || !strings.Contains(err.Error(), "working tree is not clean") || !strings.Contains(err.Error(), "?? untracked.txt") {
		t.Fatalf("verify untracked file error=%v, want untracked path and clean-tree guidance", err)
	}
}

func TestClonePinnedRepoFromLocalCacheRejectsDirtySource(t *testing.T) {
	cacheDir, head := newPinnedTestRepo(t)
	writePinnedTestFile(t, filepath.Join(cacheDir, "untracked.txt"), "untracked\n")
	dest := filepath.Join(t.TempDir(), "copy")

	err := clonePinnedRepoFromLocalCache(cacheDir, head, dest)
	if err == nil || !strings.Contains(err.Error(), "cached parity repository") || !strings.Contains(err.Error(), "?? untracked.txt") {
		t.Fatalf("clone dirty cached repository error=%v, want cache rejection and untracked path", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("dirty cache destination stat error=%v, want destination not created", statErr)
	}
}

func TestBuildParityCRefRejectsDirtyLocalRepo(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "fixture")
	head := initPinnedTestRepo(t, repoDir)
	writePinnedTestFile(t, filepath.Join(repoDir, "untracked.txt"), "untracked\n")
	t.Setenv(parityRepoRootEnv, root)

	_, err := buildParityCRef(t.TempDir(), "", parityLockEntry{
		Name:    "fixture",
		RepoURL: "https://example.invalid/tree-sitter-fixture",
		Commit:  head,
		Subdir:  "src",
	})
	if err == nil || !strings.Contains(err.Error(), "local parity repository") || !strings.Contains(err.Error(), "?? untracked.txt") {
		t.Fatalf("build dirty local repository error=%v, want local-source rejection and untracked path", err)
	}
}

func newPinnedTestRepo(t *testing.T) (string, string) {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	return repoDir, initPinnedTestRepo(t, repoDir)
}

func initPinnedTestRepo(t *testing.T, repoDir string) string {
	t.Helper()
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runPinnedTestGit(t, repoDir, "init")
	runPinnedTestGit(t, repoDir, "config", "user.name", "Gotreesitter Test")
	runPinnedTestGit(t, repoDir, "config", "user.email", "gotreesitter@example.invalid")
	writePinnedTestFile(t, filepath.Join(repoDir, "tracked.txt"), "first\n")
	runPinnedTestGit(t, repoDir, "add", "tracked.txt")
	runPinnedTestGit(t, repoDir, "commit", "-m", "first")
	return strings.TrimSpace(runPinnedTestGit(t, repoDir, "rev-parse", "HEAD"))
}

func writePinnedTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runPinnedTestGit(t *testing.T, repoDir string, args ...string) string {
	t.Helper()
	gitArgs := append([]string{"-C", repoDir}, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(gitArgs, " "), err, out)
	}
	return string(out)
}

// TestClonePinnedRepoRemovesPartialCloneAndRetries makes the first git clone
// fail after it leaves files in the destination, as a dropped connection does.
func TestClonePinnedRepoRemovesPartialCloneAndRetries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake git")
	}
	old := parityCloneRetryPause
	parityCloneRetryPause = 0
	t.Cleanup(func() { parityCloneRetryPause = old })

	binDir := t.TempDir()
	count := filepath.Join(binDir, "count")
	script := `#!/bin/sh
dest=""
for a in "$@"; do dest="$a"; done
n=$(cat "` + count + `" 2>/dev/null || echo 0)
n=$((n+1))
echo $n > "` + count + `"
if [ -d "$dest" ] && [ -n "$(ls -A "$dest")" ]; then
  echo "fatal: destination path '$dest' already exists and is not an empty directory." >&2
  exit 128
fi
mkdir -p "$dest"
echo partial > "$dest/partial"
if [ "$n" -eq 1 ]; then
  echo "fatal: early EOF" >&2
  exit 128
fi
rm "$dest/partial"
`
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dest := filepath.Join(t.TempDir(), "repo")
	if err := clonePinnedRepo("https://example.invalid/x.git", "", dest); err != nil {
		t.Fatalf("clone after one failed attempt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "partial")); !os.IsNotExist(err) {
		t.Fatalf("partial file stat error=%v, want the failed clone removed", err)
	}
}
