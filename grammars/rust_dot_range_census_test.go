//go:build !grammar_subset

package grammars

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

type rustDotRangeCensusTotals struct {
	files      int
	rangeNodes uint64
	candidates uint64
}

func TestRustDotRangeRepairSpan(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{source: "..", want: true},
		{source: " .. ", want: true},
		{source: ".. ..=", want: true},
		{source: "..=", want: false},
		{source: "1..2", want: false},
		{source: ".", want: false},
	}
	for _, test := range tests {
		if got := rustDotRangeRepairSpan([]byte(test.source), 0, uint32(len(test.source))); got != test.want {
			t.Errorf("rustDotRangeRepairSpan(%q) = %t, want %t", test.source, got, test.want)
		}
	}
}

func TestRustDotRangeProducerOverAuthenticatedCorpus(t *testing.T) {
	corpusRoot := os.Getenv("GTS_RUST_DOT_RANGE_CORPUS")
	if corpusRoot == "" {
		t.Skip("set GTS_RUST_DOT_RANGE_CORPUS to run the authenticated corpus census")
	}
	expectedFilesText := os.Getenv("GTS_RUST_DOT_RANGE_EXPECT_FILES")
	if expectedFilesText == "" {
		t.Fatal("set GTS_RUST_DOT_RANGE_EXPECT_FILES to authenticate the corpus size")
	}
	expectedFiles, err := strconv.Atoi(expectedFilesText)
	if err != nil || expectedFiles <= 0 {
		t.Fatalf("GTS_RUST_DOT_RANGE_EXPECT_FILES = %q, want a positive integer", expectedFilesText)
	}

	paths := rustSourcePaths(t, corpusRoot)
	if got := len(paths); got != expectedFiles {
		t.Fatalf("Rust corpus files = %d, want %d", got, expectedFiles)
	}

	lang := RustLanguage()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	var clean rustDotRangeCensusTotals
	var truncated rustDotRangeCensusTotals
	var candidatePaths []string
	for index, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(source) == 0 {
			continue
		}
		censusRustDotRangeSource(t, parser, lang, source, path, &clean, &candidatePaths)
		if index%2 == 0 {
			end := len(source) * 55 / 100
			if end > 0 {
				censusRustDotRangeSource(t, parser, lang, source[:end], path+"@55%", &truncated, &candidatePaths)
			}
		}
	}

	t.Logf("clean: files=%d range_nodes=%d repair_candidates=%d", clean.files, clean.rangeNodes, clean.candidates)
	t.Logf("truncated: files=%d range_nodes=%d repair_candidates=%d", truncated.files, truncated.rangeNodes, truncated.candidates)
	if total := clean.candidates + truncated.candidates; total != 0 {
		limit := len(candidatePaths)
		if limit > 20 {
			limit = 20
		}
		t.Fatalf("Rust producer left %d dot-range repair candidates; first paths: %s", total, strings.Join(candidatePaths[:limit], ", "))
	}
}

func rustSourcePaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".rs") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk Rust corpus: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func censusRustDotRangeSource(t *testing.T, parser *gotreesitter.Parser, lang *gotreesitter.Language, source []byte, path string, totals *rustDotRangeCensusTotals, candidatePaths *[]string) {
	t.Helper()
	tree, err := parser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	defer tree.Release()

	totals.files++
	var walk func(*gotreesitter.Node)
	walk = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.Type(lang) == "range_expression" {
			totals.rangeNodes++
			if node.ChildCount() == 0 && rustDotRangeRepairSpan(source, node.StartByte(), node.EndByte()) {
				totals.candidates++
				*candidatePaths = append(*candidatePaths, path)
			}
		}
		for index := 0; index < node.ChildCount(); index++ {
			walk(node.Child(index))
		}
	}
	walk(tree.RootNode())
}

func rustDotRangeRepairSpan(source []byte, start, end uint32) bool {
	if start >= end || int(end) > len(source) {
		return false
	}
	cursor := int(start)
	limit := int(end)
	tokens := 0
	for cursor < limit {
		for cursor < limit {
			switch source[cursor] {
			case ' ', '\t', '\r', '\n':
				cursor++
			default:
				goto token
			}
		}
	token:
		if cursor >= limit {
			break
		}
		if cursor+1 >= limit || source[cursor] != '.' || source[cursor+1] != '.' {
			return false
		}
		tokens++
		cursor += 2
		for cursor < limit {
			switch source[cursor] {
			case ' ', '\t', '\r', '\n':
				cursor++
			default:
				goto suffix
			}
		}
	suffix:
		if cursor < limit && source[cursor] == '=' {
			if tokens < 2 {
				return false
			}
			cursor++
		}
	}
	return tokens > 0
}
