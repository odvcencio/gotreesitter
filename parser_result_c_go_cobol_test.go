package gotreesitter

import (
	"bytes"
	"testing"
)

func TestNormalizeCTranslationUnitRootRetagsRecoveredTopLevelChildren(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "ERROR", "translation_unit", "preproc_ifdef", "declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "preproc_ifdef", Visible: true, Named: true},
			{Name: "declaration", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	ifdef := newLeafNodeInArena(arena, 3, true, 0, 7, Point{}, Point{Column: 7})
	decl := newLeafNodeInArena(arena, 4, true, 8, 18, Point{Row: 1}, Point{Row: 1, Column: 10})
	root := newParentNodeInArena(arena, 1, true, []*Node{ifdef, decl}, nil, 0)
	root.setHasError(true)

	normalizeCTranslationUnitRoot(root, lang)

	if got, want := root.Type(lang), "translation_unit"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatalf("root.HasError = false, want true")
	}
}

func TestNormalizeCCollapsedKeywordChildrenRestoresNull(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "null", "NULL"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "null", Visible: true, Named: true},
			{Name: "NULL", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	source := []byte("NULL")
	nullNode := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{nullNode}, nil, 0)

	normalizeCCompatibility(root, source, lang)

	if got, want := nullNode.ChildCount(), 1; got != want {
		t.Fatalf("null child count = %d, want %d", got, want)
	}
	child := nullNode.Child(0)
	if child == nil {
		t.Fatal("null child = nil")
	}
	if got, want := child.Type(lang), "NULL"; got != want {
		t.Fatalf("null child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored NULL child should be anonymous")
	}
}

func TestNormalizeCCollapsedKeywordChildrenUsesFinalRefsSelectively(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "null", "NULL", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "null", Visible: true, Named: true},
			{Name: "NULL", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
	}
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	source := []byte("NULL name")
	nullNode := newCompactFullLeafInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	nullNode.parseState = 11
	identifier := newCompactFullLeafInArena(arena, 4, true, 5, 9, Point{Column: 5}, Point{Column: 9})
	identifier.parseState = 12
	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{
		newStackEntryCompactFullLeaf(nullNode.parseState, nullNode),
		newStackEntryCompactFullLeaf(identifier.parseState, identifier),
	}, 0, 9, Point{}, Point{Column: 9}, false)
	parent.parseState = 13
	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)

	normalizeCCollapsedKeywordChildren(root, source, lang)

	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("single final child materializations = %d, want 1", got)
	}
	if !nodeHasFinalChildRefs(root) {
		t.Fatal("root lost final-child refs")
	}
	restored := root.Child(0)
	if restored == nil {
		t.Fatal("root child 0 = nil")
	}
	if got, want := restored.ChildCount(), 1; got != want {
		t.Fatalf("null child count = %d, want %d", got, want)
	}
	child := restored.Child(0)
	if child == nil {
		t.Fatal("null child = nil")
	}
	if got, want := child.Type(lang), "NULL"; got != want {
		t.Fatalf("null child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored NULL child should be anonymous")
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents after access = %d, want 0", got)
	}
}

func TestNormalizeCCollapsedKeywordChildrenRestoresStorageClassSpecifier(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "storage_class_specifier", "extern", "inline"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "storage_class_specifier", Visible: true, Named: true},
			{Name: "extern", Visible: true, Named: false},
			{Name: "inline", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	source := []byte("extern")
	storage := newLeafNodeInArena(arena, 2, true, 0, 6, Point{}, Point{Column: 6})
	root := newParentNodeInArena(arena, 1, true, []*Node{storage}, nil, 0)

	normalizeCCompatibility(root, source, lang)

	if got, want := storage.ChildCount(), 1; got != want {
		t.Fatalf("storage class child count = %d, want %d", got, want)
	}
	child := storage.Child(0)
	if child == nil {
		t.Fatal("storage class child = nil")
	}
	if got, want := child.Type(lang), "extern"; got != want {
		t.Fatalf("storage class child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored extern child should be anonymous")
	}
}

func TestNormalizeCCollapsedKeywordChildrenRestoresTypeQualifier(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "type_qualifier", "const"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "type_qualifier", Visible: true, Named: true},
			{Name: "const", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	source := []byte("const")
	qualifier := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})
	root := newParentNodeInArena(arena, 1, true, []*Node{qualifier}, nil, 0)

	normalizeCCompatibility(root, source, lang)

	if got, want := qualifier.ChildCount(), 1; got != want {
		t.Fatalf("type qualifier child count = %d, want %d", got, want)
	}
	child := qualifier.Child(0)
	if child == nil {
		t.Fatal("type qualifier child = nil")
	}
	if got, want := child.Type(lang), "const"; got != want {
		t.Fatalf("type qualifier child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored const child should be anonymous")
	}
}

func TestNormalizeCppCompatibilityRestoresCollapsedKeywordChildren(t *testing.T) {
	lang := &Language{
		Name: "cpp",
		SymbolNames: []string{
			"EOF",
			"translation_unit",
			"type_qualifier",
			"const",
			"noexcept",
			"noexcept",
			"lambda_default_capture",
			"&",
			"=",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "type_qualifier", Visible: true, Named: true},
			{Name: "const", Visible: true, Named: false},
			{Name: "noexcept", Visible: true, Named: true},
			{Name: "noexcept", Visible: true, Named: false},
			{Name: "lambda_default_capture", Visible: true, Named: true},
			{Name: "&", Visible: true, Named: false},
			{Name: "=", Visible: true, Named: false},
		},
	}
	source := []byte("const noexcept & =")
	arena := newNodeArena(arenaClassFull)
	qualifier := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})
	noexceptNode := newLeafNodeInArena(arena, 4, true, 6, 14, Point{Column: 6}, Point{Column: 14})
	ampCapture := newLeafNodeInArena(arena, 6, true, 15, 16, Point{Column: 15}, Point{Column: 16})
	eqCapture := newLeafNodeInArena(arena, 6, true, 17, 18, Point{Column: 17}, Point{Column: 18})
	root := newParentNodeInArena(arena, 1, true, []*Node{qualifier, noexceptNode, ampCapture, eqCapture}, nil, 0)

	runLanguageResultCompatibility(resultCompatibilityContext{
		root:   root,
		source: source,
		lang:   lang,
	})

	assertCollapsedKeywordChild(t, qualifier, lang, "const")
	assertCollapsedKeywordChild(t, noexceptNode, lang, "noexcept")
	assertCollapsedKeywordChild(t, ampCapture, lang, "&")
	assertCollapsedKeywordChild(t, eqCapture, lang, "=")
}

func assertCollapsedKeywordChild(t *testing.T, node *Node, lang *Language, want string) {
	t.Helper()
	if got := node.ChildCount(); got != 1 {
		t.Fatalf("%s child count = %d, want 1", node.Type(lang), got)
	}
	child := node.Child(0)
	if child == nil {
		t.Fatalf("%s child = nil", node.Type(lang))
	}
	if got := child.Type(lang); got != want {
		t.Fatalf("%s child type = %q, want %q", node.Type(lang), got, want)
	}
	if child.IsNamed() {
		t.Fatalf("%s child should be anonymous", node.Type(lang))
	}
}

func TestNormalizeGoSourceFileRootRetagsRecoveredTopLevelChildren(t *testing.T) {
	lang := &Language{
		Name:        "go",
		SymbolNames: []string{"EOF", "ERROR", "source_file", "package_clause", "function_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "package_clause", Visible: true, Named: true},
			{Name: "function_declaration", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	pkg := newLeafNodeInArena(arena, 3, true, 0, 12, Point{}, Point{Column: 12})
	fn := newLeafNodeInArena(arena, 4, true, 13, 30, Point{Row: 1}, Point{Row: 1, Column: 17})
	root := newParentNodeInArena(arena, 1, true, []*Node{pkg, fn}, nil, 0)
	root.setHasError(true)

	normalizeGoSourceFileRoot(root, nil, &Parser{language: lang})

	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatalf("root.HasError = true, want false")
	}
}

func TestNormalizeGoStatementListTrailingExtrasStopsBeforeComment(t *testing.T) {
	source := []byte("stmt\n// trailing comment\n")
	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	list := newParentNodeInArena(arena, 2, true, []*Node{stmt}, nil, 0)
	list.endByte = uint32(len(source))
	list.endPoint = advancePointByBytes(Point{}, source)

	normalizeGoStatementListTrailingExtras(list, source, goCompatibilitySymbols{statementList: 2})

	if got, want := list.EndByte(), uint32(5); got != want {
		t.Fatalf("statement_list.EndByte = %d, want %d", got, want)
	}
	if got, want := list.EndPoint(), (Point{Row: 1, Column: 0}); got != want {
		t.Fatalf("statement_list.EndPoint = %+v, want %+v", got, want)
	}
}

func TestNormalizeResultCompatibilityDispatchesUppercaseCobol(t *testing.T) {
	lang := &Language{
		Name:        "COBOL",
		SymbolNames: []string{"EOF", "start", "program_definition", "identification_division"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
		},
	}

	source := []byte("       identification division.\n")
	arena := newNodeArena(arenaClassFull)
	div := newLeafNodeInArena(arena, 3, true, 0, uint32(len(source)-1), Point{}, Point{Column: uint32(len(source) - 1)})
	def := newParentNodeInArena(arena, 2, true, []*Node{div}, nil, 0)
	def.startByte = 0
	def.endByte = uint32(len(source) - 1)
	root := newParentNodeInArena(arena, 1, true, []*Node{def}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.StartByte(), uint32(7); got != want {
		t.Fatalf("root.StartByte = %d, want %d", got, want)
	}
	if got, want := root.Child(0).StartByte(), uint32(7); got != want {
		t.Fatalf("program_definition.StartByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolLeadingCommentBannerKeepsProgramStartAtFirstCode(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "comment", "program_definition", "identification_division"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
		},
	}
	source := []byte("      *****************************************************************\n" +
		"      * Program:     BBANK10P.CBL                                     *\n" +
		"\n" +
		"       IDENTIFICATION DIVISION.\n")
	codeStart := uint32(bytes.Index(source, []byte("IDENTIFICATION DIVISION.")))

	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 71, 71, Point{Column: 71}, Point{Column: 71})
	id := newLeafNodeInArena(arena, 4, true, 6, uint32(len(source)-1), Point{Column: 6}, Point{Row: 3, Column: 31})
	def := newParentNodeInArena(arena, 3, true, []*Node{id}, nil, 0)
	def.startByte = 6
	def.startPoint = Point{Column: 6}
	def.endByte = uint32(len(source) - 1)
	def.endPoint = Point{Row: 3, Column: 31}
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, def}, nil, 0)
	root.startByte = 6
	root.startPoint = Point{Column: 6}
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.StartByte(), uint32(6); got != want {
		t.Fatalf("root.StartByte = %d, want %d", got, want)
	}
	if got := def.StartByte(); got != codeStart {
		t.Fatalf("program_definition.StartByte = %d, want first code byte %d", got, codeStart)
	}
	if got := id.StartByte(); got != codeStart {
		t.Fatalf("identification_division.StartByte = %d, want first code byte %d", got, codeStart)
	}
}

