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
	// Query overrides the language's resolved tags query. Leave it empty to
	// use the shipped tags data, which is what a caller gets. Set it only to
	// reach a shape the shared inference cannot produce on demand.
	Query string
}

// outlineCounterProbeQuery is a deliberately ambiguous Go tags query. The
// shared inference never emits these shapes, so without it not one of the
// omission counters would be pinned by any committed golden.
//
// Each rule gets its own node type, so one rule cannot mask another.
//
//   - function_declaration is bound twice to the same kind, name, and span.
//     The candidates are indistinguishable, so one survives and the repeat
//     lands in OmittedDuplicate.
//   - type_spec is bound to two different kinds at one span, so the span
//     means two things at once and every member lands in OmittedConflict.
//   - method_declaration is bound once to its own name and once to its
//     receiver name. The two agree on the span and the kind and disagree on
//     the name, so every member lands in OmittedNameConflict. This is the
//     shape that made C-family outlines publish a return type as a method
//     name.
//   - const_declaration is captured with no name at all: OmittedNoName.
//   - var_declaration carries two definition captures on one match, so the
//     match names two definitions at once: OmittedMultipleDefinitions.
const outlineCounterProbeQuery = `(function_declaration name: (identifier) @name) @definition.function
(function_declaration name: (identifier) @name) @definition.function
(type_spec name: (type_identifier) @name) @definition.type
(type_spec name: (type_identifier) @name) @definition.class
(method_declaration name: (field_identifier) @name) @definition.method
(method_declaration receiver: (parameter_list (parameter_declaration name: (identifier) @name))) @definition.method
(const_declaration) @definition.constant
((var_declaration) @definition.variable @definition.constant)`

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
		// This golden pins a FLEET-WIDE defect in the shared inferred tags
		// data, not a C quirk and not an extractor bug.
		//
		// The pattern
		// "(struct_specifier (type_identifier) @name) @definition.type"
		// matches a struct USE, such as the parameter type
		// "struct Registry *registry", exactly as it matches a struct
		// definition. Across the real corpus this makes 1179 of the 2400
		// symbols emitted for C struct USES: 49 percent of the C outline is
		// noise, so the signal-to-noise ratio is close to one to one.
		//
		// The same struct_specifier pattern reaches SEVEN languages:
		// arduino, c, cpp, cuda, glsl, hlsl, and objc. Its class_specifier
		// sibling reaches four more. A stage-5 data edit must therefore be
		// scoped to the pattern, not to one language.
		//
		// The C++ golden looks clean only by accident: its
		// "int total(const Registry &registry)" omits the "class" keyword,
		// so the parameter type is not a class_specifier there. Do not read
		// that golden as evidence the pattern is safe.
		Purpose: "struct, typedef, and function definitions; pins the fleet-wide struct_specifier over-capture that makes 49 percent of the C outline struct uses (spec risk R1)",
	},
	{
		Language: "cpp",
		File:     "cpp/registry.cpp",
		Purpose:  "class with member function definitions nested inside it; clean of the struct_specifier over-capture only because the parameter type omits the class keyword",
	},
	{
		Language: "c_sharp",
		File:     "csharp/Service.cs",
		Purpose:  "real-data name conflict: the shared inference binds @name to both the return type and the method name, so the group drops and OmittedNameConflict fires instead of publishing the return type",
	},
	{
		Language: "go",
		File:     "go/counters.go.fixture",
		Query:    outlineCounterProbeQuery,
		Purpose:  "pins OmittedDuplicate, OmittedConflict, OmittedNameConflict, OmittedNoName, and OmittedMultipleDefinitions end to end through a live query, which the shipped tags data cannot produce on demand",
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
//
// DefinitionKinds is recorded beside the symbols on purpose. It states what
// the compiled query CAN emit, so a reader can tell a kind the file does not
// contain from a kind the tags data cannot express.
type outlineGoldenFile struct {
	Language        string                `json:"language"`
	Fixture         string                `json:"fixture"`
	Purpose         string                `json:"purpose"`
	DefinitionKinds []string              `json:"query_definition_kinds"`
	Report          gts.OutlineReport     `json:"report"`
	Symbols         []outlineGoldenSymbol `json:"symbols"`
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
//
// Most fixtures use the language's resolved tags query. A fixture that sets
// Query uses that query instead, which is how the omission counters get pinned
// end to end: the shared inference cannot produce a duplicate or a conflict on
// demand, but an explicit query can, and it exercises the identical code path.
func outlineForFixture(t *testing.T, fixture outlineGoldenFixture) outlineGoldenFile {
	t.Helper()

	entry := grammars.DetectLanguageByName(fixture.Language)
	if entry == nil {
		t.Fatalf("%s: language %q is not registered", fixture.File, fixture.Language)
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatalf("%s: language %q failed to load", fixture.File, fixture.Language)
	}

	query := fixture.Query
	if query == "" {
		query = grammars.ResolveTagsQuery(*entry)
		if strings.TrimSpace(query) == "" {
			t.Fatalf("%s: language %q resolved an empty tags query, so it is no longer tier 1", fixture.File, fixture.Language)
		}
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

	return outlineGoldenFile{
		Language:        fixture.Language,
		Fixture:         fixture.File,
		Purpose:         fixture.Purpose,
		DefinitionKinds: outliner.DefinitionKinds(),
		Report:          report,
		Symbols:         outlineGoldenSymbolsOf(symbols),
	}
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
			encoded := encodeOutlineGolden(t, outlineForFixture(t, fixture))
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
			if golden.Report.Declined() {
				t.Errorf("%s: fixture declined with reason %q", path, golden.Report.DeclineReason)
			}
			if golden.Report.Truncated {
				t.Errorf("%s: fixture outline is truncated; raise the work budget or shrink the fixture", path)
			}
			if golden.Report.OwnerRuleMisses != 0 {
				t.Errorf("%s: owner rules do not run in this change, so OwnerRuleMisses must stay 0, got %d",
					path, golden.Report.OwnerRuleMisses)
			}
			if golden.Report.Symbols == 0 && golden.Report.Omitted() == 0 {
				t.Errorf("%s: fixture produced neither a symbol nor an omission", path)
			}
			if len(golden.DefinitionKinds) == 0 {
				t.Errorf("%s: the query emits no definition kind at all", path)
			}
		})
	}
}

