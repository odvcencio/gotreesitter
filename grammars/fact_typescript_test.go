package grammars

import (
	"reflect"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

const tsFixture = `import { x } from "m";
export const LIMIT = 10;
let counter: number = 0, other = "o";
var legacy = 1;
const double = (n: number): number => n * 2;
const bare = (n) => n;
const id = x => x;
const fexp = function (a: string): string { return a; };
export interface Shape<T> extends Base, ns.Other<T> {
  readonly id: string;
  label?: string;
  area(scale?: number, ...rest: T[]): number;
}
type Pair<A, B extends object = {}> = [A, B];
export enum Color { Red, Green = "g" }
abstract class Animal<T> extends Base implements Shape<T> {
  private name: string = "a";
  static count = 0;
  #secret = 1;
  abstract speak(volume: number): void;
  constructor(public tag: string, size?: number) { super(); }
  get label(): string { return this.name; }
  move<U extends object>(to: U, ...via: string[]): Promise<U> { helper<string>(to, ...via); return null as any; }
  over(a: string): void;
  over(a: number): void;
  over(a: any): void {}
}
export function run<T extends object>(this: Window, a: T, { b, c }: Opts, [d]: number[], e = 5, f?: string, ...rest: any[]): T {
  const local = 1;
  function inner(z: number) { return z; }
  class Local { f = 1; localMethod() {} }
  return a;
}
function* gen(): Generator<number> {}
declare function ext(x: number): void;
declare const amb: number;
declare class Amb { m(): void; p: string }
namespace NS { export const q = 1; }
function plain(a, b) {}
const obj = { method() {} };
const Expr = class { exprMethod() {} };
run(1, double(2), ...args, BQtBQ, /re/, a.b, a[0], !a, a + b, (a), <T>a, a as T, a!, await b, () => 1, function () {}, { k: 1 }, [1], null, undefined, true, this, new Foo(1), c ? 1 : 2);
BQtagBQ;
new Foo(2, ...args);
`

const tsxFixture = `import React from "react";
export interface Props { title: string; onSelect?(id: number): void }
export const Card = ({ title, onSelect }: Props): JSX.Element => {
  const label = title.toUpperCase();
  return <div className="card" onClick={() => onSelect?.(1)}>{label}</div>;
};
const ident = <T,>(x: T): T => x;
export function List<T>({ items }: { items: T[] }) { return <ul>{items.map((i) => <li key={String(i)}>{render(i, 2)}</li>)}</ul>; }
class Panel extends React.Component<Props> { state = { open: false }; render() { return <section>{this.props.title}</section>; } }
const el = <Card title="t" />;
mount(<Card title="x" />, document.body, ...rest);
new Panel(3, ...rest);
`

type tsRange struct{ from, to string }

// at returns the byte range from the first occurrence of from through the end
// of to (searched after from). An empty to means from alone.
func (r tsRange) at(t *testing.T, src string, after int) (uint32, uint32) {
	t.Helper()
	lo := strings.Index(src[after:], r.from)
	if lo < 0 {
		t.Fatalf("%q not found after %d", r.from, after)
	}
	lo += after
	hi := lo + len(r.from)
	if r.to != "" {
		end := strings.Index(src[lo:], r.to)
		if end < 0 {
			t.Fatalf("%q not found after %q", r.to, r.from)
		}
		hi = lo + end + len(r.to)
	}
	return uint32(lo), uint32(hi)
}

func tsFacts(t *testing.T, filename, source string, kinds gts.FactKind) (gts.FactSet, *LangEntry, string) {
	t.Helper()
	source = strings.ReplaceAll(source, "BQ", "`")
	entry := DetectLanguage(filename)
	if entry == nil {
		t.Fatalf("no language for %s", filename)
	}
	lang := entry.Language()
	p := gts.NewParser(lang)
	var tree *gts.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = p.ParseWithTokenSource([]byte(source), entry.TokenSourceFactory([]byte(source), lang))
	} else {
		tree, err = p.Parse([]byte(source))
	}
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.RootNode().HasError() {
		t.Fatalf("fixture has parse errors: %s", tree.RootNode().SExpr(lang))
	}
	prog, err := gts.NewFactProgramWithOptions(lang, kinds,
		gts.WithDeclarationRules(DeclarationRules(*entry)),
		gts.WithSignatureRules(SignatureRules(*entry)),
		gts.WithCallArgumentRules(CallArgumentRules(*entry)))
	if err != nil {
		t.Fatal(err)
	}
	facts := prog.Extract(tree)
	// Detach from tree storage before Release.
	return facts, entry, source
}

type tsDecl struct {
	kind, name, node, shape, container string
	construct                          tsRange
	nameCtx                            string // text holding the name; defaults to the construct start
	cont                               tsRange
	embedded                           bool
}

