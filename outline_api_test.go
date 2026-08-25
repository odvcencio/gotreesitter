package gotreesitter_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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
		if report.DeclineReason != gts.OutlineDeclineQueryEmpty {
			t.Errorf("NewOutliner(%q) DeclineReason = %q, want %q", query, report.DeclineReason, gts.OutlineDeclineQueryEmpty)
		}
		if !report.Declined() {
			t.Errorf("NewOutliner(%q) Declined() = false, want true", query)
		}
		if report.Symbols != 0 || report.Omitted() != 0 {
			t.Errorf("NewOutliner(%q) report = %+v, want an all-zero receipt beside the decline", query, report)
		}
	}
}

func TestOutlineBoundMatchesOutlineTree(t *testing.T) {
	tree, lang, query := loadOutlineFixture(t, "go", "go/service.go.fixture")
	outliner, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("NewOutliner failed: %v", err)
	}

	wantSymbols, wantReport := outliner.OutlineTree(tree)
	gotSymbols, gotReport := outliner.OutlineBound(gts.Bind(tree))
	if !reflect.DeepEqual(gotSymbols, wantSymbols) || gotReport != wantReport {
		t.Fatalf("OutlineBound = (%#v, %#v), want (%#v, %#v)", gotSymbols, gotReport, wantSymbols, wantReport)
	}

	_, nilReport := outliner.OutlineBound(nil)
	if nilReport.DeclineReason != gts.OutlineDeclineNilTree {
		t.Fatalf("OutlineBound(nil) decline = %q, want %q", nilReport.DeclineReason, gts.OutlineDeclineNilTree)
	}
}

// TestOutlineDeclineReasonsAreDistinguishable is the fix for a receipt shape
// that made four different failures byte identical. A caller must be able to
// tell a genuinely empty file from a projection that never ran.
func TestOutlineDeclineReasonsAreDistinguishable(t *testing.T) {
	goTree, goLang, goQuery := loadOutlineFixture(t, "go", "go/service.go.fixture")
	pythonTree, _, _ := loadOutlineFixture(t, "python", "python/app.py")

	working, err := gts.NewOutliner(goLang, goQuery)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	declining, err := gts.NewOutliner(goLang, "")
	if err != nil {
		t.Fatalf("build declining outliner: %v", err)
	}

	// An empty source file: the query RAN and matched nothing. This is the
	// case that must not look like a decline.
	emptyParser := gts.NewParser(goLang)
	emptyTree, err := emptyParser.Parse([]byte("package empty\n"))
	if err != nil {
		t.Fatalf("parse empty source: %v", err)
	}
	defer emptyTree.Release()

	var nilOutliner *gts.Outliner

	cases := []struct {
		name       string
		reason     string
		run        func() (int, gts.OutlineReport)
		wantSymbol bool
	}{
		{
			name:   "nil outliner",
			reason: gts.OutlineDeclineNilOutliner,
			run:    func() (int, gts.OutlineReport) { s, r := nilOutliner.OutlineTree(goTree); return len(s), r },
		},
		{
			name:   "empty query",
			reason: gts.OutlineDeclineQueryEmpty,
			run:    func() (int, gts.OutlineReport) { s, r := declining.OutlineTree(goTree); return len(s), r },
		},
		{
			name:   "nil tree",
			reason: gts.OutlineDeclineNilTree,
			run:    func() (int, gts.OutlineReport) { s, r := working.OutlineTree(nil); return len(s), r },
		},
		{
			name:   "foreign tree",
			reason: gts.OutlineDeclineLanguageMismatch,
			run:    func() (int, gts.OutlineReport) { s, r := working.OutlineTree(pythonTree); return len(s), r },
		},
		{
			name:   "empty file, query ran",
			reason: "",
			run:    func() (int, gts.OutlineReport) { s, r := working.OutlineTree(emptyTree); return len(s), r },
		},
	}

	seen := map[string]string{}
	for _, tc := range cases {
		count, report := tc.run()
		if count != 0 {
			t.Errorf("%s: produced %d symbols, want none", tc.name, count)
		}
		if report.DeclineReason != tc.reason {
			t.Errorf("%s: DeclineReason = %q, want %q", tc.name, report.DeclineReason, tc.reason)
		}
		if report.Declined() != (tc.reason != "") {
			t.Errorf("%s: Declined() = %v, want %v", tc.name, report.Declined(), tc.reason != "")
		}
		if previous, ok := seen[report.DeclineReason]; ok {
			t.Errorf("%s and %s produce the same receipt %q; a caller cannot tell them apart",
				tc.name, previous, report.DeclineReason)
		}
		seen[report.DeclineReason] = tc.name
	}

	if len(seen) != len(cases) {
		t.Errorf("%d cases collapsed into %d distinguishable receipts", len(cases), len(seen))
	}
}

