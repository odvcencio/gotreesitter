package grammars

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestHeaderRange(t *testing.T) {
	tests := []struct{ file, source, nodeType, header string }{
		{"a.py", "@dec\r\nasync def café(\r\n    x: int = 1\r\n) -> int:\r\n    return x\r\n", "function_definition", "@dec\r\nasync def café(\r\n    x: int = 1\r\n) -> int:"},
		{"a.py", "limit: int = build()\n", "assignment", "limit: int"},
		{"a.go", "package a\ntype T struct { x int }\n", "type_spec", "type T struct"},
		{"a.go", "package a\ntype (\nT interface { F() }\n)\n", "type_spec", "T interface"},
		{"a.go", "package a\nvar size int = f()\n", "var_spec", "var size int"},
		{"a.js", "export default class Client { m() {} }", "class_declaration", "export default class Client"},
		{"a.ts", "export const client: Client = /* initializer */ build();", "variable_declarator", "export const client: Client"},
		{"a.ts", "export declare function lookup(x: string): number;", "function_signature", "export declare function lookup(x: string): number;"},
		{"a.tsx", "export function App() { return <div/>; }", "function_declaration", "export function App()"},
		{"a.go", "package a\ntype Service interface { Read() error }\n", "method_elem", "Read() error"},
		{"a.ts", "type Token = { id: number };", "type_alias_declaration", "type Token = { id: number };"},
		{"a.rs", "fn run(x: i32) -> i32 { x }", "function_item", "fn run(x: i32) -> i32"},
		{"a.js", "const a = f(), b = g();", "call_expression", ""},
	}
	for _, tc := range tests {
		t.Run(tc.file+"/"+tc.nodeType, func(t *testing.T) {
			bt, err := ParseFile(tc.file, []byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer bt.Release()
			lang := bt.Language()
			n := pinFind(bt.RootNode(), lang, tc.nodeType)
			if n == nil {
				t.Fatalf("missing %s: %s", tc.nodeType, bt.RootNode().SExpr(lang))
			}
			got, ok := HeaderRange(n, lang)
			if tc.header == "" {
				if ok {
					t.Fatalf("unexpected header: %+v", got)
				}
				return
			}
			s := strings.Index(tc.source, tc.header)
			e := s + len(tc.header)
			point := func(offset int) gts.Point {
				prefix := tc.source[:offset]
				return gts.Point{Row: uint32(strings.Count(prefix, "\n")), Column: uint32(offset - strings.LastIndex(prefix, "\n") - 1)}
			}
			want := gts.Range{StartByte: uint32(s), EndByte: uint32(e), StartPoint: point(s), EndPoint: point(e)}
			if !ok || got != want {
				t.Fatalf("got %q %+v, want %q %+v", tc.source[got.StartByte:got.EndByte], got, tc.header, want)
			}
		})
	}
}

func TestHeaderRangeGroupedBindings(t *testing.T) {
	const source = "export const a = f(), b = g();"
	tree, err := ParseFile("main.js", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	var headers []string
	gts.Walk(tree.RootNode(), func(n *gts.Node, depth int) gts.WalkAction {
		if n.Type(tree.Language()) == "variable_declarator" {
			r, ok := HeaderRange(n, tree.Language())
			if !ok {
				t.Fatalf("missing header for %s", tree.NodeType(n))
			}
			headers = append(headers, source[r.StartByte:r.EndByte])
		}
		return gts.WalkContinue
	})
	if len(headers) != 2 || headers[0] != "export const a" || headers[1] != "b" {
		t.Fatalf("headers = %q", headers)
	}
}
