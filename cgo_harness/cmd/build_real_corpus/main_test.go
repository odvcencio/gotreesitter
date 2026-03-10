package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectFilesByBucketFillsToTarget(t *testing.T) {
	candidates := []corpusFile{
		{RelPath: "a.txt", Size: 400},
		{RelPath: "b.txt", Size: 800},
		{RelPath: "c.txt", Size: 2500},
		{RelPath: "d.txt", Size: 4200},
	}

	selected := selectFilesByBucket(candidates, 1, 256, 2000, 16000)
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected files, got %d", len(selected))
	}

	seen := map[string]struct{}{}
	for _, sf := range selected {
		if _, ok := seen[sf.RelPath]; ok {
			t.Fatalf("duplicate selected path: %s", sf.RelPath)
		}
		seen[sf.RelPath] = struct{}{}
		if sf.Bucket == "" {
			t.Fatalf("empty bucket for %s", sf.RelPath)
		}
	}
}

func TestSelectFilesByBucketKeepsSmallMediumLargeWhenAvailable(t *testing.T) {
	candidates := []corpusFile{
		{RelPath: "small.go", Size: 512},
		{RelPath: "medium.go", Size: 4096},
		{RelPath: "large.go", Size: 65536},
	}

	selected := selectFilesByBucket(candidates, 1, 256, 2000, 16000)
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected files, got %d", len(selected))
	}

	buckets := map[string]bool{}
	for _, sf := range selected {
		buckets[sf.Bucket] = true
	}
	for _, bucket := range []string{"small", "medium", "large"} {
		if !buckets[bucket] {
			t.Fatalf("missing bucket %q in selection: %#v", bucket, selected)
		}
	}
}

func TestCollectCandidatesWithoutExtsSkipsLockfiles(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "Cargo.lock"), 512)
	mustWriteSizedText(t, filepath.Join(tmp, "go.sum"), 512)
	mustWriteSizedText(t, filepath.Join(tmp, "package-lock.json"), 512)
	mustWriteSizedText(t, filepath.Join(tmp, "test", "corpus", "valid.chatito"), 512)

	candidates, err := collectCandidates(tmp, nil, defaultMaxBytes)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected candidates, got none")
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if seen["Cargo.lock"] || seen["go.sum"] || seen["package-lock.json"] {
		t.Fatalf("lockfiles must be excluded from candidates: %#v", candidates)
	}
	if !seen["test/corpus/valid.chatito"] {
		t.Fatalf("expected corpus candidate missing: %#v", candidates)
	}
}

func TestCollectCandidatesWithoutExtsRequiresCorpusLikePaths(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "src", "program.scala"), 600)
	mustWriteSizedText(t, filepath.Join(tmp, "examples", "hello.chatito"), 600)
	mustWriteSizedText(t, filepath.Join(tmp, ".github", "workflow.yml"), 600)

	candidates, err := collectCandidates(tmp, nil, defaultMaxBytes)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if seen[".github/workflow.yml"] {
		t.Fatalf("metadata/config files should be excluded: %#v", candidates)
	}
	if seen["src/program.scala"] {
		t.Fatalf("non-corpus source files should be excluded without explicit ext hints: %#v", candidates)
	}
	if !seen["examples/hello.chatito"] {
		t.Fatalf("expected corpus-like path missing: %#v", candidates)
	}
}

func TestCollectCandidatesWithExtsKeepsCorpusTextFixtures(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "corpus", "declarations.txt"), 1200)
	mustWriteSizedText(t, filepath.Join(tmp, "examples", "demo.swift"), 1200)
	mustWriteSizedText(t, filepath.Join(tmp, "examples", "README.txt"), 1200)

	candidates, err := collectCandidates(tmp, []string{".swift"}, defaultMaxBytes)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["corpus/declarations.txt"] {
		t.Fatalf("expected corpus text fixture to be retained: %#v", candidates)
	}
	if !seen["examples/demo.swift"] {
		t.Fatalf("expected example source file to be retained: %#v", candidates)
	}
	if seen["examples/README.txt"] {
		t.Fatalf("example docs with mismatched ext should be excluded: %#v", candidates)
	}
}

func TestRetryableGitCheckoutError(t *testing.T) {
	tests := map[string]bool{
		"fatal: unable to access 'https://github.com/x/y/': Could not resolve host: github.com": true,
		"fatal: unable to access 'https://github.com/x/y/': TLS handshake timeout":              true,
		"fatal: unable to access 'https://github.com/x/y/': Connection reset by peer":           true,
		"fatal: repository not found": false,
	}

	for msg, want := range tests {
		if got := retryableGitCheckoutError(testError(msg)); got != want {
			t.Fatalf("retryableGitCheckoutError(%q) = %v, want %v", msg, got, want)
		}
	}
}

func mustWriteSizedText(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = 'a'
	}
	buf[size-1] = '\n'
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type testError string

func (e testError) Error() string { return string(e) }