func checkDeclarations(t *testing.T, lang, source string, got []gts.DeclarationFact, specs []tsDecl) {
	t.Helper()
	var want []gts.DeclarationFact
	for _, s := range specs {
		start, end := s.construct.at(t, source, 0)
		ctx := s.nameCtx
		if ctx == "" {
			ctx = source[start:end]
		}
		ctxAt := strings.Index(source[start:], ctx)
		if ctxAt < 0 {
			t.Fatalf("context %q not in construct %q", ctx, source[start:end])
		}
		nameAt := int(start) + ctxAt + strings.Index(ctx, s.name)
		f := gts.DeclarationFact{Lang: lang, Kind: s.kind, Name: s.name, NodeType: s.node, StartByte: start, EndByte: end,
			NameStartByte: uint32(nameAt), NameEndByte: uint32(nameAt + len(s.name)), Shape: s.shape, Container: s.container, Embedded: s.embedded}
		if s.container != "" {
			f.ContainerStartByte, f.ContainerEndByte = s.cont.at(t, source, 0)
		}
		want = append(want, f)
	}
	if len(got) != len(want) {
		for _, g := range got {
			t.Logf("got %s %q %s [%d,%d) container=%q", g.Kind, g.Name, g.NodeType, g.StartByte, g.EndByte, g.Container)
		}
		t.Fatalf("declarations = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("declaration %d = %#v; want %#v", i, got[i], want[i])
		}
	}
}

func TestFactDeclarationsTypeScriptGolden(t *testing.T) {
	facts, _, src := tsFacts(t, "main.ts", tsFixture, gts.FactDeclarations)
	iface := tsRange{"interface Shape<T>", "label?: string;\n  area(scale?: number, ...rest: T[]): number;\n}"}
	color := tsRange{"enum Color", `"g" }`}
	animal := tsRange{"abstract class Animal", "over(a: any): void {}\n}"}
	amb := tsRange{"class Amb", "p: string }"}
	specs := []tsDecl{
		{kind: "constant", name: "LIMIT", node: "variable_declarator", construct: tsRange{"LIMIT = 10", ""}},
		{kind: "variable", name: "counter", node: "variable_declarator", construct: tsRange{"counter: number = 0", ""}},
		{kind: "variable", name: "other", node: "variable_declarator", construct: tsRange{`other = "o"`, ""}},
		{kind: "variable", name: "legacy", node: "variable_declarator", construct: tsRange{"legacy = 1", ""}},
		{kind: "constant", name: "double", node: "variable_declarator", construct: tsRange{"double = (n: number): number => n * 2", ""}},
		{kind: "constant", name: "bare", node: "variable_declarator", construct: tsRange{"bare = (n) => n", ""}},
		{kind: "constant", name: "id", node: "variable_declarator", construct: tsRange{"id = x => x", ""}},
		{kind: "constant", name: "fexp", node: "variable_declarator", construct: tsRange{"fexp = function (a: string): string { return a; }", ""}},
		{kind: "type", name: "Shape", node: "interface_declaration", shape: "interface", construct: iface},
		{kind: "embed", name: "Base", node: "extends_type_clause", construct: tsRange{"extends Base, ns.Other<T>", ""}, container: "Shape", cont: iface},
		{kind: "embed", name: "Other", node: "extends_type_clause", construct: tsRange{"extends Base, ns.Other<T>", ""}, nameCtx: "Other", container: "Shape", cont: iface},
		{kind: "field", name: "id", node: "property_signature", construct: tsRange{"readonly id: string", ""}, container: "Shape", cont: iface},
		{kind: "field", name: "label", node: "property_signature", construct: tsRange{"label?: string", ""}, container: "Shape", cont: iface},
		{kind: "method", name: "area", node: "method_signature", construct: tsRange{"area(scale?: number, ...rest: T[]): number", ""}, container: "Shape", cont: iface},
		{kind: "type", name: "Pair", node: "type_alias_declaration", shape: "alias", construct: tsRange{"type Pair<A, B extends object = {}> = [A, B];", ""}},
		{kind: "type", name: "Color", node: "enum_declaration", shape: "enum", construct: color},
		{kind: "constant", name: "Red", node: "property_identifier", construct: tsRange{"Red", ""}, container: "Color", cont: color},
		{kind: "constant", name: "Green", node: "enum_assignment", construct: tsRange{`Green = "g"`, ""}, container: "Color", cont: color},
		{kind: "type", name: "Animal", node: "abstract_class_declaration", shape: "class", construct: animal},
		{kind: "field", name: "name", node: "public_field_definition", construct: tsRange{`private name: string = "a"`, ""}, container: "Animal", cont: animal},
		{kind: "field", name: "count", node: "public_field_definition", construct: tsRange{"static count = 0", ""}, container: "Animal", cont: animal},
		{kind: "field", name: "#secret", node: "public_field_definition", construct: tsRange{"#secret = 1", ""}, container: "Animal", cont: animal},
		{kind: "method", name: "speak", node: "abstract_method_signature", construct: tsRange{"abstract speak(volume: number): void", ""}, container: "Animal", cont: animal},
		{kind: "method", name: "constructor", node: "method_definition", construct: tsRange{"constructor(public tag: string, size?: number) { super(); }", ""}, container: "Animal", cont: animal},
		{kind: "method", name: "label", node: "method_definition", construct: tsRange{"get label(): string { return this.name; }", ""}, container: "Animal", cont: animal},
		{kind: "method", name: "move", node: "method_definition", construct: tsRange{"move<U extends object>", "return null as any; }"}, container: "Animal", cont: animal},
		{kind: "method", name: "over", node: "method_signature", construct: tsRange{"over(a: string): void", ""}, container: "Animal", cont: animal},
		{kind: "method", name: "over", node: "method_signature", construct: tsRange{"over(a: number): void", ""}, container: "Animal", cont: animal},
		{kind: "method", name: "over", node: "method_definition", construct: tsRange{"over(a: any): void {}", ""}, container: "Animal", cont: animal},
		{kind: "constant", name: "amb", node: "variable_declarator", construct: tsRange{"amb: number", ""}},
		{kind: "type", name: "Amb", node: "class_declaration", shape: "class", construct: amb},
		{kind: "method", name: "m", node: "method_signature", construct: tsRange{"m(): void", ""}, container: "Amb", cont: amb},
		{kind: "field", name: "p", node: "public_field_definition", construct: tsRange{"p: string", ""}, container: "Amb", cont: amb},
		{kind: "constant", name: "obj", node: "variable_declarator", construct: tsRange{"obj = { method() {} }", ""}},
		{kind: "constant", name: "Expr", node: "variable_declarator", construct: tsRange{"Expr = class { exprMethod() {} }", ""}},
	}
	checkDeclarations(t, "typescript", src, facts.Declarations, specs)
	// Locals, namespaces, class expressions, object methods and parameter
	// properties must never become facts.
	for _, g := range facts.Declarations {
		switch g.Name {
		case "local", "inner", "Local", "NS", "q", "tag", "method", "plain", "run", "x":
			t.Errorf("excluded declaration emitted: %#v", g)
		}
		if g.Name == "localMethod" || g.Name == "exprMethod" || g.Name == "f" && g.Container != "" {
			t.Errorf("class-expression or local member emitted: %#v", g)
		}
	}
}

