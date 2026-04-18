package gotreesitter

import "testing"

func TestNormalizeBashProgramVariableAssignmentsSplitsTopLevelWrapper(t *testing.T) {
	lang := &Language{
		Name:        "bash",
		SymbolNames: []string{"EOF", "program", "comment", "variable_assignments", "variable_assignment", "if_statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "variable_assignments", Visible: true, Named: true},
			{Name: "variable_assignment", Visible: true, Named: true},
			{Name: "if_statement", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 0, 2, Point{}, Point{Column: 2})
	assign1 := newLeafNodeInArena(arena, 4, true, 3, 6, Point{Column: 3}, Point{Column: 6})
	assign2 := newLeafNodeInArena(arena, 4, true, 7, 10, Point{Column: 7}, Point{Column: 10})
	assigns := newParentNodeInArena(arena, 3, true, []*Node{assign1, assign2}, nil, 0)
	ifStmt := newLeafNodeInArena(arena, 5, true, 11, 15, Point{Column: 11}, Point{Column: 15})
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, assigns, ifStmt}, nil, 0)

	normalizeBashProgramVariableAssignments(root, lang)

	if got, want := len(root.children), 4; got != want {
		t.Fatalf("len(root.children) = %d, want %d", got, want)
	}
	if got, want := root.children[1].Type(lang), "variable_assignment"; got != want {
		t.Fatalf("root.children[1].Type = %q, want %q", got, want)
	}
	if got, want := root.children[2].Type(lang), "variable_assignment"; got != want {
		t.Fatalf("root.children[2].Type = %q, want %q", got, want)
	}
	if got, want := root.children[3].Type(lang), "if_statement"; got != want {
		t.Fatalf("root.children[3].Type = %q, want %q", got, want)
	}
}

func TestNormalizeBashProgramVariableAssignmentsSplitsNestedIfWrapper(t *testing.T) {
	lang := &Language{
		Name:        "bash",
		SymbolNames: []string{"EOF", "program", "variable_assignments", "variable_assignment", "if_statement", "fi"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "variable_assignments", Visible: true, Named: true},
			{Name: "variable_assignment", Visible: true, Named: true},
			{Name: "if_statement", Visible: true, Named: true},
			{Name: "fi", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	assign1 := newLeafNodeInArena(arena, 3, true, 0, 3, Point{}, Point{Column: 3})
	assign2 := newLeafNodeInArena(arena, 3, true, 4, 7, Point{Column: 4}, Point{Column: 7})
	assigns := newParentNodeInArena(arena, 2, true, []*Node{assign1, assign2}, nil, 0)
	fi := newLeafNodeInArena(arena, 5, false, 8, 10, Point{Column: 8}, Point{Column: 10})
	ifStmt := newParentNodeInArena(arena, 4, true, []*Node{assigns, fi}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{ifStmt}, nil, 0)

	normalizeBashProgramVariableAssignments(root, lang)

	if got, want := len(ifStmt.children), 3; got != want {
		t.Fatalf("len(ifStmt.children) = %d, want %d", got, want)
	}
	if got, want := ifStmt.children[0].Type(lang), "variable_assignment"; got != want {
		t.Fatalf("ifStmt.children[0].Type = %q, want %q", got, want)
	}
	if got, want := ifStmt.children[1].Type(lang), "variable_assignment"; got != want {
		t.Fatalf("ifStmt.children[1].Type = %q, want %q", got, want)
	}
}

func TestNormalizeBashProgramVariableAssignmentsAssignsIfConditionField(t *testing.T) {
	lang := &Language{
		Name:        "bash",
		SymbolNames: []string{"EOF", "program", "if_statement", "if", "test_command", "fi"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "if_statement", Visible: true, Named: true},
			{Name: "if", Visible: true, Named: false},
			{Name: "test_command", Visible: true, Named: true},
			{Name: "fi", Visible: true, Named: false},
		},
		FieldNames: []string{"", "condition"},
	}

	arena := newNodeArena(arenaClassFull)
	ifTok := newLeafNodeInArena(arena, 3, false, 0, 2, Point{}, Point{Column: 2})
	testCmd := newLeafNodeInArena(arena, 4, true, 3, 8, Point{Column: 3}, Point{Column: 8})
	fi := newLeafNodeInArena(arena, 5, false, 9, 11, Point{Column: 9}, Point{Column: 11})
	ifStmt := newParentNodeInArena(arena, 2, true, []*Node{ifTok, testCmd, fi}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{ifStmt}, nil, 0)

	normalizeBashProgramVariableAssignments(root, lang)

	if got, want := ifStmt.fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("ifStmt.fieldIDs[1] = %d, want %d", got, want)
	}
	if got, want := ifStmt.fieldSources[1], fieldSourceDirect; got != want {
		t.Fatalf("ifStmt.fieldSources[1] = %v, want %v", got, want)
	}
}

