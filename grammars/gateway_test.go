package grammars

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DefaultPolicy
// ---------------------------------------------------------------------------

func TestDefaultPolicyValues(t *testing.T) {
	p := DefaultPolicy()

	if p.LargeFileThreshold != 256*1024 {
		t.Errorf("LargeFileThreshold = %d, want %d", p.LargeFileThreshold, 256*1024)
	}
	if p.MaxConcurrent != runtime.GOMAXPROCS(0) {
		t.Errorf("MaxConcurrent = %d, want %d", p.MaxConcurrent, runtime.GOMAXPROCS(0))
	}
	if p.ChannelBuffer != p.MaxConcurrent+1 {
		t.Errorf("ChannelBuffer = %d, want %d", p.ChannelBuffer, p.MaxConcurrent+1)
	}

	wantDirs := map[string]bool{
		".git": true, ".graft": true, ".hg": true,
		".svn": true, "vendor": true, "node_modules": true,
	}
	for _, d := range p.SkipDirs {
		if !wantDirs[d] {
			t.Errorf("unexpected SkipDir: %s", d)
		}
		delete(wantDirs, d)
	}
	for d := range wantDirs {
		t.Errorf("missing SkipDir: %s", d)
	}

	wantExts := map[string]bool{
		".min.js": true, ".min.css": true, ".map": true, ".wasm": true,
	}
	for _, e := range p.SkipExtensions {
		if !wantExts[e] {
			t.Errorf("unexpected SkipExtension: %s", e)
		}
		delete(wantExts, e)
	}
	for e := range wantExts {
		t.Errorf("missing SkipExtension: %s", e)
	}
}

func TestDefaultPolicyEnvThreshold(t *testing.T) {
	t.Setenv("GTS_LARGE_FILE_THRESHOLD", "1024")
	// Force re-read by calling DefaultPolicy (it reads env each call).
	p := DefaultPolicy()
	if p.LargeFileThreshold != 1024 {
		t.Errorf("LargeFileThreshold = %d, want 1024", p.LargeFileThreshold)
	}
}

func TestDefaultPolicyEnvMaxConcurrent(t *testing.T) {
	t.Setenv("GTS_MAX_CONCURRENT", "3")
	p := DefaultPolicy()
	if p.MaxConcurrent != 3 {
		t.Errorf("MaxConcurrent = %d, want 3", p.MaxConcurrent)
	}
	if p.ChannelBuffer != 4 {
		t.Errorf("ChannelBuffer = %d, want 4", p.ChannelBuffer)
	}
}

func TestDefaultPolicyEnvInvalid(t *testing.T) {
	t.Setenv("GTS_LARGE_FILE_THRESHOLD", "not-a-number")
	t.Setenv("GTS_MAX_CONCURRENT", "bad")
	p := DefaultPolicy()
	// Should fall back to defaults.
	if p.LargeFileThreshold != 256*1024 {
		t.Errorf("LargeFileThreshold = %d, want default", p.LargeFileThreshold)
	}
	if p.MaxConcurrent != runtime.GOMAXPROCS(0) {
		t.Errorf("MaxConcurrent = %d, want default", p.MaxConcurrent)
	}
}

// ---------------------------------------------------------------------------
// ParsedFile.Close
// ---------------------------------------------------------------------------

func TestParsedFileCloseNilTree(t *testing.T) {
	pf := &ParsedFile{
		Path:   "test.go",
		Source: []byte("package main"),
	}
	// Should not panic.
	pf.Close()
	if pf.Source != nil {
		t.Error("Source should be nil after Close")
	}
}

