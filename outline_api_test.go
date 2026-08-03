package gotreesitter_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// outlineGoReceiverRule is the Go rule the specification writes out. Stage 1
// ships no owner resolution, so this rule is used only to prove that supplying
// it changes nothing.
var outlineGoReceiverRule = gts.OutlineOwnerRule{
	NodeType:   "method_declaration",
	OwnerField: "receiver",
	Unwrap:     []string{"parameter_list", "parameter_declaration", "pointer_type", "generic_type"},
	NameTypes:  []string{"type_identifier"},
}

// loadOutlineFixture parses one committed fixture and returns its tree, its
// language, and its resolved tags query.
func loadOutlineFixture(t *testing.T, language, file string) (*gts.Tree, *gts.Language, string) {
	t.Helper()

	entry := grammars.DetectLanguageByName(language)
	if entry == nil {
		t.Fatalf("language %q is not registered", language)
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatalf("language %q failed to load", language)
	}
	query := grammars.ResolveTagsQuery(*entry)

	source, err := os.ReadFile(filepath.Join(outlineGoldenRoot, file))
	if err != nil {
		t.Fatalf("read fixture %s: %v", file, err)
	}
	parser := gts.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	t.Cleanup(tree.Release)
	return tree, lang, query
}

func TestNewOutlinerRejectsNilLanguage(t *testing.T) {
	if _, err := gts.NewOutliner(nil, "(x) @definition.function"); err == nil {
		t.Fatal("NewOutliner accepted a nil language")
	}
}

func TestNewOutlinerRejectsBrokenQuery(t *testing.T) {
	_, lang, _ := loadOutlineFixture(t, "go", "go/service.go.fixture")
	if _, err := gts.NewOutliner(lang, "(this_node_type_does_not_exist) @definition.function"); err == nil {
		t.Fatal("NewOutliner accepted a query the language cannot compile")
	}
}

func TestNewOutlinerRejectsUnusableOwnerRules(t *testing.T) {
	_, lang, query := loadOutlineFixture(t, "go", "go/service.go.fixture")
	cases := []struct {
		name string
		rule gts.OutlineOwnerRule
	}{
		{"no node type", gts.OutlineOwnerRule{OwnerField: "receiver"}},
		{"no owner field", gts.OutlineOwnerRule{NodeType: "method_declaration"}},
	}
	for _, tc := range cases {
		if _, err := gts.NewOutliner(lang, query, gts.WithOutlineOwnerRules([]gts.OutlineOwnerRule{tc.rule})); err == nil {
			t.Errorf("%s: NewOutliner accepted an owner rule that can never resolve an owner", tc.name)
		}
	}
}

// TestOutlineDeclinesWithoutTagsQuery proves requirement 5 of the issue: a
// language with no tags data does not fail and does not pretend to be empty.
// It declines, and the receipt says so.
func TestOutlineDeclinesWithoutTagsQuery(t *testing.T) {
	tree, lang, _ := loadOutlineFixture(t, "go", "go/service.go.fixture")

	for _, query := range []string{"", "   ", "\n\t "} {
		outliner, err := gts.NewOutliner(lang, query)
		if err != nil {
			t.Fatalf("NewOutliner(%q) = %v, want a declining outliner and no error", query, err)
		}
		if !outliner.QueryEmpty() {
			t.Errorf("NewOutliner(%q) did not record an empty query", query)
		}
		symbols, report := outliner.OutlineTree(tree)
		if len(symbols) != 0 {
			t.Errorf("NewOutliner(%q) produced %d symbols, want none", query, len(symbols))
		}
		if !report.QueryEmpty {
			t.Errorf("NewOutliner(%q) report.QueryEmpty = false, want true", query)
		}
		if report.Symbols != 0 || report.Omitted() != 0 {
			t.Errorf("NewOutliner(%q) report = %+v, want an all-zero receipt beside QueryEmpty", query, report)
		}
	}
}

// TestOutlineDeclinesForeignTree proves the outliner does not run a query
// whose symbol identifiers belong to another grammar. It declines and reports.
func TestOutlineDeclinesForeignTree(t *testing.T) {
	pythonTree, _, _ := loadOutlineFixture(t, "python", "python/app.py")
	_, goLang, goQuery := loadOutlineFixture(t, "go", "go/service.go.fixture")

	outliner, err := gts.NewOutliner(goLang, goQuery)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	symbols, report := outliner.OutlineTree(pythonTree)
	if len(symbols) != 0 {
		t.Errorf("outlining a foreign tree produced %d symbols, want none", len(symbols))
	}
	if !report.LanguageMismatch {
		t.Error("report.LanguageMismatch = false, want true")
	}
}

