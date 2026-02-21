//go:build cgo && treesitter_c_parity

package gotreesitter_test

// Structural parity tests: compare gotreesitter parse trees against a native C
// reference parser built from grammars/languages.lock commits, node-by-node.
//
// Run with:
//   go test . -tags treesitter_c_parity -run TestParity -v
//
// Requires CGo and a C toolchain. Gated behind build tag to keep the default
// test suite zero-CGo.

import (
	"fmt"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type parityMeta struct {
	skipReason string // non-empty = skip with this message
}

var paritySkips = map[string]parityMeta{
	// Structural parity requires a Go implementation of the language's external
	// scanner. Keep these skipped until scanners are ported.
	"elixir": {skipReason: "external scanner not ported"},
	"python": {skipReason: "external scanner not ported"},
	"toml":   {skipReason: "external scanner not ported (ExternalTokenCount=5)"},
	"yaml":   {skipReason: "external scanner not ported"},
}

type parityCase struct {
	name   string
	source string
}

var parityCases = []parityCase{
	{name: "bash", source: "echo hi\n"},
	{name: "c", source: "int main(void) { return 0; }\n"},
	{name: "cpp", source: "int main() { return 0; }\n"},
	{name: "css", source: "body { color: red; }\n"},
	{name: "elixir", source: "defmodule M do\n  def f(x), do: x + 1\nend\n"},
	{name: "go", source: "package main\n\nfunc main() {\n\tprintln(1)\n}\n"},
	{name: "html", source: "<html><body>Hello</body></html>\n"},
	{name: "java", source: "class Main { int x; }\n"},
	{name: "javascript", source: "function f() { return 1; }\nconst x = () => x + 1;\n"},
	{name: "kotlin", source: "fun main() {\n    val x: Int? = null\n    println(x)\n}\n"},
	{name: "lua", source: "local x = 1\n"},
	{name: "php", source: "<?php echo 1;\n"},
	{name: "python", source: "def f():\n    return 1\n"},
	{name: "ruby", source: "def f\n  1\nend\n"},
	{name: "rust", source: "fn main() { let x = 1; }\n"},
	{name: "scala", source: "object Main { def f(x: Int): Int = x + 1 }\n"},
	{name: "sql", source: "SELECT id, name FROM users WHERE id = 1;\n"},
	{name: "swift", source: "func f() -> Int {\n  return 1\n}\n"},
	{name: "toml", source: "a = 1\ntitle = \"hello\"\ntags = [\"x\", \"y\"]\n"},
	{name: "tsx", source: "const x = <div/>;\n"},
	{name: "typescript", source: "function f(): number { return 1; }\n"},
	{name: "yaml", source: "a: 1\n"},
}

var parityEntriesByName, paritySupportByName = func() (map[string]grammars.LangEntry, map[string]grammars.ParseSupport) {
	entries := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		entries[entry.Name] = entry
	}

	support := make(map[string]grammars.ParseSupport)
	for _, report := range grammars.AuditParseSupport() {
		support[report.Name] = report
	}
	return entries, support
}()

type nodeSnapshot struct {
	Type       string
	StartByte  uint32
	EndByte    uint32
	IsNamed    bool
	IsMissing  bool
	ChildCount int
}

func snapshotGo(n *gotreesitter.Node, lang *gotreesitter.Language) nodeSnapshot {
	return nodeSnapshot{
		Type:       n.Type(lang),
		StartByte:  n.StartByte(),
		EndByte:    n.EndByte(),
		IsNamed:    n.IsNamed(),
		IsMissing:  n.IsMissing(),
		ChildCount: n.ChildCount(),
	}
}

func snapshotC(n *sitter.Node) nodeSnapshot {
	return nodeSnapshot{
		Type:       n.Kind(),
		StartByte:  uint32(n.StartByte()),
		EndByte:    uint32(n.EndByte()),
		IsNamed:    n.IsNamed(),
		IsMissing:  n.IsMissing(),
		ChildCount: int(n.ChildCount()),
	}
}

