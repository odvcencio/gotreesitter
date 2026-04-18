package parserresult_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestGoLargeProcRecoversTopLevelDeclarations(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/large__proc.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q: %s", got, want, root.SExpr(lang))
	}
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	casgstatus := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "function_declaration" &&
			strings.Contains(n.Text(src), "func casgstatus(")
	})
	if casgstatus == nil {
		t.Fatalf("missing casgstatus function declaration after recovery: %s", root.SExpr(lang))
	}

	worldsema := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "var_declaration" &&
			strings.Contains(n.Text(src), "var worldsema")
	})
	if worldsema == nil {
		t.Fatalf("missing worldsema var declaration after recovery: %s", root.SExpr(lang))
	}

	forEachNode(casgstatus, func(n *gotreesitter.Node) {
		if n.Type(lang) != "statement_list" && n.Type(lang) != "statement_list_repeat1" {
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			if child != nil && child.Type(lang) == ";" {
				t.Fatalf("recovered go statement list still contains semicolon child: %s", n.SExpr(lang))
			}
		}
	})

	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "function_declaration", "method_declaration":
			text := string(src[child.StartByte():child.EndByte()])
			if !strings.HasPrefix(text, "func ") {
				t.Fatalf("top-level %s does not start at func keyword: start=%d text=%q", child.Type(lang), child.StartByte(), text[:min(len(text), 32)])
			}
		}
	}
}

func TestGoLargeProcMainStatementListCarriesTrailingNewlineBeforeBrace(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/large__proc.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	mainFn := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "function_declaration" &&
			strings.HasPrefix(n.Text(src), "func main()")
	})
	if mainFn == nil {
		t.Fatalf("missing main function declaration: %s", root.SExpr(lang))
	}

	var body *gotreesitter.Node
	for i := 0; i < mainFn.ChildCount(); i++ {
		child := mainFn.Child(i)
		if child != nil && child.Type(lang) == "block" {
			body = child
			break
		}
	}
	if body == nil || body.ChildCount() < 3 {
		t.Fatalf("missing main block: %s", mainFn.SExpr(lang))
	}
	var stmtList, closeBrace *gotreesitter.Node
	for i := 0; i < body.ChildCount(); i++ {
		child := body.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "statement_list":
			stmtList = child
		case "}":
			closeBrace = child
		}
	}
	if stmtList == nil || closeBrace == nil {
		t.Fatalf("missing main statement_list/close brace: %s", body.SExpr(lang))
	}
	if got, want := stmtList.EndByte(), closeBrace.StartByte(); got != want {
		t.Fatalf("main statement_list endByte = %d, want %d before close brace", got, want)
	}
}

func TestGoLargeProcGroupedDeclarationsDropSemicolons(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/large__proc.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	groupedConst := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "const_declaration" &&
			strings.HasPrefix(n.Text(src), "const (\n")
	})
	if groupedConst == nil {
		t.Fatalf("missing grouped const declaration: %s", root.SExpr(lang))
	}
	for i := 0; i < groupedConst.ChildCount(); i++ {
		child := groupedConst.Child(i)
		if child != nil && child.Type(lang) == ";" {
			t.Fatalf("grouped const declaration still contains semicolon child: %s", groupedConst.SExpr(lang))
		}
	}

	cgothreadstart := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_declaration" &&
			strings.Contains(n.Text(src), "type cgothreadstart struct")
	})
	if cgothreadstart == nil {
		t.Fatalf("missing cgothreadstart type declaration: %s", root.SExpr(lang))
	}
	structFields := firstNode(cgothreadstart, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "field_declaration_list"
	})
	if structFields == nil {
		t.Fatalf("missing cgothreadstart field_declaration_list: %s", cgothreadstart.SExpr(lang))
	}
	for i := 0; i < structFields.ChildCount(); i++ {
		child := structFields.Child(i)
		if child != nil && child.Type(lang) == ";" {
			t.Fatalf("field_declaration_list still contains semicolon child: %s", structFields.SExpr(lang))
		}
	}
}

func TestGoMediumLetterDropsTopLevelSemicolons(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/medium__letter_test.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q: %s", got, want, root.SExpr(lang))
	}
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child != nil && child.Type(lang) == ";" {
			t.Fatalf("source_file still contains semicolon child: %s", root.SExpr(lang))
		}
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("source_file endByte = %d, want %d", got, want)
	}
}

func TestGoMediumLetterCaseStatementListCarriesTrailingNewlineBeforeNextCase(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/medium__letter_test.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	caseNode := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "expression_case" &&
			strings.Contains(n.Text(src), "case UpperCase:")
	})
	if caseNode == nil {
		t.Fatalf("missing UpperCase expression_case: %s", root.SExpr(lang))
	}
	next := caseNode.NextSibling()
	if next == nil {
		t.Fatalf("expression_case missing next sibling: %s", caseNode.SExpr(lang))
	}
	stmtList := caseNode.Child(caseNode.ChildCount() - 1)
	if stmtList == nil || stmtList.Type(lang) != "statement_list" {
		t.Fatalf("expression_case missing trailing statement_list: %s", caseNode.SExpr(lang))
	}
	want := trailingNewlineBoundary(src, stmtList.EndByte(), next.StartByte())
	if got := caseNode.EndByte(); got != want {
		t.Fatalf("expression_case endByte = %d, want %d before next case", got, want)
	}
	if got := stmtList.EndByte(); got != want {
		t.Fatalf("expression_case statement_list endByte = %d, want %d before next case", got, want)
	}
}

