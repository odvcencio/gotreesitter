package grammargen

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestGapDiagnostic prints detailed divergence info for all parity gaps.
func TestGapDiagnostic(t *testing.T) {
	type entry struct {
		name     string
		blobFunc func() *gotreesitter.Language
		samples  []string
		hasExt   bool // true if grammar has external tokens
	}

	entries := []entry{
		{"ini", grammars.IniLanguage, iniSamples(), false},
		{"requirements", grammars.RequirementsLanguage, requirementsSamples(), false},
		{"jsdoc", grammars.JsdocLanguage, jsdocSamples(), true},
		{"html", grammars.HtmlLanguage, htmlSamples(), true},
		{"scala", grammars.ScalaLanguage, scalaSamples(), true},
		{"diff", grammars.DiffLanguage, diffSamples(), false},
		{"gitcommit", grammars.GitcommitLanguage, gitcommitSamples(), false},
		{"ron", grammars.RonLanguage, ronSamples(), true},
		{"proto", grammars.ProtoLanguage, protoSamples(), false},
		{"dockerfile", grammars.DockerfileLanguage, dockerfileSamples(), true},
		{"nix", grammars.NixLanguage, nixSamples(), true},
		{"jq", grammars.JqLanguage, jqSamples(), false},
		{"hcl", grammars.HclLanguage, hclSamples(), true},
		{"eex", grammars.EexLanguage, eexSamples(), true},
		{"gomod", grammars.GomodLanguage, gomodSamples(), false},
		{"corn", grammars.CornLanguage, cornSamples(), false},
		{"make", grammars.MakeLanguage, makeSamples(), false},
		{"ssh_config", grammars.SshConfigLanguage, sshConfigSamples(), false},
		{"c_lang", grammars.CLanguage, cSamples(), false},
		{"sql", grammars.SqlLanguage, sqlSamples(), true},
	}

	categories := map[string]int{}
	totalGaps := 0

	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			// Some grammars have different directory names
			dirName := e.name
			switch dirName {
			case "c_lang":
				dirName = "c"
			case "go_lang":
				dirName = "go"
			}
			jsonPath := fmt.Sprintf("/tmp/grammar_parity/%s/src/grammar.json", dirName)
			data, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Skipf("no grammar.json")
				return
			}
			g, err := ImportGrammarJSON(data)
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			blob, err := Generate(g)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			genLang, err := decodeLanguageBlob(blob)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			refLang := e.blobFunc()
			genParser := gotreesitter.NewParser(genLang)
			refParser := gotreesitter.NewParser(refLang)

			for i, sample := range e.samples {
				genTree, _ := genParser.Parse([]byte(sample))
				refTree, _ := refParser.Parse([]byte(sample))
				genSexp := genTree.RootNode().SExpr(genLang)
				refSexp := refTree.RootNode().SExpr(refLang)
				if genSexp == refSexp {
					continue
				}
				totalGaps++
				cat := classifyGapDetailed(genSexp, refSexp, e.hasExt)
				categories[cat]++
				truncInput := sample
				if len(truncInput) > 70 {
					truncInput = truncInput[:70] + "..."
				}
				t.Logf("[%d] %s input=%q", i, cat, truncInput)
				t.Logf("  REF: %.200s", refSexp)
				t.Logf("  GEN: %.200s", genSexp)
			}
		})
	}

	t.Logf("\n=== GAP SUMMARY ===")
	t.Logf("Total: %d gaps", totalGaps)
	for cat, count := range categories {
		t.Logf("  %s: %d", cat, count)
	}
}

func classifyGapDetailed(genSexp, refSexp string, hasExt bool) string {
	genRoot := extractRoot(genSexp)
	refRoot := extractRoot(refSexp)

	if hasExt && genRoot != refRoot {
		return "EXT_ROOT"
	}
	if hasExt {
		return "EXT_STRUCTURAL"
	}
	if strings.Contains(genSexp, "ERROR") || strings.Contains(genSexp, "MISSING") {
		return "PARSE_ERROR"
	}
	if genRoot != refRoot {
		return fmt.Sprintf("ROOT(%s→%s)", refRoot, genRoot)
	}
	return "STRUCTURAL"
}

func extractRoot(sexp string) string {
	if len(sexp) < 2 || sexp[0] != '(' {
		return sexp
	}
	end := strings.IndexAny(sexp[1:], " )")
	if end < 0 {
		return sexp[1:]
	}
	return sexp[1 : 1+end]
}