func TestOutlineHandlesNilAndEmptyInput(t *testing.T) {
	_, lang, query := loadOutlineFixture(t, "go", "go/service.go.fixture")
	outliner, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}

	symbols, report := outliner.OutlineTree(nil)
	if len(symbols) != 0 || report.Symbols != 0 {
		t.Errorf("OutlineTree(nil) = %+v / %+v, want empty", symbols, report)
	}

	var nilOutliner *gts.Outliner
	if symbols, report := nilOutliner.OutlineTree(nil); len(symbols) != 0 || report.Symbols != 0 {
		t.Errorf("nil outliner returned %+v / %+v, want empty", symbols, report)
	}
	if nilOutliner.QueryEmpty() {
		t.Error("nil outliner reported an empty query rather than staying inert")
	}
	if nilOutliner.Language() != nil {
		t.Error("nil outliner returned a language")
	}
}

// TestOutlineOwnerStaysEmpty is the stage-1 ownership proof. It supplies the
// exact Go receiver rule from the specification, outlines a fixture whose
// methods all carry receivers, and asserts that every Owner is still empty and
// that no owner-rule miss was recorded. Ownership is field-derived and ships in
// its own change, behind its own differential gate.
func TestOutlineOwnerStaysEmpty(t *testing.T) {
	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			tree, lang, query := loadOutlineFixture(t, fixture.Language, fixture.File)

			withRules, err := gts.NewOutliner(lang, query,
				gts.WithOutlineOwnerRules([]gts.OutlineOwnerRule{outlineGoReceiverRule}))
			if err != nil {
				t.Fatalf("build outliner with owner rules: %v", err)
			}
			withoutRules, err := gts.NewOutliner(lang, query)
			if err != nil {
				t.Fatalf("build outliner without owner rules: %v", err)
			}

			ruled, ruledReport := withRules.OutlineTree(tree)
			plain, plainReport := withoutRules.OutlineTree(tree)

			if ruledReport != plainReport {
				t.Errorf("owner rules changed the receipt: %+v with rules, %+v without", ruledReport, plainReport)
			}
			if !outlineSymbolsEqual(ruled, plain) {
				t.Error("owner rules changed the symbol forest; stage 1 must not resolve ownership")
			}
			if ruledReport.OwnerRuleMisses != 0 {
				t.Errorf("OwnerRuleMisses = %d, want 0 while ownership does not run", ruledReport.OwnerRuleMisses)
			}
			owners := collectOutlineOwners(ruled)
			if len(owners) != 0 {
				t.Errorf("Owner is populated on %d symbols (%v); stage 1 must leave it empty", len(owners), owners)
			}
		})
	}
}

// TestOutlineGoFixtureHasReceiverMethods proves the ownership test above is
// not vacuous: the Go fixture really does hold methods whose receiver field a
// later owner rule would read.
func TestOutlineGoFixtureHasReceiverMethods(t *testing.T) {
	tree, lang, query := loadOutlineFixture(t, "go", "go/service.go.fixture")
	outliner, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	symbols, _ := outliner.OutlineTree(tree)

	methods := 0
	withReceiverField := 0
	for _, symbol := range symbols {
		if symbol.Kind != "method" {
			continue
		}
		methods++
		node := tree.NamedNodeAtByte(symbol.Range.StartByte)
		for ; node != nil; node = node.Parent() {
			if node.Type(lang) != symbol.NodeType {
				continue
			}
			if node.ChildByFieldName("receiver", lang) != nil {
				withReceiverField++
			}
			break
		}
	}
	if methods == 0 {
		t.Fatal("the Go fixture holds no method symbols, so the ownership proof is vacuous")
	}
	if withReceiverField != methods {
		t.Errorf("%d of %d method symbols expose a receiver field; the ownership stage needs them all", withReceiverField, methods)
	}
}

// TestOutlinerHoldsNoParser proves structurally that an Outliner cannot parse:
// no field it holds, at any depth, is a parser.
func TestOutlinerHoldsNoParser(t *testing.T) {
	parserType := reflect.TypeOf(&gts.Parser{})
	outlinerType := reflect.TypeOf(gts.Outliner{})

	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, path string) {
		if typ == nil || seen[typ] {
			return
		}
		seen[typ] = true
		if typ == parserType || typ == parserType.Elem() {
			t.Errorf("Outliner reaches a parser through %s", path)
			return
		}
		switch typ.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Array, reflect.Chan:
			walk(typ.Elem(), path+".*")
		case reflect.Map:
			walk(typ.Key(), path+".key")
			walk(typ.Elem(), path+".value")
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				walk(field.Type, path+"."+field.Name)
			}
		}
	}
	walk(outlinerType, "Outliner")
}