func TestParsedFileCloseDoubleClose(t *testing.T) {
	src := []byte("package main\n")
	tree, err := ParseFilePooled("test.go", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	pf := &ParsedFile{
		Path:   "test.go",
		Tree:   tree,
		Source: src,
	}
	pf.Close()
	// Second close should not panic.
	pf.Close()

	if pf.Tree != nil {
		t.Error("Tree should be nil after Close")
	}
	if pf.Source != nil {
		t.Error("Source should be nil after Close")
	}
}

func TestParsedFileCloseNilReceiver(t *testing.T) {
	var pf *ParsedFile
	// Should not panic.
	pf.Close()
}

// ---------------------------------------------------------------------------
// WalkAndParse — Go files
// ---------------------------------------------------------------------------

func TestWalkAndParseGoFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(dir, "lib.go"), "package main\n\nfunc helper() int { return 42 }\n")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 2
	policy.ChannelBuffer = 3

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var results []ParsedFile
	for pf := range ch {
		if pf.Err != nil {
			t.Errorf("unexpected error for %s: %v", pf.Path, pf.Err)
		}
		results = append(results, pf)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	for i := range results {
		pf := &results[i]
		if pf.Tree == nil {
			t.Errorf("result %d (%s): Tree is nil", i, pf.Path)
			continue
		}
		root := pf.Tree.RootNode()
		if root == nil {
			t.Errorf("result %d (%s): RootNode is nil", i, pf.Path)
			continue
		}
		if got := pf.Tree.NodeType(root); got != "source_file" {
			t.Errorf("result %d (%s): root type = %q, want source_file", i, pf.Path, got)
		}
		if pf.Lang == nil {
			t.Errorf("result %d (%s): Lang is nil", i, pf.Path)
		}
		if pf.Lang != nil && pf.Lang.Name != "go" {
			t.Errorf("result %d (%s): Lang.Name = %q, want go", i, pf.Path, pf.Lang.Name)
		}
		if len(pf.Source) == 0 {
			t.Errorf("result %d (%s): Source is empty", i, pf.Path)
		}
		pf.Close()
	}

	stats := statsFn()
	if stats.FilesFound != 2 {
		t.Errorf("FilesFound = %d, want 2", stats.FilesFound)
	}
	if stats.FilesParsed != 2 {
		t.Errorf("FilesParsed = %d, want 2", stats.FilesParsed)
	}
	if stats.FilesFailed != 0 {
		t.Errorf("FilesFailed = %d, want 0", stats.FilesFailed)
	}
	if stats.BytesParsed == 0 {
		t.Error("BytesParsed should be > 0")
	}
}

// ---------------------------------------------------------------------------
// SkipDirs
// ---------------------------------------------------------------------------

func TestWalkAndParseSkipDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "root.go"), "package main\n")

	// These should be skipped.
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0o755)
	writeFile(t, filepath.Join(gitDir, "config.go"), "package git\n")

	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0o755)
	writeFile(t, filepath.Join(vendorDir, "dep.go"), "package dep\n")

	nodeDir := filepath.Join(dir, "node_modules")
	os.MkdirAll(nodeDir, 0o755)
	writeFile(t, filepath.Join(nodeDir, "index.js"), "module.exports = {};\n")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var paths []string
	for pf := range ch {
		paths = append(paths, pf.Path)
		pf.Close()
	}

	if len(paths) != 1 {
		t.Fatalf("got %d files, want 1 (only root.go); paths: %v", len(paths), paths)
	}
	if filepath.Base(paths[0]) != "root.go" {
		t.Errorf("expected root.go, got %s", paths[0])
	}

	stats := statsFn()
	if stats.FilesFound != 1 {
		t.Errorf("FilesFound = %d, want 1", stats.FilesFound)
	}
}

func TestWalkAndParseShouldSkipDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "root.go"), "package main\n")

	skipDir := filepath.Join(dir, "skip", "nested")
	if err := os.MkdirAll(skipDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(skipDir, "ignored.go"), "package ignored\n")

	keepDir := filepath.Join(dir, "keep")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(keepDir, "kept.go"), "package keep\n")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	policy.ShouldSkipDir = func(path string) bool {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return false
		}
		return filepath.ToSlash(rel) == "skip"
	}
	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var bases []string
	for pf := range ch {
		bases = append(bases, filepath.Base(pf.Path))
		pf.Close()
	}
	stats := statsFn()

	if stats.FilesFound != 2 {
		t.Fatalf("FilesFound = %d, want 2; files: %v", stats.FilesFound, bases)
	}
	if !slices.Contains(bases, "root.go") || !slices.Contains(bases, "kept.go") {
		t.Fatalf("expected root.go and kept.go, got %v", bases)
	}
	if slices.Contains(bases, "ignored.go") {
		t.Fatalf("expected ignored.go to be pruned, got %v", bases)
	}
}

