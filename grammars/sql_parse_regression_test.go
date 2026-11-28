package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// The certified core serves SQL by default and returns pinned C's tree for this
// input: the select statement, an ERROR node, and the trailing comment as three
// root children. The present engine folds the comment into the statement.
func TestSQLTrailingCommaAtEOFRecoversStatementPrefix(t *testing.T) {
	src := []byte("SELECT a::int,\n-- x\n")
	for _, tc := range []struct {
		name      string
		coreRoute bool
		children  int
	}{
		{name: "default_route_equals_c", coreRoute: true, children: 3},
		{name: "present_engine", coreRoute: false, children: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := ts.NewParser(SqlLanguage())
			if !tc.coreRoute {
				parser.SetAdmissionCandidateRoute(false)
			}
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatal("missing root node")
			}
			if tree.ParseStopReason() != ts.ParseStopAccepted {
				t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
			}
			if !root.HasError() {
				t.Fatalf("expected recovered SQL tree to retain error flag, got %s", root.SExpr(SqlLanguage()))
			}
			if got := root.ChildCount(); got != tc.children {
				t.Fatalf("root child count = %d, want %d; tree=%s", got, tc.children, root.SExpr(SqlLanguage()))
			}
			if first := root.Child(0); first == nil || first.Type(SqlLanguage()) != "select_statement" {
				t.Fatalf("first child = %v, want select_statement; tree=%s", first, root.SExpr(SqlLanguage()))
			} else if !tc.coreRoute && !first.HasError() {
				t.Fatalf("recovered select_statement should retain error flag; tree=%s", root.SExpr(SqlLanguage()))
			}
		})
	}
}

func TestSQLDollarQuotedStringsAllowDollarContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{name: "empty_tag", src: "SELECT $$a$$;\n"},
		{name: "named_tag", src: "SELECT $a$baz$a$;\n"},
		{name: "embedded_empty_tag_text", src: "SELECT $a$$$$a$;\n"},
		{name: "embedded_dollars", src: "SELECT $a$b$$a$;\n"},
		{name: "list", src: "SELECT $$a$$, $a$baz$a$, $a$$$$a$, $a$b$$a$;\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			parser := ts.NewParser(SqlLanguage())
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatal("missing root node")
			}
			if root.HasError() {
				t.Fatalf("unexpected SQL dollar string parse error: %s", root.SExpr(SqlLanguage()))
			}
			if got := root.ChildCount(); got != 2 {
				t.Fatalf("root child count = %d, want 2; tree=%s", got, root.SExpr(SqlLanguage()))
			}
			if first := root.Child(0); first == nil || first.Type(SqlLanguage()) != "select_statement" {
				t.Fatalf("first child = %v, want select_statement; tree=%s", first, root.SExpr(SqlLanguage()))
			}
			if second := root.Child(1); second == nil || second.Type(SqlLanguage()) != ";" {
				t.Fatalf("second child = %v, want semicolon; tree=%s", second, root.SExpr(SqlLanguage()))
			}
		})
	}
}

