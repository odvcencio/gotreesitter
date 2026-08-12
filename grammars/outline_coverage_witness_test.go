package grammars

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// outlineWitnessCase pins one language's real coverage as DATA: a tiny
// source snippet and one symbol -- exact name, exact kind -- that the
// language's resolved tags query MUST produce through the real Outliner
// pipeline (gotreesitter.NewOutliner + OutlineTree, the same mechanism
// grammars.ExtractOutline is specified to delegate to). A pattern that only
// "compiles" but never fires (the class of defect the C-oracle sweep found
// in seven core-nine rows, spec.outline-api.v1 G-2) cannot pass this test:
// an empty or wrong result fails loudly instead of hiding behind a
// non-empty query string.
//
// Every case here is a language this change either moved out of query-empty
// (T3/T4) or corrected from a vacuous/degraded shared-inference match
// (non-empty query, zero or wrong captures) to a real one. The source
// snippets and expected symbols were verified against this repository's
// OWN compiled grammar blobs before being written here (Node.SExpr(lang)
// plus an isolated single-pattern firing test), never against upstream
// tree-sitter.
type outlineWitnessCase struct {
	Language string
	Source   string
	WantName string
	WantKind string
	// WantNodeType, when set, additionally pins the captured definition
	// node's grammar type, catching a kind-normalization or refinement
	// change that a name+kind check alone could miss.
	WantNodeType string
}

var outlineWitnessCases = []outlineWitnessCase{
	// --- moved out of query-empty (T3/T4 -> T1/T2) ---
	{
		Language: "php",
		Source: "<?php\n" +
			"class Person {\n" +
			"    function greet() {\n" +
			"        return \"hi\";\n" +
			"    }\n" +
			"}\n",
		WantName: "greet", WantKind: "method",
	},
	{
		Language: "erlang",
		Source:   "-module(m).\n\nadd(A, B) ->\n    A + B.\n",
		WantName: "add", WantKind: "function",
	},
	{
		Language: "ocaml",
		Source:   "let add a b = a + b\n",
		WantName: "add", WantKind: "function",
	},
	{
		Language: "bash",
		Source:   "greet() {\n  echo \"hi\"\n}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "fish",
		Source:   "function greet\n    echo \"hi\"\nend\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "graphql",
		Source:   "type Query {\n  hero: String\n}\n",
		WantName: "Query", WantKind: "type",
	},
	{
		Language: "hcl",
		Source:   "resource \"aws_instance\" \"web\" {\n  ami = \"abc\"\n}\n",
		WantName: "web", WantKind: "block",
	},
	{
		Language: "cmake",
		Source:   "function(greet name)\n  message(STATUS \"${name}\")\nendfunction()\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "powershell",
		Source:   "function Greet {\n    param($Name)\n}\n",
		WantName: "Greet", WantKind: "function",
	},
	{
		Language: "sql",
		Source:   "CREATE TABLE users (\n  id INT PRIMARY KEY\n);\n",
		WantName: "users", WantKind: "type",
	},
	{
		Language: "proto",
		Source:   "syntax = \"proto3\";\n\nmessage Person {\n  string name = 1;\n}\n",
		WantName: "Person", WantKind: "message",
	},
	{
		Language: "clojure",
		Source:   "(defn greet [name]\n  (str \"hi \" name))\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "commonlisp",
		Source:   "(defun greet (name)\n  (format nil \"hi ~a\" name))\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "scheme",
		Source:   "(define (greet name)\n  (string-append \"hi \" name))\n",
		WantName: "greet", WantKind: "function",
	},

	// --- corrected from a vacuous or wrong shared-inference match ---
	{
		Language: "ruby",
		Source:   "class Person\n  def greet\n    \"hi\"\n  end\nend\n",
		WantName: "greet", WantKind: "method",
	},
	{
		Language: "kotlin",
		Source: "class Person {\n" +
			"    fun greet(): String {\n" +
			"        return \"hi\"\n" +
			"    }\n" +
			"}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "swift",
		Source: "class Person {\n" +
			"    func greet() -> String {\n" +
			"        return \"hi\"\n" +
			"    }\n" +
			"}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "scala",
		Source:   "class Person {\n  def greet(): String = \"hi\"\n}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "dart",
		Source: "class Person {\n" +
			"  String greet() {\n" +
			"    return \"hi\";\n" +
			"  }\n" +
			"}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "zig",
		Source:   "pub fn greet() void {\n    return;\n}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "elixir",
		Source: "defmodule Greeter do\n" +
			"  def greet(name) do\n" +
			"    name\n" +
			"  end\n" +
			"end\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "julia",
		Source:   "function greet(name)\n    return name\nend\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "groovy",
		Source:   "String greet() {\n    return \"hi\"\n}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "solidity",
		Source: "pragma solidity ^0.8.0;\n\n" +
			"contract Greeter {\n" +
			"    function greet() public pure returns (string memory) {\n" +
			"        return \"hi\";\n" +
			"    }\n" +
			"}\n",
		WantName: "Greeter", WantKind: "class",
	},
	{
		Language: "nim",
		Source:   "proc greet(name: string): string =\n  return name\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "crystal",
		Source:   "class Person\n  def greet\n    \"hi\"\n  end\nend\n",
		WantName: "greet", WantKind: "method",
	},
	{
		Language: "d",
		Source:   "string greet() {\n    return \"hi\";\n}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "v",
		Source:   "fn greet() string {\n\treturn 'hi'\n}\n",
		WantName: "greet", WantKind: "function",
	},
	{
		Language: "thrift",
		Source:   "service Greeter {\n  string greet(1: string name);\n}\n",
		WantName: "Greeter", WantKind: "service",
	},
	{
		// Lua: the shared table's function_definition row was found dead
		// (0 matches, C-side "Impossible pattern") via the cgo_harness
		// FleetCensus C-oracle sweep, not this repository's own reasoning;
		// see the "lua" entry comment in tags_query_infer.go.
		Language: "lua",
		Source:   "function obj.method(self)\n    return self\nend\n",
		WantName: "method", WantKind: "method",
	},
}