// ---------------------------------------------------------------------------
// SkipExtensions
// ---------------------------------------------------------------------------

func TestWalkAndParseSkipExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.js"), "var x = 1;\n")
	writeFile(t, filepath.Join(dir, "app.min.js"), "var x=1;\n")
	writeFile(t, filepath.Join(dir, "style.min.css"), "body{}\n")
	writeFile(t, filepath.Join(dir, "data.wasm"), "\x00\x61\x73\x6d")
	writeFile(t, filepath.Join(dir, "source.map"), "{}")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var paths []string
	for pf := range ch {
		paths = append(paths, filepath.Base(pf.Path))
		pf.Close()
	}

	if len(paths) != 1 {
		t.Fatalf("got %d files, want 1; paths: %v", len(paths), paths)
	}
	if paths[0] != "app.js" {
		t.Errorf("expected app.js, got %s", paths[0])
	}

	stats := statsFn()
	if stats.FilesFiltered < 1 {
		t.Errorf("FilesFiltered = %d, want >= 1", stats.FilesFiltered)
	}
}

// ---------------------------------------------------------------------------
// ShouldParse hook
// ---------------------------------------------------------------------------

func TestWalkAndParseShouldParse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "include.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "exclude.go"), "package main\n")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	policy.ShouldParse = func(path string, size int64, modTime time.Time) bool {
		return filepath.Base(path) == "include.go"
	}

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var paths []string
	for pf := range ch {
		paths = append(paths, filepath.Base(pf.Path))
		pf.Close()
	}

	if len(paths) != 1 {
		t.Fatalf("got %d files, want 1; paths: %v", len(paths), paths)
	}
	if paths[0] != "include.go" {
		t.Errorf("expected include.go, got %s", paths[0])
	}

	stats := statsFn()
	if stats.FilesFiltered != 1 {
		t.Errorf("FilesFiltered = %d, want 1", stats.FilesFiltered)
	}
}

// ---------------------------------------------------------------------------
// Empty directory
// ---------------------------------------------------------------------------

func TestWalkAndParseEmptyDir(t *testing.T) {
	dir := t.TempDir()

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	count := 0
	for range ch {
		count++
	}

	if count != 0 {
		t.Errorf("got %d results from empty dir, want 0", count)
	}

	stats := statsFn()
	if stats.FilesFound != 0 {
		t.Errorf("FilesFound = %d, want 0", stats.FilesFound)
	}
	if stats.FilesParsed != 0 {
		t.Errorf("FilesParsed = %d, want 0", stats.FilesParsed)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestWalkAndParseCancellation(t *testing.T) {
	const totalFiles = 50
	dir := t.TempDir()
	for i := 0; i < totalFiles; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("file%03d.go", i)),
			"package main\n")
	}

	ctx, cancel := context.WithCancel(context.Background())

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	ch, statsFn := WalkAndParse(ctx, dir, policy)

	// Read 3 results then cancel.
	received := 0
	for range 3 {
		pf, ok := <-ch
		if !ok {
			break
		}
		pf.Close()
		received++
	}
	cancel()

	// Drain remaining — channel must close eventually.
	for pf := range ch {
		pf.Close()
		received++
	}

	stats := statsFn()
	processed := stats.FilesParsed + stats.FilesFailed
	if processed >= totalFiles {
		t.Errorf("expected fewer than %d processed after cancel, got %d", totalFiles, processed)
	}
	t.Logf("cancel after 3: received=%d, parsed=%d, failed=%d, found=%d",
		received, stats.FilesParsed, stats.FilesFailed, stats.FilesFound)
}

