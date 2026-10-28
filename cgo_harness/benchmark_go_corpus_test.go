//go:build cgo && treesitter_c_bench

package cgoharness

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type goCorpusFile struct {
	path   string
	source []byte
}

func loadGoCorpus(tb testing.TB) []goCorpusFile {
	tb.Helper()

	root := strings.TrimSpace(os.Getenv("GOT_GO_CORPUS_ROOT"))
	if root == "" {
		for _, candidate := range []string{"corpus_real/go", filepath.Join("cgo_harness", "corpus_real", "go")} {
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				root = candidate
				break
			}
		}
	}
	if root == "" {
		tb.Fatal("set GOT_GO_CORPUS_ROOT or run from the repository/cgo_harness root")
	}

	var files []goCorpusFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bazel-bin", "bazel-out", "bazel-testlogs", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if strings.ToLower(filepath.Ext(path)) != ".go" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, goCorpusFile{path: path, source: src})
		return nil
	})
	if err != nil {
		tb.Fatalf("load go corpus %s: %v", root, err)
	}
	if len(files) == 0 {
		tb.Fatalf("no .go files under %s", root)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].path < files[j].path
	})
	tb.Logf("go corpus: root=%s files=%d bytes=%d", root, len(files), totalGoCorpusBytes(files))
	return files
}

func totalGoCorpusBytes(files []goCorpusFile) int64 {
	var total int64
	for _, file := range files {
		total += int64(len(file.source))
	}
	return total
}

func requireCompleteGoCorpusTree(tb testing.TB, tree *gotreesitter.Tree, src []byte, path string) {
	tb.Helper()
	if tree == nil || tree.RootNode() == nil {
		tb.Fatalf("%s: parse returned nil root", path)
	}
	root := tree.RootNode()
	rt := tree.ParseRuntime()
	if root.HasError() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(src)) || rt.Truncated {
		tb.Fatalf("%s: incomplete parse has_error=%v runtime=%s", path, root.HasError(), rt.Summary())
	}
}

func BenchmarkGoCorpusGoTreeSitterParseDFAWithImportExtract(b *testing.B) {
	files := loadGoCorpus(b)
	totalBytes := totalGoCorpusBytes(files)
	lang := grammars.GoLanguage()
	pool := gotreesitter.NewParserPool(lang)

	b.ReportAllocs()
	b.SetBytes(totalBytes)
	b.ResetTimer()

	var imports int64
	for i := 0; i < b.N; i++ {
		for _, file := range files {
			tree, err := pool.Parse(file.source)
			if err != nil {
				if tree != nil {
					tree.Release()
				}
				b.Fatalf("%s: %v", file.path, err)
			}
			requireCompleteGoCorpusTree(b, tree, file.source, file.path)
			imports += int64(len(gotreesitter.ExtractImports(tree)))
			tree.Release()
		}
	}
	if b.N > 0 {
		b.ReportMetric(float64(imports)/float64(b.N), "imports/op")
	}
}

func BenchmarkGoCorpusSourceImportExtract(b *testing.B) {
	files := loadGoCorpus(b)
	totalBytes := totalGoCorpusBytes(files)
	lang := grammars.GoLanguage()

	b.ReportAllocs()
	b.SetBytes(totalBytes)
	b.ResetTimer()

	var imports int64
	var fallbacks int64
	for i := 0; i < b.N; i++ {
		for _, file := range files {
			report := gotreesitter.ExtractImportsFromSourceWithReport(lang, file.source)
			imports += int64(len(report.Imports))
			if report.FallbackRecommended {
				fallbacks++
			}
		}
	}
	if b.N > 0 {
		b.ReportMetric(float64(imports)/float64(b.N), "imports/op")
		b.ReportMetric(float64(fallbacks)/float64(b.N), "fallbacks/op")
	}
}
