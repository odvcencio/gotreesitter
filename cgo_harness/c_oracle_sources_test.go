//go:build cgo && (treesitter_c_parity || treesitter_c_bench)

package cgoharness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCOracleVendoredSources(t *testing.T) {
	var manifest struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(sitter.SourceManifest(), &manifest); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", COracleBindingModule).Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSpace(string(out))
	want, err := filepath.Abs("internal/coracle")
	if err != nil {
		t.Fatal(err)
	}
	if dir != want {
		t.Fatalf("binding source=%s want harness-local %s", dir, want)
	}
	actual := make(map[string]string)
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "upstream.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		actual[filepath.ToSlash(relative)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, manifest.Files) {
		t.Fatal("oracle source bytes differ from compiled provenance; regenerate from pinned commits")
	}
}

func TestCOracleProgressCancellation(t *testing.T) {
	language, err := COracleLanguage("json")
	if err != nil {
		t.Fatal(err)
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	source := []byte("[" + strings.Repeat("0,", 10000) + "0]")
	parser.SetTimeoutMicros(1)
	if tree := parser.Parse(source, nil); tree != nil {
		tree.Close()
		t.Fatal("timeout returned a completed tree")
	}
	parser.Reset()
	parser.SetTimeoutMicros(0)
	var flag uintptr = 1
	parser.SetCancellationFlag(&flag)
	if tree := parser.Parse(source, nil); tree != nil {
		tree.Close()
		t.Fatal("cancellation returned a completed tree")
	}
	parser.Reset()
	atomic.StoreUintptr(&flag, 0)
	calls := 0
	tree := parser.ParseWithOptions(func(i int, _ sitter.Point) []byte {
		if i < len(source) {
			return source[i:]
		}
		return nil
	}, nil, &sitter.ParseOptions{ProgressCallback: func(sitter.ParseState) bool { calls++; return true }})
	if tree != nil {
		tree.Close()
		t.Fatal("explicit progress cancellation returned a tree")
	}
	if calls == 0 {
		t.Fatal("explicit progress callback was lost")
	}
	parser.Reset()
	parser.SetCancellationFlag(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if tree := parser.ParseCtx(ctx, source, nil); tree != nil {
		tree.Close()
		t.Fatal("canceled context returned a tree")
	}
	parser.Reset()
	tree = parser.Parse(source, nil)
	if tree == nil {
		t.Fatal("reset parser did not complete")
	}
	defer tree.Close()
	if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint(len(source)) {
		t.Fatal("reset parser returned an incomplete tree")
	}
}