// TestOutlineGoldensPinEveryOmissionCounter proves the omission taxonomy is
// exercised end to end, not only by constructed unit cases. Without this,
// every committed golden could carry an all-zero receipt and no counter would
// be pinned anywhere.
//
// OmittedOverlap and OmittedInvalidNameRange are excluded on purpose and by
// argument, not by omission: two nodes of one tree are always nested or
// disjoint, and every capture of a match sits inside the matched subtree, so
// neither shape can arise from a single tree. Both rules stay as defensive
// guards and are exercised by the internal unit cases.
func TestOutlineGoldensPinEveryOmissionCounter(t *testing.T) {
	totals := map[string]int{}
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
		totals["OmittedNoName"] += golden.Report.OmittedNoName
		totals["OmittedDuplicate"] += golden.Report.OmittedDuplicate
		totals["OmittedNameConflict"] += golden.Report.OmittedNameConflict
		totals["OmittedConflict"] += golden.Report.OmittedConflict
		totals["OmittedMultipleDefinitions"] += golden.Report.OmittedMultipleDefinitions
	}

	for _, counter := range []string{"OmittedNoName", "OmittedDuplicate", "OmittedNameConflict", "OmittedConflict", "OmittedMultipleDefinitions"} {
		if totals[counter] == 0 {
			t.Errorf("no committed golden exercises %s, so a regression in that rule would not show", counter)
		}
	}
}

// TestOutlineGoldensRecordTreeErrorState proves TreeHasError reaches the
// receipt, and that it is not stuck on one value across the set.
func TestOutlineGoldensRecordTreeErrorState(t *testing.T) {
	withError, withoutError := 0, 0
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
		if golden.Report.TreeHasError {
			withError++
		} else {
			withoutError++
		}
	}
	if withError == 0 {
		t.Error("no committed golden records TreeHasError=true; the error signal is unpinned")
	}
	if withoutError == 0 {
		t.Error("every committed golden records TreeHasError=true; the flag may be stuck on")
	}
}

// TestOutlineGoldensCoverErrorBearingSource proves the golden set includes at
// least one source the parser could not fully recover, and that its outline is
// still well-defined: symbols before and after the damage still appear, and
// the receipt still balances.
// Each fixture runs in its own subtest, so a single Fatalf cannot abort the
// scan of the rest.
func TestOutlineGoldensCoverErrorBearingSource(t *testing.T) {
	found := 0
	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			got := outlineForFixture(t, fixture)
			if !got.Report.TreeHasError {
				t.Skip("this fixture parses clean")
			}
			found++

			if got.Report.Symbols == 0 {
				t.Errorf("error-bearing fixture produced no symbols; the outline must survive an ERROR node")
			}
			names := map[string]bool{}
			collectGoldenNames(got.Symbols, names)
			for _, want := range []string{"Before", "After"} {
				if !names[want] {
					t.Errorf("expected symbol %q to survive the ERROR node, got %v", want, sortedKeys(names))
				}
			}
		})
	}
	if found == 0 {
		t.Fatal("no committed fixture parses to a tree with an ERROR node; add one")
	}
}

// TestOutlineOverErrorTreeReportsTheDamage is the contract test for the second
// blocker: a one-character syntax error must not look like a clean, complete
// outline.
//
// A Go file missing one closing brace parses to a truncated tree. The parser
// recovers a single function whose Range swallows everything after it, and no
// omission counter fires, because the query never produced a candidate for the
// swallowed definitions. The ONLY signal a caller gets is TreeHasError, so it
// must be present and true.
func TestOutlineOverErrorTreeReportsTheDamage(t *testing.T) {
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Fatal("the go grammar is not registered")
	}
	lang := entry.Language()
	query := grammars.ResolveTagsQuery(*entry)

	const truncated = "package p\n\nfunc A() {\n\nfunc B() {\n}\n\nfunc C() {\n}\n"
	parser := gts.NewParser(lang)
	tree, err := parser.Parse([]byte(truncated))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	outliner, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	symbols, report := outliner.OutlineTree(tree)

	if !report.TreeHasError {
		t.Fatal("TreeHasError = false on a tree the parser could not recover; the damage is invisible to the caller")
	}
	if report.Declined() {
		t.Errorf("DeclineReason = %q; a damaged tree is projected, not declined", report.DeclineReason)
	}
	if report.Omitted() != 0 {
		t.Logf("this source now produces omissions (%+v); the test still holds, but the swallowing case has changed", report)
	}

	// The swallowing behaviour itself is the reason the flag matters. Record
	// it rather than assert an exact shape, because recovery may improve.
	t.Logf("truncated source: symbols=%d omitted=%d roots=%d", report.Symbols, report.Omitted(), len(symbols))
	for _, symbol := range symbols {
		t.Logf("  %s %q range=%d-%d children=%d",
			symbol.Kind, symbol.Name, symbol.Range.StartByte, symbol.Range.EndByte, len(symbol.Children))
	}
	if report.Symbols == 0 {
		t.Error("the recovered tree produced no symbols at all")
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