func TestNormalizeCobolRecoveredRootProgramDefinitionSplitsTailError(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			"start",
			"comment",
			"program_definition",
			"identification_division",
			"environment_division",
			"data_division",
			"procedure_division",
			"paragraph_header",
			"display_statement",
			"move_statement",
			"comment_entry",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "environment_division", Visible: true, Named: true},
			{Name: "data_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "paragraph_header", Visible: true, Named: true},
			{Name: "display_statement", Visible: true, Named: true},
			{Name: "move_statement", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("      * banner\n" +
		"       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. A.\n" +
		"       ENVIRONMENT DIVISION.\n" +
		"       DATA DIVISION.\n" +
		"      *COPY CENTRY.\n" +
		"       PROCEDURE DIVISION.\n" +
		"       PARA-1.\n" +
		"           DISPLAY A.\n" +
		"       BAD-PARA.\n" +
		"           MOVE A TO B.\n" +
		"           EXEC CICS RETURN\n" +
		"      * trailer\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	leadingComment := cobolTestLeaf(arena, 2, true, source, end("banner"), end("banner"))
	id := cobolTestLeaf(arena, 4, true, source, idx("IDENTIFICATION"), end("PROGRAM-ID. A."))
	env := cobolTestLeaf(arena, 5, true, source, idx("ENVIRONMENT"), end("ENVIRONMENT DIVISION."))
	data := cobolTestLeaf(arena, 6, true, source, idx("DATA DIVISION."), idx("PROCEDURE DIVISION."))
	copyComment := cobolTestLeaf(arena, 2, true, source, end("CENTRY."), end("CENTRY."))
	para1 := cobolTestLeaf(arena, 8, true, source, idx("PARA-1."), end("PARA-1."))
	display := cobolTestLeaf(arena, 9, true, source, idx("DISPLAY A."), end("DISPLAY A."))
	badPara := cobolTestLeaf(arena, 8, true, source, idx("BAD-PARA."), end("BAD-PARA."))
	tailMove := cobolTestLeaf(arena, 10, true, source, idx("MOVE A TO B."), end("MOVE A TO B."))
	execErr := cobolTestLeaf(arena, errorSymbol, true, source, idx("EXEC CICS RETURN"), end("EXEC CICS RETURN"))
	execErr.setHasError(true)
	trailingComment := cobolTestLeaf(arena, 2, true, source, end("trailer"), end("trailer"))
	recovered := newParentNodeInArena(arena, errorSymbol, true, []*Node{
		leadingComment,
		id,
		env,
		data,
		copyComment,
		para1,
		display,
		badPara,
		tailMove,
		execErr,
		trailingComment,
	}, nil, 0)
	recovered.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{recovered}, nil, 0)
	root.setHasError(true)
	root.startByte = 6
	root.startPoint = Point{Column: 6}
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 4; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(0).Type(lang), "comment"; got != want {
		t.Fatalf("root child 0 type = %q, want %q", got, want)
	}
	program := root.Child(1)
	if got, want := program.Type(lang), "program_definition"; got != want {
		t.Fatalf("root child 1 type = %q, want %q", got, want)
	}
	if got, want := program.ChildCount(), 5; got != want {
		t.Fatalf("program_definition.ChildCount = %d, want %d", got, want)
	}
	dataDiv := program.Child(2)
	if got, want := dataDiv.Type(lang), "data_division"; got != want {
		t.Fatalf("program child 2 type = %q, want %q", got, want)
	}
	if got, want := dataDiv.EndByte(), end("DATA DIVISION."); got != want {
		t.Fatalf("data_division.EndByte = %d, want %d", got, want)
	}
	procedure := program.Child(4)
	if got, want := procedure.Type(lang), "procedure_division"; got != want {
		t.Fatalf("program child 4 type = %q, want %q", got, want)
	}
	if got, want := procedure.StartByte(), idx("PROCEDURE DIVISION."); got != want {
		t.Fatalf("procedure_division.StartByte = %d, want %d", got, want)
	}
	if got, want := procedure.EndByte(), end("BAD-PARA."); got != want {
		t.Fatalf("procedure_division.EndByte = %d, want %d", got, want)
	}
	tail := root.Child(2)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("root child 2 type = %q, want %q", got, want)
	}
	if !tail.IsExtra() || !tail.HasError() {
		t.Fatalf("tail ERROR flags extra=%v hasError=%v, want both true", tail.IsExtra(), tail.HasError())
	}
	if got, want := tail.StartByte(), idx("MOVE A TO B."); got != want {
		t.Fatalf("tail ERROR StartByte = %d, want %d", got, want)
	}
	if got, want := tail.EndByte(), end("EXEC CICS RETURN"); got != want {
		t.Fatalf("tail ERROR EndByte = %d, want %d", got, want)
	}
	if got, want := tail.ChildCount(), 2; got != want {
		t.Fatalf("tail ERROR ChildCount = %d, want %d", got, want)
	}
	if got, want := tail.Child(1).Type(lang), "comment_entry"; got != want {
		t.Fatalf("tail child 1 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(1).StartByte(), end("EXEC CICS RETURN"); got != want {
		t.Fatalf("tail comment_entry StartByte = %d, want %d", got, want)
	}
	if got, want := root.Child(3).Type(lang), "comment"; got != want {
		t.Fatalf("root child 3 type = %q, want %q", got, want)
	}
}

func TestNormalizeCobolAcceptedRootProgramDefinitionSplitsTailError(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			"start",
			"comment",
			"program_definition",
			"identification_division",
			"data_division",
			"procedure_division",
			".",
			"move_statement",
			"comment_entry",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "data_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "move_statement", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("      * banner\n" +
		"       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. A.\n" +
		"       DATA DIVISION.\n" +
		"       PROCEDURE DIVISION.\n" +
		"           MOVE A TO B.\n" +
		"           EXEC CICS RETURN\n" +
		"      * trailer\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	leadingComment := cobolTestLeaf(arena, 2, true, source, end("banner"), end("banner"))
	id := cobolTestLeaf(arena, 4, true, source, idx("IDENTIFICATION"), end("PROGRAM-ID. A."))
	data := cobolTestLeaf(arena, 5, true, source, idx("DATA DIVISION."), end("DATA DIVISION."))
	procedurePeriod := cobolTestLeaf(arena, 7, false, source, end("PROCEDURE DIVISION"), end("PROCEDURE DIVISION."))
	move := cobolTestLeaf(arena, 8, true, source, idx("MOVE A TO B."), end("MOVE A TO B."))
	execErr := cobolTestLeaf(arena, errorSymbol, true, source, idx("EXEC CICS RETURN"), end("EXEC CICS RETURN"))
	execErr.setHasError(true)
	trailingComment := cobolTestLeaf(arena, 2, true, source, end("trailer"), end("trailer"))
	root := newParentNodeInArena(arena, 1, true, []*Node{
		leadingComment,
		id,
		data,
		procedurePeriod,
		move,
		execErr,
		trailingComment,
	}, nil, 0)
	root.setHasError(true)
	root.startByte = 6
	root.startPoint = Point{Column: 6}
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 4; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(0).Type(lang), "comment"; got != want {
		t.Fatalf("root child 0 type = %q, want %q", got, want)
	}
	program := root.Child(1)
	if got, want := program.Type(lang), "program_definition"; got != want {
		t.Fatalf("root child 1 type = %q, want %q", got, want)
	}
	procedure := program.Child(program.ChildCount() - 1)
	if got, want := procedure.Type(lang), "procedure_division"; got != want {
		t.Fatalf("last program child type = %q, want %q", got, want)
	}
	if got, want := procedure.StartByte(), idx("PROCEDURE DIVISION."); got != want {
		t.Fatalf("procedure_division.StartByte = %d, want %d", got, want)
	}
	tail := root.Child(2)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("root child 2 type = %q, want %q", got, want)
	}
	if !tail.IsExtra() || !tail.HasError() {
		t.Fatalf("tail ERROR flags extra=%v hasError=%v, want both true", tail.IsExtra(), tail.HasError())
	}
	if got, want := tail.StartByte(), idx("MOVE A TO B."); got != want {
		t.Fatalf("tail ERROR StartByte = %d, want %d", got, want)
	}
	if got, want := tail.EndByte(), end("EXEC CICS RETURN"); got != want {
		t.Fatalf("tail ERROR EndByte = %d, want %d", got, want)
	}
	if got, want := tail.ChildCount(), 2; got != want {
		t.Fatalf("tail ERROR ChildCount = %d, want %d; tail=%s", got, want, tail.SExpr(lang))
	}
	if got, want := tail.Child(0).Type(lang), "move_statement"; got != want {
		t.Fatalf("tail child 0 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(1).Type(lang), "comment_entry"; got != want {
		t.Fatalf("tail child 1 type = %q, want %q", got, want)
	}
	if got, want := root.Child(3).Type(lang), "comment"; got != want {
		t.Fatalf("root child 3 type = %q, want %q", got, want)
	}
}

func TestNormalizeCobolProcedureRootRecoveryHoistsComments(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "comment", "program_definition", "procedure_division", ".", "comment_entry"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("       PROCEDURE DIVISION.\n" +
		"      *****************************************************************\n" +
		"      * banner                                                       *\n" +
		"      *****************************************************************\n" +
		"           EXEC CICS LINK PROGRAM('A')\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	headerDot := cobolTestLeaf(arena, 5, false, source, end("PROCEDURE DIVISION"), end("PROCEDURE DIVISION."))
	comment1 := cobolTestLeaf(arena, 2, true, source, end("*****************************************************************"), end("*****************************************************************"))
	comment2 := cobolTestLeaf(arena, 2, true, source, end("banner                                                       *"), end("banner                                                       *"))
	comment3 := cobolTestLeaf(arena, 2, true, source, end("*****************************************************************\n           EXEC")-uint32(len("\n           EXEC")), end("*****************************************************************\n           EXEC")-uint32(len("\n           EXEC")))
	proc := newParentNodeInArena(arena, 4, true, []*Node{headerDot, comment1, comment2, comment3}, nil, 0)
	proc.startByte = idx("PROCEDURE DIVISION.")
	proc.startPoint = advancePointByBytes(Point{}, source[:proc.startByte])
	proc.endByte = comment3.endByte
	proc.endPoint = comment3.endPoint
	program := newParentNodeInArena(arena, 3, true, []*Node{proc}, nil, 0)
	program.startByte = proc.startByte
	program.startPoint = proc.startPoint
	program.endByte = proc.endByte
	program.endPoint = proc.endPoint
	errEntry := cobolTestLeaf(arena, 6, true, source, end("EXEC CICS LINK PROGRAM('A')"), end("EXEC CICS LINK PROGRAM('A')"))
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{errEntry}, nil, 0)
	err.startByte = idx("PROGRAM('A')")
	err.startPoint = advancePointByBytes(Point{}, source[:err.startByte])
	err.endByte = errEntry.endByte
	err.endPoint = errEntry.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{program, err}, nil, 0)
	root.startByte = proc.startByte
	root.startPoint = proc.startPoint
	root.endByte = err.endByte
	root.endPoint = err.endPoint
	// Real recovered COBOL roots can be under-marked even when an adjacent
	// ERROR child is present; the recovery must key off the child shape.

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 5; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if got, want := program.EndByte(), end("PROCEDURE DIVISION."); got != want {
		t.Fatalf("program.EndByte = %d, want %d", got, want)
	}
	if got, want := proc.ChildCount(), 1; got != want {
		t.Fatalf("procedure child count = %d, want %d", got, want)
	}
	for i := 1; i <= 3; i++ {
		if got, want := root.Child(i).Type(lang), "comment"; got != want {
			t.Fatalf("root child %d type = %q, want %q", i, got, want)
		}
	}
	tail := root.Child(4)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
	if got, want := tail.StartByte(), idx("EXEC CICS LINK"); got != want {
		t.Fatalf("tail.StartByte = %d, want %d", got, want)
	}
	if got, want := tail.ChildCount(), 1; got != want {
		t.Fatalf("tail.ChildCount = %d, want %d; tail=%s", got, want, tail.SExpr(lang))
	}
}