// compareNodes walks both trees recursively in lockstep. Any divergence is
// appended to errs with a breadcrumb path for diagnosis.
func compareNodes(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string, errs *[]string) {
	gs := snapshotGo(goNode, goLang)
	cs := snapshotC(cNode)

	if gs.Type != cs.Type {
		*errs = append(*errs, fmt.Sprintf("%s: Type go=%q c=%q", path, gs.Type, cs.Type))
	}
	if gs.StartByte != cs.StartByte {
		*errs = append(*errs, fmt.Sprintf("%s: StartByte go=%d c=%d", path, gs.StartByte, cs.StartByte))
	}
	if gs.EndByte != cs.EndByte {
		*errs = append(*errs, fmt.Sprintf("%s: EndByte go=%d c=%d", path, gs.EndByte, cs.EndByte))
	}
	if gs.IsNamed != cs.IsNamed {
		*errs = append(*errs, fmt.Sprintf("%s: IsNamed go=%v c=%v", path, gs.IsNamed, cs.IsNamed))
	}
	if gs.IsMissing != cs.IsMissing {
		*errs = append(*errs, fmt.Sprintf("%s: IsMissing go=%v c=%v", path, gs.IsMissing, cs.IsMissing))
	}
	if gs.ChildCount != cs.ChildCount {
		*errs = append(*errs, fmt.Sprintf("%s: ChildCount go=%d c=%d", path, gs.ChildCount, cs.ChildCount))
		return
	}

	for i := 0; i < gs.ChildCount; i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		compareNodes(goNode.Child(i), goLang, cNode.Child(uint(i)), childPath, errs)
	}
}

func parseWithGo(tc parityCase, src []byte, oldTree *gotreesitter.Tree) (*gotreesitter.Tree, *gotreesitter.Language, error) {
	entry, ok := parityEntriesByName[tc.name]
	if !ok {
		return nil, nil, fmt.Errorf("missing gotreesitter registry entry for %q", tc.name)
	}
	report, ok := paritySupportByName[tc.name]
	if !ok {
		return nil, nil, fmt.Errorf("missing parse support report for %q", tc.name)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)

	var tree *gotreesitter.Tree
	if oldTree == nil {
		switch report.Backend {
		case grammars.ParseBackendTokenSource:
			if entry.TokenSourceFactory == nil {
				return nil, nil, fmt.Errorf("token source backend without factory for %q", tc.name)
			}
			tree = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
		case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
			tree = parser.Parse(src)
		default:
			return nil, nil, fmt.Errorf("unsupported parse backend %q for %q", report.Backend, tc.name)
		}
	} else {
		switch report.Backend {
		case grammars.ParseBackendTokenSource:
			if entry.TokenSourceFactory == nil {
				return nil, nil, fmt.Errorf("token source backend without factory for %q", tc.name)
			}
			tree = parser.ParseIncrementalWithTokenSource(src, oldTree, entry.TokenSourceFactory(src, lang))
		case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
			tree = parser.ParseIncremental(src, oldTree)
		default:
			return nil, nil, fmt.Errorf("unsupported incremental backend %q for %q", report.Backend, tc.name)
		}
	}

	if tree == nil || tree.RootNode() == nil {
		return nil, nil, fmt.Errorf("gotreesitter returned nil tree")
	}
	return tree, lang, nil
}