func TestFactDeclarationsTSXGolden(t *testing.T) {
	facts, _, src := tsFacts(t, "main.tsx", tsxFixture, gts.FactDeclarations)
	props := tsRange{"interface Props", "void }"}
	panel := tsRange{"class Panel", "</section>; } }"}
	specs := []tsDecl{
		{kind: "type", name: "Props", node: "interface_declaration", shape: "interface", construct: props},
		{kind: "field", name: "title", node: "property_signature", construct: tsRange{"title: string", ""}, container: "Props", cont: props},
		{kind: "method", name: "onSelect", node: "method_signature", construct: tsRange{"onSelect?(id: number): void", ""}, container: "Props", cont: props},
		{kind: "constant", name: "Card", node: "variable_declarator", construct: tsRange{"Card = ({ title", "</div>;\n}"}},
		{kind: "constant", name: "ident", node: "variable_declarator", construct: tsRange{"ident = <T,>(x: T): T => x", ""}},
		{kind: "type", name: "Panel", node: "class_declaration", shape: "class", construct: panel},
		{kind: "field", name: "state", node: "public_field_definition", construct: tsRange{"state = { open: false }", ""}, container: "Panel", cont: panel},
		{kind: "method", name: "render", node: "method_definition", construct: tsRange{"render() {", "</section>; }"}, container: "Panel", cont: panel},
		{kind: "constant", name: "el", node: "variable_declarator", construct: tsRange{`el = <Card title="t" />`, ""}},
	}
	checkDeclarations(t, "tsx", src, facts.Declarations, specs)
	for _, g := range facts.Declarations {
		if g.Name == "label" || g.Name == "React" {
			t.Errorf("local or import emitted: %#v", g)
		}
	}
}

type tsParam struct {
	anchor, decl, name, typ string
	variadic, optional      bool
}

type tsSig struct {
	kind, name, node, container string
	construct                   tsRange
	nameCtx                     string
	types, params, results      []tsParam
}

