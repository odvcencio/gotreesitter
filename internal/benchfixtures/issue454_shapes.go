package benchfixtures

import (
	"bytes"
	"fmt"
	"strings"
)

// The C# and scala_report shapes reproduce the report byte for byte.
// Other shapes are deterministic reconstructions because the reporter did not
// publish their generators.
func GeneratedSource(lang string, n int) ([]byte, string, error) {
	var b bytes.Buffer
	m := "x0"
	switch lang {
	case "nushell-comments", "zig-comments", "scss-comments", "diff-comments", "http-comments":
		prefix := "#"
		if lang == "zig-comments" || lang == "scss-comments" {
			prefix = "//"
		}
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "%s note %d\n", prefix, i)
		}
		m = "note"
	case "c_sharp":
		b.WriteString("using System;\n\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "class C%d {\n\tpublic int F%d(int a, int b) {\n\t\tvar x%d = a + b;\n\t\treturn x%d;\n\t}\n}\n\n", i, i, i, i)
		}
	case "go":
		b.WriteString("package main\n\nimport \"fmt\"\n\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
		}
		m = "x := a + b"
	case "rust":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "fn f%d(a: i32, b: i32) -> i32 {\n    let x0 = a + b;\n    println!(\"f%d {}\", x0);\n    x0\n}\n\n", i, i)
		}
	case "scala_report":
		b.WriteString("package demo\n\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "object O%d {\n  def f%d(a: Int, b: Int): Int = {\n    val x%d = a + b\n    x%d\n  }\n}\n\n", i, i, i, i)
		}
	case "scala":
		b.WriteString("object Main {\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "  def f%d(a: Int, b: Int): Int = {\n    val x0 = a + b\n    println(x0)\n    x0\n  }\n\n", i)
		}
		b.WriteString("}\n")
	case "cmake":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "function(f%d a b)\n  set(x0 ${a})\n  message(STATUS \"f%d ${x0}\")\nendfunction()\n\n", i, i)
		}
	case "toml":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "[section%d]\nx0 = %d\nname = \"f%d\"\nenabled = true\n\n", i, i, i)
		}
	case "ini":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "[section%d]\nx0 = %d\nname = f%d\n\n", i, i, i)
		}
	case "make":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "f%d: x0.o\n\t$(CC) -o f%d x0.o\n\n", i, i)
		}
	case "make-report":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "VAR%d = value%d\ntarget%d: dep%d\n\t@echo target%d\n\n", i, i, i, i, i)
		}
		m = "target0:"
	case "dart-report", "dart-report-class":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "class C%d {\n  int f%d(int a, int b) {\n    var x%d = a + b;\n    return x%d;\n  }\n}\n\n", i, i, i, i)
		}
		m = "x0"
		if lang == "dart-report-class" {
			m = "class C0"
		}
	case "dart-report-single":
		b.WriteString("class C {\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "  int f%d(int a, int b) {\n    var x%d = a + b;\n    return x%d;\n  }\n", i, i, i)
		}
		b.WriteString("}\n")
	case "css", "scss", "less":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, ".f%d {\n  color: red;\n  width: 10px;\n}\n\n", i)
		}
		m = "red"
	case "typescript", "tsx", "javascript":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "function f%d(a: number, b: number): number {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n", i, i)
		}
		if lang == "javascript" {
			b.Reset()
			for i := 0; b.Len() < n; i++ {
				fmt.Fprintf(&b, "function f%d(a, b) {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n", i, i)
			}
		}
	case "json":
		b.WriteString("[\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "  {\"x0\": %d, \"name\": \"f%d\"},\n", i, i)
		}
		s := strings.TrimSuffix(b.String(), ",\n") + "\n]\n"
		b.Reset()
		b.WriteString(s)
	case "haskell":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "f%d :: Int -> Int -> Int\nf%d a b = x0\n  where x0 = a + b\n\n", i, i)
		}
	case "hcl":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "resource \"aws_instance\" \"f%d\" {\n  x0 = %d\n  name = \"f%d\"\n}\n\n", i, i, i)
		}
	case "diff":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "diff --git a/f%d.txt b/f%d.txt\n--- a/f%d.txt\n+++ b/f%d.txt\n@@ -1,2 +1,2 @@\n-x0 old\n+x0 new\n", i, i, i, i)
		}
	case "c":
		b.WriteString("#include <stdio.h>\n\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "int f%d(int a, int b) {\n    int x0 = a + b;\n    printf(\"f%d %%d\\n\", x0);\n    return x0;\n}\n\n", i, i)
		}
	case "python":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "def f%d(a, b):\n    x0 = a + b\n    print(\"f%d\", x0)\n    return x0\n\n\n", i, i)
		}
	case "cpp", "objc":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "int f%d(int a, int b) { int x0 = a + b; return x0; }\n", i)
		}
	case "elixir":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "defmodule M%d do\n  def f(a, b), do: a + b\nend\n\n", i)
		}
		m = "M0"
	case "sql":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "CREATE TABLE t%d (x0 INTEGER, name TEXT);\n", i)
		}
	case "ruby":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "def f%d(a, b)\n  x0 = a + b\n  x0\nend\n\n", i)
		}
	case "perl":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "sub f%d { my ($a, $b) = @_; my $x0 = $a + $b; return $x0; }\n", i)
		}
	case "sh":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "f%d() { x0=%d; echo \"$x0\"; }\n", i, i)
		}
	case "ps1":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "function f%d { $x0 = %d; Write-Output $x0 }\n", i, i)
		}
	case "kotlin":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "fun f%d(a: Int, b: Int): Int { val x0 = a + b; return x0 }\n", i)
		}
	case "xml":
		b.WriteString("<root>\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "<item id=\"%d\"><x0>value</x0></item>\n", i)
		}
		b.WriteString("</root>\n")
	case "vue":
		b.WriteString("<template><main>\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "<div class=\"x0\">note %d</div>\n", i)
		}
		b.WriteString("</main></template>\n")
	case "svelte":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "<div class=\"x0\">note %d</div>\n", i)
		}
	case "java":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "class C%d { int f(int a, int b) { int x0 = a + b; return x0; } }\n", i)
		}
	case "graphql":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "type T%d { x0: Int, name: String }\n", i)
		}
	case "r":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "f%d <- function(a, b) { x0 <- a + b; x0 }\n", i)
		}
	case "php":
		b.WriteString("<?php\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "function f%d($a, $b) { $x0 = $a + $b; return $x0; }\n", i)
		}
	case "zig":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "pub fn f%d(a: i32, b: i32) i32 { const x0 = a + b; return x0; }\n", i)
		}
	case "nix":
		b.WriteString("{\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "  x%d = %d;\n", i, i)
		}
		b.WriteString("}\n")
	case "yaml":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "item%d:\n  x0: %d\n", i, i)
		}
	case "swift":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "func f%d(_ a: Int, _ b: Int) -> Int { let x0 = a + b; return x0 }\n", i)
		}
	case "rst":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "Section %d\n==========\n\nx0 is a note.\n\n", i)
		}
	case "lua":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "function f%d(a, b) local x0 = a + b; return x0 end\n", i)
		}
	case "proto":
		b.WriteString("syntax = \"proto3\";\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "message M%d { int32 x0 = 1; }\n", i)
		}
	case "dockerfile":
		b.WriteString("FROM alpine\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "RUN echo x0-%d\n", i)
		}
	case "md":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "## Section %d\n\nx0 is a note.\n\n", i)
		}
	case "groovy":
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "class C%d { int f(int a, int b) { def x0 = a + b; return x0 } }\n", i)
		}
	case "templ":
		b.WriteString("package demo\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "templ C%d() {\n  <div class=\"x0\">note</div>\n}\n", i)
		}
	case "csv":
		b.WriteString("x0,name\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "%d,item%d\n", i, i)
		}
	case "html":
		b.WriteString("<!DOCTYPE html><html><body>\n")
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "<div class=\"x0\">note %d</div>\n", i)
		}
		b.WriteString("</body></html>\n")
	default:
		return nil, "", fmt.Errorf("no generator for %s", lang)
	}
	return b.Bytes(), m, nil
}

// GeneratedLanguages lists the generator shapes from cmd/issue454bench.
func GeneratedLanguages() []string {
	return []string{
		"nushell-comments",
		"zig-comments",
		"scss-comments",
		"diff-comments",
		"http-comments",
		"c_sharp",
		"go",
		"rust",
		"scala_report",
		"scala",
		"cmake",
		"toml",
		"ini",
		"make",
		"make-report",
		"dart-report",
		"dart-report-class",
		"dart-report-single",
		"css",
		"scss",
		"less",
		"typescript",
		"tsx",
		"javascript",
		"json",
		"haskell",
		"hcl",
		"diff",
		"c",
		"python",
		"cpp",
		"objc",
		"elixir",
		"sql",
		"ruby",
		"perl",
		"sh",
		"ps1",
		"kotlin",
		"xml",
		"vue",
		"svelte",
		"java",
		"graphql",
		"r",
		"php",
		"zig",
		"nix",
		"yaml",
		"swift",
		"rst",
		"lua",
		"proto",
		"dockerfile",
		"md",
		"groovy",
		"templ",
		"csv",
		"html",
	}
}