// TestOutlineReportsTruncationUnderAMatchLimit proves the truncation receipt
// is real: a limit low enough to bite sets Truncated, and the default does
// not.
func TestOutlineReportsTruncationUnderAMatchLimit(t *testing.T) {
	tree, lang, query := loadOutlineFixture(t, "python", "python/app.py")

	full, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	fullSymbols, fullReport := full.OutlineTree(tree)
	if fullReport.Truncated {
		t.Fatal("the fixture truncates at the default budget; shrink it")
	}
	if len(fullSymbols) == 0 {
		t.Fatal("the fixture produced no symbols")
	}

	limited, err := gts.NewOutliner(lang, query, gts.WithOutlineMatchLimit(2))
	if err != nil {
		t.Fatalf("build limited outliner: %v", err)
	}
	limitedSymbols, limitedReport := limited.OutlineTree(tree)
	if !limitedReport.Truncated {
		t.Errorf("a match limit of 2 did not set Truncated; report = %+v", limitedReport)
	}
	if limitedReport.Symbols > fullReport.Symbols {
		t.Errorf("a truncated outline reports %d symbols, more than the full %d", limitedReport.Symbols, fullReport.Symbols)
	}
	_ = limitedSymbols
}

// TestOutlineReceiptBalancesOnEveryFixture asserts the accounting identity
// against live extraction, not against the committed goldens.
func TestOutlineReceiptBalancesOnEveryFixture(t *testing.T) {
	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			tree, lang, query := loadOutlineFixture(t, fixture.Language, fixture.File)
			outliner, err := gts.NewOutliner(lang, query)
			if err != nil {
				t.Fatalf("build outliner: %v", err)
			}
			symbols, report := outliner.OutlineTree(tree)

			if got := countLiveOutlineSymbols(symbols); got != report.Symbols {
				t.Errorf("forest holds %d symbols, receipt says %d", got, report.Symbols)
			}
			if report.Candidates() != report.Symbols+report.Omitted() {
				t.Errorf("receipt does not balance: %+v", report)
			}
			assertOutlineForestIsWellFormed(t, symbols, nil)
		})
	}
}

// assertOutlineForestIsWellFormed checks the two structural promises of the
// forest: every child sits inside its parent, and siblings run in source order
// without overlap.
func assertOutlineForestIsWellFormed(t *testing.T, symbols []gts.OutlineSymbol, parent *gts.OutlineSymbol) {
	t.Helper()
	var previous *gts.OutlineSymbol
	for i := range symbols {
		symbol := symbols[i]
		if symbol.NameRange.StartByte < symbol.Range.StartByte || symbol.NameRange.EndByte > symbol.Range.EndByte {
			t.Errorf("%s %q: name span %d-%d is outside the definition span %d-%d",
				symbol.Kind, symbol.Name,
				symbol.NameRange.StartByte, symbol.NameRange.EndByte,
				symbol.Range.StartByte, symbol.Range.EndByte)
		}
		if parent != nil {
			if symbol.Range.StartByte < parent.Range.StartByte || symbol.Range.EndByte > parent.Range.EndByte {
				t.Errorf("%s %q is not contained in its parent %q", symbol.Kind, symbol.Name, parent.Name)
			}
		}
		if previous != nil {
			if symbol.Range.StartByte < previous.Range.StartByte {
				t.Errorf("%s %q precedes its earlier sibling %q; siblings must run in source order",
					symbol.Kind, symbol.Name, previous.Name)
			}
			if symbol.Range.StartByte < previous.Range.EndByte {
				t.Errorf("%s %q overlaps its sibling %q", symbol.Kind, symbol.Name, previous.Name)
			}
		}
		previous = &symbols[i]
		assertOutlineForestIsWellFormed(t, symbol.Children, &symbols[i])
	}
}

func collectOutlineOwners(symbols []gts.OutlineSymbol) []string {
	var out []string
	for _, symbol := range symbols {
		if strings.TrimSpace(symbol.Owner) != "" {
			out = append(out, symbol.Name+"="+symbol.Owner)
		}
		out = append(out, collectOutlineOwners(symbol.Children)...)
	}
	return out
}

func countLiveOutlineSymbols(symbols []gts.OutlineSymbol) int {
	total := len(symbols)
	for _, symbol := range symbols {
		total += countLiveOutlineSymbols(symbol.Children)
	}
	return total
}