func TestGoLargeProcDefaultCaseCarriesTrailingNewlineBeforeNextCase(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/large__proc.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	defaultCase := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "default_case" &&
			strings.Contains(n.Text(src), "casfrom_Gscanstatus bad oldval")
	})
	if defaultCase == nil {
		t.Fatalf("missing casfrom_Gscanstatus default_case: %s", root.SExpr(lang))
	}
	next := defaultCase.NextSibling()
	if next == nil {
		t.Fatalf("default_case missing next sibling: %s", defaultCase.SExpr(lang))
	}
	stmtList := defaultCase.Child(defaultCase.ChildCount() - 1)
	if stmtList == nil || stmtList.Type(lang) != "statement_list" {
		t.Fatalf("default_case missing trailing statement_list: %s", defaultCase.SExpr(lang))
	}
	want := trailingNewlineBoundary(src, stmtList.EndByte(), next.StartByte())
	if got := defaultCase.EndByte(); got != want {
		t.Fatalf("default_case endByte = %d, want %d before next case", got, want)
	}
	if got := stmtList.EndByte(); got != want {
		t.Fatalf("default_case statement_list endByte = %d, want %d before next case", got, want)
	}
}

func TestGoLargeProcStatementListCarriesTrailingNewlineBeforeTrailingComments(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/large__proc.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	loop := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "for_statement" &&
			strings.Contains(n.Text(src), "atomic.Cas(&gp.atomicstatus")
	})
	if loop == nil {
		t.Fatalf("missing casfrom_Gscanstatus loop: %s", root.SExpr(lang))
	}
	var body *gotreesitter.Node
	for i := 0; i < loop.ChildCount(); i++ {
		child := loop.Child(i)
		if child != nil && child.Type(lang) == "block" {
			body = child
			break
		}
	}
	if body == nil || body.ChildCount() < 4 {
		t.Fatalf("missing loop body: %s", loop.SExpr(lang))
	}
	stmtList := body.Child(1)
	next := body.Child(2)
	if stmtList == nil || next == nil || stmtList.Type(lang) != "statement_list" || next.Type(lang) != "comment" {
		t.Fatalf("unexpected loop body shape: %s", body.SExpr(lang))
	}
	want := trailingNewlineBoundary(src, stmtList.EndByte(), next.StartByte())
	if got := stmtList.EndByte(); got != want {
		t.Fatalf("loop statement_list endByte = %d, want %d before trailing comments", got, want)
	}
}

func TestGoLargeProcExpressionCaseCarriesTrailingBlankLineBeforeComments(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/large__proc.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	caseNode := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "expression_case" &&
			strings.Contains(n.Text(src), "casfrom_Gscanstatus(gp, s, s&^_Gscan)")
	})
	if caseNode == nil {
		t.Fatalf("missing expression_case before trailing comments: %s", root.SExpr(lang))
	}
	next := caseNode.NextSibling()
	if next == nil || next.Type(lang) != "comment" {
		t.Fatalf("expression_case missing trailing comment sibling: %s", caseNode.SExpr(lang))
	}
	stmtList := caseNode.Child(caseNode.ChildCount() - 1)
	if stmtList == nil || stmtList.Type(lang) != "statement_list" {
		t.Fatalf("expression_case missing trailing statement_list: %s", caseNode.SExpr(lang))
	}
	want := trailingNewlineBoundary(src, stmtList.EndByte(), next.StartByte())
	if got := caseNode.EndByte(); got != want {
		t.Fatalf("expression_case endByte = %d, want %d before trailing comments", got, want)
	}
	if got := stmtList.EndByte(); got != want {
		t.Fatalf("expression_case statement_list endByte = %d, want %d before trailing comments", got, want)
	}
}

func TestGoLargeProcHeaderOnlyCaseStopsAtColon(t *testing.T) {
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/go/large__proc.go")

	tree, lang := parseByLanguageName(t, "go", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected go parse error: %s", root.SExpr(lang))
	}

	caseNode := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "expression_case" &&
			strings.TrimSpace(n.Text(src)) == "case _Gcopystack:"
	})
	if caseNode == nil {
		t.Fatalf("missing header-only expression_case: %s", root.SExpr(lang))
	}
	if got, want := caseNode.EndByte(), caseNode.StartByte()+uint32(len("case _Gcopystack:")); got != want {
		t.Fatalf("header-only expression_case endByte = %d, want %d at colon", got, want)
	}
}
