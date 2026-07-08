package gotreesitter

import "testing"

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