// TestOutlineCoverageWitnesses runs every outlineWitnessCase through the
// real core Outliner (gotreesitter.NewOutliner, OutlineTree) and asserts
// the named symbol is present, in exactly the reported kind, somewhere in
// the resulting forest. It is the fail-closed complement to
// TestOutlineTierCensus: the census proves a query is non-empty text, this
// proves the query actually PRODUCES the definition it claims to.
func TestOutlineCoverageWitnesses(t *testing.T) {
	for _, tc := range outlineWitnessCases {
		tc := tc
		t.Run(tc.Language, func(t *testing.T) {
			entry := DetectLanguageByName(tc.Language)
			if entry == nil {
				t.Fatalf("language %q is not registered", tc.Language)
			}
			if entry.Language == nil {
				t.Fatalf("language %q has no loader", tc.Language)
			}
			lang := entry.Language()
			if lang == nil {
				t.Fatalf("language %q loader returned nil", tc.Language)
			}

			query := ResolveTagsQuery(*entry)
			if strings.TrimSpace(query) == "" {
				t.Fatalf("language %q resolved an empty tags query", tc.Language)
			}

			source := []byte(tc.Source)
			support := EvaluateParseSupport(*entry, lang)
			parser := gotreesitter.NewParser(lang)
			var tree *gotreesitter.Tree
			var err error
			if support.Backend == ParseBackendTokenSource && entry.TokenSourceFactory != nil {
				tree, err = parser.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
			} else {
				tree, err = parser.Parse(source)
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("parse produced a nil tree")
			}

			outliner, err := gotreesitter.NewOutliner(lang, query)
			if err != nil {
				t.Fatalf("build outliner: %v", err)
			}

			symbols, report := outliner.OutlineTree(tree)
			got, gotType, found := findOutlineWitness(symbols, tc.WantName, tc.WantKind)
			if !found {
				t.Fatalf("%s: expected a %q symbol named %q, not found in forest %s\nreport=%+v",
					tc.Language, tc.WantKind, tc.WantName, describeOutlineSymbols(symbols), report)
			}
			if tc.WantNodeType != "" && gotType != tc.WantNodeType {
				t.Errorf("%s: symbol %q kind %q has NodeType %q, want %q", tc.Language, tc.WantName, tc.WantKind, gotType, tc.WantNodeType)
			}
			_ = got
		})
	}
}

// findOutlineWitness searches the whole forest, nested symbols included,
// for a symbol with the exact name and kind. It returns the matched
// symbol's NodeType alongside the found flag.
func findOutlineWitness(symbols []gotreesitter.OutlineSymbol, name, kind string) (gotreesitter.OutlineSymbol, string, bool) {
	for _, sym := range symbols {
		if sym.Name == name && sym.Kind == kind {
			return sym, sym.NodeType, true
		}
		if found, nodeType, ok := findOutlineWitness(sym.Children, name, kind); ok {
			return found, nodeType, true
		}
	}
	return gotreesitter.OutlineSymbol{}, "", false
}

// describeOutlineSymbols renders a compact "kind:name" forest for a test
// failure message.
func describeOutlineSymbols(symbols []gotreesitter.OutlineSymbol) string {
	var b strings.Builder
	var walk func([]gotreesitter.OutlineSymbol, int)
	walk = func(list []gotreesitter.OutlineSymbol, depth int) {
		for _, sym := range list {
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString(sym.Kind)
			b.WriteString(":")
			b.WriteString(sym.Name)
			b.WriteString("\n")
			walk(sym.Children, depth+1)
		}
	}
	walk(symbols, 0)
	if b.Len() == 0 {
		return "(empty)"
	}
	return "\n" + b.String()
}
