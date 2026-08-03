package gotreesitter

import (
	"reflect"
	"testing"
)

func outlineTestRange(start, end uint32) Range {
	return Range{
		StartByte:  start,
		EndByte:    end,
		StartPoint: Point{Row: 0, Column: start},
		EndPoint:   Point{Row: 0, Column: end},
	}
}

func outlineTestCandidate(order int, kind, name string, start, end, nameStart, nameEnd uint32) outlineCandidate {
	return outlineCandidate{
		order:     order,
		kind:      kind,
		name:      name,
		nodeType:  kind + "_node",
		rng:       outlineTestRange(start, end),
		nameRange: outlineTestRange(nameStart, nameEnd),
	}
}

// runOutlineRules drives the two pure stages the way OutlineTree does, so the
// unit cases exercise the shipped code path rather than a copy of it.
func runOutlineRules(candidates []outlineCandidate) ([]OutlineSymbol, OutlineReport) {
	var report OutlineReport
	kept := filterOutlineCandidates(candidates, &report)
	symbols := buildOutlineForest(kept, &report)
	report.Symbols = countOutlineSymbols(symbols)
	return symbols, report
}

// assertOutlineReceiptBalances checks the accounting identity that makes
// fail-closed observable: every candidate is emitted or counted exactly once.
func assertOutlineReceiptBalances(t *testing.T, candidates int, report OutlineReport) {
	t.Helper()
	if got := report.Candidates(); got != candidates {
		t.Errorf("receipt does not balance: %d candidates in, symbols=%d omitted=%d (sum %d)",
			candidates, report.Symbols, report.Omitted(), got)
	}
}

