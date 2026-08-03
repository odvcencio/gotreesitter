package gotreesitter_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// outlineGoldenRoot holds the committed outline fixtures and their goldens.
const outlineGoldenRoot = "testdata/outline"

// outlineGoldenUpdateEnv regenerates the goldens when it is set. The
// assertions always run; only the WRITE is opt-in, so no continuous
// integration lane can silently skip the comparison.
const outlineGoldenUpdateEnv = "GTS_UPDATE_OUTLINE_GOLDENS"

// outlineGoldenFixture names one committed source file and the language that
// parses it. The language is stated, never detected from the extension, so a
// renamed fixture fails loudly instead of quietly changing grammar.
type outlineGoldenFixture struct {
	Language string
	File     string
	// Purpose records why this fixture exists, so a reviewer can see what
	// the golden is meant to protect.
	Purpose string
}

// outlineGoldenFixtures is the stage-1 golden set. Every language listed is
// tier 1 in the outline coverage census (grammars.TestOutlineTierCensus):
// each has a non-empty inferred tags query and real-corpus coverage. The
// fixtures are committed here, not read from the corpus, so the goldens
// reproduce on any host and in every continuous integration lane.
var outlineGoldenFixtures = []outlineGoldenFixture{
	{
		Language: "go",
		File:     "go/service.go.fixture",
		Purpose:  "functions and methods from the hand-written Go tags override; a nested function literal that the query does not capture",
	},
	{
		Language: "go",
		File:     "go/broken.go.fixture",
		Purpose:  "error-bearing source: the outline stays well-defined across an ERROR node",
	},
	{
		Language: "python",
		File:     "python/app.py",
		Purpose:  "three nesting levels: module function, class, method, class inside a method",
	},
	{
		Language: "javascript",
		File:     "javascript/widget.js",
		Purpose:  "class with methods, function nested in a method, empty class",
	},
	{
		Language: "typescript",
		File:     "typescript/model.ts",
		Purpose:  "enum refinement by node type, interface, class, nested function",
	},
	{
		Language: "tsx",
		File:     "tsx/view.tsx",
		Purpose:  "interface and class beside JSX; nested function in a component",
	},
	{
		Language: "java",
		File:     "java/Service.java",
		Purpose:  "interface, enum refinement, constructor, method, nested class",
	},
	{
		Language: "rust",
		File:     "rust/lib.rs",
		Purpose:  "struct, enum refinement by enum_item, trait, function nested in a function",
	},
	{
		Language: "c",
		File:     "c/registry.c",
		// This golden also pins a known limit of the shared inferred tags
		// data, not of the extractor: the pattern
		// "(struct_specifier (type_identifier) @name) @definition.type"
		// matches a struct USE, such as the parameter type
		// "struct Registry *registry", as well as a struct definition. The
		// outline reports what the query captured, so the extra "Registry"
		// entries nest inside the functions that mention the type. This is
		// spec risk R1, inferred-query over-capture. Its remedy is a data
		// edit to the pattern table or a per-language override, which is a
		// later stage; the golden makes the behavior visible today and
		// turns that data edit into a reviewable diff.
		Purpose: "struct, typedef, and function definitions; also pins the inferred-query over-capture of struct uses (spec risk R1)",
	},
	{
		Language: "cpp",
		File:     "cpp/registry.cpp",
		Purpose:  "class with member function definitions nested inside it",
	},
}

// outlineGoldenRange is the readable serialization of a Range. Byte offsets
// stay separate fields so a diff shows exactly which offset moved; the points
// collapse to "row:column" so line moves read at a glance.
type outlineGoldenRange struct {
	StartByte  uint32 `json:"start_byte"`
	EndByte    uint32 `json:"end_byte"`
	StartPoint string `json:"start_point"`
	EndPoint   string `json:"end_point"`
}

// outlineGoldenSymbol mirrors gts.OutlineSymbol with stable JSON names.
type outlineGoldenSymbol struct {
	Kind      string                `json:"kind"`
	Name      string                `json:"name"`
	NodeType  string                `json:"node_type"`
	Range     outlineGoldenRange    `json:"range"`
	NameRange outlineGoldenRange    `json:"name_range"`
	Owner     string                `json:"owner"`
	Children  []outlineGoldenSymbol `json:"children,omitempty"`
}

// outlineGoldenFile is the whole committed artifact for one fixture.
type outlineGoldenFile struct {
	Language string                `json:"language"`
	Fixture  string                `json:"fixture"`
	Purpose  string                `json:"purpose"`
	HasError bool                  `json:"tree_has_error"`
	Report   gts.OutlineReport     `json:"report"`
	Symbols  []outlineGoldenSymbol `json:"symbols"`
}

