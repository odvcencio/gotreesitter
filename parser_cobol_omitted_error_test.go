package gotreesitter

import "testing"

func makeOmittedCobolFixture() (*Tree, *Parser, []byte) {
	lang := &Language{
		Name:                  "cobol",
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

	tree := newTreeWithArenas(root, source, lang, arena, nil)
	tree.resultCompatibilityApplied = true
	return tree, &Parser{language: lang}, source
}

func TestCobolSecondNormalizationPreservesOmittedError(t *testing.T) {
	for _, omitted := range []bool{false, true} {
		tree, p, source := makeOmittedCobolFixture()
		tree.omittedRootError = omitted
		tree.resultErrorSummary = resultErrorSummaryPresent
		p.normalizeReturnedTreeForParse(tree, source)
		if tree.root.HasError() != omitted {
			t.Errorf("omitted=%v: root.HasError=%v", omitted, tree.root.HasError())
		}
		if omitted && tree.resultErrorSummary != resultErrorSummaryPresent {
			t.Error("omitted error summary lost")
		}
		tree.Release()
	}
}
