package gotreesitter

import (
	"testing"
	"time"
)

// buildContainerLanguage constructs a hand-built grammar for a simple container
// language that wraps content between '[' and ']' markers, used for injection tests.
//
// Grammar:
//
//	document -> LBRACKET content RBRACKET
//	content  -> TEXT*
//
// Symbols:
//
//	0: EOF
//	1: LBRACKET "[" (terminal, anonymous)
//	2: RBRACKET "]" (terminal, anonymous)
//	3: TEXT (terminal, named) — any non-bracket character sequence
//	4: document (nonterminal, named)
//	5: content (nonterminal, named)
//
// LR States:
//
//	State 0 (start):        LBRACKET -> shift 1, document -> goto 5
//	State 1 (saw [):        TEXT -> shift 2, RBRACKET -> shift 4, content -> goto 3
//	State 2 (saw text):     any -> reduce content -> TEXT (1 child)
//	State 3 (saw content):  RBRACKET -> shift 4
//	State 4 (saw ]):        any -> reduce document -> LBRACKET content RBRACKET (3 children)
//	State 5 (saw document): EOF -> accept
func buildContainerLanguage() *Language {
	return &Language{
		Name:               "container",
		SymbolCount:        6,
		TokenCount:         4,
		ExternalTokenCount: 0,
		StateCount:         6,
		LargeStateCount:    0,
		FieldCount:         1,
		ProductionIDCount:  2,

		SymbolNames: []string{"EOF", "[", "]", "TEXT", "document", "content"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "[", Visible: true, Named: false},
			{Name: "]", Visible: true, Named: false},
			{Name: "TEXT", Visible: true, Named: true},
			{Name: "document", Visible: true, Named: true},
			{Name: "content", Visible: true, Named: true},
		},
		FieldNames: []string{"", "body"},

		FieldMapSlices: [][2]uint16{
			{0, 0}, // production 0 (content -> TEXT): no fields
			{0, 1}, // production 1 (document -> [ content ]): 1 field entry
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 1}, // field "body" at child index 1 (the content node)
		},

		ParseActions: []ParseActionEntry{
			{Actions: nil}, // 0: error
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},                                   // 1: shift to 1
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},                                   // 2: shift to 2
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 1, ProductionID: 0}}}, // 3: reduce content
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},                                   // 4: goto 3
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},                                   // 5: shift to 4
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 3, ProductionID: 1}}}, // 6: reduce document
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},                                   // 7: goto 5
			{Actions: []ParseAction{{Type: ParseActionAccept}}},                                            // 8: accept
		},

		// Columns: EOF(0), LBRACKET(1), RBRACKET(2), TEXT(3), document(4), content(5)
		ParseTable: [][]uint16{
			{0, 1, 0, 0, 7, 0}, // state 0
			{0, 0, 5, 2, 0, 4}, // state 1
			{3, 3, 3, 3, 0, 0}, // state 2
			{0, 0, 5, 0, 0, 0}, // state 3
			{6, 6, 6, 6, 0, 0}, // state 4
			{8, 0, 0, 0, 0, 0}, // state 5
		},

		LexModes: []LexMode{
			{LexState: 0}, {LexState: 0}, {LexState: 0},
			{LexState: 0}, {LexState: 0}, {LexState: 0},
		},

		LexStates: []LexState{
			// State 0: start
			{
				AcceptToken: 0, Skip: false, Default: -1, EOF: -1,
				Transitions: []LexTransition{
					{Lo: '[', Hi: '[', NextState: 1},
					{Lo: ']', Hi: ']', NextState: 2},
					{Lo: '!', Hi: 'Z', NextState: 3}, // anything except [ ] in ASCII
					{Lo: '^', Hi: '~', NextState: 3},
					{Lo: ' ', Hi: ' ', NextState: 4},
					{Lo: '\t', Hi: '\t', NextState: 4},
					{Lo: '\n', Hi: '\n', NextState: 4},
				},
			},
			// State 1: saw '['
			{AcceptToken: 1, Skip: false, Default: -1, EOF: -1},
			// State 2: saw ']'
			{AcceptToken: 2, Skip: false, Default: -1, EOF: -1},
			// State 3: text (non-bracket chars)
			{
				AcceptToken: 3, Skip: false, Default: -1, EOF: -1,
				Transitions: []LexTransition{
					{Lo: '!', Hi: 'Z', NextState: 3},
					{Lo: '^', Hi: '~', NextState: 3},
				},
			},
			// State 4: whitespace (skip)
			{
				AcceptToken: 0, Skip: true, Default: -1, EOF: -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 4},
					{Lo: '\t', Hi: '\t', NextState: 4},
					{Lo: '\n', Hi: '\n', NextState: 4},
				},
			},
		},
	}
}