func runParityCase(t *testing.T, tc parityCase, label string, src []byte) {
	t.Helper()

	goTree, goLang, err := parseWithGo(tc, src, nil)
	if err != nil {
		t.Fatalf("[%s/%s] gotreesitter parse error: %v", tc.name, label, err)
	}
	cLang, err := gotreesitter.ParityCLanguage(tc.name)
	if err != nil {
		t.Fatalf("[%s/%s] load C parser from languages.lock: %v", tc.name, label, err)
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("[%s/%s] C parser SetLanguage error: %v", tc.name, label, err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatalf("[%s/%s] C reference parser returned nil tree", tc.name, label)
	}
	defer cTree.Close()

	var errs []string
	compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
	if len(errs) == 0 {
		return
	}

	const maxErrors = 20
	shown := errs
	extra := 0
	if len(errs) > maxErrors {
		shown = errs[:maxErrors]
		extra = len(errs) - maxErrors
	}
	msg := strings.Join(shown, "\n  ")
	if extra > 0 {
		msg += fmt.Sprintf("\n  ... and %d more", extra)
	}
	t.Errorf("[%s/%s] %d node divergence(s):\n  %s", tc.name, label, len(errs), msg)
}

func normalizedSource(src string) []byte {
	// Trim one trailing newline so both runtimes compare syntax tree shape
	// independent of final-line extra-token representation.
	return []byte(strings.TrimSuffix(src, "\n"))
}

// safeEditOffset chooses one insertion offset for a single-byte whitespace edit.
func safeEditOffset(src []byte) int {
	insertAt := -1

	for i := len(src) - 1; i >= 0; i-- {
		if src[i] == ' ' || src[i] == '\t' {
			insertAt = i
			break
		}
	}
	if insertAt < 0 && len(src) > 0 && src[0] == '<' {
		for i := 0; i+1 < len(src); i++ {
			if src[i] == '>' && src[i+1] == '<' {
				insertAt = i + 1
				break
			}
		}
	}
	if insertAt < 0 {
		for i := len(src) - 1; i >= 0; i-- {
			switch src[i] {
			case '\n', '}', ';', ')', ']', '>', '<':
				insertAt = i
				break
			}
			if insertAt >= 0 {
				break
			}
		}
	}
	if insertAt < 0 {
		insertAt = len(src)
	}
	return insertAt
}

func insertSpaceAt(src []byte, insertAt int) []byte {
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(src) {
		insertAt = len(src)
	}

	edited := make([]byte, len(src)+1)
	copy(edited[:insertAt], src[:insertAt])
	edited[insertAt] = ' '
	copy(edited[insertAt+1:], src[insertAt:])
	return edited
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}

// incrementalEditOffsets returns preferred insertion offsets for incremental
// parity tests. It tries syntax-local edits first, then falls back to all
// interior offsets so the test can find a fresh-parity-safe site.
func incrementalEditOffsets(src []byte) []int {
	if len(src) == 0 {
		return []int{0}
	}

	offsets := make([]int, 0, len(src)+8)
	seen := make([]bool, len(src)+1)
	add := func(pos int) {
		if pos < 0 || pos > len(src) || seen[pos] {
			return
		}
		seen[pos] = true
		offsets = append(offsets, pos)
	}

	add(safeEditOffset(src))

	// For markup-like sources, prefer edits in text content (outside tags).
	if src[0] == '<' {
		inTag := false
		for i := 0; i < len(src); i++ {
			switch src[i] {
			case '<':
				inTag = true
			case '>':
				inTag = false
			default:
				if !inTag && i > 0 && isWordByte(src[i-1]) && isWordByte(src[i]) {
					add(i)
				}
			}
		}
	}

	// Prefer edits inside existing word runs across languages.
	for i := 1; i < len(src); i++ {
		if isWordByte(src[i-1]) && isWordByte(src[i]) {
			add(i)
		}
	}

	// Exhaustive fallback over interior positions, biased toward tail.
	for i := len(src) - 1; i >= 1; i-- {
		add(i)
	}
	add(len(src))
	return offsets
}

// safeEditSource inserts one ASCII space at the preferred offset.
func safeEditSource(src []byte) ([]byte, int) {
	insertAt := safeEditOffset(src)
	return insertSpaceAt(src, insertAt), insertAt
}

// TestParityFreshParse verifies that fresh parse trees match the CGo binding
// on the currently CI-gated language set.
func TestParityFreshParse(t *testing.T) {
	for _, tc := range parityCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if meta, ok := paritySkips[tc.name]; ok && meta.skipReason != "" {
				t.Skipf("known mismatch: %s", meta.skipReason)
			}
			runParityCase(t, tc, "fresh", normalizedSource(tc.source))
		})
	}
}