func TestOutlineOmitsCandidateWithoutName(t *testing.T) {
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "", 0, 10, 0, 0),
		outlineTestCandidate(1, "function", "   ", 20, 30, 20, 23),
		outlineTestCandidate(2, "function", "kept", 40, 50, 40, 44),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedNoName != 2 {
		t.Errorf("OmittedNoName = %d, want 2", report.OmittedNoName)
	}
	if len(symbols) != 1 || symbols[0].Name != "kept" {
		t.Errorf("symbols = %+v, want only the named candidate", symbols)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineOmitsNameRangeOutsideDefinition(t *testing.T) {
	candidates := []outlineCandidate{
		// Name span starts before the definition span.
		outlineTestCandidate(0, "function", "early", 10, 20, 5, 12),
		// Name span ends after the definition span.
		outlineTestCandidate(1, "function", "late", 30, 40, 35, 45),
		// Inverted definition span contains nothing.
		outlineTestCandidate(2, "function", "inverted", 60, 50, 60, 62),
		outlineTestCandidate(3, "function", "kept", 70, 80, 70, 74),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedInvalidNameRange != 3 {
		t.Errorf("OmittedInvalidNameRange = %d, want 3", report.OmittedInvalidNameRange)
	}
	if len(symbols) != 1 || symbols[0].Name != "kept" {
		t.Errorf("symbols = %+v, want only the valid candidate", symbols)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

// TestOutlineCollapsesExactRepeats covers rule 5. Only indistinguishable
// candidates collapse, so keeping the first is not a choice between them.
func TestOutlineCollapsesExactRepeats(t *testing.T) {
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "same", 0, 10, 0, 5),
		outlineTestCandidate(1, "function", "same", 0, 10, 0, 5),
		outlineTestCandidate(2, "function", "same", 0, 10, 0, 5),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedDuplicate != 2 {
		t.Errorf("OmittedDuplicate = %d, want 2", report.OmittedDuplicate)
	}
	if report.OmittedNameConflict != 0 {
		t.Errorf("OmittedNameConflict = %d, want 0: the members are identical", report.OmittedNameConflict)
	}
	if len(symbols) != 1 || symbols[0].Name != "same" {
		t.Errorf("symbols = %+v, want one symbol", symbols)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

// TestOutlineDropsSpanWithConflictingNames covers rule 4, the defect that
// published a C-family return type as a method name. The shared inference can
// bind "@name" twice on one definition node; the two bindings disagree, and no
// language-neutral rule ranks them. The group drops and the counter says why.
func TestOutlineDropsSpanWithConflictingNames(t *testing.T) {
	candidates := []outlineCandidate{
		// "Uri PathToUri(string p) {...}": the return type is captured
		// first, the method name second. Choosing by order publishes "Uri".
		outlineTestCandidate(0, "method", "Uri", 0, 40, 0, 3),
		outlineTestCandidate(1, "method", "PathToUri", 0, 40, 4, 13),
		outlineTestCandidate(2, "method", "kept", 50, 60, 50, 54),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedNameConflict != 2 {
		t.Errorf("OmittedNameConflict = %d, want 2 (both members of the group)", report.OmittedNameConflict)
	}
	if report.OmittedDuplicate != 0 {
		t.Errorf("OmittedDuplicate = %d, want 0: a name conflict is not a duplicate", report.OmittedDuplicate)
	}
	if len(symbols) != 1 || symbols[0].Name != "kept" {
		t.Fatalf("symbols = %+v, want only the unambiguous candidate", symbols)
	}
	for _, symbol := range symbols {
		if symbol.Name == "Uri" {
			t.Error("the outline published the return type as the symbol name")
		}
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

// TestOutlineTreatsDifferingNameSpansAsAConflict pins the stricter half of the
// rule: identical name TEXT at different spans still leaves the emitted symbol
// undetermined, because a symbol carries both.
func TestOutlineTreatsDifferingNameSpansAsAConflict(t *testing.T) {
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "run", 0, 40, 0, 3),
		outlineTestCandidate(1, "function", "run", 0, 40, 10, 13),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedNameConflict != 2 {
		t.Errorf("OmittedNameConflict = %d, want 2", report.OmittedNameConflict)
	}
	if len(symbols) != 0 {
		t.Errorf("symbols = %+v, want none", symbols)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

// TestOutlineKindConflictOutranksNameConflict pins the precedence: a group
// that disagrees about the kind is counted as a kind conflict, not twice.
func TestOutlineKindConflictOutranksNameConflict(t *testing.T) {
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "a", 0, 10, 0, 1),
		outlineTestCandidate(1, "method", "b", 0, 10, 2, 3),
	}
	_, report := runOutlineRules(candidates)

	if report.OmittedConflict != 2 {
		t.Errorf("OmittedConflict = %d, want 2", report.OmittedConflict)
	}
	if report.OmittedNameConflict != 0 {
		t.Errorf("OmittedNameConflict = %d, want 0", report.OmittedNameConflict)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineDropsSpanWithConflictingKinds(t *testing.T) {
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "same", 0, 10, 0, 4),
		outlineTestCandidate(1, "method", "same", 0, 10, 0, 4),
		outlineTestCandidate(2, "class", "kept", 20, 30, 20, 24),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedConflict != 2 {
		t.Errorf("OmittedConflict = %d, want 2 (both members of the conflicting group)", report.OmittedConflict)
	}
	if report.OmittedDuplicate != 0 {
		t.Errorf("OmittedDuplicate = %d, want 0: a conflict is not a duplicate", report.OmittedDuplicate)
	}
	if len(symbols) != 1 || symbols[0].Name != "kept" {
		t.Errorf("symbols = %+v, want the conflicting span dropped and nothing guessed", symbols)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineDropsWholeGroupWhenOneMemberConflicts(t *testing.T) {
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "a", 0, 10, 0, 1),
		outlineTestCandidate(1, "function", "b", 0, 10, 0, 1),
		outlineTestCandidate(2, "method", "c", 0, 10, 0, 1),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedConflict != 3 {
		t.Errorf("OmittedConflict = %d, want 3", report.OmittedConflict)
	}
	if len(symbols) != 0 {
		t.Errorf("symbols = %+v, want none", symbols)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineDropsPartialOverlap(t *testing.T) {
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "early", 0, 20, 0, 5),
		// Starts inside "early" and ends after it: neither contains the
		// other, so the later start is dropped.
		outlineTestCandidate(1, "function", "straddles", 10, 30, 10, 19),
		outlineTestCandidate(2, "function", "nested", 2, 8, 2, 8),
	}
	symbols, report := runOutlineRules(candidates)

	if report.OmittedOverlap != 1 {
		t.Errorf("OmittedOverlap = %d, want 1", report.OmittedOverlap)
	}
	if len(symbols) != 1 || symbols[0].Name != "early" {
		t.Fatalf("symbols = %+v, want the earlier start kept", symbols)
	}
	if len(symbols[0].Children) != 1 || symbols[0].Children[0].Name != "nested" {
		t.Errorf("children = %+v, want the contained candidate nested", symbols[0].Children)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineBuildsNestingInSourceOrder(t *testing.T) {
	candidates := []outlineCandidate{
		// Deliberately out of source order to prove the sort drives nesting.
		outlineTestCandidate(0, "function", "deep", 30, 40, 30, 34),
		outlineTestCandidate(1, "class", "outer", 0, 100, 5, 10),
		outlineTestCandidate(2, "method", "second", 50, 60, 50, 56),
		outlineTestCandidate(3, "method", "first", 20, 45, 20, 25),
		outlineTestCandidate(4, "class", "sibling", 200, 300, 205, 212),
	}
	symbols, report := runOutlineRules(candidates)

	if len(symbols) != 2 {
		t.Fatalf("roots = %d, want 2: %+v", len(symbols), symbols)
	}
	if symbols[0].Name != "outer" || symbols[1].Name != "sibling" {
		t.Fatalf("roots are %q and %q, want source order", symbols[0].Name, symbols[1].Name)
	}
	gotChildren := []string{}
	for _, child := range symbols[0].Children {
		gotChildren = append(gotChildren, child.Name)
	}
	if !reflect.DeepEqual(gotChildren, []string{"first", "second"}) {
		t.Errorf("children = %v, want [first second] in source order", gotChildren)
	}
	if len(symbols[0].Children[0].Children) != 1 || symbols[0].Children[0].Children[0].Name != "deep" {
		t.Errorf("grandchildren = %+v, want [deep]", symbols[0].Children[0].Children)
	}
	if report.Symbols != 5 {
		t.Errorf("Symbols = %d, want 5 including nested", report.Symbols)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineNestsEqualEndSpans(t *testing.T) {
	// A container and its last child can share an end byte. The sort places
	// the wider span first, so the narrower one nests instead of being
	// treated as an overlap.
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "tail", 10, 40, 10, 14),
		outlineTestCandidate(1, "class", "outer", 0, 40, 0, 5),
	}
	symbols, report := runOutlineRules(candidates)

	if len(symbols) != 1 || symbols[0].Name != "outer" {
		t.Fatalf("roots = %+v, want only outer", symbols)
	}
	if len(symbols[0].Children) != 1 || symbols[0].Children[0].Name != "tail" {
		t.Errorf("children = %+v, want tail nested", symbols[0].Children)
	}
	if report.OmittedOverlap != 0 {
		t.Errorf("OmittedOverlap = %d, want 0", report.OmittedOverlap)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineTreatsAdjacentSpansAsSiblings(t *testing.T) {
	// End byte is exclusive, so a span that starts exactly where another
	// ends is a sibling, not a child and not an overlap.
	candidates := []outlineCandidate{
		outlineTestCandidate(0, "function", "left", 0, 10, 0, 4),
		outlineTestCandidate(1, "function", "right", 10, 20, 10, 15),
	}
	symbols, report := runOutlineRules(candidates)

	if len(symbols) != 2 {
		t.Fatalf("roots = %+v, want two siblings", symbols)
	}
	if report.OmittedOverlap != 0 {
		t.Errorf("OmittedOverlap = %d, want 0", report.OmittedOverlap)
	}
	assertOutlineReceiptBalances(t, len(candidates), report)
}

func TestOutlineKindNormalizationIsTableDriven(t *testing.T) {
	cases := []struct {
		capture  string
		nodeType string
		want     string
	}{
		{"definition.function", "function_declaration", "function"},
		{"definition.method", "method_declaration", "method"},
		{"definition.class", "class_declaration", "class"},
		{"definition.interface", "interface_declaration", "interface"},
		{"definition.type", "type_spec", "type"},
		{"definition.constructor", "constructor_declaration", "constructor"},
		{"definition.constant", "const_spec", "constant"},
		{"definition.variable", "var_spec", "variable"},
		{"definition.module", "module", "module"},
		// Node-type refinement, keyed by cross-language node types.
		{"definition.type", "enum_declaration", "enum"},
		{"definition.type", "enum_item", "enum"},
		{"definition.class", "enum_declaration", "enum"},
		{"definition.class", "record_declaration", "record"},
		// A refinement never overrules a more specific capture.
		{"definition.method", "enum_declaration", "method"},
		{"definition.function", "record_declaration", "function"},
		// An unknown suffix passes through, so new tags data needs no code.
		{"definition.macro", "macro_definition", "macro"},
		{"definition.trait", "trait_item", "trait"},
	}
	for _, tc := range cases {
		if got := normalizeOutlineKind(tc.capture, tc.nodeType); got != tc.want {
			t.Errorf("normalizeOutlineKind(%q, %q) = %q, want %q", tc.capture, tc.nodeType, got, tc.want)
		}
	}
}

func TestOutlineDefinitionCaptureNeedsSuffix(t *testing.T) {
	cases := map[string]bool{
		"definition.function": true,
		"definition.x":        true,
		"definition.":         false,
		"definition":          false,
		"name":                false,
		"reference.call":      false,
		"":                    false,
	}
	for capture, want := range cases {
		if got := isOutlineDefinitionCapture(capture); got != want {
			t.Errorf("isOutlineDefinitionCapture(%q) = %v, want %v", capture, got, want)
		}
	}
}

// TestOutlineKindTablesAreFrozenByExactKeySet is the doctrine gate for the
// normalization data. The core cannot import the grammars registry, so it
// cannot check a key against the 206 language names. Blocklisting a hand
// written sample of those names has only partial teeth: 192 names would still
// slip through.
//
// This gate freezes the whole key set instead. Any new row in either table
// fails the test, whatever it is named, so a language-name row cannot land
// without the reviewer seeing it here. Adding a genuine cross-language row is
// a one-line edit to the expected set, in the same change, with its reason.
func TestOutlineKindTablesAreFrozenByExactKeySet(t *testing.T) {
	wantKindTable := map[string]string{
		"function":    "function",
		"method":      "method",
		"class":       "class",
		"interface":   "interface",
		"type":        "type",
		"constructor": "constructor",
		"constant":    "constant",
		"variable":    "variable",
		"module":      "module",
	}
	// Every key here is a grammar node type shared by many grammars.
	// "enum_declaration" alone appears in 18 registered languages.
	wantRefinement := map[string]string{
		"enum_declaration":   "enum",
		"enum_item":          "enum",
		"record_declaration": "record",
	}
	wantRefinable := map[string]bool{
		"type":  true,
		"class": true,
	}

	assertOutlineTableFrozen(t, "outlineKindTable", outlineKindTable, wantKindTable)
	assertOutlineTableFrozen(t, "outlineKindRefinement", outlineKindRefinement, wantRefinement)

	if len(outlineRefinableKinds) != len(wantRefinable) {
		t.Errorf("outlineRefinableKinds holds %d rows, want exactly %d", len(outlineRefinableKinds), len(wantRefinable))
	}
	for kind, want := range wantRefinable {
		if outlineRefinableKinds[kind] != want {
			t.Errorf("outlineRefinableKinds[%q] = %v, want %v", kind, outlineRefinableKinds[kind], want)
		}
	}
	for kind := range outlineRefinableKinds {
		if !wantRefinable[kind] {
			t.Errorf("outlineRefinableKinds holds an unexpected row %q; add it to the frozen set with its reason", kind)
		}
	}
}

func assertOutlineTableFrozen(t *testing.T, name string, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s holds %d rows, want exactly %d", name, len(got), len(want))
	}
	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			t.Errorf("%s is missing the frozen row %q", name, key)
			continue
		}
		if gotValue != wantValue {
			t.Errorf("%s[%q] = %q, want %q", name, key, gotValue, wantValue)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("%s holds an unexpected row %q. If it is a genuine cross-language row, add it to the frozen set in this change. If it is a language name, it does not belong in this table at all.", name, key)
		}
	}
}

// TestOutlineKindTableFreezeCatchesALanguageNameRow proves the freeze has full
// teeth for the case it exists to stop: a row keyed by a language name that no
// hand-written blocklist happens to mention.
func TestOutlineKindTableFreezeCatchesALanguageNameRow(t *testing.T) {
	frozen := map[string]string{"function": "function"}
	for _, injected := range []string{"go", "zig", "haskell", "gleam", "wgsl"} {
		tampered := map[string]string{"function": "function", injected: "function"}
		probe := &testing.T{}
		assertOutlineTableFrozen(probe, "probe", tampered, frozen)
		if !probe.Failed() {
			t.Errorf("the freeze accepted an injected row keyed by the language name %q", injected)
		}
	}
}

func TestOutlineOwnerRuleValidationFailsClosed(t *testing.T) {
	cases := []struct {
		name  string
		rules []OutlineOwnerRule
		want  bool
	}{
		{
			name:  "valid go receiver rule",
			rules: []OutlineOwnerRule{{NodeType: "method_declaration", OwnerField: "receiver", Unwrap: []string{"parameter_list"}, NameTypes: []string{"type_identifier"}}},
			want:  false,
		},
		{
			name:  "missing node type",
			rules: []OutlineOwnerRule{{OwnerField: "receiver"}},
			want:  true,
		},
		{
			name:  "missing owner field",
			rules: []OutlineOwnerRule{{NodeType: "method_declaration"}},
			want:  true,
		},
		{
			name:  "blank owner field",
			rules: []OutlineOwnerRule{{NodeType: "method_declaration", OwnerField: "  "}},
			want:  true,
		},
	}
	for _, tc := range cases {
		indexed := map[string][]OutlineOwnerRule{}
		for _, rule := range tc.rules {
			indexed[rule.NodeType] = append(indexed[rule.NodeType], rule)
		}
		err := validateOutlineOwnerRules(indexed)
		if (err != nil) != tc.want {
			t.Errorf("%s: validateOutlineOwnerRules error = %v, want error %v", tc.name, err, tc.want)
		}
	}
}