// Sample sets — pulled from parity_test.go expectations.
func iniSamples() []string {
	return []string{"key=value\n", "[section]\nkey=value\n", "; comment\nkey=value\n",
		"[section]\n; comment\nkey = value\nkey2 = value2\n"}
}
func requirementsSamples() []string {
	return []string{"flask", "flask>=2.0", "flask>=2.0,<3.0"}
}
func jsdocSamples() []string {
	return []string{"/** @param {string} name */", "/** @returns {number} */"}
}
func htmlSamples() []string {
	return []string{"<div></div>", "<div class=\"foo\">text</div>"}
}
func scalaSamples() []string {
	return []string{"object Main { def main(args: Array[String]): Unit = {} }",
		"val x = 1"}
}
func diffSamples() []string {
	return []string{
		"--- a/file.txt\n+++ b/file.txt\n@@ -1,3 +1,3 @@\n-old\n+new\n context",
		"diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b",
		"--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n-line1\n-line2\n+new1\n+new2",
		"--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+new file",
	}
}
func gitcommitSamples() []string {
	return []string{"Initial commit", "fix: bug in parser\n\nMore details here",
		"feat: add feature\n\nSigned-off-by: Test <test@test.com>",
		"chore: cleanup\n\n# This is a comment"}
}
func ronSamples() []string {
	return []string{"()", "(a: 1)", "(a: 1, b: 2)", "[]", "[1, 2, 3]",
		`"hello"`, "42", "3.14", "true", "false",
		"Foo(a: 1)", "Some(42)", "None", "#{}", `#{"a": 1}`}
}
func protoSamples() []string {
	return []string{
		`syntax = "proto3";`, `message Foo {}`,
		`message Foo { string name = 1; }`,
		`enum Bar { UNKNOWN = 0; KNOWN = 1; }`,
		`service MyService { rpc Get(Req) returns (Resp); }`,
		`package my.package;`, `import "other.proto";`,
		`syntax = "proto3"; message Foo { string name = 1; int32 id = 2; }`,
		`syntax = "proto3"; enum Color { RED = 0; GREEN = 1; BLUE = 2; }`,
		`message Foo { repeated string tags = 1; }`,
		`message Foo { map<string, int32> attrs = 1; }`,
		`message Foo { oneof val { string s = 1; int32 i = 2; } }`,
		`syntax = "proto3"; import "google/protobuf/timestamp.proto";`,
		`option java_package = "com.example";`,
		`message Foo { option deprecated = true; string name = 1; }`,
		`syntax = "proto3"; package test; message Empty {}`,
		`extend Foo { string extra = 100; }`,
	}
}
func dockerfileSamples() []string {
	return []string{
		"FROM ubuntu", "FROM ubuntu:20.04", "RUN apt-get update",
		"COPY . /app", "WORKDIR /app", "EXPOSE 8080",
		"CMD [\"python\", \"app.py\"]", "ENV FOO=bar",
		"FROM ubuntu\nRUN apt-get update\nCOPY . /app",
		"ARG VERSION=latest\nFROM ubuntu:${VERSION}",
	}
}
func nixSamples() []string {
	return []string{
		"42", `"hello"`, "true", "false", "null",
		"{ a = 1; }", "{ a = 1; b = 2; }", "let x = 1; in x",
		"x: x + 1", "{ a, b }: a + b",
		`import ./foo.nix`, "[ 1 2 3 ]", "a.b.c",
		"if true then 1 else 2", "with pkgs; [ foo bar ]",
	}
}
func jqSamples() []string {
	return []string{
		".", ".foo", ".foo.bar", ".[]", ".[0]",
		"select(.age > 21)", "map(.name)", "{name: .first, age: .years}",
		"[.[] | select(. > 2)]", "if .foo then .bar else .baz end",
		"def double: . * 2; [1,2,3] | map(double)",
		".foo // \"default\"", "(.foo | length) > 0",
		"[range(5)]", "null",
	}
}
func hclSamples() []string {
	return []string{
		`x = 1`, `x = "hello"`, `x = true`,
		"resource \"aws_instance\" \"web\" {\n  ami = \"abc\"\n}",
		`x = [1, 2, 3]`, `x = { a = 1 }`,
		"block {\n  x = 1\n}", "block \"label\" {\n  x = 1\n}",
		`x = var.foo`, `x = 1 + 2`,
	}
}
func eexSamples() []string {
	return []string{"hello", "<%= expr %>", "<% code %>", "<%# comment %>",
		"hello <%= name %> world"}
}
func gomodSamples() []string {
	return []string{
		"module example.com/foo\n\ngo 1.21\n",
		"module example.com/foo\n",
		"module example.com/foo\n\nrequire (\n\tgolang.org/x/text v0.3.0\n)\n",
		"module example.com/foo\n\ngo 1.21\n\nrequire golang.org/x/text v0.3.0\n",
		"module example.com/foo\n\nreplace golang.org/x/text => ./text\n",
	}
}
func cornSamples() []string {
	return []string{
		"{ foo = 42 }", `{ foo = "bar" }`, "{ foo = true }",
		"{ foo = [1, 2, 3] }", "{ foo = { bar = 42 } }",
	}
}
func makeSamples() []string {
	return []string{
		"all:\n\techo hello", "CC=gcc", "all: main.o\n\t$(CC) -o main main.o",
		".PHONY: clean\nclean:\n\trm -f *.o", "include config.mk",
	}
}
func sshConfigSamples() []string {
	return []string{
		"Host example\n  HostName example.com\n", "Host *\n  User root\n",
		"Host example\n  HostName example.com\n  Port 22\n  User admin\n",
		"Match host example\n  User root\n",
	}
}
func cSamples() []string {
	return []string{
		"int main() { return 0; }",
		"int x = 42;",
		"struct Foo { int x; };",
		"#include <stdio.h>\nint main() { printf(\"hello\"); return 0; }",
		"typedef int myint;",
	}
}
func sqlSamples() []string {
	return []string{
		"SELECT * FROM users;",
		"INSERT INTO users (name) VALUES ('test');",
		"CREATE TABLE t (id INT PRIMARY KEY);",
		"SELECT a, b FROM t WHERE a > 1 ORDER BY b;",
	}
}