// ---------------------------------------------------------------------------
// Large file handling
// ---------------------------------------------------------------------------

func TestWalkAndParseLargeFile(t *testing.T) {
	dir := t.TempDir()

	// Small file.
	writeFile(t, filepath.Join(dir, "small.go"), "package main\n")

	// "Large" file — we set a tiny threshold to trigger the large-file path.
	large := "package main\n\nfunc big() {}\n"
	writeFile(t, filepath.Join(dir, "big.go"), large)

	policy := DefaultPolicy()
	policy.LargeFileThreshold = 10 // anything > 10 bytes is "large"
	policy.MaxConcurrent = 2
	policy.ChannelBuffer = 3

	var mu sync.Mutex
	var largeFileSeen bool
	policy.OnProgress = func(ev ProgressEvent) {
		if ev.Phase == "large_file" {
			mu.Lock()
			largeFileSeen = true
			mu.Unlock()
		}
	}

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	for pf := range ch {
		if pf.Err != nil {
			t.Errorf("error for %s: %v", pf.Path, pf.Err)
		}
		pf.Close()
	}

	stats := statsFn()
	if stats.FilesParsed != 2 {
		t.Errorf("FilesParsed = %d, want 2", stats.FilesParsed)
	}
	if stats.LargeFiles < 1 {
		t.Errorf("LargeFiles = %d, want >= 1", stats.LargeFiles)
	}
	mu.Lock()
	seenLargeFile := largeFileSeen
	mu.Unlock()
	if !seenLargeFile {
		t.Error("OnProgress never received large_file event")
	}
}

// ---------------------------------------------------------------------------
// Progress callback
// ---------------------------------------------------------------------------

func TestWalkAndParseProgress(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2

	var mu sync.Mutex
	phases := map[string]int{}
	policy.OnProgress = func(ev ProgressEvent) {
		mu.Lock()
		phases[ev.Phase]++
		mu.Unlock()
	}

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)
	for pf := range ch {
		pf.Close()
	}
	_ = statsFn()

	mu.Lock()
	phaseCounts := make(map[string]int, len(phases))
	for phase, count := range phases {
		phaseCounts[phase] = count
	}
	mu.Unlock()

	for _, required := range []string{"walking", "parsing", "walk_complete", "done"} {
		if phaseCounts[required] == 0 {
			t.Errorf("missing progress phase: %s", required)
		}
	}
}

// ---------------------------------------------------------------------------
// Large file throttled — verify stats and progress
// ---------------------------------------------------------------------------

func TestWalkAndParse_LargeFileThrottled(t *testing.T) {
	dir := t.TempDir()

	// Below threshold.
	writeFile(t, filepath.Join(dir, "tiny.go"), "package a\n")

	// Above threshold — two large files to verify count.
	padding := strings.Repeat("x", 200)
	writeFile(t, filepath.Join(dir, "huge1.go"), "package main\n\nfunc f1() { /* "+padding+" */ }\n")
	writeFile(t, filepath.Join(dir, "huge2.go"), "package main\n\nfunc f2() { /* "+padding+" */ }\n")

	policy := DefaultPolicy()
	policy.LargeFileThreshold = 50 // both huge files exceed this
	policy.MaxConcurrent = 2
	policy.ChannelBuffer = 3

	var mu sync.Mutex
	var largeFilePaths []string
	policy.OnProgress = func(ev ProgressEvent) {
		if ev.Phase == "large_file" {
			mu.Lock()
			largeFilePaths = append(largeFilePaths, ev.Path)
			mu.Unlock()
		}
	}

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)
	for pf := range ch {
		if pf.Err != nil {
			t.Errorf("error for %s: %v", pf.Path, pf.Err)
		}
		pf.Close()
	}

	stats := statsFn()
	if stats.LargeFiles != 2 {
		t.Errorf("LargeFiles = %d, want 2", stats.LargeFiles)
	}
	if len(largeFilePaths) != 2 {
		t.Errorf("large_file progress events = %d, want 2", len(largeFilePaths))
	}
	if stats.FilesParsed != 3 {
		t.Errorf("FilesParsed = %d, want 3", stats.FilesParsed)
	}
}