func TestSQLSelectClauseBodyIntoFieldRequiresIntoKeyword(t *testing.T) {
	lang := SqlLanguage()

	tree, err := ts.NewParser(lang).Parse([]byte("SELECT (SELECT 1), a\nFROM (SELECT a FROM table) AS b;\n"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	body := sqlFirstSelectClauseBody(t, tree, lang)
	if got := body.ChildCount(); got < 3 {
		t.Fatalf("select_clause_body child count = %d, want at least 3; tree=%s", got, tree.RootNode().SExpr(lang))
	}
	if child := body.Child(2); child == nil || child.Type(lang) != "identifier" {
		t.Fatalf("body child 2 = %v, want identifier; tree=%s", child, tree.RootNode().SExpr(lang))
	}
	if got := body.FieldNameForChild(2, lang); got != "" {
		t.Fatalf("body child 2 field = %q, want empty; tree=%s", got, tree.RootNode().SExpr(lang))
	}

	tree, err = ts.NewParser(lang).Parse([]byte("SELECT a INTO b;\n"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	body = sqlFirstSelectClauseBody(t, tree, lang)
	foundInto := false
	for i := 0; i < body.ChildCount(); i++ {
		child := body.Child(i)
		if child != nil && child.Type(lang) == "identifier" && body.FieldNameForChild(i, lang) == "into" {
			foundInto = true
			break
		}
	}
	if !foundInto {
		t.Fatalf("explicit SELECT INTO target did not keep into field; tree=%s", tree.RootNode().SExpr(lang))
	}
}

func TestSQLNoTreeBenchmarkSkipsFullParseRetry(t *testing.T) {
	src := []byte(`SELECT a, b::INT;
-- <- keyword
--         ^ operator
--            ^ type.builtin

SELECT a, b  ::  INT;
--           ^ operator
--               ^ type.builtin
--        ^ variable

SELECT foo(a)
-- <- keyword
--      ^ function
FROM table1
-- <- keyword
LEFT JOIN table2 ON table1.a = table2.a
-- <- keyword
--    ^ keyword
--               ^ keyword
WHERE a = b
-- <- keyword
--      ^ operator
GROUP BY a, b
-- <- keyword
--    ^ keyword
ORDER BY lower(a), b
-- <- keyword
--    ^ keyword
--        ^ function
select a, b::int;
-- <- keyword
--            ^ type.builtin
from table1
-- <- keyword
where a = b
-- <- keyword
group by a, b
-- <- keyword
--    ^ keyword
order by lower(a), b;
-- <- keyword
--    ^ keyword

SELECT (SELECT 1), a
-- <- keyword
--         ^ keyword
--             ^ number
FROM (SELECT a FROM table) AS b;
-- <- keyword
--     ^ keyword
--             ^ keyword
--                         ^ keyword

SELECT a, b
FROM a
ORDER    by a, b
-- <- keyword
--       ^ keyword
GrOUP
-- <- keyword
By a, b
-- <- keyword

SELECT $$a$$, $a$baz$a$, $a$$$$a$, $a$b$$a$;
-- <- keyword
--       ^ string
--              ^ string
--                          ^ string
--                                    ^ string
`)

	parser := ts.NewParser(SqlLanguage())
	ts.ResetPerfCounters()
	defer ts.ResetPerfCounters()
	tree, err := parser.ParseNoTreeBenchmarkOnly(src)
	if err != nil {
		t.Fatalf("no-tree parse failed: %v", err)
	}
	defer tree.Release()

	rt := tree.ParseRuntime()
	perf := ts.PerfCountersSnapshot()
	if perf.LexBytes == 0 && perf.LexTokens == 0 {
		t.Skip("perf counters are disabled")
	}
	if perf.LexBytes > uint64(len(src))*2 {
		t.Fatalf("no-tree parse lexed %d bytes for %d-byte source, expected no hidden full-parse retry; runtime=%s", perf.LexBytes, len(src), rt.Summary())
	}
	if perf.LexTokens > rt.TokensConsumed*2 {
		t.Fatalf("no-tree parse lexed %d tokens after consuming %d, expected no hidden full-parse retry; runtime=%s", perf.LexTokens, rt.TokensConsumed, rt.Summary())
	}
}

func sqlFirstSelectClauseBody(t *testing.T, tree *ts.Tree, lang *ts.Language) *ts.Node {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.ChildCount() == 0 {
		t.Fatalf("missing SQL root child; tree=%v", root)
	}
	stmt := root.Child(0)
	if stmt == nil || stmt.Type(lang) != "select_statement" || stmt.ChildCount() == 0 {
		t.Fatalf("root child 0 = %v, want select_statement; tree=%s", stmt, root.SExpr(lang))
	}
	clause := stmt.Child(0)
	if clause == nil || clause.Type(lang) != "select_clause" || clause.ChildCount() < 2 {
		t.Fatalf("statement child 0 = %v, want select_clause with body; tree=%s", clause, root.SExpr(lang))
	}
	body := clause.Child(1)
	if body == nil || body.Type(lang) != "select_clause_body" {
		t.Fatalf("select clause child 1 = %v, want select_clause_body; tree=%s", body, root.SExpr(lang))
	}
	return body
}