func TestInjectionParserBasic(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)

	// Injection query: content nodes contain arithmetic.
	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`)
	if err != nil {
		t.Fatal(err)
	}

	source := []byte("[1+2]")
	result, err := ip.Parse(source, "container")
	if err != nil {
		t.Fatal(err)
	}

	if result.Tree == nil {
		t.Fatal("parent tree is nil")
	}
	if result.Tree.RootNode() == nil {
		t.Fatal("parent tree root is nil")
	}

	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}

	inj := result.Injections[0]
	if inj.Language != "arithmetic" {
		t.Errorf("injection language = %q, want %q", inj.Language, "arithmetic")
	}
	if inj.Tree == nil {
		t.Fatal("injection tree is nil")
	}
}

func TestInjectionParserUTF16(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)

	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ip.ParseUTF16(utf16Units("[1+2]"), "container")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tree == nil || result.Tree.SourceEncoding() != InputEncodingUTF16 {
		t.Fatalf("parent tree encoding = %v, want UTF16", result.Tree.SourceEncoding())
	}
	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}
	inj := result.Injections[0]
	if inj.Language != "arithmetic" {
		t.Fatalf("injection language = %q, want arithmetic", inj.Language)
	}
	if inj.Tree == nil || inj.Tree.SourceEncoding() != InputEncodingUTF16 {
		t.Fatalf("injection tree encoding = %v, want UTF16", inj.Tree.SourceEncoding())
	}
	if len(inj.Ranges) != 1 {
		t.Fatalf("expected 1 UTF16 range, got %d", len(inj.Ranges))
	}
	if got, want := inj.Ranges[0].StartCodeUnit, uint32(1); got != want {
		t.Fatalf("injection start unit = %d, want %d", got, want)
	}
	if got, want := inj.Ranges[0].EndCodeUnit, uint32(4); got != want {
		t.Fatalf("injection end unit = %d, want %d", got, want)
	}
	childRange, ok := inj.Tree.UTF16RangeForNode(inj.Tree.RootNode())
	if !ok {
		t.Fatal("child tree UTF16RangeForNode returned !ok")
	}
	if childRange.StartCodeUnit != inj.Ranges[0].StartCodeUnit || childRange.EndCodeUnit != inj.Ranges[0].EndCodeUnit {
		t.Fatalf("child tree UTF16 range = [%d,%d), want [%d,%d)",
			childRange.StartCodeUnit, childRange.EndCodeUnit, inj.Ranges[0].StartCodeUnit, inj.Ranges[0].EndCodeUnit)
	}

	byteResult, err := ip.ParseUTF16Bytes(utf16BytesForTest(t, "[1+2]", UTF16LittleEndian), "container", UTF16LittleEndian)
	if err != nil {
		t.Fatalf("ParseUTF16Bytes failed: %v", err)
	}
	if len(byteResult.Injections) != 1 {
		t.Fatalf("ParseUTF16Bytes injection len = %d, want 1", len(byteResult.Injections))
	}
}

func TestInjectionParserIncrementalUTF16(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)

	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`)
	if err != nil {
		t.Fatal(err)
	}

	oldSource := utf16Units("[1+2]")
	oldResult, err := ip.ParseUTF16(oldSource, "container")
	if err != nil {
		t.Fatalf("ParseUTF16 old failed: %v", err)
	}
	newSource := utf16Units("[1+3]")
	if ok := oldResult.Tree.EditUTF16(UTF16Edit{
		StartCodeUnit:  3,
		OldEndCodeUnit: 4,
		NewEndCodeUnit: 4,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}

	newResult, err := ip.ParseIncrementalUTF16(newSource, "container", oldResult)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16 failed: %v", err)
	}
	if len(newResult.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(newResult.Injections))
	}
	if got, want := newResult.Injections[0].Ranges[0].StartCodeUnit, uint32(1); got != want {
		t.Fatalf("incremental injection start unit = %d, want %d", got, want)
	}
	if got, want := newResult.Injections[0].Ranges[0].EndCodeUnit, uint32(4); got != want {
		t.Fatalf("incremental injection end unit = %d, want %d", got, want)
	}
}

