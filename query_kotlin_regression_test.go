package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestKotlinNestedImportQueryMatchesRepeatedHeaders(t *testing.T) {
	src := []byte("package com.example\n\nimport foo.bar.Baz\nimport foo.qux.*\nfun main() {}\n")
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()
	if tree.RootNode().HasError() {
		t.Fatalf("root has error:\n%s", tree.RootNode().SExpr(lang))
	}

	q, err := gotreesitter.NewQuery(`
		(source_file
			(import_list
				(import_header (identifier) @imp (wildcard_import)? @is_star)
			)
		)
	`, lang)
	if err != nil {
		t.Fatalf("NewQuery failed: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), lang, src)
	var got []string
	var wildcard bool
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, capture := range match.Captures {
			switch capture.Name {
			case "imp":
				got = append(got, capture.Text(src))
			case "is_star":
				wildcard = true
			}
		}
	}

	want := []string{"foo.bar.Baz", "foo.qux"}
	if len(got) != len(want) {
		t.Fatalf("captures = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("captures = %v, want %v", got, want)
		}
	}
	if !wildcard {
		t.Fatal("expected wildcard_import capture")
	}
}

func TestKotlinRecoveredRootPreservesSourceFileQueries(t *testing.T) {
	src := []byte(`package com.example.deepimport

import com.google.common.base.pretend.deep.Thing
import com.google.common.base.pretend.deep.Things as ThingsHelper

class DeepCompare() {}

fun main(vararg args: string) {
    var app = new DeepCompare();
    System.out.println("Success: " + app.compare(Thing(1), Thing(2)));
}
`)
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()

	if got := tree.RootNode().Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file:\n%s", got, tree.RootNode().SExpr(lang))
	}

	q, err := gotreesitter.NewQuery(`
		(source_file
			(import_list
				(import_header (identifier) @imp)
			)
		)

		(source_file
			(function_declaration
				"fun" @fn_keyword
				(simple_identifier) @fn
				(#eq? @fn "main")
			)
		)
	`, lang)
	if err != nil {
		t.Fatalf("NewQuery failed: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), lang, src)
	var imports []string
	var hasMain bool
	var hasFunKeyword bool
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, capture := range match.Captures {
			switch capture.Name {
			case "imp":
				imports = append(imports, capture.Text(src))
			case "fn":
				hasMain = true
			case "fn_keyword":
				hasFunKeyword = capture.Text(src) == "fun"
			}
		}
	}

	wantImports := []string{
		"com.google.common.base.pretend.deep.Thing",
		"com.google.common.base.pretend.deep.Things",
	}
	if len(imports) != len(wantImports) {
		t.Fatalf("imports = %v, want %v", imports, wantImports)
	}
	for i := range wantImports {
		if imports[i] != wantImports[i] {
			t.Fatalf("imports = %v, want %v", imports, wantImports)
		}
	}
	if !hasMain {
		t.Fatal("expected recovered main function to match source_file-rooted query")
	}
	if !hasFunKeyword {
		t.Fatal("expected recovered function declaration to expose fun keyword")
	}
}