func (s tsSig) parameters(t *testing.T, src string, from int, list []tsParam) []gts.ParameterFact {
	t.Helper()
	var out []gts.ParameterFact
	for _, p := range list {
		anchor := p.anchor
		if anchor == "" {
			anchor = p.decl
		}
		a := strings.Index(src[from:], anchor)
		if a < 0 {
			t.Fatalf("anchor %q not found for %s", anchor, s.name)
		}
		declAt := from + a + strings.Index(anchor, p.decl)
		f := gts.ParameterFact{StartByte: uint32(declAt), EndByte: uint32(declAt + len(p.decl)), Variadic: p.variadic, Optional: p.optional}
		if p.name != "" {
			n := declAt + strings.Index(p.decl, p.name)
			f.Name, f.NameStartByte, f.NameEndByte = p.name, uint32(n), uint32(n+len(p.name))
		}
		if p.typ != "" {
			ty := declAt + strings.LastIndex(p.decl, p.typ)
			f.Type, f.TypeStartByte, f.TypeEndByte = p.typ, uint32(ty), uint32(ty+len(p.typ))
		}
		out = append(out, f)
	}
	return out
}

func checkSignatures(t *testing.T, lang, source string, got []gts.SignatureFact, specs []tsSig) {
	t.Helper()
	if len(got) != len(specs) {
		for _, g := range got {
			t.Logf("got %s %q %s [%d,%d) container=%q", g.Kind, g.Name, g.NodeType, g.StartByte, g.EndByte, g.Container)
		}
		t.Fatalf("signatures = %d; want %d", len(got), len(specs))
	}
	for i, s := range specs {
		start, end := s.construct.at(t, source, 0)
		ctx := s.nameCtx
		if ctx == "" {
			ctx = s.name
		}
		nameAt := int(start) + strings.Index(source[start:end], ctx) + strings.Index(ctx, s.name)
		from := int(start)
		want := gts.SignatureFact{Lang: lang, Kind: s.kind, Name: s.name, Container: s.container, NodeType: s.node,
			StartByte: start, EndByte: end, NameStartByte: uint32(nameAt), NameEndByte: uint32(nameAt + len(s.name)),
			TypeParameters: s.parameters(t, source, from, s.types), Parameters: s.parameters(t, source, from, s.params), Results: s.parameters(t, source, from, s.results)}
		if !reflect.DeepEqual(got[i], want) {
			t.Errorf("signature %d (%s) = %#v\nwant %#v", i, s.name, got[i], want)
		}
	}
}