func TestNormalizeBashProgramVariableAssignmentsExtendsIfConditionFieldToThenBoundary(t *testing.T) {
	lang := &Language{
		Name:        "bash",
		SymbolNames: []string{"EOF", "program", "if_statement", "if", "test_command", ";", "then", "fi"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "if_statement", Visible: true, Named: true},
			{Name: "if", Visible: true, Named: false},
			{Name: "test_command", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "then", Visible: true, Named: false},
			{Name: "fi", Visible: true, Named: false},
		},
		FieldNames: []string{"", "condition"},
	}

	arena := newNodeArena(arenaClassFull)
	ifTok := newLeafNodeInArena(arena, 3, false, 0, 2, Point{}, Point{Column: 2})
	testCmd := newLeafNodeInArena(arena, 4, true, 3, 8, Point{Column: 3}, Point{Column: 8})
	semi := newLeafNodeInArena(arena, 5, false, 8, 9, Point{Column: 8}, Point{Column: 9})
	thenTok := newLeafNodeInArena(arena, 6, false, 10, 14, Point{Column: 10}, Point{Column: 14})
	fi := newLeafNodeInArena(arena, 7, false, 15, 17, Point{Column: 15}, Point{Column: 17})
	ifStmt := newParentNodeInArena(arena, 2, true, []*Node{ifTok, testCmd, semi, thenTok, fi}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{ifStmt}, nil, 0)

	normalizeBashProgramVariableAssignments(root, lang)

	if got, want := ifStmt.fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("ifStmt.fieldIDs[1] = %d, want %d", got, want)
	}
	if got, want := ifStmt.fieldIDs[2], FieldID(1); got != want {
		t.Fatalf("ifStmt.fieldIDs[2] = %d, want %d", got, want)
	}
}

