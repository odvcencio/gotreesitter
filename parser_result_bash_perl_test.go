package gotreesitter

import (
	"strings"
	"testing"
)

func TestNormalizeBashGeneratedCommandAssignmentsRewritesAssignmentShapedCommand(t *testing.T) {
	lang := &Language{
		Name:                  "bash",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"program", "command", "command_name", "concatenation", "variable_assignment", "variable_name", "word", "command_substitution"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "program", Visible: true, Named: true},
			{Name: "command", Visible: true, Named: true},
			{Name: "command_name", Visible: true, Named: true},
			{Name: "concatenation", Visible: true, Named: true},
			{Name: "variable_assignment", Visible: true, Named: true},
			{Name: "variable_name", Visible: true, Named: true},
			{Name: "word", Visible: true, Named: true},
			{Name: "command_substitution", Visible: true, Named: true},
		},
		FieldNames: []string{"", "name", "value"},
	}
	source := []byte("zipname=npm-$(node ../cli.js -v).zip")
	subStart := strings.Index(string(source), "$(")
	subEnd := strings.Index(string(source), ").zip") + 1
	if subStart < 0 || subEnd <= subStart {
		t.Fatal("bad test source")
	}

	arena := newNodeArena(arenaClassFull)
	word0 := newLeafNodeInArena(arena, 6, true, 0, uint32(subStart), Point{}, Point{Column: uint32(subStart)})
	sub := newLeafNodeInArena(arena, 7, true, uint32(subStart), uint32(subEnd), Point{Column: uint32(subStart)}, Point{Column: uint32(subEnd)})
	word1 := newLeafNodeInArena(arena, 6, true, uint32(subEnd), uint32(len(source)), Point{Column: uint32(subEnd)}, Point{Column: uint32(len(source))})
	concat := newParentNodeInArena(arena, 3, true, []*Node{word0, sub, word1}, nil, 0)
	commandName := newParentNodeInArena(arena, 2, true, []*Node{concat}, nil, 0)
	command := newParentNodeInArena(arena, 1, true, []*Node{commandName}, nil, 0)

	normalizeBashGeneratedCommandAssignments(command, source, lang)

	if got, want := command.Type(lang), "variable_assignment"; got != want {
		t.Fatalf("command.Type = %q, want %q", got, want)
	}
	if got, want := len(command.children), 2; got != want {
		t.Fatalf("len(command.children) = %d, want %d", got, want)
	}
	if got, want := command.children[0].Type(lang), "variable_name"; got != want {
		t.Fatalf("name type = %q, want %q", got, want)
	}
	if got, want := command.children[0].Text(source), "zipname"; got != want {
		t.Fatalf("name text = %q, want %q", got, want)
	}
	value := command.children[1]
	if got, want := value.Type(lang), "concatenation"; got != want {
		t.Fatalf("value type = %q, want %q", got, want)
	}
	if got, want := value.children[0].Text(source), "npm-"; got != want {
		t.Fatalf("value prefix = %q, want %q", got, want)
	}
	if got, want := value.children[2].Text(source), ".zip"; got != want {
		t.Fatalf("value suffix = %q, want %q", got, want)
	}
	if got, want := command.fieldIDs()[0], FieldID(1); got != want {
		t.Fatalf("name field = %d, want %d", got, want)
	}
	if got, want := command.fieldIDs()[1], FieldID(2); got != want {
		t.Fatalf("value field = %d, want %d", got, want)
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