func TestOutlineHandlesNilAndEmptyInput(t *testing.T) {
	var nilOutliner *gts.Outliner
	if nilOutliner.QueryEmpty() {
		t.Error("nil outliner reported an empty query rather than staying inert")
	}
	if nilOutliner.Language() != nil {
		t.Error("nil outliner returned a language")
	}
	if nilOutliner.DefinitionKinds() != nil {
		t.Error("nil outliner returned definition kinds")
	}
}

// TestOutlineDefinitionKindsBoundWhatTheOutlineCanContain covers the reading
// trap behind "every counter is zero". The Go tags override carries no type,
// constant, or variable pattern, so a Go outline can only ever hold functions
// and methods, however many types the file declares.
func TestOutlineDefinitionKindsBoundWhatTheOutlineCanContain(t *testing.T) {
	tree, lang, query := loadOutlineFixture(t, "go", "go/service.go.fixture")
	outliner, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}

	kinds := outliner.DefinitionKinds()
	want := []string{"function", "method"}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("go DefinitionKinds() = %v, want %v", kinds, want)
	}

	symbols, report := outliner.OutlineTree(tree)
	allowed := map[string]bool{}
	for _, kind := range kinds {
		allowed[kind] = true
	}
	var check func([]gts.OutlineSymbol)
	check = func(list []gts.OutlineSymbol) {
		for _, symbol := range list {
			if !allowed[symbol.Kind] {
				t.Errorf("symbol %q has kind %q, which DefinitionKinds() does not list", symbol.Name, symbol.Kind)
			}
			check(symbol.Children)
		}
	}
	check(symbols)

	// The fixture declares a struct, an interface, and a constant. None
	// reaches a candidate, so none reaches an omission counter either. That
	// is the fact DefinitionKinds exists to make visible.
	if report.Omitted() != 0 {
		t.Errorf("Omitted() = %d, want 0 for this fixture", report.Omitted())
	}
	source, err := os.ReadFile(filepath.Join(outlineGoldenRoot, "go/service.go.fixture"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	for _, declared := range []string{"type Registry struct", "type Named interface", "const defaultName"} {
		if !strings.Contains(string(source), declared) {
			t.Fatalf("fixture no longer declares %q, so this test proves nothing", declared)
		}
	}
	if report.Symbols != 4 {
		t.Errorf("Symbols = %d, want 4: the query reaches only functions and methods", report.Symbols)
	}
}

// TestOutlineTreeIsSafeForConcurrentUse backs the concurrency promise in the
// Outliner documentation. Under the race detector this fails loudly if
// OutlineTree writes shared state.
func TestOutlineTreeIsSafeForConcurrentUse(t *testing.T) {
	tree, lang, query := loadOutlineFixture(t, "python", "python/app.py")
	outliner, err := gts.NewOutliner(lang, query)
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	wantSymbols, wantReport := outliner.OutlineTree(tree)
	if wantReport.Symbols == 0 {
		t.Fatal("the fixture produced no symbols")
	}

	const goroutines = 16
	const callsEach = 20
	var wg sync.WaitGroup
	errs := make(chan string, goroutines*callsEach)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < callsEach; i++ {
				symbols, report := outliner.OutlineTree(tree)
				if report != wantReport {
					errs <- fmt.Sprintf("receipt diverged: %+v", report)
					return
				}
				if !outlineSymbolsEqual(symbols, wantSymbols) {
					errs <- "symbols diverged under concurrent use"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
}

// outlineGoFixtureOwners names, for each committed Go outline fixture, the
// "Name=Owner" pairs collectOutlineOwners must read back once
// outlineGoReceiverRule resolves every method symbol's receiver. Every
// committed Go fixture uses a pointer receiver;
// TestOutlineOwnerResolvesEveryGoReceiverShape below covers the value and
// generic shapes this committed set does not exercise.
var outlineGoFixtureOwners = map[string][]string{
	"go/service.go.fixture":  {"Add=Registry", "Name=Registry"},
	"go/broken.go.fixture":   {"After=Holder"},
	"go/counters.go.fixture": {"Value=Holder"},
}

// TestOutlineOwnerResolvesGoReceiver is the ownership proof. It supplies the
// exact Go receiver rule from the specification, outlines every committed Go
// fixture, and asserts that ownership resolves the receiver type of every
// method, changes no other field, and records no miss.
func TestOutlineOwnerResolvesGoReceiver(t *testing.T) {
	for _, fixture := range outlineGoldenFixtures {
		if fixture.Language != "go" {
			continue
		}
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
			if ruledReport.OwnerRuleMisses != 0 {
				t.Errorf("OwnerRuleMisses = %d, want 0: every method in this fixture carries a resolvable receiver", ruledReport.OwnerRuleMisses)
			}
			if !outlineSymbolsEqual(stripOutlineOwners(ruled), plain) {
				t.Error("owner rules changed a field other than Owner")
			}

			want := outlineGoFixtureOwners[fixture.File]
			got := collectOutlineOwners(ruled)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("owners = %v, want %v", got, want)
			}
		})
	}
}