// ---------------------------------------------------------------------------
// Progress phases — verify ordering
// ---------------------------------------------------------------------------

func TestWalkAndParse_ProgressPhases(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "b.go"), "package main\n")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2

	var mu sync.Mutex
	var phases []string
	policy.OnProgress = func(ev ProgressEvent) {
		mu.Lock()
		phases = append(phases, ev.Phase)
		mu.Unlock()
	}

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)
	for pf := range ch {
		pf.Close()
	}
	_ = statsFn()

	// Check that all required phases appear.
	phaseSet := map[string]bool{}
	for _, p := range phases {
		phaseSet[p] = true
	}
	for _, required := range []string{"walking", "parsing", "walk_complete", "done"} {
		if !phaseSet[required] {
			t.Errorf("missing progress phase: %s; got %v", required, phases)
		}
	}

	// walk_complete and done must come after all walking/parsing events.
	lastWalking := -1
	lastParsing := -1
	walkCompleteIdx := -1
	doneIdx := -1
	for i, p := range phases {
		switch p {
		case "walking":
			lastWalking = i
		case "parsing":
			lastParsing = i
		case "walk_complete":
			walkCompleteIdx = i
		case "done":
			doneIdx = i
		}
	}

	if walkCompleteIdx <= lastWalking {
		t.Errorf("walk_complete (idx=%d) should come after last walking (idx=%d)", walkCompleteIdx, lastWalking)
	}
	if doneIdx <= walkCompleteIdx {
		t.Errorf("done (idx=%d) should come after walk_complete (idx=%d)", doneIdx, walkCompleteIdx)
	}
	_ = lastParsing // parsing may interleave with walking due to concurrency
}

// ---------------------------------------------------------------------------
// Binary file detection
// ---------------------------------------------------------------------------

func TestWalkAndParse_BinaryFileSkipped(t *testing.T) {
	dir := t.TempDir()

	// Normal Go file.
	writeFile(t, filepath.Join(dir, "good.go"), "package main\n")

	// Go file with NUL bytes — should be detected as binary and skipped.
	binContent := []byte("package main\n\x00\x00\x00func binary() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "binary.go"), binContent, 0o644); err != nil {
		t.Fatal(err)
	}

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var paths []string
	for pf := range ch {
		paths = append(paths, filepath.Base(pf.Path))
		pf.Close()
	}

	stats := statsFn()

	if len(paths) != 1 {
		t.Fatalf("got %d files, want 1; paths: %v", len(paths), paths)
	}
	if paths[0] != "good.go" {
		t.Errorf("expected good.go, got %s", paths[0])
	}
	if stats.BinarySkipped != 1 {
		t.Errorf("BinarySkipped = %d, want 1", stats.BinarySkipped)
	}
}

// ---------------------------------------------------------------------------
// Read error handling
// ---------------------------------------------------------------------------

func TestWalkAndParse_ReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root to enforce file permissions")
	}
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod cannot make a file unreadable on Windows; it only sets the read-only attribute")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "good.go"), "package main\n")

	// Create an unreadable file.
	unreadable := filepath.Join(dir, "secret.go")
	writeFile(t, unreadable, "package main\n")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0o644) })

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var readErrors int
	var parseSuccesses int
	for pf := range ch {
		if pf.Err != nil {
			if pf.IsRead {
				t.Errorf("expected IsRead=false for I/O error on %s", pf.Path)
			}
			readErrors++
		} else {
			parseSuccesses++
		}
		pf.Close()
	}

	stats := statsFn()

	// The unreadable file should fail the binary check open too, so it may
	// either be skipped silently (checkBinaryFile fails to open, returns false)
	// and then fail in parseOne, or it may be silently skipped. Let's verify:
	// checkBinaryFile on an unreadable file returns (false, err), so it won't
	// be marked binary — it proceeds to parseOne where ReadFile fails.
	if readErrors != 1 {
		t.Errorf("read errors = %d, want 1 (FilesFailed=%d)", readErrors, stats.FilesFailed)
	}
	if parseSuccesses != 1 {
		t.Errorf("parse successes = %d, want 1", parseSuccesses)
	}
	if stats.FilesFailed != 1 {
		t.Errorf("FilesFailed = %d, want 1", stats.FilesFailed)
	}
}

