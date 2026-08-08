package lean

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// TestOfficialLeanCorpusCharacterization measures the native grammar against
// an external Lean checkout. Set GOT_LEAN_CORPUS_ROOTS to enable this test.
func TestOfficialLeanCorpusCharacterization(t *testing.T) {
	rawRoots := os.Getenv("GOT_LEAN_CORPUS_ROOTS")
	if rawRoots == "" {
		t.Skip("set GOT_LEAN_CORPUS_ROOTS to one or more Lean corpus directories")
	}
	limit := 0
	if raw := os.Getenv("GOT_LEAN_CORPUS_LIMIT"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			t.Fatalf("GOT_LEAN_CORPUS_LIMIT must be a positive integer")
		}
		limit = value
	}

	var paths []string
	for _, root := range filepath.SplitList(rawRoots) {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".lean") {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk corpus root %s: %v", root, err)
		}
	}
	if len(paths) == 0 {
		t.Fatal("Lean corpus has no .lean files")
	}
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
	}

	parser := gotreesitter.NewParser(Language())
	started := time.Now()
	var files, clean, parseErrors, stopped, totalBytes int
	var errorSamples []string
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		files++
		totalBytes += len(source)
		tree, err := parser.Parse(source)
		if err != nil {
			parseErrors++
			if len(errorSamples) < 12 {
				errorSamples = append(errorSamples, path+": "+err.Error())
			}
			continue
		}
		root := tree.RootNode()
		if tree.ParseStoppedEarly() {
			stopped++
		}
		if root != nil && !root.HasError() && !tree.ParseStoppedEarly() {
			clean++
		} else if len(errorSamples) < 12 {
			errorSamples = append(errorSamples, path+leanFirstError(root, source))
		}
		tree.Release()
	}

	t.Logf("Lean corpus: files=%d clean=%d errors=%d stopped=%d bytes=%d elapsed=%s",
		files, clean, files-clean-parseErrors, stopped, totalBytes, time.Since(started).Round(time.Millisecond))
	for _, sample := range errorSamples {
		t.Logf("error sample: %s", sample)
	}
	if clean != files {
		t.Fatalf("Lean corpus had %d parser failures and %d parses with recovery",
			parseErrors, files-clean-parseErrors)
	}
}

func leanFirstError(root *gotreesitter.Node, source []byte) string {
	var visit func(*gotreesitter.Node) *gotreesitter.Node
	visit = func(node *gotreesitter.Node) *gotreesitter.Node {
		if node == nil {
			return nil
		}
		for i := 0; i < node.ChildCount(); i++ {
			if found := visit(node.Child(i)); found != nil {
				return found
			}
		}
		if node.IsError() || node.IsMissing() {
			return node
		}
		return nil
	}
	node := visit(root)
	if node == nil {
		return ": parse stopped without an error node"
	}
	start, end := int(node.StartByte()), int(node.EndByte())
	if start < 0 || start > len(source) {
		start = 0
	}
	if end < start || end > len(source) {
		end = start
	}
	line := bytes.Count(source[:start], []byte{'\n'}) + 1
	if end > start+120 {
		end = start + 120
	}
	return fmt.Sprintf(": line %d %s %q", line, node.Type(Language()), source[start:end])
}