// TestOutlineOwnerResolvesEveryGoReceiverShape extends the fixture proof
// above to the receiver shapes no committed Go fixture exercises: a value
// receiver, a generic type's value receiver, and a generic type's pointer
// receiver. It parses a small snippet directly instead of reading a
// committed fixture, so covering all four shapes costs no fixture edit.
func TestOutlineOwnerResolvesEveryGoReceiverShape(t *testing.T) {
	const src = `package shapes

type Value struct{}
type Box[T any] struct{ item T }

func (v Value) ByValue() int { return 0 }
func (v *Value) ByPointer() int { return 0 }
func (b Box[T]) GenericByValue() int { return 0 }
func (b *Box[T]) GenericByPointer() int { return 0 }
`
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Fatal("the go grammar is not registered")
	}
	lang := entry.Language()
	query := grammars.ResolveTagsQuery(*entry)

	parser := gts.NewParser(lang)
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	outliner, err := gts.NewOutliner(lang, query,
		gts.WithOutlineOwnerRules([]gts.OutlineOwnerRule{outlineGoReceiverRule}))
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	symbols, report := outliner.OutlineTree(tree)
	if report.OwnerRuleMisses != 0 {
		t.Errorf("OwnerRuleMisses = %d, want 0", report.OwnerRuleMisses)
	}

	want := map[string]string{
		"ByValue":          "Value",
		"ByPointer":        "Value",
		"GenericByValue":   "Box",
		"GenericByPointer": "Box",
	}
	got := map[string]string{}
	for _, symbol := range symbols {
		if symbol.Kind == "method" {
			got[symbol.Name] = symbol.Owner
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("owners = %v, want %v", got, want)
	}
}

// TestOutlineOwnerRuleFailsClosedWhenFieldIsAbsent is the fail-closed witness:
// a rule whose NodeType matches a symbol but whose OwnerField the language
// does not carry must leave Owner empty and count the miss, never guess.
// Java's grammar names its method node "method_declaration" too, exactly
// like Go's, but carries no "receiver" field, so applying the Go receiver
// rule to real Java source is a genuine matched-node-type, absent-field
// miss, not a constructed one.
func TestOutlineOwnerRuleFailsClosedWhenFieldIsAbsent(t *testing.T) {
	tree, lang, query := loadOutlineFixture(t, "java", "java/Service.java")

	outliner, err := gts.NewOutliner(lang, query,
		gts.WithOutlineOwnerRules([]gts.OutlineOwnerRule{outlineGoReceiverRule}))
	if err != nil {
		t.Fatalf("build outliner with owner rules: %v", err)
	}
	symbols, report := outliner.OutlineTree(tree)

	methodSymbols := 0
	var check func([]gts.OutlineSymbol)
	check = func(list []gts.OutlineSymbol) {
		for _, symbol := range list {
			if symbol.NodeType == "method_declaration" {
				methodSymbols++
				if symbol.Owner != "" {
					t.Errorf("%s: Owner = %q, want empty: java carries no receiver field", symbol.Name, symbol.Owner)
				}
			}
			check(symbol.Children)
		}
	}
	check(symbols)

	if methodSymbols == 0 {
		t.Fatal("the fixture holds no method_declaration symbol, so this fail-closed proof is vacuous")
	}
	if report.OwnerRuleMisses != methodSymbols {
		t.Errorf("OwnerRuleMisses = %d, want %d (one per method_declaration symbol)", report.OwnerRuleMisses, methodSymbols)
	}
}

// TestOutlineGoFixtureHasReceiverMethods proves
// TestOutlineOwnerResolvesGoReceiver is not vacuous: the Go fixture really
// does hold methods whose receiver field the owner rule reads.
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