func TestInjectionParserIncrementalUTF16NoEditKeepsReusedTrees(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)

	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`)
	if err != nil {
		t.Fatal(err)
	}

	source := utf16Units("[1+2]")
	oldResult, err := ip.ParseUTF16(source, "container")
	if err != nil {
		t.Fatalf("ParseUTF16 old failed: %v", err)
	}

	newResult, err := ip.ParseIncrementalUTF16(source, "container", oldResult)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16 no-edit failed: %v", err)
	}
	if newResult.Tree == nil || newResult.Tree.RootNode() == nil {
		t.Fatal("no-edit incremental result has nil parent tree")
	}
	if len(newResult.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(newResult.Injections))
	}
	inj := newResult.Injections[0]
	if inj.Tree == nil || inj.Tree.RootNode() == nil {
		t.Fatal("no-edit incremental result has nil child tree")
	}
	if got, want := inj.Tree.SourceEncoding(), InputEncodingUTF16; got != want {
		t.Fatalf("child tree encoding = %v, want %v", got, want)
	}
	if got := inj.Tree.RootNode().Text(inj.Tree.Source()); got != "1+2" {
		t.Fatalf("child root text = %q, want %q", got, "1+2")
	}
}

func TestInjectionParserChildTreeCoordinatesStayDocumentRelative(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)

	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`)
	if err != nil {
		t.Fatal(err)
	}

	source := []byte("\n[1+2]")
	result, err := ip.Parse(source, "container")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}

	inj := result.Injections[0]
	if inj.Tree == nil {
		t.Fatal("injection tree is nil")
	}
	if len(inj.Ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(inj.Ranges))
	}

	root := inj.Tree.RootNode()
	if root == nil {
		t.Fatal("child root is nil")
	}
	if root.StartByte() != inj.Ranges[0].StartByte || root.EndByte() != inj.Ranges[0].EndByte {
		t.Fatalf("child root bytes = [%d,%d), want [%d,%d)",
			root.StartByte(), root.EndByte(), inj.Ranges[0].StartByte, inj.Ranges[0].EndByte)
	}
	if root.StartPoint() != inj.Ranges[0].StartPoint || root.EndPoint() != inj.Ranges[0].EndPoint {
		t.Fatalf("child root points = [%v,%v), want [%v,%v)",
			root.StartPoint(), root.EndPoint(), inj.Ranges[0].StartPoint, inj.Ranges[0].EndPoint)
	}
	if got := root.Text(source); got != "1+2" {
		t.Fatalf("child root text from document source = %q, want %q", got, "1+2")
	}
}

func TestInjectionParserUnregisteredChild(t *testing.T) {
	parentLang := buildContainerLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)

	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "unknown_lang")`)
	if err != nil {
		t.Fatal(err)
	}

	source := []byte("[hello]")
	result, err := ip.Parse(source, "container")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}

	inj := result.Injections[0]
	if inj.Language != "unknown_lang" {
		t.Errorf("injection language = %q, want %q", inj.Language, "unknown_lang")
	}
	if inj.Tree != nil {
		t.Error("expected nil tree for unregistered language")
	}
	if len(inj.Ranges) == 0 {
		t.Error("expected non-empty ranges even for unregistered language")
	}
}

func TestInjectionParserNoQuery(t *testing.T) {
	parentLang := buildContainerLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)

	source := []byte("[hello]")
	result, err := ip.Parse(source, "container")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Injections) != 0 {
		t.Errorf("expected 0 injections without query, got %d", len(result.Injections))
	}
}

func TestInjectionParserUnregisteredParent(t *testing.T) {
	ip := NewInjectionParser()
	_, err := ip.Parse([]byte("hello"), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unregistered parent language")
	}
}

func TestInjectionParserQueryCompileError(t *testing.T) {
	parentLang := buildContainerLanguage()
	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)

	err := ip.RegisterInjectionQuery("container", `(nonexistent_node_type) @cap`)
	if err == nil {
		t.Fatal("expected error for invalid query")
	}
}

func TestInjectionParserEmptySource(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)

	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`)
	if err != nil {
		t.Fatal(err)
	}

	// Empty source — parser returns nil root, no injections.
	result, err := ip.Parse([]byte(""), "container")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Injections) != 0 {
		t.Errorf("expected 0 injections for empty source, got %d", len(result.Injections))
	}
}

func TestProbeInjectionParserParseIncrementalNilOldResult(t *testing.T) {
	parentLang := buildContainerLanguage()
	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)

	result, err := ip.ParseIncremental([]byte("[value]"), "container", nil)
	if err != nil {
		t.Fatalf("ParseIncremental with nil old result: %v", err)
	}
	if result == nil || result.Tree == nil {
		t.Fatal("ParseIncremental with nil old result returned no tree")
	}
	if result.Tree.RootNode() == nil {
		t.Fatal("ParseIncremental with nil old result returned no root node")
	}
	root := result.Tree.RootNode()
	if got := string(root.Text([]byte("[value]"))); got != "[value]" {
		t.Errorf("root text = %q, want %q", got, "[value]")
	}
}

