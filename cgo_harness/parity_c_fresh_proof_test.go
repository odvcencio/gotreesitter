//go:build cgo && (treesitter_c_parity || treesitter_c_bench)

package cgoharness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func freshProofRepo(t *testing.T, parser string) (string, parityLockEntry) {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	runPinnedTestGit(t, repo, "init")
	runPinnedTestGit(t, repo, "config", "user.name", "Test")
	runPinnedTestGit(t, repo, "config", "user.email", "test@example.invalid")
	if parser != "" {
		writePinnedTestFile(t, filepath.Join(repo, "src", "parser.c"), parser)
	}
	writePinnedTestFile(t, filepath.Join(repo, "tracked.txt"), "tracked\n")
	runPinnedTestGit(t, repo, "add", ".")
	runPinnedTestGit(t, repo, "commit", "-m", "fixture")
	commit := strings.TrimSpace(runPinnedTestGit(t, repo, "rev-parse", "HEAD"))
	return repo, parityLockEntry{Name: "example", RepoURL: "https://example.invalid/grammar", Commit: commit, Subdir: "src"}
}

func TestW5FreshProofColdSources(t *testing.T) {
	repo, entry := freshProofRepo(t, "#define LANGUAGE_VERSION 15\nint parser(void) { return 1; }\n")
	candidate := freshParserCandidate(repo, entry)
	if candidate == nil {
		t.Fatal("committed compatible parser was not qualified")
	}
	build := t.TempDir()
	artifact := filepath.Join(build, "parser.so")
	var capture paritySourceCapture
	if err := compileParserShared(entry, repo, artifact, build, &capture); err != nil {
		t.Fatal(err)
	}
	proof := finishFreshParserProof(repo, entry, artifact, candidate, capture)
	if proof == nil || proof.ActualParser != "src/parser.c" || proof.Sources[proof.ActualParser] != proof.ParserSHA256 || proof.ArtifactSHA256 == "" {
		t.Fatalf("invalid cold proof: %#v", proof)
	}
	wrong := capture
	wrong.parserPath = filepath.Join(repo, "other", "parser.c")
	if finishFreshParserProof(repo, entry, artifact, candidate, wrong) != nil {
		t.Fatal("accepted alternate resolved parser")
	}
	wrong = capture
	wrong.regenerated = true
	if finishFreshParserProof(repo, entry, artifact, candidate, wrong) != nil {
		t.Fatal("accepted regeneration path")
	}
	writePinnedTestFile(t, filepath.Join(repo, "src", "parser.c"), "changed\n")
	if finishFreshParserProof(repo, entry, artifact, candidate, capture) != nil {
		t.Fatal("accepted modified parser")
	}
}

func TestW5FreshProofRejectsUnqualifiedSources(t *testing.T) {
	for _, parser := range []string{"", "int parser(void) { return 1; }\n", "#define LANGUAGE_VERSION 99\n"} {
		repo, entry := freshProofRepo(t, parser)
		if freshParserCandidate(repo, entry) != nil {
			t.Fatal("accepted missing or incompatible parser")
		}
	}
	repo, entry := freshProofRepo(t, "#define LANGUAGE_VERSION 15\n")
	entry.Name = "make" // A configured patch always disqualifies this proof.
	if freshParserCandidate(repo, entry) != nil {
		t.Fatal("accepted patched grammar")
	}
}

func TestW5FreshProofCacheHitAbsent(t *testing.T) {
	parityCRefState.mu.Lock()
	old := parityCRefState.refs
	parityCRefState.refs = map[string]*parityCRef{"cached": {}}
	parityCRefState.mu.Unlock()
	t.Cleanup(func() { parityCRefState.mu.Lock(); parityCRefState.refs = old; parityCRefState.mu.Unlock() })
	if COracleFreshProof("cached") != nil {
		t.Fatal("cache hit synthesized a fresh proof")
	}
}