// TestOutlineReceiptMatchesIndependentGroundTruth is the real accounting gate.
//
// Comparing Candidates() to Symbols + Omitted() proves nothing, because
// Candidates() is DEFINED as that sum. This test instead runs the same
// compiled query a second time, independently of the outliner, counts the
// matches that carry exactly one definition capture, and compares that ground
// truth to the receipt. A candidate that vanished without a counter fails
// here.
func TestOutlineReceiptMatchesIndependentGroundTruth(t *testing.T) {
	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			tree, lang, query := loadOutlineFixture(t, fixture.Language, fixture.File)
			outliner, err := gts.NewOutliner(lang, query)
			if err != nil {
				t.Fatalf("build outliner: %v", err)
			}
			symbols, report := outliner.OutlineTree(tree)

			singleDefinition, multipleDefinitions := countDefinitionBearingMatches(t, lang, query, tree)
			if singleDefinition == 0 {
				t.Fatal("the query produced no definition-bearing match, so this gate is vacuous")
			}
			if report.Candidates() != singleDefinition+multipleDefinitions {
				t.Errorf("the query produced %d definition-bearing matches (%d single, %d multiple) but the receipt accounts for %d: symbols=%d omitted=%d %+v",
					singleDefinition+multipleDefinitions, singleDefinition, multipleDefinitions,
					report.Candidates(), report.Symbols, report.Omitted(), report)
			}
			if report.OmittedMultipleDefinitions != multipleDefinitions {
				t.Errorf("OmittedMultipleDefinitions = %d, ground truth %d", report.OmittedMultipleDefinitions, multipleDefinitions)
			}
			if got := countLiveOutlineSymbols(symbols); got != report.Symbols {
				t.Errorf("forest holds %d symbols, receipt says %d", got, report.Symbols)
			}
			assertOutlineForestIsWellFormed(t, symbols, nil)
		})
	}
}

// countDefinitionBearingMatches runs the compiled query independently of the
// outliner and counts matches by how many definition captures they carry.
func countDefinitionBearingMatches(t *testing.T, lang *gts.Language, queryText string, tree *gts.Tree) (single, multiple int) {
	t.Helper()
	compiled, err := gts.NewQuery(queryText, lang)
	if err != nil {
		t.Fatalf("compile ground-truth query: %v", err)
	}
	for _, match := range compiled.Execute(tree) {
		definitions := 0
		for _, capture := range match.Captures {
			if capture.Node == nil {
				continue
			}
			if strings.HasPrefix(capture.Name, "definition.") && len(capture.Name) > len("definition.") {
				definitions++
			}
		}
		switch {
		case definitions == 1:
			single++
		case definitions > 1:
			multiple++
		}
	}
	return single, multiple
}

// TestOutlineNameAgreesWithNameRange enforces the invariant documented on
// OutlineSymbol.Name: the emitted name is the trimmed text of the capture
// node, so the source bytes at NameRange must trim to exactly Name. A language
// where a directive rewrites the text would surface here rather than ship a
// name that does not match its own span.
func TestOutlineNameAgreesWithNameRange(t *testing.T) {
	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			tree, lang, query := loadOutlineFixture(t, fixture.Language, fixture.File)
			outliner, err := gts.NewOutliner(lang, query)
			if err != nil {
				t.Fatalf("build outliner: %v", err)
			}
			symbols, _ := outliner.OutlineTree(tree)
			source := tree.Source()

			var check func([]gts.OutlineSymbol)
			check = func(list []gts.OutlineSymbol) {
				for _, symbol := range list {
					if int(symbol.NameRange.EndByte) > len(source) {
						t.Errorf("%q: NameRange ends past the source", symbol.Name)
						continue
					}
					span := strings.TrimSpace(string(source[symbol.NameRange.StartByte:symbol.NameRange.EndByte]))
					if span != symbol.Name {
						t.Errorf("%s %q: source at NameRange is %q; Name and NameRange disagree",
							symbol.Kind, symbol.Name, span)
					}
					check(symbol.Children)
				}
			}
			check(symbols)
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

// stripOutlineOwners returns a deep copy of symbols with every Owner cleared,
// so a forest resolved with owner rules attached can compare structurally
// equal, via outlineSymbolsEqual, to the same forest resolved without them.
func stripOutlineOwners(symbols []gts.OutlineSymbol) []gts.OutlineSymbol {
	if len(symbols) == 0 {
		return nil
	}
	out := make([]gts.OutlineSymbol, len(symbols))
	for i, symbol := range symbols {
		symbol.Owner = ""
		symbol.Children = stripOutlineOwners(symbol.Children)
		out[i] = symbol
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
