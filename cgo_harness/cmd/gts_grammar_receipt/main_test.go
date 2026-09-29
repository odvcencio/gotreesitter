//go:build cgo && treesitter_c_parity

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsUnavailableCorpusSample(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tmux"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no matching files", err: errors.New("no corpus files selected for tmux"), want: true},
		{name: "missing selected subdir", err: &os.PathError{Op: "lstat", Path: filepath.Join(root, "tmux", ".gts-extracted"), Err: os.ErrNotExist}, want: true},
		{name: "missing grammar checkout", err: &os.PathError{Op: "lstat", Path: filepath.Join(root, "missing"), Err: os.ErrNotExist}, want: false},
		{name: "other failure", err: errors.New("corpus lock mismatch"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnavailableCorpusSample(tt.err, root, "tmux"); got != tt.want {
				t.Fatalf("isUnavailableCorpusSample() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmptyCorpusManifestPinsLockWithoutFiles(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "corpus_sources.lock")
	if err := os.WriteFile(lock, []byte("tmux https://example.com/tmux 0123456789012345678901234567890123456789 . .conf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := hashFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := emptyCorpusManifest("0123456789012345678901234567890123456789", lock)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Files == nil || len(manifest.Files) != 0 {
		t.Fatalf("Files = %#v, want a non-nil empty slice", manifest.Files)
	}
	if manifest.CorpusLock.SHA256 != want || manifest.CorpusLock.Path != "corpus_sources.lock" {
		t.Fatalf("CorpusLock = %+v, want sha256 %s", manifest.CorpusLock, want)
	}
	if manifest.Selection.MaxFiles != maxCorpusFiles || manifest.Selection.MaxFileBytes != maxCorpusFileSize {
		t.Fatalf("Selection = %+v", manifest.Selection)
	}
	if _, err := emptyCorpusManifest("0123456789012345678901234567890123456789", filepath.Join(t.TempDir(), "missing.lock")); err == nil {
		t.Fatal("missing lock returned no error")
	}
}