func TestNormalizeBashProgramVariableAssignmentsSplitsSubshellWrapper(t *testing.T) {
	lang := &Language{
		Name:        "bash",
		SymbolNames: []string{"EOF", "program", "subshell", "variable_assignments", "variable_assignment", "(", ")"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "subshell", Visible: true, Named: true},
			{Name: "variable_assignments", Visible: true, Named: true},
			{Name: "variable_assignment", Visible: true, Named: true},
			{Name: "(", Visible: true, Named: false},
			{Name: ")", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	open := newLeafNodeInArena(arena, 5, false, 0, 1, Point{}, Point{Column: 1})
	assign1 := newLeafNodeInArena(arena, 4, true, 1, 4, Point{Column: 1}, Point{Column: 4})
	assign2 := newLeafNodeInArena(arena, 4, true, 5, 8, Point{Column: 5}, Point{Column: 8})
	assigns := newParentNodeInArena(arena, 3, true, []*Node{assign1, assign2}, nil, 0)
	close := newLeafNodeInArena(arena, 6, false, 9, 10, Point{Column: 9}, Point{Column: 10})
	subshell := newParentNodeInArena(arena, 2, true, []*Node{open, assigns, close}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{subshell}, nil, 0)

	normalizeBashProgramVariableAssignments(root, lang)

	if got, want := len(subshell.children), 4; got != want {
		t.Fatalf("len(subshell.children) = %d, want %d", got, want)
	}
	if got, want := subshell.children[1].Type(lang), "variable_assignment"; got != want {
		t.Fatalf("subshell.children[1].Type = %q, want %q", got, want)
	}
	if got, want := subshell.children[2].Type(lang), "variable_assignment"; got != want {
		t.Fatalf("subshell.children[2].Type = %q, want %q", got, want)
	}
}

func TestNormalizePerlJoinAssignmentListsRewritesBareListOperatorShape(t *testing.T) {
	lang := &Language{
		Name:        "perl",
		SymbolNames: []string{"EOF", "source_file", "expression_statement", "assignment_expression", "variable_declaration", "=", "ambiguous_function_call_expression", "function", "list_expression", ",", "string_literal"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "assignment_expression", Visible: true, Named: true},
			{Name: "variable_declaration", Visible: true, Named: true},
			{Name: "=", Visible: true, Named: false},
			{Name: "ambiguous_function_call_expression", Visible: true, Named: true},
			{Name: "function", Visible: true, Named: true},
			{Name: "list_expression", Visible: true, Named: true},
			{Name: ",", Visible: true, Named: false},
			{Name: "string_literal", Visible: true, Named: true},
		},
	}

	source := []byte("my $x = join \"\\n\", \"a\", \"b\"")
	arena := newNodeArena(arenaClassFull)

	lhs := newLeafNodeInArena(arena, 4, true, 0, 5, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 5})
	eq := newLeafNodeInArena(arena, 5, false, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})
	fn := newLeafNodeInArena(arena, 7, true, 8, 12, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 12})
	arg0 := newLeafNodeInArena(arena, 10, true, 13, 17, Point{Row: 0, Column: 13}, Point{Row: 0, Column: 17})
	comma0 := newLeafNodeInArena(arena, 9, false, 17, 18, Point{Row: 0, Column: 17}, Point{Row: 0, Column: 18})
	arg1 := newLeafNodeInArena(arena, 10, true, 19, 22, Point{Row: 0, Column: 19}, Point{Row: 0, Column: 22})
	comma1 := newLeafNodeInArena(arena, 9, false, 22, 23, Point{Row: 0, Column: 22}, Point{Row: 0, Column: 23})
	arg2 := newLeafNodeInArena(arena, 10, true, 24, 27, Point{Row: 0, Column: 24}, Point{Row: 0, Column: 27})

	args := newParentNodeInArena(arena, 8, true, []*Node{arg0, comma0, arg1, comma1, arg2}, nil, 0)
	call := newParentNodeInArena(arena, 6, true, []*Node{fn, args}, nil, 0)
	assign := newParentNodeInArena(arena, 3, true, []*Node{lhs, eq, call}, nil, 0)
	stmt := newParentNodeInArena(arena, 2, true, []*Node{assign}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizePerlJoinAssignmentLists(root, source, lang)

	rewritten := stmt.Child(0)
	if rewritten == nil {
		t.Fatal("expression_statement lost child after normalization")
	}
	if got := rewritten.Type(lang); got != "list_expression" {
		t.Fatalf("rewritten child type = %q, want list_expression", got)
	}
	if got, want := rewritten.ChildCount(), 5; got != want {
		t.Fatalf("rewritten child count = %d, want %d", got, want)
	}
	assign = rewritten.Child(0)
	if assign == nil || assign.Type(lang) != "assignment_expression" {
		t.Fatalf("rewritten first child = %v, want assignment_expression", assign)
	}
	call = assign.Child(2)
	if call == nil || call.Type(lang) != "ambiguous_function_call_expression" {
		t.Fatalf("rewritten assignment rhs = %v, want ambiguous_function_call_expression", call)
	}
	if got, want := call.ChildCount(), 2; got != want {
		t.Fatalf("rewritten call child count = %d, want %d", got, want)
	}
	if got := call.Child(1).Type(lang); got != "string_literal" {
		t.Fatalf("rewritten first argument type = %q, want string_literal", got)
	}
	if got, want := call.EndByte(), uint32(17); got != want {
		t.Fatalf("rewritten call end byte = %d, want %d", got, want)
	}
	if got := rewritten.Child(2).Type(lang); got != "string_literal" {
		t.Fatalf("rewritten third child type = %q, want string_literal", got)
	}
	if got := rewritten.Child(4).Type(lang); got != "string_literal" {
		t.Fatalf("rewritten fifth child type = %q, want string_literal", got)
	}
}