func outlineGoldenRangeOf(r gts.Range) outlineGoldenRange {
	return outlineGoldenRange{
		StartByte:  r.StartByte,
		EndByte:    r.EndByte,
		StartPoint: fmt.Sprintf("%d:%d", r.StartPoint.Row, r.StartPoint.Column),
		EndPoint:   fmt.Sprintf("%d:%d", r.EndPoint.Row, r.EndPoint.Column),
	}
}

func outlineGoldenSymbolsOf(symbols []gts.OutlineSymbol) []outlineGoldenSymbol {
	if len(symbols) == 0 {
		return nil
	}
	out := make([]outlineGoldenSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		out = append(out, outlineGoldenSymbol{
			Kind:      symbol.Kind,
			Name:      symbol.Name,
			NodeType:  symbol.NodeType,
			Range:     outlineGoldenRangeOf(symbol.Range),
			NameRange: outlineGoldenRangeOf(symbol.NameRange),
			Owner:     symbol.Owner,
			Children:  outlineGoldenSymbolsOf(symbol.Children),
		})
	}
	return out
}

// outlineForFixture parses a committed fixture and projects its outline.
func outlineForFixture(t *testing.T, fixture outlineGoldenFixture) (outlineGoldenFile, bool) {
	t.Helper()

	entry := grammars.DetectLanguageByName(fixture.Language)
	if entry == nil {
		t.Fatalf("%s: language %q is not registered", fixture.File, fixture.Language)
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatalf("%s: language %q failed to load", fixture.File, fixture.Language)
	}

	query := grammars.ResolveTagsQuery(*entry)
	if strings.TrimSpace(query) == "" {
		t.Fatalf("%s: language %q resolved an empty tags query, so it is no longer tier 1", fixture.File, fixture.Language)
	}

	source, err := os.ReadFile(filepath.Join(outlineGoldenRoot, fixture.File))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	parser := gts.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("%s: parse: %v", fixture.File, err)
	}
	defer tree.Release()

	outliner, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("%s: build outliner: %v", fixture.File, err)
	}

	symbols, report := outliner.OutlineTree(tree)

	root := tree.RootNode()
	hasError := root != nil && root.HasError()

	return outlineGoldenFile{
		Language: fixture.Language,
		Fixture:  fixture.File,
		Purpose:  fixture.Purpose,
		HasError: hasError,
		Report:   report,
		Symbols:  outlineGoldenSymbolsOf(symbols),
	}, true
}

func outlineGoldenPath(fixture outlineGoldenFixture) string {
	return filepath.Join(outlineGoldenRoot, fixture.File+".golden.json")
}

func encodeOutlineGolden(t *testing.T, got outlineGoldenFile) []byte {
	t.Helper()
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("encode golden: %v", err)
	}
	return append(data, '\n')
}

// TestOutlineGoldens pins the outline of every committed fixture. It always
// compares; it writes only when GTS_UPDATE_OUTLINE_GOLDENS is set. A golden
// change must therefore ship in the same commit as its cause.
func TestOutlineGoldens(t *testing.T) {
	update := strings.TrimSpace(os.Getenv(outlineGoldenUpdateEnv)) != ""

	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			got, ok := outlineForFixture(t, fixture)
			if !ok {
				return
			}
			encoded := encodeOutlineGolden(t, got)
			path := outlineGoldenPath(fixture)

			if update {
				if err := os.WriteFile(path, encoded, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", path, err)
				}
				t.Logf("wrote %s", path)
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v -- regenerate with %s=1 go test . -run TestOutlineGoldens",
					path, err, outlineGoldenUpdateEnv)
			}
			if string(want) != string(encoded) {
				t.Errorf("outline golden drift for %s.\nRegenerate with %s=1 go test . -run TestOutlineGoldens and ship the change with its cause.\n--- want ---\n%s\n--- got ---\n%s",
					path, outlineGoldenUpdateEnv, want, encoded)
			}
		})
	}
}