func TestFactSignaturesTypeScriptGolden(t *testing.T) {
	facts, _, src := tsFacts(t, "main.ts", tsFixture, gts.FactSignatures)
	specs := []tsSig{
		{kind: "function", name: "double", node: "variable_declarator", construct: tsRange{"double = (n: number): number => n * 2", ""},
			params: []tsParam{{decl: "n: number", name: "n", typ: "number"}}, results: []tsParam{{anchor: "): number", decl: "number", typ: "number"}}},
		{kind: "function", name: "bare", node: "variable_declarator", construct: tsRange{"bare = (n) => n", ""},
			params: []tsParam{{anchor: "(n)", decl: "n", name: "n"}}},
		{kind: "function", name: "id", node: "variable_declarator", construct: tsRange{"id = x => x", ""},
			params: []tsParam{{anchor: "= x", decl: "x", name: "x"}}},
		{kind: "function", name: "fexp", node: "variable_declarator", construct: tsRange{"fexp = function (a: string): string { return a; }", ""},
			params: []tsParam{{decl: "a: string", name: "a", typ: "string"}}, results: []tsParam{{anchor: "): string", decl: "string", typ: "string"}}},
		{kind: "method", name: "area", node: "method_signature", container: "Shape", construct: tsRange{"area(scale?: number, ...rest: T[]): number", ""},
			params:  []tsParam{{decl: "scale?: number", name: "scale", typ: "number", optional: true}, {decl: "...rest: T[]", name: "rest", typ: "T[]", variadic: true}},
			results: []tsParam{{anchor: "): number", decl: "number", typ: "number"}}},
		{kind: "method", name: "speak", node: "abstract_method_signature", container: "Animal", construct: tsRange{"abstract speak(volume: number): void", ""}, nameCtx: "speak",
			params: []tsParam{{decl: "volume: number", name: "volume", typ: "number"}}, results: []tsParam{{anchor: "): void", decl: "void", typ: "void"}}},
		{kind: "method", name: "constructor", node: "method_definition", container: "Animal", construct: tsRange{"constructor(public tag: string, size?: number) { super(); }", ""},
			params: []tsParam{{decl: "public tag: string", name: "tag", typ: "string"}, {decl: "size?: number", name: "size", typ: "number", optional: true}}},
		{kind: "method", name: "label", node: "method_definition", container: "Animal", construct: tsRange{"get label(): string { return this.name; }", ""},
			results: []tsParam{{anchor: "): string", decl: "string", typ: "string"}}},
		{kind: "method", name: "move", node: "method_definition", container: "Animal", construct: tsRange{"move<U extends object>", "return null as any; }"},
			types:   []tsParam{{decl: "U extends object", name: "U", typ: "object"}},
			params:  []tsParam{{decl: "to: U", name: "to", typ: "U"}, {decl: "...via: string[]", name: "via", typ: "string[]", variadic: true}},
			results: []tsParam{{anchor: ": Promise<U>", decl: "Promise<U>", typ: "Promise<U>"}}},
		{kind: "method", name: "over", node: "method_signature", container: "Animal", construct: tsRange{"over(a: string): void", ""},
			params: []tsParam{{decl: "a: string", name: "a", typ: "string"}}, results: []tsParam{{anchor: "): void", decl: "void", typ: "void"}}},
		{kind: "method", name: "over", node: "method_signature", container: "Animal", construct: tsRange{"over(a: number): void", ""},
			params: []tsParam{{decl: "a: number", name: "a", typ: "number"}}, results: []tsParam{{anchor: "): void", decl: "void", typ: "void"}}},
		{kind: "method", name: "over", node: "method_definition", container: "Animal", construct: tsRange{"over(a: any): void {}", ""},
			params: []tsParam{{decl: "a: any", name: "a", typ: "any"}}, results: []tsParam{{anchor: "): void", decl: "void", typ: "void"}}},
		{kind: "function", name: "run", node: "function_declaration", construct: tsRange{"function run<T extends object>", "return a;\n}"},
			types: []tsParam{{decl: "T extends object", name: "T", typ: "object"}},
			params: []tsParam{{decl: "this: Window", typ: "Window"}, {decl: "a: T", name: "a", typ: "T"}, {decl: "{ b, c }: Opts", typ: "Opts"}, {decl: "[d]: number[]", typ: "number[]"},
				{decl: "e = 5", name: "e"}, {decl: "f?: string", name: "f", typ: "string", optional: true}, {decl: "...rest: any[]", name: "rest", typ: "any[]", variadic: true}},
			results: []tsParam{{anchor: "): T {", decl: "T", typ: "T"}}},
		{kind: "function", name: "inner", node: "function_declaration", construct: tsRange{"function inner(z: number) { return z; }", ""},
			params: []tsParam{{decl: "z: number", name: "z", typ: "number"}}},
		{kind: "method", name: "localMethod", node: "method_definition", container: "Local", construct: tsRange{"localMethod() {}", ""}},
		{kind: "function", name: "gen", node: "generator_function_declaration", construct: tsRange{"function* gen(): Generator<number> {}", ""},
			results: []tsParam{{anchor: ": Generator<number>", decl: "Generator<number>", typ: "Generator<number>"}}},
		{kind: "function", name: "ext", node: "function_signature", construct: tsRange{"function ext(x: number): void;", ""},
			params: []tsParam{{decl: "x: number", name: "x", typ: "number"}}, results: []tsParam{{anchor: "): void", decl: "void", typ: "void"}}},
		{kind: "method", name: "m", node: "method_signature", container: "Amb", construct: tsRange{"m(): void", ""}, nameCtx: "m",
			results: []tsParam{{anchor: ": void", decl: "void", typ: "void"}}},
		{kind: "function", name: "plain", node: "function_declaration", construct: tsRange{"function plain(a, b) {}", ""},
			params: []tsParam{{anchor: "(a", decl: "a", name: "a"}, {anchor: ", b", decl: "b", name: "b"}}},
		{kind: "method", name: "exprMethod", node: "method_definition", construct: tsRange{"exprMethod() {}", ""}},
	}
	checkSignatures(t, "typescript", src, facts.Signatures, specs)
	for _, g := range facts.Signatures {
		switch g.Name {
		case "method", "obj", "Expr", "Local":
			t.Errorf("excluded signature emitted: %#v", g)
		}
	}
}

func TestFactSignaturesTSXGolden(t *testing.T) {
	facts, _, src := tsFacts(t, "main.tsx", tsxFixture, gts.FactSignatures)
	specs := []tsSig{
		{kind: "method", name: "onSelect", node: "method_signature", container: "Props", construct: tsRange{"onSelect?(id: number): void", ""},
			params: []tsParam{{decl: "id: number", name: "id", typ: "number"}}, results: []tsParam{{anchor: "): void", decl: "void", typ: "void"}}},
		{kind: "function", name: "Card", node: "variable_declarator", construct: tsRange{"Card = ({ title", "</div>;\n}"},
			params:  []tsParam{{decl: "{ title, onSelect }: Props", typ: "Props"}},
			results: []tsParam{{anchor: "): JSX.Element", decl: "JSX.Element", typ: "JSX.Element"}}},
		{kind: "function", name: "ident", node: "variable_declarator", construct: tsRange{"ident = <T,>(x: T): T => x", ""},
			types:   []tsParam{{anchor: "<T,>", decl: "T", name: "T"}},
			params:  []tsParam{{decl: "x: T", name: "x", typ: "T"}},
			results: []tsParam{{anchor: "): T", decl: "T", typ: "T"}}},
		{kind: "function", name: "List", node: "function_declaration", construct: tsRange{"function List<T>", "</ul>; }"},
			types:  []tsParam{{anchor: "<T>", decl: "T", name: "T"}},
			params: []tsParam{{decl: "{ items }: { items: T[] }", typ: "{ items: T[] }"}}},
		{kind: "method", name: "render", node: "method_definition", container: "Panel", construct: tsRange{"render() {", "</section>; }"}},
	}
	checkSignatures(t, "tsx", src, facts.Signatures, specs)
}