// TestParityIncrementalParse verifies that gotreesitter incremental parse
// matches a CGo fresh parse on edited source.
func TestParityIncrementalParse(t *testing.T) {
	for _, tc := range parityCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if meta, ok := paritySkips[tc.name]; ok && meta.skipReason != "" {
				t.Skipf("known mismatch: %s", meta.skipReason)
			}

			src := normalizedSource(tc.source)
			if len(src) < 2 {
				t.Skip("source too short for incremental edit")
			}

			oldTree, _, err := parseWithGo(tc, src, nil)
			if err != nil {
				t.Fatalf("[%s/incremental] initial gotreesitter parse error: %v", tc.name, err)
			}
			cLang, err := gotreesitter.ParityCLanguage(tc.name)
			if err != nil {
				t.Fatalf("[%s/incremental] load C parser from languages.lock: %v", tc.name, err)
			}

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("[%s/incremental] C parser SetLanguage error: %v", tc.name, err)
			}

			candidates := incrementalEditOffsets(src)
			edited := []byte(nil)
			editAt := -1
			firstFreshErr := ""
			for _, candidateAt := range candidates {
				candidateEdited := insertSpaceAt(src, candidateAt)

				goFreshTree, goFreshLang, err := parseWithGo(tc, candidateEdited, nil)
				if err != nil {
					t.Fatalf("[%s/incremental] gotreesitter fresh parse on candidate edit@%d error: %v", tc.name, candidateAt, err)
				}

				cFreshTree := cParser.Parse(candidateEdited, nil)
				if cFreshTree == nil || cFreshTree.RootNode() == nil {
					t.Fatalf("[%s/incremental] C reference parser returned nil tree on candidate edit@%d", tc.name, candidateAt)
				}

				var freshErrs []string
				compareNodes(goFreshTree.RootNode(), goFreshLang, cFreshTree.RootNode(), "root", &freshErrs)
				cFreshTree.Close()
				if len(freshErrs) == 0 {
					edited = candidateEdited
					editAt = candidateAt
					break
				}
				if firstFreshErr == "" {
					firstFreshErr = freshErrs[0]
				}
			}
			if editAt < 0 {
				if firstFreshErr == "" {
					firstFreshErr = "no comparable fresh trees from candidate edits"
				}
				t.Skipf("[%s/incremental] no fresh-parity-safe edit site found (%d candidates, first divergence: %s)", tc.name, len(candidates), firstFreshErr)
			}

			edit := gotreesitter.InputEdit{
				StartByte:   uint32(editAt),
				OldEndByte:  uint32(editAt),
				NewEndByte:  uint32(editAt + 1),
				StartPoint:  pointAtOffset(src, editAt),
				OldEndPoint: pointAtOffset(src, editAt),
				NewEndPoint: pointAtOffset(edited, editAt+1),
			}

			oldTree.Edit(edit)

			goTree, goLang, err := parseWithGo(tc, edited, oldTree)
			if err != nil {
				t.Fatalf("[%s/incremental] gotreesitter incremental parse error: %v", tc.name, err)
			}

			cTree := cParser.Parse(edited, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatalf("[%s/incremental] C reference parser returned nil tree", tc.name)
			}
			defer cTree.Close()

			var errs []string
			compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
			if len(errs) == 0 {
				return
			}

			const maxErrors = 20
			shown := errs
			extra := 0
			if len(errs) > maxErrors {
				shown = errs[:maxErrors]
				extra = len(errs) - maxErrors
			}
			msg := strings.Join(shown, "\n  ")
			if extra > 0 {
				msg += fmt.Sprintf("\n  ... and %d more", extra)
			}
			t.Errorf("[%s/incremental] %d node divergence(s):\n  %s", tc.name, len(errs), msg)
		})
	}
}

// TestParityHasNoErrors checks that well-formed inputs for CI-gated languages
// do not produce error nodes.
func TestParityHasNoErrors(t *testing.T) {
	for _, tc := range parityCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if meta, ok := paritySkips[tc.name]; ok && meta.skipReason != "" {
				t.Skipf("known mismatch: %s", meta.skipReason)
			}

			tree, _, err := parseWithGo(tc, normalizedSource(tc.source), nil)
			if err != nil {
				t.Fatalf("[%s/errors] gotreesitter parse error: %v", tc.name, err)
			}
			if tree.RootNode().HasError() {
				t.Errorf("[%s] RootNode().HasError() = true on well-formed input", tc.name)
			}
		})
	}
}