func TestNormalizeCobolProcedureRootRecoveryMovesStatements(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "comment", "program_definition", "procedure_division", ".", "move_statement", "period", "comment_entry"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "move_statement", Visible: true, Named: true},
			{Name: "period", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("       PROCEDURE DIVISION.\n" +
		"      *****************************************************************\n" +
		"      * banner                                                       *\n" +
		"      *****************************************************************\n" +
		"           MOVE A TO B.\n" +
		"           EXEC CICS GETMAIN\n" +
		"                     SET(X)\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	headerDot := cobolTestLeaf(arena, 5, false, source, end("PROCEDURE DIVISION"), end("PROCEDURE DIVISION."))
	comment1 := cobolTestLeaf(arena, 2, true, source, end("*****************************************************************"), end("*****************************************************************"))
	comment2 := cobolTestLeaf(arena, 2, true, source, end("banner                                                       *"), end("banner                                                       *"))
	comment3 := cobolTestLeaf(arena, 2, true, source, end("*****************************************************************\n           MOVE")-uint32(len("\n           MOVE")), end("*****************************************************************\n           MOVE")-uint32(len("\n           MOVE")))
	moveStmt := cobolTestLeaf(arena, 6, true, source, idx("MOVE A TO B"), end("MOVE A TO B"))
	movePeriod := cobolTestLeaf(arena, 7, true, source, end("MOVE A TO B"), end("MOVE A TO B."))
	proc := newParentNodeInArena(arena, 4, true, []*Node{headerDot, comment1, comment2, comment3, moveStmt, movePeriod}, nil, 0)
	proc.startByte = idx("PROCEDURE DIVISION.")
	proc.startPoint = advancePointByBytes(Point{}, source[:proc.startByte])
	proc.endByte = movePeriod.endByte
	proc.endPoint = movePeriod.endPoint
	program := newParentNodeInArena(arena, 3, true, []*Node{proc}, nil, 0)
	program.startByte = proc.startByte
	program.startPoint = proc.startPoint
	program.endByte = proc.endByte
	program.endPoint = proc.endPoint
	setEntry := cobolTestLeaf(arena, 8, true, source, end("SET(X)"), end("SET(X)"))
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{setEntry}, nil, 0)
	err.startByte = idx("SET(X)")
	err.startPoint = advancePointByBytes(Point{}, source[:err.startByte])
	err.endByte = setEntry.endByte
	err.endPoint = setEntry.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{program, err}, nil, 0)
	root.startByte = proc.startByte
	root.startPoint = proc.startPoint
	root.endByte = err.endByte
	root.endPoint = err.endPoint
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 5; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if got, want := program.EndByte(), end("PROCEDURE DIVISION."); got != want {
		t.Fatalf("program.EndByte = %d, want %d", got, want)
	}
	tail := root.Child(4)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
	if got, want := tail.StartByte(), idx("MOVE A TO B"); got != want {
		t.Fatalf("tail.StartByte = %d, want %d", got, want)
	}
	if got, want := tail.ChildCount(), 4; got != want {
		t.Fatalf("tail.ChildCount = %d, want %d; tail=%s", got, want, tail.SExpr(lang))
	}
	if got, want := tail.Child(0).Type(lang), "move_statement"; got != want {
		t.Fatalf("tail child 0 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(1).Type(lang), "period"; got != want {
		t.Fatalf("tail child 1 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(2).StartByte(), end("EXEC CICS GETMAIN"); got != want {
		t.Fatalf("tail child 2 StartByte = %d, want %d", got, want)
	}
	if got, want := tail.Child(3).StartByte(), end("SET(X)"); got != want {
		t.Fatalf("tail child 3 StartByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolRootErrorLeadingComments(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "comment", "program_definition", "comment_entry"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("       PROCEDURE DIVISION.\n" +
		"      *****************************************************************\n" +
		"      * banner                                                       *\n" +
		"      *****************************************************************\n" +
		"           EXEC CICS LINK PROGRAM('A')\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	program := cobolTestLeaf(arena, 3, true, source, idx("PROCEDURE DIVISION."), end("PROCEDURE DIVISION."))
	entry := cobolTestLeaf(arena, 4, true, source, end("EXEC CICS LINK PROGRAM('A')"), end("EXEC CICS LINK PROGRAM('A')"))
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{entry}, nil, 0)
	err.startByte = end("PROCEDURE DIVISION.\n")
	err.startPoint = advancePointByBytes(Point{}, source[:err.startByte])
	err.endByte = entry.endByte
	err.endPoint = entry.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{program, err}, nil, 0)
	root.startByte = program.startByte
	root.startPoint = program.startPoint
	root.endByte = err.endByte
	root.endPoint = err.endPoint
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 5; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	for i := 1; i <= 3; i++ {
		if got, want := root.Child(i).Type(lang), "comment"; got != want {
			t.Fatalf("root child %d type = %q, want %q", i, got, want)
		}
	}
	tail := root.Child(4)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
	if got, want := tail.StartByte(), idx("EXEC CICS LINK"); got != want {
		t.Fatalf("tail.StartByte = %d, want %d", got, want)
	}
	if got, want := tail.ChildCount(), 1; got != want {
		t.Fatalf("tail.ChildCount = %d, want %d", got, want)
	}
}

func TestNormalizeCobolZLiteralDataRootRecovery(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			"start",
			"program_definition",
			"data_division",
			"working_storage_section",
			"data_description",
			"level_number",
			"entry_name",
			"picture_clause",
			"picture_x",
			"value_clause",
			"comment_entry",
			"comment",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "data_division", Visible: true, Named: true},
			{Name: "working_storage_section", Visible: true, Named: true},
			{Name: "data_description", Visible: true, Named: true},
			{Name: "level_number", Visible: true, Named: true},
			{Name: "entry_name", Visible: true, Named: true},
			{Name: "picture_clause", Visible: true, Named: true},
			{Name: "picture_x", Visible: true, Named: true},
			{Name: "value_clause", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
		},
	}
	source := []byte("       DATA DIVISION.\n" +
		"       WORKING-STORAGE SECTION.\n" +
		"       01  OK PIC X.\n" +
		"       01  BAD PIC X(120)\n" +
		"           VALUE Z'HELLO'.\n" +
		"      * banner\n" +
		"       PROCEDURE DIVISION.\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	okDesc := cobolTestLeaf(arena, 5, true, source, idx("01  OK"), end("01  OK PIC X."))
	level := cobolTestLeaf(arena, 6, true, source, idx("01  BAD"), idx("01  BAD")+2)
	name := cobolTestLeaf(arena, 7, true, source, idx("BAD"), end("BAD"))
	picture := cobolTestLeaf(arena, 8, true, source, idx("PIC X(120)"), end("PIC X(120)"))
	value := cobolTestLeaf(arena, 10, true, source, idx("VALUE Z'HELLO'"), end("VALUE Z'HELLO'"))
	badDesc := newParentNodeInArena(arena, 5, true, []*Node{level, name, picture, value}, nil, 0)
	badDesc.startByte = idx("01  BAD")
	badDesc.startPoint = advancePointByBytes(Point{}, source[:badDesc.startByte])
	badDesc.endByte = end("VALUE Z'HELLO'.")
	badDesc.endPoint = advancePointByBytes(Point{}, source[:badDesc.endByte])
	working := newParentNodeInArena(arena, 4, true, []*Node{okDesc, badDesc}, nil, 0)
	working.startByte = idx("WORKING-STORAGE SECTION.")
	working.startPoint = advancePointByBytes(Point{}, source[:working.startByte])
	working.endByte = badDesc.endByte
	working.endPoint = badDesc.endPoint
	data := newParentNodeInArena(arena, 3, true, []*Node{working}, nil, 0)
	data.startByte = idx("DATA DIVISION.")
	data.startPoint = advancePointByBytes(Point{}, source[:data.startByte])
	data.endByte = badDesc.endByte
	data.endPoint = badDesc.endPoint
	program := newParentNodeInArena(arena, 2, true, []*Node{data}, nil, 0)
	program.startByte = data.startByte
	program.startPoint = data.startPoint
	program.endByte = data.endByte
	program.endPoint = data.endPoint
	errEntry := cobolTestLeaf(arena, 11, true, source, end("PROCEDURE DIVISION."), end("PROCEDURE DIVISION."))
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{errEntry}, nil, 0)
	err.startByte = idx("PROCEDURE DIVISION.")
	err.startPoint = advancePointByBytes(Point{}, source[:err.startByte])
	err.endByte = errEntry.endByte
	err.endPoint = errEntry.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{program, err}, nil, 0)
	root.startByte = program.startByte
	root.startPoint = program.startPoint
	root.endByte = err.endByte
	root.endPoint = err.endPoint
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := program.EndByte(), end("01  OK PIC X."); got != want {
		t.Fatalf("program.EndByte = %d, want %d", got, want)
	}
	tail := root.Child(1)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
	if got, want := tail.StartByte(), idx("01  BAD"); got != want {
		t.Fatalf("tail.StartByte = %d, want %d", got, want)
	}
	if got, want := tail.ChildCount(), 6; got != want {
		t.Fatalf("tail.ChildCount = %d, want %d; tail=%s", got, want, tail.SExpr(lang))
	}
	if got, want := tail.Child(0).Type(lang), "level_number"; got != want {
		t.Fatalf("tail child 0 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(2).Type(lang), "picture_clause"; got != want {
		t.Fatalf("tail child 2 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(3).Type(lang), "comment_entry"; got != want {
		t.Fatalf("tail child 3 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(4).Type(lang), "comment"; got != want {
		t.Fatalf("tail child 4 type = %q, want %q", got, want)
	}
	if got, want := tail.Child(5).StartByte(), end("PROCEDURE DIVISION."); got != want {
		t.Fatalf("tail child 5 StartByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolRootProgramDefinitionStopsBeforeCopySibling(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			"start",
			"program_definition",
			"identification_division",
			"data_division",
			"working_storage_section",
			"data_description",
			"copy_statement",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "data_division", Visible: true, Named: true},
			{Name: "working_storage_section", Visible: true, Named: true},
			{Name: "data_description", Visible: true, Named: true},
			{Name: "copy_statement", Visible: true, Named: true},
		},
	}
	source := []byte("       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. SAMPLE.\n" +
		"       DATA DIVISION.\n" +
		"       WORKING-STORAGE SECTION.\n" +
		"       01  WS-COMMAREA PIC X.\n" +
		"       COPY CENTRY.\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	id := cobolTestLeaf(arena, 3, true, source, idx("IDENTIFICATION"), end("PROGRAM-ID. SAMPLE."))
	desc := cobolTestLeaf(arena, 6, true, source, idx("01  WS-COMMAREA"), end("01  WS-COMMAREA PIC X."))
	working := newParentNodeInArena(arena, 5, true, []*Node{desc}, nil, 0)
	working.startByte = idx("WORKING-STORAGE SECTION.")
	working.startPoint = advancePointByBytes(Point{}, source[:working.startByte])
	working.endByte = uint32(len(source))
	working.endPoint = advancePointByBytes(Point{}, source)
	data := newParentNodeInArena(arena, 4, true, []*Node{working}, nil, 0)
	data.startByte = idx("DATA DIVISION.")
	data.startPoint = advancePointByBytes(Point{}, source[:data.startByte])
	data.endByte = uint32(len(source))
	data.endPoint = advancePointByBytes(Point{}, source)
	program := newParentNodeInArena(arena, 2, true, []*Node{id, data}, nil, 0)
	program.startByte = id.startByte
	program.startPoint = id.startPoint
	program.endByte = uint32(len(source))
	program.endPoint = advancePointByBytes(Point{}, source)
	copyStart := idx("COPY CENTRY.")
	copyStmt := cobolTestLeaf(arena, 7, true, source, copyStart, end("COPY CENTRY."))
	copyStmt.setExtra(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{program, copyStmt}, nil, 0)
	root.startByte = program.startByte
	root.startPoint = program.startPoint
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	clampedEnd := end("01  WS-COMMAREA PIC X.")
	if got, want := program.EndByte(), clampedEnd; got != want {
		t.Fatalf("program.EndByte = %d, want %d", got, want)
	}
	if got, want := data.EndByte(), clampedEnd; got != want {
		t.Fatalf("data_division.EndByte = %d, want %d", got, want)
	}
	if got, want := working.EndByte(), clampedEnd; got != want {
		t.Fatalf("working_storage_section.EndByte = %d, want %d", got, want)
	}
	if got, want := root.Child(1).StartByte(), copyStart; got != want {
		t.Fatalf("copy_statement.StartByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolRootProcedurePrefixErrorMovesCleanPrefix(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			".",
			"start",
			"program_definition",
			"identification_division",
			"procedure_division",
			"comment",
			"move_statement",
			"SPACE",
			"qualified_word",
			"copy_statement",
			"if_header",
			"comment_entry",
			"expr",
			"is_class",
			"WORD",
			"AND",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "move_statement", Visible: true, Named: true},
			{Name: "SPACE", Visible: true, Named: true},
			{Name: "qualified_word", Visible: true, Named: true},
			{Name: "copy_statement", Visible: true, Named: true},
			{Name: "if_header", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
			{Name: "expr", Visible: true, Named: true},
			{Name: "is_class", Visible: true, Named: true},
			{Name: "WORD", Visible: true, Named: true},
			{Name: "AND", Visible: true, Named: true},
		},
	}
	source := []byte("       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. A.\n" +
		"       PROCEDURE DIVISION.\n" +
		"      * Procedure comment\n" +
		"           MOVE SPACES TO ABEND-REASON\n" +
		"           COPY CABENDPO.\n" +
		"           IF BANK-MAP-FUNCTION-GET\n" +
		"              EXEC CICS LINK PROGRAM(X)\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	id := cobolTestLeaf(arena, 4, true, source, idx("IDENTIFICATION"), end("PROGRAM-ID. A."))
	dot := cobolTestLeaf(arena, 1, false, source, end("PROCEDURE DIVISION"), end("PROCEDURE DIVISION."))
	proc := newParentNodeInArena(arena, 5, true, []*Node{dot}, nil, 0)
	proc.startByte = idx("PROCEDURE DIVISION.")
	proc.startPoint = advancePointByBytes(Point{}, source[:proc.startByte])
	proc.endByte = dot.endByte
	proc.endPoint = dot.endPoint
	def := newParentNodeInArena(arena, 3, true, []*Node{id, proc}, nil, 0)
	def.startByte = id.startByte
	def.startPoint = id.startPoint
	def.endByte = proc.endByte
	def.endPoint = proc.endPoint

	comment := cobolTestLeaf(arena, 6, true, source, end("Procedure comment"), end("Procedure comment"))
	moveStart := idx("MOVE SPACES")
	moveTargetEnd := end("ABEND-REASON")
	space := cobolTestLeaf(arena, 8, true, source, idx("SPACES"), end("SPACES"))
	target := cobolTestLeaf(arena, 9, true, source, idx("ABEND-REASON"), moveTargetEnd)
	move := newParentNodeInArena(arena, 7, true, []*Node{space, target}, nil, 0)
	move.startByte = moveStart
	move.startPoint = advancePointByBytes(Point{}, source[:move.startByte])
	move.endByte = end("COPY CABENDPO.")
	move.endPoint = advancePointByBytes(Point{}, source[:move.endByte])
	copyStmt := cobolTestLeaf(arena, 10, true, source, idx("COPY CABENDPO."), end("COPY CABENDPO."))
	copyStmt.setExtra(true)
	execStart := idx("EXEC CICS")
	execEnd := execStart + uint32(len("EXEC"))
	cicsStart := execStart + uint32(len("EXEC "))
	bankWord := cobolTestLeaf(arena, 9, true, source, idx("BANK-MAP-FUNCTION-GET"), end("BANK-MAP-FUNCTION-GET"))
	execWord := cobolTestLeaf(arena, 15, true, source, execStart, execEnd)
	isClass := newParentNodeInArena(arena, 14, true, []*Node{bankWord, execWord}, nil, 0)
	isClass.startByte = bankWord.startByte
	isClass.startPoint = bankWord.startPoint
	isClass.endByte = execEnd
	isClass.endPoint = advancePointByBytes(Point{}, source[:execEnd])
	innerExpr := newParentNodeInArena(arena, 13, true, []*Node{isClass}, nil, 0)
	innerExpr.startByte = isClass.startByte
	innerExpr.startPoint = isClass.startPoint
	innerExpr.endByte = isClass.endByte
	innerExpr.endPoint = isClass.endPoint
	and := cobolTestLeaf(arena, 16, true, source, cicsStart, cicsStart)
	cicsTail := cobolTestLeaf(arena, 13, true, source, cicsStart, end("CICS LINK"))
	outerExpr := newParentNodeInArena(arena, 13, true, []*Node{innerExpr, and, cicsTail}, nil, 0)
	outerExpr.startByte = innerExpr.startByte
	outerExpr.startPoint = innerExpr.startPoint
	outerExpr.endByte = cicsTail.endByte
	outerExpr.endPoint = cicsTail.endPoint
	ifHeader := newParentNodeInArena(arena, 11, true, []*Node{outerExpr}, nil, 0)
	ifHeader.startByte = idx("IF BANK-MAP-FUNCTION-GET")
	ifHeader.startPoint = advancePointByBytes(Point{}, source[:ifHeader.startByte])
	ifHeader.endByte = cicsTail.endByte
	ifHeader.endPoint = cicsTail.endPoint
	execLineEnd := uint32(cobolLineStart(source, int(execStart)))
	for int(execLineEnd) < len(source) && source[execLineEnd] != '\n' && source[execLineEnd] != '\r' {
		execLineEnd++
	}
	marker := cobolTestLeaf(arena, 12, true, source, execLineEnd, execLineEnd)
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{move, copyStmt, ifHeader, marker}, nil, 0)
	err.startByte = move.startByte
	err.startPoint = move.startPoint
	err.endByte = marker.endByte
	err.endPoint = marker.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 2, true, []*Node{def, comment, err}, nil, 0)
	root.startByte = def.startByte
	root.startPoint = def.startPoint
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	procedure := root.Child(0).Child(1)
	if got, want := procedure.ChildCount(), 5; got != want {
		t.Fatalf("procedure_division.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := procedure.Child(1).Type(lang), "comment"; got != want {
		t.Fatalf("moved child type = %q, want %q", got, want)
	}
	if got, want := procedure.Child(2).EndByte(), moveTargetEnd; got != want {
		t.Fatalf("trimmed move_statement.EndByte = %d, want %d", got, want)
	}
	if got, want := procedure.Child(4).EndByte(), execEnd; got != want {
		t.Fatalf("trimmed if_header.EndByte = %d, want %d", got, want)
	}
	if got, want := procedure.Child(4).Child(0).Child(0).Type(lang), "is_class"; got != want {
		t.Fatalf("trimmed if_header expr child type = %q, want %q", got, want)
	}
	if got, want := procedure.StartByte(), idx("PROCEDURE DIVISION."); got != want {
		t.Fatalf("procedure_division.StartByte = %d, want %d", got, want)
	}
	if got, want := procedure.EndByte(), execEnd; got != want {
		t.Fatalf("procedure_division.EndByte = %d, want %d", got, want)
	}
	tail := root.Child(1)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
	if got, want := tail.StartByte(), cicsStart; got != want {
		t.Fatalf("tail.StartByte = %d, want %d", got, want)
	}
	if got, want := tail.ChildCount(), 1; got != want {
		t.Fatalf("tail.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := tail.Child(0).Type(lang), "comment_entry"; got != want {
		t.Fatalf("tail child type = %q, want %q", got, want)
	}
}

func TestNormalizeCobolRootProcedurePrefixErrorSkipsMoveLedPrefix(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			".",
			"start",
			"program_definition",
			"identification_division",
			"procedure_division",
			"comment",
			"move_statement",
			"if_header",
			"comment_entry",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "move_statement", Visible: true, Named: true},
			{Name: "if_header", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. A.\n" +
		"       PROCEDURE DIVISION.\n" +
		"      * Procedure comment\n" +
		"           MOVE A TO B\n" +
		"           IF FLAG\n" +
		"              EXEC CICS RETURN\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	id := cobolTestLeaf(arena, 4, true, source, idx("IDENTIFICATION"), end("PROGRAM-ID. A."))
	dot := cobolTestLeaf(arena, 1, false, source, end("PROCEDURE DIVISION"), end("PROCEDURE DIVISION."))
	proc := newParentNodeInArena(arena, 5, true, []*Node{dot}, nil, 0)
	proc.startByte = idx("PROCEDURE DIVISION.")
	proc.startPoint = advancePointByBytes(Point{}, source[:proc.startByte])
	proc.endByte = dot.endByte
	proc.endPoint = dot.endPoint
	def := newParentNodeInArena(arena, 3, true, []*Node{id, proc}, nil, 0)
	def.startByte = id.startByte
	def.startPoint = id.startPoint
	def.endByte = proc.endByte
	def.endPoint = proc.endPoint

	comment := cobolTestLeaf(arena, 6, true, source, end("Procedure comment"), end("Procedure comment"))
	move := cobolTestLeaf(arena, 7, true, source, idx("MOVE A TO B"), end("MOVE A TO B"))
	execStart := idx("EXEC CICS")
	ifHeader := cobolTestLeaf(arena, 8, true, source, idx("IF FLAG"), execStart+uint32(len("EXEC")))
	execLineEnd := uint32(cobolLineEnd(source, int(execStart)))
	marker := cobolTestLeaf(arena, 9, true, source, execLineEnd, execLineEnd)
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{move, ifHeader, marker}, nil, 0)
	err.startByte = move.startByte
	err.startPoint = move.startPoint
	err.endByte = marker.endByte
	err.endPoint = marker.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 2, true, []*Node{def, comment, err}, nil, 0)
	root.startByte = def.startByte
	root.startPoint = def.startPoint
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 3; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(0).Child(1).ChildCount(), 1; got != want {
		t.Fatalf("procedure_division.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	tail := root.Child(2)
	if got, want := tail.Type(lang), "ERROR"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
	if got, want := tail.StartByte(), move.StartByte(); got != want {
		t.Fatalf("tail.StartByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolRootProcedureEvaluateErrorMovesCleanBody(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			".",
			"start",
			"program_definition",
			"identification_division",
			"procedure_division",
			"evaluate_header",
			"when",
			"goto_statement",
			"END_EVALUATE",
			"period",
			"paragraph_header",
			"comment_entry",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "evaluate_header", Visible: true, Named: true},
			{Name: "when", Visible: true, Named: true},
			{Name: "goto_statement", Visible: true, Named: true},
			{Name: "END_EVALUATE", Visible: true, Named: true},
			{Name: "period", Visible: true, Named: true},
			{Name: "paragraph_header", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       evaluate 1\n" +
		"       when 1\n" +
		"         go to aa\n" +
		"       end-evaluate.\n" +
		"       aa.\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	id := cobolTestLeaf(arena, 4, true, source, idx("identification"), end("program-id. a."))
	dot := cobolTestLeaf(arena, 1, false, source, end("procedure division"), end("procedure division."))
	proc := newParentNodeInArena(arena, 5, true, []*Node{dot}, nil, 0)
	proc.startByte = dot.startByte
	proc.startPoint = advancePointByBytes(Point{}, source[:proc.startByte])
	proc.endByte = dot.endByte
	proc.endPoint = dot.endPoint
	def := newParentNodeInArena(arena, 3, true, []*Node{id, proc}, nil, 0)
	def.startByte = id.startByte
	def.startPoint = id.startPoint
	def.endByte = proc.endByte
	def.endPoint = proc.endPoint

	evaluate := cobolTestLeaf(arena, 6, true, source, idx("evaluate 1"), end("evaluate 1"))
	when := cobolTestLeaf(arena, 7, true, source, idx("when 1"), end("when 1"))
	gotoStmt := cobolTestLeaf(arena, 8, true, source, idx("go to aa"), end("go to aa"))
	endEvaluate := cobolTestLeaf(arena, 9, true, source, idx("end-evaluate"), end("end-evaluate"))
	period := cobolTestLeaf(arena, 10, true, source, end("end-evaluate"), end("end-evaluate."))
	paragraphMarker := cobolTestLeaf(arena, 12, true, source, end("aa."), end("aa."))
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{evaluate, when, gotoStmt, endEvaluate, period, paragraphMarker}, nil, 0)
	err.startByte = evaluate.startByte
	err.startPoint = evaluate.startPoint
	err.endByte = paragraphMarker.endByte
	err.endPoint = paragraphMarker.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 2, true, []*Node{def, err}, nil, 0)
	root.startByte = def.startByte
	root.startPoint = def.startPoint
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if root.HasError() {
		t.Fatalf("root.HasError = true, want false; tree=%s", root.SExpr(lang))
	}
	procedure := root.Child(0).Child(1)
	if got, want := procedure.ChildCount(), 7; got != want {
		t.Fatalf("procedure_division.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := procedure.StartByte(), idx("procedure division."); got != want {
		t.Fatalf("procedure_division.StartByte = %d, want %d", got, want)
	}
	if got, want := procedure.Child(6).Type(lang), "paragraph_header"; got != want {
		t.Fatalf("procedure child 6 type = %q, want %q; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := procedure.EndByte(), end("aa."); got != want {
		t.Fatalf("procedure_division.EndByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolProcedureLooseIfHeader(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			".",
			"start",
			"program_definition",
			"procedure_division",
			"qualified_word",
			"ne",
			"if_header",
			"expr",
			"WORD",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "qualified_word", Visible: true, Named: true},
			{Name: "ne", Visible: true, Named: true},
			{Name: "if_header", Visible: true, Named: true},
			{Name: "expr", Visible: true, Named: true},
			{Name: "WORD", Visible: true, Named: true},
		},
	}
	source := []byte("       PROCEDURE DIVISION.\n" +
		"              IF EIBAID IS NOT EQUAL TO DFHCLEAR\n" +
		"                 EXEC CICS RECEIVE MAP('CSGM')\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	dot := cobolTestLeaf(arena, 1, false, source, end("PROCEDURE DIVISION"), end("PROCEDURE DIVISION."))
	left := cobolTestLeaf(arena, 5, true, source, idx("EIBAID"), end("EIBAID"))
	op := cobolTestLeaf(arena, 6, true, source, idx("IS NOT EQUAL TO"), end("IS NOT EQUAL TO"))
	rightWord := cobolTestLeaf(arena, 9, true, source, idx("DFHCLEAR"), end("DFHCLEAR"))
	execWord := cobolTestLeaf(arena, 9, true, source, idx("EXEC"), end("EXEC"))
	right := newParentNodeInArena(arena, 5, true, []*Node{rightWord, execWord}, nil, 0)
	right.startByte = rightWord.startByte
	right.startPoint = rightWord.startPoint
	right.endByte = execWord.endByte
	right.endPoint = execWord.endPoint
	proc := newParentNodeInArena(arena, 4, true, []*Node{dot, left, op, right}, nil, 0)
	proc.startByte = idx("PROCEDURE DIVISION.")
	proc.startPoint = advancePointByBytes(Point{}, source[:proc.startByte])
	proc.endByte = right.endByte
	proc.endPoint = right.endPoint
	def := newParentNodeInArena(arena, 3, true, []*Node{proc}, nil, 0)
	def.startByte = proc.startByte
	def.startPoint = proc.startPoint
	def.endByte = proc.endByte
	def.endPoint = proc.endPoint
	root := newParentNodeInArena(arena, 2, true, []*Node{def}, nil, 0)
	root.startByte = def.startByte
	root.startPoint = def.startPoint
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	procedure := root.Child(0).Child(0)
	if got, want := procedure.ChildCount(), 2; got != want {
		t.Fatalf("procedure_division.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	header := procedure.Child(1)
	if got, want := header.Type(lang), "if_header"; got != want {
		t.Fatalf("rebuilt child type = %q, want %q", got, want)
	}
	if got, want := header.StartByte(), idx("IF EIBAID"); got != want {
		t.Fatalf("if_header.StartByte = %d, want %d", got, want)
	}
	if got, want := header.EndByte(), end("DFHCLEAR"); got != want {
		t.Fatalf("if_header.EndByte = %d, want %d", got, want)
	}
	if got, want := header.Child(0).Child(2).EndByte(), end("DFHCLEAR"); got != want {
		t.Fatalf("right operand EndByte = %d, want %d", got, want)
	}
	if got, want := procedure.StartByte(), idx("PROCEDURE DIVISION."); got != want {
		t.Fatalf("procedure_division.StartByte = %d, want %d", got, want)
	}
	if got, want := procedure.EndByte(), end("DFHCLEAR"); got != want {
		t.Fatalf("procedure_division.EndByte = %d, want %d", got, want)
	}
	if got, want := root.Child(0).EndByte(), end("DFHCLEAR"); got != want {
		t.Fatalf("program_definition.EndByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolRootExecCICSErrorMarkers(t *testing.T) {
	lang := &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			".",
			"start",
			"program_definition",
			"procedure_division",
			"if_header",
			"comment",
			"comment_entry",
			"qualified_word",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "if_header", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
			{Name: "qualified_word", Visible: true, Named: true},
		},
	}
	source := []byte("       PROCEDURE DIVISION.\n" +
		"              IF EIBAID IS NOT EQUAL TO DFHCLEAR\n" +
		"                 EXEC CICS RECEIVE MAP('CSGM')\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	dot := cobolTestLeaf(arena, 1, false, source, end("PROCEDURE DIVISION"), end("PROCEDURE DIVISION."))
	header := cobolTestLeaf(arena, 5, true, source, idx("IF EIBAID"), end("DFHCLEAR"))
	proc := newParentNodeInArena(arena, 4, true, []*Node{dot, header}, nil, 0)
	proc.startByte = idx("PROCEDURE DIVISION.")
	proc.startPoint = advancePointByBytes(Point{}, source[:proc.startByte])
	proc.endByte = header.endByte
	proc.endPoint = header.endPoint
	def := newParentNodeInArena(arena, 3, true, []*Node{proc}, nil, 0)
	def.startByte = proc.startByte
	def.startPoint = proc.startPoint
	def.endByte = proc.endByte
	def.endPoint = proc.endPoint
	execStart := idx("EXEC CICS")
	cicsStart := idx("CICS RECEIVE")
	lineEnd := uint32(cobolLineEnd(source, int(execStart)))
	raw := cobolTestLeaf(arena, 8, true, source, cicsStart, end("CICS"))
	marker := cobolTestLeaf(arena, 7, true, source, lineEnd, lineEnd)
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{raw, marker}, nil, 0)
	err.startByte = cicsStart
	err.startPoint = advancePointByBytes(Point{}, source[:cicsStart])
	err.endByte = lineEnd
	err.endPoint = advancePointByBytes(Point{}, source[:lineEnd])
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 2, true, []*Node{def, err}, nil, 0)
	root.startByte = def.startByte
	root.startPoint = def.startPoint
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)
	root.setHasError(true)

	normalizeCobolRootExecCICSErrorMarkers(root, source, lang)

	var tail *Node
	for i := 0; i < root.ChildCount(); i++ {
		if child := root.Child(i); child != nil && child.IsError() {
			tail = child
			break
		}
	}
	if tail == nil {
		t.Fatalf("missing root ERROR; tree=%s", root.SExpr(lang))
	}
	if got, want := tail.StartByte(), execStart; got != want {
		t.Fatalf("tail.StartByte = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := tail.ChildCount(), 1; got != want {
		t.Fatalf("tail.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := tail.Child(0).Type(lang), "comment_entry"; got != want {
		t.Fatalf("tail child type = %q, want %q", got, want)
	}
	if got, want := tail.Child(0).StartByte(), lineEnd; got != want {
		t.Fatalf("tail marker StartByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolRecoveredErrorAddsMissingCommentEntryGap(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "comment", "comment_entry", "move_statement", "period"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
			{Name: "move_statement", Visible: true, Named: true},
			{Name: "period", Visible: true, Named: true},
		},
	}
	source := []byte("           MOVE A TO B.\n" +
		"      *COPY CRETURN.\n" +
		"           EXEC CICS RETURN\n" +
		"           END-EXEC.\n" +
		"              MOVE '0001-01-01 00:00:00.000000' TO CD05I-START-ID       \n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	move := cobolTestLeaf(arena, 4, true, source, idx("MOVE A TO B"), end("MOVE A TO B"))
	period := cobolTestLeaf(arena, 5, true, source, end("MOVE A TO B"), end("MOVE A TO B."))
	comment := cobolTestLeaf(arena, 2, true, source, end("*COPY CRETURN."), end("*COPY CRETURN."))
	paddedCarrier := cobolTestLeaf(arena, 3, true, source, end("CD05I-START-ID       "), end("CD05I-START-ID       "))
	paddedCarrier.setHasError(true)
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{move, period, comment, paddedCarrier}, nil, 0)
	err.startByte = move.startByte
	err.startPoint = move.startPoint
	err.endByte = paddedCarrier.endByte
	err.endPoint = paddedCarrier.endPoint
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{err}, nil, 0)
	root.startByte = err.startByte
	root.startPoint = err.startPoint
	root.endByte = err.endByte
	root.endPoint = err.endPoint
	root.setHasError(true)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := err.ChildCount(), 6; got != want {
		t.Fatalf("ERROR ChildCount = %d, want %d; err=%s", got, want, err.SExpr(lang))
	}
	if got, want := err.Child(3).Type(lang), "comment_entry"; got != want {
		t.Fatalf("missing gap marker type = %q, want %q", got, want)
	}
	if got, want := err.Child(3).StartByte(), end("EXEC CICS RETURN"); got != want {
		t.Fatalf("gap marker StartByte = %d, want %d", got, want)
	}
	if got, want := err.Child(4).StartByte(), end("END-EXEC."); got != want {
		t.Fatalf("END-EXEC marker StartByte = %d, want %d", got, want)
	}
	paddedLineStart := uint32(cobolLineStart(source, int(idx("MOVE '0001-01-01"))))
	if got, want := err.Child(5).StartByte(), paddedLineStart+71; got != want {
		t.Fatalf("padded marker StartByte = %d, want %d", got, want)
	}
	if err.Child(3).HasError() || err.Child(4).HasError() || err.Child(5).HasError() {
		t.Fatal("synthesized comment_entry markers should not carry child error flags")
	}
}

func TestNormalizeCobolRootCommentsCoveredByError(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "program_definition", "comment", "comment_entry"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "comment_entry", Visible: true, Named: true},
		},
	}
	source := []byte("PROGRAM.\n       01  X PIC X.\n      * duplicate\n       GOBACK.\n      * trailing\n")
	idx := func(needle string) uint32 {
		t.Helper()
		pos := bytes.Index(source, []byte(needle))
		if pos < 0 {
			t.Fatalf("missing %q in test source", needle)
		}
		return uint32(pos)
	}
	end := func(needle string) uint32 { return idx(needle) + uint32(len(needle)) }

	arena := newNodeArena(arenaClassFull)
	program := cobolTestLeaf(arena, 2, true, source, idx("PROGRAM."), end("PROGRAM."))
	coveredComment := cobolTestLeaf(arena, 3, true, source, end("duplicate"), end("duplicate"))
	entry := cobolTestLeaf(arena, 4, true, source, end("GOBACK."), end("GOBACK."))
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{entry}, nil, 0)
	err.startByte = idx("01  X")
	err.startPoint = advancePointByBytes(Point{}, source[:err.startByte])
	err.endByte = entry.endByte
	err.endPoint = entry.endPoint
	err.setExtra(true)
	err.setHasError(true)
	trailingComment := cobolTestLeaf(arena, 3, true, source, end("trailing"), end("trailing"))
	root := newParentNodeInArena(arena, 1, true, []*Node{program, coveredComment, err, trailingComment}, nil, 0)
	root.startByte = program.startByte
	root.startPoint = program.startPoint
	root.endByte = trailingComment.endByte
	root.endPoint = trailingComment.endPoint
	root.setHasError(true)

	normalizeCobolRootCommentsCoveredByError(root, lang)

	if got, want := root.ChildCount(), 3; got != want {
		t.Fatalf("root.ChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(1).Type(lang), "ERROR"; got != want {
		t.Fatalf("root child 1 type = %q, want %q", got, want)
	}
	if got, want := root.Child(2).Type(lang), "comment"; got != want {
		t.Fatalf("root child 2 type = %q, want %q", got, want)
	}
}

func TestCobolAppendCommentEntryMarkersSplitsColumn72Content(t *testing.T) {
	source := []byte("              MOVE BANK-SCR60-NEW-SEND-EMAIL TO CD02I-CONTACT-SEND-EMAIL\n")
	line := bytes.TrimSuffix(source, []byte("\n"))
	if got, want := len(line), 72; got != want {
		t.Fatalf("test line length = %d, want %d", got, want)
	}

	arena := newNodeArena(arenaClassFull)
	out := cobolAppendCommentEntryMarkers(nil, arena, source, 0, uint32(len(source)), 3, true)

	if got, want := len(out), 2; got != want {
		t.Fatalf("marker count = %d, want %d", got, want)
	}
	if got, want := out[0].StartByte(), uint32(71); got != want {
		t.Fatalf("source-area marker StartByte = %d, want %d", got, want)
	}
	if got, want := out[1].StartByte(), uint32(72); got != want {
		t.Fatalf("physical-end marker StartByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolFixedFormatProgramIDStopsBeforeTrailingJunk(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "program_definition", "identification_division"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
		},
	}
	source := []byte("aaaaaa identification division.\n" +
		"aaaaaa program-id. a.  ,,, ;;;                                          aaaaa\n" +
		"      *aaaa\n")

	arena := newNodeArena(arenaClassFull)
	div := newLeafNodeInArena(arena, 3, true, 6, 116, Point{Column: 6}, Point{Row: 2, Column: 6})
	def := newParentNodeInArena(arena, 2, true, []*Node{div}, nil, 0)
	def.startByte = 6
	def.startPoint = Point{Column: 6}
	def.endByte = 116
	def.endPoint = Point{Row: 2, Column: 6}
	root := newParentNodeInArena(arena, 1, true, []*Node{def}, nil, 0)
	root.startByte = 6
	root.startPoint = Point{Column: 6}
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.StartByte(), uint32(6); got != want {
		t.Fatalf("root.StartByte = %d, want %d", got, want)
	}
	program := root.Child(0)
	if got, want := program.StartByte(), uint32(7); got != want {
		t.Fatalf("program_definition.StartByte = %d, want %d", got, want)
	}
	if got, want := program.EndByte(), uint32(53); got != want {
		t.Fatalf("program_definition.EndByte = %d, want %d", got, want)
	}
	idDiv := program.Child(0)
	if got, want := idDiv.StartByte(), uint32(7); got != want {
		t.Fatalf("identification_division.StartByte = %d, want %d", got, want)
	}
	if got, want := idDiv.EndByte(), uint32(53); got != want {
		t.Fatalf("identification_division.EndByte = %d, want %d", got, want)
	}
}

func TestNormalizeCobolTrimsStatementTrailingTriviaSpans(t *testing.T) {
	lang := cobolTrailingTriviaTestLanguage()
	source := []byte("       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       perform forever\n" +
		"         continue\n" +
		"       end-perform.\n")

	arena := newNodeArena(arenaClassFull)
	id := cobolTestLeaf(arena, 3, true, source, 7, uint32(bytes.Index(source, []byte("procedure division."))))
	performStart := uint32(bytes.Index(source, []byte("perform forever")))
	performEnd := performStart + uint32(len("perform forever"))
	continueStart := uint32(bytes.Index(source, []byte("continue")))
	continueEnd := continueStart + uint32(len("continue"))
	endPerformStart := uint32(bytes.Index(source, []byte("end-perform")))
	periodStart := uint32(bytes.LastIndexByte(source, '.'))
	periodEnd := periodStart + 1

	performText := cobolTestLeaf(arena, 8, false, source, performStart, performEnd)
	perform := newParentNodeInArena(arena, 5, true, []*Node{performText}, nil, 0)
	perform.startByte = performStart
	perform.startPoint = advancePointByBytes(Point{}, source[:performStart])
	perform.endByte = continueStart
	perform.endPoint = advancePointByBytes(Point{}, source[:continueStart])
	continueStmt := cobolTestLeaf(arena, 6, true, source, continueStart, endPerformStart)
	continueStmt.startByte = continueStart
	continueStmt.startPoint = advancePointByBytes(Point{}, source[:continueStart])
	continueStmt.endByte = endPerformStart
	continueStmt.endPoint = advancePointByBytes(Point{}, source[:endPerformStart])
	period := cobolTestLeaf(arena, 7, true, source, periodStart, periodEnd)
	procedureStart := uint32(bytes.Index(source, []byte("procedure division.")))
	proc := newParentNodeInArena(arena, 4, true, []*Node{perform, continueStmt, period}, nil, 0)
	proc.startByte = procedureStart
	proc.startPoint = advancePointByBytes(Point{}, source[:procedureStart])
	proc.endByte = uint32(len(source))
	proc.endPoint = advancePointByBytes(Point{}, source)
	def := newParentNodeInArena(arena, 2, true, []*Node{id, proc}, nil, 0)
	def.startByte = 7
	def.startPoint = advancePointByBytes(Point{}, source[:7])
	def.endByte = uint32(len(source))
	def.endPoint = advancePointByBytes(Point{}, source)
	root := newParentNodeInArena(arena, 1, true, []*Node{def}, nil, 0)
	root.startByte = 7
	root.startPoint = advancePointByBytes(Point{}, source[:7])
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := perform.EndByte(), performEnd; got != want {
		t.Fatalf("perform_statement_loop.EndByte = %d, want %d", got, want)
	}
	if got, want := continueStmt.EndByte(), continueEnd; got != want {
		t.Fatalf("continue_statement.EndByte = %d, want %d", got, want)
	}
	if got, want := proc.EndByte(), periodEnd; got != want {
		t.Fatalf("procedure_division.EndByte = %d, want %d", got, want)
	}
	if got, want := def.EndByte(), periodEnd; got != want {
		t.Fatalf("program_definition.EndByte = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("start.EndByte = %d, want unchanged %d", got, want)
	}
}

func TestNormalizeCobolTrimsGotoTrailingTriviaSpan(t *testing.T) {
	lang := cobolTrailingTriviaTestLanguage()
	source := []byte("       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       evaluate 1\n" +
		"       when 1\n" +
		"         go to aa\n" +
		"       end-evaluate.\n" +
		"       aa.\n")

	arena := newNodeArena(arenaClassFull)
	id := cobolTestLeaf(arena, 3, true, source, 7, uint32(bytes.Index(source, []byte("procedure division."))))
	gotoStart := uint32(bytes.Index(source, []byte("go to aa")))
	gotoEnd := gotoStart + uint32(len("go to aa"))
	endEvaluateStart := uint32(bytes.Index(source, []byte("end-evaluate")))
	periodStart := uint32(bytes.LastIndexByte(source, '.'))
	periodEnd := periodStart + 1

	gotoText := cobolTestLeaf(arena, 10, false, source, gotoStart, gotoEnd)
	gotoStmt := newParentNodeInArena(arena, 11, true, []*Node{gotoText}, nil, 0)
	gotoStmt.startByte = gotoStart
	gotoStmt.startPoint = advancePointByBytes(Point{}, source[:gotoStart])
	gotoStmt.endByte = endEvaluateStart
	gotoStmt.endPoint = advancePointByBytes(Point{}, source[:endEvaluateStart])
	period := cobolTestLeaf(arena, 7, true, source, periodStart, periodEnd)
	procedureStart := uint32(bytes.Index(source, []byte("procedure division.")))
	proc := newParentNodeInArena(arena, 4, true, []*Node{gotoStmt, period}, nil, 0)
	proc.startByte = procedureStart
	proc.startPoint = advancePointByBytes(Point{}, source[:procedureStart])
	proc.endByte = uint32(len(source))
	proc.endPoint = advancePointByBytes(Point{}, source)
	def := newParentNodeInArena(arena, 2, true, []*Node{id, proc}, nil, 0)
	def.startByte = 7
	def.startPoint = advancePointByBytes(Point{}, source[:7])
	def.endByte = uint32(len(source))
	def.endPoint = advancePointByBytes(Point{}, source)
	root := newParentNodeInArena(arena, 1, true, []*Node{def}, nil, 0)
	root.startByte = 7
	root.startPoint = advancePointByBytes(Point{}, source[:7])
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := gotoStmt.EndByte(), gotoEnd; got != want {
		t.Fatalf("goto_statement.EndByte = %d, want %d", got, want)
	}
	if got, want := proc.EndByte(), periodEnd; got != want {
		t.Fatalf("procedure_division.EndByte = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("start.EndByte = %d, want unchanged %d", got, want)
	}
}

func TestNormalizeCobolSectionSiblingEndsBeforeCopySibling(t *testing.T) {
	lang := &Language{
		Name:        "cobol",
		SymbolNames: []string{"EOF", "start", "program_definition", "data_division", "working_storage_section", "copy_statement", "linkage_section"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "data_division", Visible: true, Named: true},
			{Name: "working_storage_section", Visible: true, Named: true},
			{Name: "copy_statement", Visible: true, Named: true},
			{Name: "linkage_section", Visible: true, Named: true},
		},
	}
	source := []byte("       DATA DIVISION.\n" +
		"       WORKING-STORAGE SECTION.\n" +
		"       01  WS-SECURITY.\n" +
		"       COPY CPSWDD01.\n" +
		"       LINKAGE SECTION.\n" +
		"      *COPY CENTRY.\n" +
		"       PROCEDURE DIVISION.\n")

	arena := newNodeArena(arenaClassFull)
	dataStart := uint32(bytes.Index(source, []byte("DATA DIVISION.")))
	workingStart := uint32(bytes.Index(source, []byte("WORKING-STORAGE SECTION.")))
	workingEnd := uint32(bytes.Index(source, []byte("01  WS-SECURITY.")) + len("01  WS-SECURITY."))
	copyStart := uint32(bytes.Index(source, []byte("COPY CPSWDD01.")))
	copyEnd := copyStart + uint32(len("COPY CPSWDD01."))
	linkageStart := uint32(bytes.Index(source, []byte("LINKAGE SECTION.")))
	linkageEnd := linkageStart + uint32(len("LINKAGE SECTION."))
	commentStart := uint32(bytes.Index(source, []byte("*COPY CENTRY.")) + len("*COPY CENTRY."))
	working := cobolTestLeaf(arena, 4, true, source, workingStart, linkageStart)
	copyStmt := cobolTestLeaf(arena, 5, true, source, copyStart, copyEnd)
	copyStmt.setExtra(true)
	linkage := cobolTestLeaf(arena, 6, true, source, linkageStart, commentStart)
	data := newParentNodeInArena(arena, 3, true, []*Node{working, copyStmt, linkage}, nil, 0)
	data.startByte = dataStart
	data.startPoint = advancePointByBytes(Point{}, source[:dataStart])
	data.endByte = linkageEnd
	data.endPoint = advancePointByBytes(Point{}, source[:linkageEnd])
	def := newParentNodeInArena(arena, 2, true, []*Node{data}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{def}, nil, 0)
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := working.EndByte(), workingEnd; got != want {
		t.Fatalf("working_storage_section.EndByte = %d, want %d", got, want)
	}
	if got, want := linkage.EndByte(), linkageEnd; got != want {
		t.Fatalf("linkage_section.EndByte = %d, want %d", got, want)
	}
}

func TestCobolTrailingTriviaAcceptsFixedFormatLineTails(t *testing.T) {
	source := []byte("       identification division.\n" +
		"       program-id. a.\n" +
		"003300 ENVIRONMENT DIVISION.                                            NC1014.2\n" +
		"004200     \"report.log\".                                                NC1014.2\n" +
		"004300 DATA DIVISION.                                                   NC1014.2\n" +
		"004900 77  WRK-DS-18V00                PICTURE S9(18).                  NC1014.2\n" +
		"       PROCEDURE DIVISION.")

	programIDEnd := uint32(bytes.Index(source, []byte("program-id. a.")) + len("program-id. a."))
	envSequenceEnd := uint32(bytes.Index(source, []byte("003300")) + len("003300"))
	if !cobolBytesAreTrailingTrivia(source, programIDEnd, envSequenceEnd) {
		t.Fatalf("expected fixed-format sequence prefix to be trailing trivia")
	}

	reportEnd := uint32(bytes.Index(source, []byte("\"report.log\".")) + len("\"report.log\"."))
	dataStart := uint32(bytes.Index(source, []byte("DATA DIVISION.")))
	if !cobolBytesAreTrailingTrivia(source, reportEnd, dataStart) {
		t.Fatalf("expected fixed-format identification area and next prefix to be trailing trivia")
	}

	pictureEnd := uint32(bytes.Index(source, []byte("PICTURE S9(18).")) + len("PICTURE S9(18)."))
	procedureStart := uint32(bytes.Index(source, []byte("PROCEDURE DIVISION.")))
	if !cobolBytesAreTrailingTrivia(source, pictureEnd, procedureStart) {
		t.Fatalf("expected trailing identification area before procedure division to be trivia")
	}

	gobackEnd := uint32(bytes.Index(source, []byte("PROCEDURE DIVISION.")) + len("PROCEDURE DIVISION."))
	versionComment := []byte("\n      * $ Version 5.99c sequenced on Wednesday 3 Mar 2011 at 1:00pm\n")
	withTrailingComment := append(append([]byte(nil), source[:gobackEnd]...), versionComment...)
	if !cobolBytesAreTrailingTrivia(withTrailingComment, gobackEnd, uint32(len(withTrailingComment))) {
		t.Fatalf("expected trailing fixed-format comment line to be trivia")
	}
}

func TestCobolTrailingTriviaRejectsFreeFormatCode(t *testing.T) {
	source := []byte("display 1.\nmove a to b.")
	displayEnd := uint32(bytes.Index(source, []byte("display 1.")) + len("display 1."))
	if cobolBytesAreTrailingTrivia(source, displayEnd, uint32(len(source))) {
		t.Fatalf("free-format code on the next line must not be trimmed as fixed-format trivia")
	}

	inline := []byte("       display 1. real-code")
	inlineEnd := uint32(bytes.Index(inline, []byte("display 1.")) + len("display 1."))
	if cobolBytesAreTrailingTrivia(inline, inlineEnd, uint32(len(inline))) {
		t.Fatalf("inline content before the identification area must not be trailing trivia")
	}

	longFree := append([]byte("display 1."), bytes.Repeat([]byte(" "), 72-len("display 1."))...)
	longFree = append(longFree, []byte("real-code")...)
	longFreeEnd := uint32(bytes.Index(longFree, []byte("display 1.")) + len("display 1."))
	if cobolBytesAreTrailingTrivia(longFree, longFreeEnd, uint32(len(longFree))) {
		t.Fatalf("column-73 content on a non-fixed-format line must not be trailing trivia")
	}
}

func cobolTrailingTriviaTestLanguage() *Language {
	return &Language{
		Name: "cobol",
		SymbolNames: []string{
			"EOF",
			"start",
			"program_definition",
			"identification_division",
			"procedure_division",
			"perform_statement_loop",
			"continue_statement",
			"period",
			"perform forever",
			"continue",
			"go to aa",
			"goto_statement",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "perform_statement_loop", Visible: true, Named: true},
			{Name: "continue_statement", Visible: true, Named: true},
			{Name: "period", Visible: true, Named: true},
			{Name: "perform forever", Visible: true, Named: false},
			{Name: "continue", Visible: true, Named: false},
			{Name: "go to aa", Visible: true, Named: false},
			{Name: "goto_statement", Visible: true, Named: true},
		},
	}
}

func cobolTestLeaf(arena *nodeArena, sym Symbol, named bool, source []byte, start, end uint32) *Node {
	return newLeafNodeInArena(
		arena,
		sym,
		named,
		start,
		end,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:end]),
	)
}

func TestNormalizeCobolRecoveredParagraphHeader(t *testing.T) {
	lang := &Language{
		Name:                  "COBOL",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"EOF", ".", "start", "program_definition", "identification_division", "procedure_division", "END_EVALUATE", "period", "paragraph_header"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "END_EVALUATE", Visible: true, Named: true},
			{Name: "period", Visible: true, Named: true},
			{Name: "paragraph_header", Visible: true, Named: true},
		},
	}
	source := []byte("       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       evaluate 1\n" +
		"       when 1\n" +
		"         go to aa\n" +
		"       when 2\n" +
		"         go to aa\n" +
		"       when other\n" +
		"         go to aa\n" +
		"       end-evaluate.\n" +
		"       aa.\n")

	arena := newNodeArena(arenaClassFull)
	id := newLeafNodeInArena(arena, 4, true, 7, 53, advancePointByBytes(Point{}, source[:7]), advancePointByBytes(Point{}, source[:53]))
	endEvaluate := newLeafNodeInArena(arena, 6, true, 206, 218, advancePointByBytes(Point{}, source[:206]), advancePointByBytes(Point{}, source[:218]))
	period := newLeafNodeInArena(arena, 7, true, 218, 219, advancePointByBytes(Point{}, source[:218]), advancePointByBytes(Point{}, source[:219]))
	proc := newParentNodeInArena(arena, 5, true, []*Node{endEvaluate, period}, nil, 0)
	proc.startByte = 61
	proc.startPoint = advancePointByBytes(Point{}, source[:61])
	proc.endByte = 230
	proc.endPoint = advancePointByBytes(Point{}, source[:230])
	def := newParentNodeInArena(arena, 3, true, []*Node{id, proc}, nil, 0)
	def.startByte = 7
	def.startPoint = advancePointByBytes(Point{}, source[:7])
	def.endByte = 230
	def.endPoint = advancePointByBytes(Point{}, source[:230])
	errDot := newLeafNodeInArena(arena, 1, false, 229, 230, advancePointByBytes(Point{}, source[:229]), advancePointByBytes(Point{}, source[:230]))
	errDot.setHasError(true)
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{errDot}, nil, 0)
	err.setExtra(true)
	err.setHasError(true)
	err.startByte = 227
	err.startPoint = advancePointByBytes(Point{}, source[:227])
	err.endByte = 230
	err.endPoint = advancePointByBytes(Point{}, source[:230])
	root := newParentNodeInArena(arena, 2, true, []*Node{def, err}, nil, 0)
	root.startByte = 7
	root.startPoint = advancePointByBytes(Point{}, source[:7])
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if root.HasError() {
		t.Fatalf("root.HasError = true, want false")
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root.ChildCount = %d, want %d", got, want)
	}
	procedure := root.Child(0).Child(1)
	if got, want := procedure.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("procedure_division.EndByte = %d, want %d", got, want)
	}
	header := procedure.Child(procedure.ChildCount() - 1)
	if got, want := header.Type(lang), "paragraph_header"; got != want {
		t.Fatalf("last procedure child type = %q, want %q", got, want)
	}
	if got, want := header.StartByte(), uint32(227); got != want {
		t.Fatalf("paragraph_header.StartByte = %d, want %d", got, want)
	}
	if got, want := header.EndByte(), uint32(230); got != want {
		t.Fatalf("paragraph_header.EndByte = %d, want %d", got, want)
	}
	if header.HasError() {
		t.Fatalf("paragraph_header.HasError = true, want false")
	}
}