type tsCall struct {
	call string
	args []tsArg
}
type tsArg struct{ text, kind, node string }

func (c tsCall) want(t *testing.T, src string) []gts.CallArgumentFact {
	t.Helper()
	callAt := strings.Index(src, c.call)
	if callAt < 0 {
		t.Fatalf("call %q missing", c.call)
	}
	next := strings.Index(c.call, "(") + 1
	var out []gts.CallArgumentFact
	for i, a := range c.args {
		at := strings.Index(c.call[next:], a.text) + next
		next = at + len(a.text)
		f := gts.CallArgumentFact{CallStartByte: uint32(callAt), CallEndByte: uint32(callAt + len(c.call)), Index: i, Kind: a.kind, NodeType: a.node,
			StartByte: uint32(callAt + at), EndByte: uint32(callAt + at + len(a.text)), Spread: a.node == "spread_element"}
		out = append(out, f)
	}
	return out
}

func checkJoin(t *testing.T, facts gts.FactSet, source string) {
	t.Helper()
	for _, a := range facts.CallArguments {
		n := 0
		for _, c := range facts.Calls {
			if c.StartByte == a.CallStartByte && c.EndByte == a.CallEndByte {
				n++
			}
		}
		want := 1
		if strings.HasPrefix(source[a.CallStartByte:a.CallEndByte], "new ") {
			want = 0
		}
		if n != want {
			t.Errorf("argument joins %d calls: %#v", n, a)
		}
	}
}

func TestFactCallArgumentsTypeScriptGolden(t *testing.T) {
	facts, _, src := tsFacts(t, "main.ts", tsFixture, gts.FactCalls|gts.FactCallArguments)
	runCall := "run(1, double(2), ...args, `t`, /re/, a.b, a[0], !a, a + b, (a), <T>a, a as T, a!, await b, () => 1, function () {}, { k: 1 }, [1], null, undefined, true, this, new Foo(1), c ? 1 : 2)"
	calls := []tsCall{
		{"helper<string>(to, ...via)", []tsArg{{"to", "identifier", "identifier"}, {"...via", "identifier", "spread_element"}}},
		{runCall, []tsArg{
			{"1", "literal", "number"}, {"double(2)", "call", "call_expression"}, {"...args", "identifier", "spread_element"}, {"`t`", "literal", "template_string"},
			{"/re/", "literal", "regex"}, {"a.b", "selector", "member_expression"}, {"a[0]", "index", "subscript_expression"}, {"!a", "unary", "unary_expression"},
			{"a + b", "binary", "binary_expression"}, {"(a)", "parenthesized", "parenthesized_expression"}, {"<T>a", "type_assertion", "type_assertion"},
			{"a as T", "type_assertion", "as_expression"}, {"a!", "other", "non_null_expression"}, {"await b", "other", "await_expression"},
			{"() => 1", "func_literal", "arrow_function"}, {"function () {}", "func_literal", "function_expression"}, {"{ k: 1 }", "composite_literal", "object"},
			{"[1]", "composite_literal", "array"}, {"null", "literal", "null"}, {"undefined", "literal", "undefined"}, {"true", "literal", "true"},
			{"this", "identifier", "this"}, {"new Foo(1)", "call", "new_expression"}, {"c ? 1 : 2", "other", "ternary_expression"}}},
		{"double(2)", []tsArg{{"2", "literal", "number"}}},
		{"new Foo(1)", []tsArg{{"1", "literal", "number"}}},
		{"new Foo(2, ...args)", []tsArg{{"2", "literal", "number"}, {"...args", "identifier", "spread_element"}}},
	}
	var want []gts.CallArgumentFact
	for _, c := range calls {
		want = append(want, c.want(t, src)...)
	}
	if !reflect.DeepEqual(facts.CallArguments, want) {
		for i := range facts.CallArguments {
			if i < len(want) && !reflect.DeepEqual(facts.CallArguments[i], want[i]) {
				t.Errorf("argument %d = %#v; want %#v", i, facts.CallArguments[i], want[i])
			}
		}
		t.Fatalf("arguments = %d; want %d", len(facts.CallArguments), len(want))
	}
	checkJoin(t, facts, src)
	// Type arguments and tagged templates are not value arguments.
	// Constructor arguments are standalone and join no CallRef.
	for _, a := range facts.CallArguments {
		if src[a.StartByte:a.EndByte] == "string" || src[a.StartByte:a.EndByte] == "`tag`" {
			t.Errorf("non-value argument emitted: %#v", a)
		}
	}
	only, _, _ := tsFacts(t, "main.ts", tsFixture, gts.FactCallArguments)
	if !reflect.DeepEqual(only.CallArguments, facts.CallArguments) || len(only.Calls) != 0 {
		t.Fatal("argument-only selection changed the join")
	}
}