// ---------------------------------------------------------------------------
// SkipTreeParse hook
// ---------------------------------------------------------------------------

func TestWalkAndParse_SkipTreeParse(t *testing.T) {
	dir := t.TempDir()

	// Normal file — should get full parse (Tree non-nil).
	writeFile(t, filepath.Join(dir, "normal.go"), "package main\n\nfunc main() {}\n")

	// "Generated" file — should be read-only (Tree nil, Source non-nil).
	writeFile(t, filepath.Join(dir, "service.pb.go"), "package main\n\ntype Service struct{}\n")

	policy := DefaultPolicy()
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	policy.SkipTreeParse = func(path string, size int64) bool {
		return strings.HasSuffix(path, ".pb.go")
	}

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var normalPF, genPF *ParsedFile
	for pf := range ch {
		pfCopy := pf
		if strings.HasSuffix(pf.Path, "normal.go") {
			normalPF = &pfCopy
		} else if strings.HasSuffix(pf.Path, "service.pb.go") {
			genPF = &pfCopy
		}
	}

	if normalPF == nil {
		t.Fatal("normal.go not found in results")
	}
	if normalPF.Tree == nil {
		t.Error("normal.go: expected Tree to be non-nil (full parse)")
	}
	if normalPF.Err != nil {
		t.Errorf("normal.go: unexpected error: %v", normalPF.Err)
	}
	normalPF.Close()

	if genPF == nil {
		t.Fatal("service.pb.go not found in results")
	}
	if genPF.Tree != nil {
		t.Error("service.pb.go: expected Tree to be nil (skip tree parse)")
	}
	if genPF.Source == nil {
		t.Error("service.pb.go: expected Source to be non-nil (read-only)")
	}
	if genPF.Err != nil {
		t.Errorf("service.pb.go: unexpected error: %v", genPF.Err)
	}
	if !genPF.IsRead {
		t.Error("service.pb.go: expected IsRead=true")
	}
	genPF.Close()

	stats := statsFn()
	if stats.FilesParsed != 2 {
		t.Errorf("FilesParsed = %d, want 2 (both count as parsed)", stats.FilesParsed)
	}
}

func TestWalkAndParse_SkipTreeParseLargeFile(t *testing.T) {
	dir := t.TempDir()

	// Large file that should skip tree parse.
	padding := strings.Repeat("x", 200)
	writeFile(t, filepath.Join(dir, "big.pb.go"),
		"package main\n\nfunc big() { /* "+padding+" */ }\n")

	policy := DefaultPolicy()
	policy.LargeFileThreshold = 50 // big.pb.go exceeds this
	policy.MaxConcurrent = 1
	policy.ChannelBuffer = 2
	policy.SkipTreeParse = func(path string, size int64) bool {
		return strings.HasSuffix(path, ".pb.go")
	}

	ch, statsFn := WalkAndParse(context.Background(), dir, policy)

	var result *ParsedFile
	for pf := range ch {
		pfCopy := pf
		result = &pfCopy
	}

	if result == nil {
		t.Fatal("no results")
	}
	if result.Tree != nil {
		t.Error("expected Tree to be nil for large skipped file")
	}
	if result.Source == nil {
		t.Error("expected Source to be non-nil for large skipped file")
	}
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	result.Close()

	stats := statsFn()
	if stats.LargeFiles != 1 {
		t.Errorf("LargeFiles = %d, want 1", stats.LargeFiles)
	}
}

// ---------------------------------------------------------------------------
// helper
// ---------------------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