func TestNormalizeCobolRecoveredParagraphHeaderPreservesOtherErrors(t *testing.T) {
	lang := &Language{
		Name:                  "COBOL",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"EOF", ".", "start", "program_definition", "identification_division", "procedure_division", "END_EVALUATE", "period", "paragraph_header"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "END_EVALUATE", Visible: true, Named: true},
			{Name: "period", Visible: true, Named: true},
			{Name: "paragraph_header", Visible: true, Named: true},
		},
	}
	source := []byte("       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       evaluate 1\n" +
		"       when 1\n" +
		"         go to aa\n" +
		"       when 2\n" +
		"         go to aa\n" +
		"       when other\n" +
		"         go to aa\n" +
		"       end-evaluate.\n" +
		"       aa.\n")

	arena := newNodeArena(arenaClassFull)
	id := newLeafNodeInArena(arena, 4, true, 7, 53, advancePointByBytes(Point{}, source[:7]), advancePointByBytes(Point{}, source[:53]))
	retainedErrDot := newLeafNodeInArena(arena, 1, false, 140, 141, advancePointByBytes(Point{}, source[:140]), advancePointByBytes(Point{}, source[:141]))
	retainedErrDot.setHasError(true)
	retainedErr := newParentNodeInArena(arena, errorSymbol, true, []*Node{retainedErrDot}, nil, 0)
	retainedErr.setExtra(true)
	retainedErr.setHasError(true)
	retainedErr.startByte = 138
	retainedErr.startPoint = advancePointByBytes(Point{}, source[:138])
	retainedErr.endByte = 141
	retainedErr.endPoint = advancePointByBytes(Point{}, source[:141])
	endEvaluate := newLeafNodeInArena(arena, 6, true, 206, 218, advancePointByBytes(Point{}, source[:206]), advancePointByBytes(Point{}, source[:218]))
	period := newLeafNodeInArena(arena, 7, true, 218, 219, advancePointByBytes(Point{}, source[:218]), advancePointByBytes(Point{}, source[:219]))
	proc := newParentNodeInArena(arena, 5, true, []*Node{retainedErr, endEvaluate, period}, nil, 0)
	proc.setHasError(true)
	proc.startByte = 61
	proc.startPoint = advancePointByBytes(Point{}, source[:61])
	proc.endByte = 230
	proc.endPoint = advancePointByBytes(Point{}, source[:230])
	def := newParentNodeInArena(arena, 3, true, []*Node{id, proc}, nil, 0)
	def.setHasError(true)
	def.startByte = 7
	def.startPoint = advancePointByBytes(Point{}, source[:7])
	def.endByte = 230
	def.endPoint = advancePointByBytes(Point{}, source[:230])
	errDot := newLeafNodeInArena(arena, 1, false, 229, 230, advancePointByBytes(Point{}, source[:229]), advancePointByBytes(Point{}, source[:230]))
	errDot.setHasError(true)
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{errDot}, nil, 0)
	err.setExtra(true)
	err.setHasError(true)
	err.startByte = 227
	err.startPoint = advancePointByBytes(Point{}, source[:227])
	err.endByte = 230
	err.endPoint = advancePointByBytes(Point{}, source[:230])
	root := newParentNodeInArena(arena, 2, true, []*Node{def, err}, nil, 0)
	root.setHasError(true)
	root.startByte = 7
	root.startPoint = advancePointByBytes(Point{}, source[:7])
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if !root.HasError() {
		t.Fatalf("root.HasError = false, want true for retained non-header error")
	}
	procedure := root.Child(0).Child(1)
	if !procedure.HasError() {
		t.Fatalf("procedure_division.HasError = false, want true for retained non-header error")
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root.ChildCount = %d, want %d", got, want)
	}
	header := procedure.Child(procedure.ChildCount() - 1)
	if got, want := header.Type(lang), "paragraph_header"; got != want {
		t.Fatalf("last procedure child type = %q, want %q", got, want)
	}
	if header.HasError() {
		t.Fatalf("paragraph_header.HasError = true, want false")
	}
}