func TestFactCallArgumentsTSXGolden(t *testing.T) {
	facts, _, src := tsFacts(t, "main.tsx", tsxFixture, gts.FactCalls|gts.FactCallArguments)
	calls := []tsCall{
		{"onSelect?.(1)", []tsArg{{"1", "literal", "number"}}},
		{"items.map((i) => <li key={String(i)}>{render(i, 2)}</li>)", []tsArg{{"(i) => <li key={String(i)}>{render(i, 2)}</li>", "func_literal", "arrow_function"}}},
		{"String(i)", []tsArg{{"i", "identifier", "identifier"}}},
		{"render(i, 2)", []tsArg{{"i", "identifier", "identifier"}, {"2", "literal", "number"}}},
		{`mount(<Card title="x" />, document.body, ...rest)`, []tsArg{{`<Card title="x" />`, "other", "jsx_self_closing_element"}, {"document.body", "selector", "member_expression"}, {"...rest", "identifier", "spread_element"}}},
		{"new Panel(3, ...rest)", []tsArg{{"3", "literal", "number"}, {"...rest", "identifier", "spread_element"}}},
	}
	var want []gts.CallArgumentFact
	for _, c := range calls {
		want = append(want, c.want(t, src)...)
	}
	if !reflect.DeepEqual(facts.CallArguments, want) {
		for _, a := range facts.CallArguments {
			t.Logf("got %#v %q", a, src[a.StartByte:a.EndByte])
		}
		t.Fatalf("arguments = %d; want %d", len(facts.CallArguments), len(want))
	}
	checkJoin(t, facts, src)
}

func tsEntries() []LangEntry {
	return []LangEntry{{Name: "typescript", Language: TypescriptLanguage}, {Name: "tsx", Language: TsxLanguage}}
}

func TestTypeScriptRulesResolveFully(t *testing.T) {
	for _, entry := range tsEntries() {
		t.Run(entry.Name, func(t *testing.T) {
			if got, want := len(DeclarationRules(entry)), len(declarationRuleTable[entry.Name]); want == 0 || got != want {
				t.Errorf("declaration rules = %d of %d", got, want)
			}
			if got, want := len(SignatureRules(entry)), len(signatureRuleTable[entry.Name]); want == 0 || got != want {
				t.Errorf("signature rules = %d of %d", got, want)
			}
			if got, want := len(CallArgumentRules(entry)), len(callArgumentRuleTable[entry.Name]); want == 0 || got != want {
				t.Errorf("call argument rules = %d of %d", got, want)
			}
		})
	}
}

