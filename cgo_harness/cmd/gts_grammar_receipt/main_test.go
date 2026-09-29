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