// TestOutlineGoldenAccounting asserts the receipt identity on every committed
// golden: each candidate the tags query produced is either an emitted symbol
// or exactly one omission. A drop that lands in no counter is a silent
// coverage loss, and this test makes it impossible.
func TestOutlineGoldenAccounting(t *testing.T) {
	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			path := outlineGoldenPath(fixture)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v", path, err)
			}
			var golden outlineGoldenFile
			if err := json.Unmarshal(data, &golden); err != nil {
				t.Fatalf("decode golden %s: %v", path, err)
			}

			counted := countGoldenSymbols(golden.Symbols)
			if counted != golden.Report.Symbols {
				t.Errorf("%s: golden holds %d symbols, report says %d", path, counted, golden.Report.Symbols)
			}
			if golden.Report.Candidates() != golden.Report.Symbols+golden.Report.Omitted() {
				t.Errorf("%s: receipt identity broken: candidates=%d symbols=%d omitted=%d",
					path, golden.Report.Candidates(), golden.Report.Symbols, golden.Report.Omitted())
			}
			if golden.Report.QueryEmpty {
				t.Errorf("%s: tier-1 fixture reports an empty query", path)
			}
			if golden.Report.LanguageMismatch {
				t.Errorf("%s: fixture reports a language mismatch", path)
			}
			if golden.Report.Truncated {
				t.Errorf("%s: fixture outline is truncated; raise the work budget or shrink the fixture", path)
			}
			if golden.Report.OwnerRuleMisses != 0 {
				t.Errorf("%s: owner rules do not run in this change, so OwnerRuleMisses must stay 0, got %d",
					path, golden.Report.OwnerRuleMisses)
			}
			if golden.Report.Symbols == 0 {
				t.Errorf("%s: tier-1 fixture produced no symbols", path)
			}
		})
	}
}

// TestOutlineGoldensCoverErrorBearingSource proves the golden set includes at
// least one source the parser could not fully recover, and that its outline is
// still well-defined: symbols before and after the damage still appear, and
// the receipt still balances.
func TestOutlineGoldensCoverErrorBearingSource(t *testing.T) {
	found := 0
	for _, fixture := range outlineGoldenFixtures {
		got, ok := outlineForFixture(t, fixture)
		if !ok {
			continue
		}
		if !got.HasError {
			continue
		}
		found++

		if got.Report.Symbols == 0 {
			t.Errorf("%s: error-bearing fixture produced no symbols; the outline must survive an ERROR node", fixture.File)
		}
		if got.Report.Candidates() != got.Report.Symbols+got.Report.Omitted() {
			t.Errorf("%s: receipt identity broken on an error-bearing tree", fixture.File)
		}
		names := map[string]bool{}
		collectGoldenNames(got.Symbols, names)
		for _, want := range []string{"Before", "After"} {
			if !names[want] {
				t.Errorf("%s: expected symbol %q to survive the ERROR node, got %v", fixture.File, want, sortedKeys(names))
			}
		}
	}
	if found == 0 {
		t.Fatal("no committed fixture parses to a tree with an ERROR node; add one")
	}
}

// TestOutlineGoldensNestAndSpread proves the golden set is not a flat, single
// kind sample: at least one fixture nests symbols, and the set as a whole
// covers several normalized kinds.
func TestOutlineGoldensNestAndSpread(t *testing.T) {
	kinds := map[string]bool{}
	maxDepth := 0
	nestedFixtures := 0

	for _, fixture := range outlineGoldenFixtures {
		path := outlineGoldenPath(fixture)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read golden %s: %v", path, err)
		}
		var golden outlineGoldenFile
		if err := json.Unmarshal(data, &golden); err != nil {
			t.Fatalf("decode golden %s: %v", path, err)
		}
		depth := goldenDepth(golden.Symbols)
		if depth > 1 {
			nestedFixtures++
		}
		if depth > maxDepth {
			maxDepth = depth
		}
		collectGoldenKinds(golden.Symbols, kinds)
	}

	if maxDepth < 3 {
		t.Errorf("golden set nests only %d levels deep; a nesting regression would not show", maxDepth)
	}
	if nestedFixtures < 3 {
		t.Errorf("only %d golden fixtures nest; widen the spread", nestedFixtures)
	}
	for _, want := range []string{"function", "method", "class", "interface", "type", "enum", "constructor"} {
		if !kinds[want] {
			t.Errorf("golden set covers no %q symbol; kinds present: %v", want, sortedKeys(kinds))
		}
	}
}

func countGoldenSymbols(symbols []outlineGoldenSymbol) int {
	total := len(symbols)
	for _, symbol := range symbols {
		total += countGoldenSymbols(symbol.Children)
	}
	return total
}

func collectGoldenKinds(symbols []outlineGoldenSymbol, into map[string]bool) {
	for _, symbol := range symbols {
		into[symbol.Kind] = true
		collectGoldenKinds(symbol.Children, into)
	}
}

func collectGoldenNames(symbols []outlineGoldenSymbol, into map[string]bool) {
	for _, symbol := range symbols {
		into[symbol.Name] = true
		collectGoldenNames(symbol.Children, into)
	}
}

func goldenDepth(symbols []outlineGoldenSymbol) int {
	best := 0
	for _, symbol := range symbols {
		depth := 1 + goldenDepth(symbol.Children)
		if depth > best {
			best = depth
		}
	}
	return best
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