func TestNormalizePerlPushExpressionListsRewritesRootListShape(t *testing.T) {
	lang := &Language{
		Name:        "perl",
		SymbolNames: []string{"EOF", "source_file", "expression_statement", "ambiguous_function_call_expression", "function", "list_expression", ",", "array", "scalar"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "ambiguous_function_call_expression", Visible: true, Named: true},
			{Name: "function", Visible: true, Named: true},
			{Name: "list_expression", Visible: true, Named: true},
			{Name: ",", Visible: true, Named: false},
			{Name: "array", Visible: true, Named: true},
			{Name: "scalar", Visible: true, Named: true},
		},
	}

	source := []byte("push @found, $_")
	arena := newNodeArena(arenaClassFull)

	fn := newLeafNodeInArena(arena, 4, true, 0, 4, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 4})
	arg0 := newLeafNodeInArena(arena, 7, true, 5, 11, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 11})
	comma := newLeafNodeInArena(arena, 6, false, 11, 12, Point{Row: 0, Column: 11}, Point{Row: 0, Column: 12})
	arg1 := newLeafNodeInArena(arena, 8, true, 13, 15, Point{Row: 0, Column: 13}, Point{Row: 0, Column: 15})

	call := newParentNodeInArena(arena, 3, true, []*Node{fn, arg0}, nil, 0)
	list := newParentNodeInArena(arena, 5, true, []*Node{call, comma, arg1}, nil, 0)
	stmt := newParentNodeInArena(arena, 2, true, []*Node{list}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizePerlPushExpressionLists(root, source, lang)

	rewritten := stmt.Child(0)
	if rewritten == nil {
		t.Fatal("expression_statement lost child after normalization")
	}
	if got := rewritten.Type(lang); got != "ambiguous_function_call_expression" {
		t.Fatalf("rewritten child type = %q, want ambiguous_function_call_expression", got)
	}
	if got, want := rewritten.ChildCount(), 2; got != want {
		t.Fatalf("rewritten child count = %d, want %d", got, want)
	}
	args := rewritten.Child(1)
	if args == nil || args.Type(lang) != "list_expression" {
		t.Fatalf("rewritten arguments = %v, want list_expression", args)
	}
	if got, want := args.ChildCount(), 3; got != want {
		t.Fatalf("rewritten args child count = %d, want %d", got, want)
	}
	if got := args.Child(0).Type(lang); got != "array" {
		t.Fatalf("rewritten first arg type = %q, want array", got)
	}
	if got := args.Child(2).Type(lang); got != "scalar" {
		t.Fatalf("rewritten third arg type = %q, want scalar", got)
	}
}

func TestNormalizePerlReturnExpressionListsPromotesCommaTail(t *testing.T) {
	lang := &Language{
		Name:        "perl",
		SymbolNames: []string{"EOF", "source_file", "expression_statement", "return_expression", "return", "list_expression", ",", "ambiguous_function_call_expression", "function", "string_literal", "array_element_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "return_expression", Visible: true, Named: true},
			{Name: "return", Visible: true, Named: false},
			{Name: "list_expression", Visible: true, Named: true},
			{Name: ",", Visible: true, Named: false},
			{Name: "ambiguous_function_call_expression", Visible: true, Named: true},
			{Name: "function", Visible: true, Named: true},
			{Name: "string_literal", Visible: true, Named: true},
			{Name: "array_element_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)

	retTok := newLeafNodeInArena(arena, 4, false, 0, 6, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 6})
	fn := newLeafNodeInArena(arena, 8, true, 7, 14, Point{Row: 0, Column: 7}, Point{Row: 0, Column: 14})
	arg0 := newLeafNodeInArena(arena, 9, true, 15, 18, Point{Row: 0, Column: 15}, Point{Row: 0, Column: 18})
	comma := newLeafNodeInArena(arena, 6, false, 18, 19, Point{Row: 0, Column: 18}, Point{Row: 0, Column: 19})
	arg1 := newLeafNodeInArena(arena, 10, true, 20, 31, Point{Row: 0, Column: 20}, Point{Row: 0, Column: 31})

	call := newParentNodeInArena(arena, 7, true, []*Node{fn, arg0}, nil, 0)
	list := newParentNodeInArena(arena, 5, true, []*Node{call, comma, arg1}, nil, 0)
	ret := newParentNodeInArena(arena, 3, true, []*Node{retTok, list}, nil, 0)
	stmt := newParentNodeInArena(arena, 2, true, []*Node{ret}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizePerlReturnExpressionLists(root, lang)

	rewritten := stmt.Child(0)
	if rewritten == nil {
		t.Fatal("expression_statement lost child after normalization")
	}
	if got := rewritten.Type(lang); got != "list_expression" {
		t.Fatalf("rewritten child type = %q, want list_expression", got)
	}
	if got, want := rewritten.ChildCount(), 3; got != want {
		t.Fatalf("rewritten child count = %d, want %d", got, want)
	}
	ret = rewritten.Child(0)
	if ret == nil || ret.Type(lang) != "return_expression" {
		t.Fatalf("rewritten first child = %v, want return_expression", ret)
	}
	if got, want := ret.ChildCount(), 2; got != want {
		t.Fatalf("rewritten return child count = %d, want %d", got, want)
	}
	if got := ret.Child(1).Type(lang); got != "ambiguous_function_call_expression" {
		t.Fatalf("rewritten return payload type = %q, want ambiguous_function_call_expression", got)
	}
	if got := rewritten.Child(2).Type(lang); got != "array_element_expression" {
		t.Fatalf("rewritten third child type = %q, want array_element_expression", got)
	}
}