func TestTypeScriptRulesGateMissingGrammarData(t *testing.T) {
	// A grammar lacking the TypeScript node types must produce no rows at all.
	foreign := LangEntry{Name: "typescript", Language: GoLanguage}
	if got := DeclarationRules(foreign); len(got) != 0 {
		t.Errorf("declaration rules on a foreign grammar = %d", len(got))
	}
	if got := SignatureRules(foreign); len(got) != 0 {
		t.Errorf("signature rules on a foreign grammar = %d", len(got))
	}
	if got := CallArgumentRules(foreign); len(got) != 0 {
		t.Errorf("call argument rules on a foreign grammar = %d", len(got))
	}
	entry := LangEntry{Name: "typescript", Language: TypescriptLanguage}
	good := declarationRuleTable["typescript"][0]
	for _, part := range []string{"node", "name field", "name type", "ancestor", "parent field", "parent node", "container node", "container field", "base type node"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			bad.ContainerPath = append([]gts.ContainerRule(nil), good.ContainerPath...)
			bad.TypeNames = append([]gts.TypeNameRule(nil), good.TypeNames...)
			switch part {
			case "node":
				bad.NodeType = "absent_node"
			case "name field":
				bad.NameField = "absent_field"
			case "name type":
				bad.NameNodeType = "absent_node"
			case "ancestor":
				bad.Ancestors = []string{"absent_node"}
			case "parent field":
				bad.ParentChildField, bad.ParentChildNodeType = "absent_field", "const"
			case "parent node":
				bad.ParentChildField, bad.ParentChildNodeType = "kind", "absent_node"
			case "container node":
				bad.ContainerPath = []gts.ContainerRule{{NodeType: "absent_node", NameField: "name", NameNodeType: "type_identifier", TypeField: "body"}}
			case "container field":
				bad.ContainerPath = []gts.ContainerRule{{NodeType: "class_declaration", NameField: "absent_field", NameNodeType: "type_identifier", TypeField: "body"}}
			case "base type node":
				bad.TypeNames = []gts.TypeNameRule{{NodeType: "absent_node"}}
			}
			if got := gateDeclarationRules(entry, []gts.DeclarationRule{bad, good}); len(got) != 1 {
				t.Fatalf("gate = %d rows; want only the valid row", len(got))
			}
		})
	}
	sig := signatureRuleTable["typescript"][0]
	for _, part := range []string{"delegate field", "delegate node", "parent field", "wrapper node", "optional parameter field", "variadic wrapper", "anonymous pattern", "single parameter field"} {
		t.Run("signature "+part, func(t *testing.T) {
			bad := sig
			bad.Parameters = append([]gts.ParameterRule(nil), sig.Parameters...)
			bad.TypeWrapperNodeTypes = append([]string(nil), sig.TypeWrapperNodeTypes...)
			switch part {
			case "delegate field":
				bad.DelegateField, bad.DelegateNodeTypes = "absent_field", []string{"arrow_function"}
			case "delegate node":
				bad.DelegateField, bad.DelegateNodeTypes = "value", []string{"absent_node"}
			case "parent field":
				bad.ParentChildField, bad.ParentChildNodeType = "absent_field", "const"
			case "wrapper node":
				bad.TypeWrapperNodeTypes = []string{"absent_node"}
			case "optional parameter field":
				bad.Parameters[1].NameField = "absent_field"
			case "variadic wrapper":
				bad.Parameters[0].VariadicNodeTypes = []string{"absent_node"}
			case "anonymous pattern":
				bad.Parameters[0].AnonymousPatterns = []string{"absent_node"}
			case "single parameter field":
				bad.SingleParameterField = "absent_field"
			}
			if got := gateSignatureRules(entry, []gts.SignatureRule{bad, sig}); len(got) != 1 {
				t.Fatalf("gate = %d rows; want only the valid row", len(got))
			}
		})
	}
	// Rows must not alias the table: mutate the gated copy and compare.
	rules := SignatureRules(entry)
	rules[0].TypeWrapperNodeTypes[0] = "changed"
	rules[0].Parameters[1].AnonymousPatterns = nil
	if again := SignatureRules(entry); again[0].TypeWrapperNodeTypes[0] == "changed" || len(again[0].Parameters[1].AnonymousPatterns) == 0 {
		t.Fatal("gated signature rows alias the table")
	}
}

func TestTypeScriptCallArgumentKindsPerGrammar(t *testing.T) {
	has := func(entry LangEntry, node string) bool {
		for _, row := range CallArgumentRules(entry) {
			for _, k := range row.Kinds {
				if k.NodeType == node {
					return true
				}
			}
		}
		return false
	}
	entries := tsEntries()
	if !has(entries[0], "type_assertion") || has(entries[1], "type_assertion") {
		t.Fatal("angle-bracket assertions belong to the typescript grammar only")
	}
	if !has(entries[1], "as_expression") {
		t.Fatal("tsx lost the shared as_expression kind")
	}
}

func TestTypeScriptFactsTolerateSyntaxErrors(t *testing.T) {
	for _, name := range []string{"main.ts", "main.tsx"} {
		entry := DetectLanguage(name)
		lang := entry.Language()
		prog, err := gts.NewFactProgramWithOptions(lang, gts.FactDeclarations|gts.FactSignatures|gts.FactCallArguments|gts.FactCalls,
			gts.WithDeclarationRules(DeclarationRules(*entry)), gts.WithSignatureRules(SignatureRules(*entry)), gts.WithCallArgumentRules(CallArgumentRules(*entry)))
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range []string{
			"interface I extends { a: ; m(: void }\nclass { x = ; m( }\nenum E { = 1 }\nconst = ;\nf(a, , ...);",
			"const f = (a: , ...: number => ;\nfunction (x) {}\nexport const",
		} {
			p := gts.NewParser(lang)
			var tree *gts.Tree
			if entry.TokenSourceFactory != nil {
				tree, err = p.ParseWithTokenSource([]byte(source), entry.TokenSourceFactory([]byte(source), lang))
			} else {
				tree, err = p.Parse([]byte(source))
			}
			if err != nil {
				t.Fatal(err)
			}
			facts := prog.Extract(tree)
			for _, d := range facts.Declarations {
				if d.Name == "" || d.EndByte > uint32(len(source)) || d.NameEndByte > d.EndByte {
					t.Errorf("%s: bad declaration %#v", name, d)
				}
			}
			tree.Release()
		}
	}
}