func TestInjectionResultRanges(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)

	err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`)
	if err != nil {
		t.Fatal(err)
	}

	source := []byte("[1+2]")
	result, err := ip.Parse(source, "container")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}

	inj := result.Injections[0]
	if len(inj.Ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(inj.Ranges))
	}

	r := inj.Ranges[0]
	// The content node "1+2" should be within the brackets.
	if r.StartByte < 1 || r.EndByte > 4 {
		t.Errorf("injection range [%d, %d) unexpected for source %q", r.StartByte, r.EndByte, source)
	}
}

func TestSetValues(t *testing.T) {
	lang := buildContainerLanguage()
	q, err := NewQuery(`(content) @cap (#set! injection.language "arithmetic")`, lang)
	if err != nil {
		t.Fatal(err)
	}

	parser := NewParser(lang)
	source := []byte("[hello]")
	tree := mustParse(t, parser, source)

	matches := q.Execute(tree)
	if len(matches) == 0 {
		t.Fatal("expected at least 1 match")
	}

	vals := matches[0].SetValues(q, "injection.language")
	if len(vals) != 1 || vals[0] != "arithmetic" {
		t.Errorf("SetValues = %v, want [\"arithmetic\"]", vals)
	}

	// Non-existent key.
	vals = matches[0].SetValues(q, "nonexistent")
	if vals != nil {
		t.Errorf("SetValues for nonexistent key = %v, want nil", vals)
	}
}

// TestInjectionParserTimeoutBoundsInjectedParse verifies that
// InjectionParser.SetTimeoutMicros bounds every injected parse, not just the
// root parse. Before InjectionParser had a timeout API, a large injected
// region ignored the caller's time budget entirely and could run unbounded.
func TestInjectionParserTimeoutBoundsInjectedParse(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)
	if err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`); err != nil {
		t.Fatal(err)
	}
	// 30ms comfortably bounds the trivial container (root) parse of this
	// document (a few ms regardless of input size, since the grammar lexes
	// the whole bracketed body as one token) while cutting off the
	// arithmetic child parse partway through its left-recursive reduction
	// chain (an unbounded parse of the same content takes well over 30ms),
	// so the root succeeds and the injected parse times out.
	ip.SetTimeoutMicros(30_000)

	content := bigArithmeticSource(50_000)
	source := append(append([]byte{'['}, content...), ']')

	start := time.Now()
	result, err := ip.Parse(source, "container")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Parse took %v, want < 2s", elapsed)
	}

	if len(result.Injections) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(result.Injections))
	}
	inj := result.Injections[0]
	if inj.Tree == nil {
		t.Fatal("injection tree is nil")
	}
	if got := inj.Tree.ParseStopReason(); got != ParseStopTimeout {
		t.Fatalf("injection ParseStopReason = %v, want %v", got, ParseStopTimeout)
	}
}

// TestInjectionParserSetTimeoutMicrosAppliesToCachedParser verifies that
// SetTimeoutMicros applies retroactively to a parser InjectionParser has
// already cached, not just to parsers created afterward.
func TestInjectionParserSetTimeoutMicrosAppliesToCachedParser(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	ip.RegisterLanguage("arithmetic", childLang)
	if err := ip.RegisterInjectionQuery("container",
		`(content) @injection.content (#set! injection.language "arithmetic")`); err != nil {
		t.Fatal(err)
	}

	// Warm the arithmetic parser cache with a small, fast parse before the
	// timeout is configured.
	if _, err := ip.Parse([]byte("[1+2]"), "container"); err != nil {
		t.Fatalf("warm-up Parse: %v", err)
	}

	ip.SetTimeoutMicros(30_000)

	content := bigArithmeticSource(50_000)
	source := append(append([]byte{'['}, content...), ']')

	start := time.Now()
	result, err := ip.Parse(source, "container")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Parse took %v, want < 2s", elapsed)
	}
	if len(result.Injections) != 1 || result.Injections[0].Tree == nil {
		t.Fatalf("expected 1 injection with a tree, got %+v", result.Injections)
	}
	if got := result.Injections[0].Tree.ParseStopReason(); got != ParseStopTimeout {
		t.Fatalf("injection ParseStopReason = %v, want %v", got, ParseStopTimeout)
	}
}

func TestSetValuesNilQuery(t *testing.T) {
	m := QueryMatch{PatternIndex: 0}
	vals := m.SetValues(nil, "key")
	if vals != nil {
		t.Errorf("SetValues with nil query = %v, want nil", vals)
	}
}
