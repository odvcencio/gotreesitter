package parse_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestExtractImportsGo(t *testing.T) {
	source := []byte(`package main

import (
	alias "example.com/aliased"
	_ "example.com/sideeffect"
	. "example.com/dot"
	"example.com/plain"
)
`)
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	refs := gotreesitter.ExtractImports(tree)
	if got, want := len(refs), 5; got != want {
		t.Fatalf("ExtractImports len = %d, want %d: %#v", got, want, refs)
	}
	if refs[0].Kind != "package" || refs[0].Name != "main" {
		t.Fatalf("package ref = %#v, want main package", refs[0])
	}
	assertImportRef(t, refs[1], "go", "import", "example.com/aliased", "aliased", "alias")
	assertImportRef(t, refs[2], "go", "import", "example.com/sideeffect", "sideeffect", "_")
	assertImportRef(t, refs[3], "go", "import", "example.com/dot", "dot", ".")
	assertImportRef(t, refs[4], "go", "import", "example.com/plain", "plain", "")

	sourceRefs := gotreesitter.ExtractImportsFromSource(lang, source)
	assertImportRefsEqualShape(t, sourceRefs, refs)
}

func TestExtractImportsJava(t *testing.T) {
	source := []byte(`package example.app;

import java.util.List;
import static java.util.Collections.*;
`)
	lang := grammars.JavaLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	refs := gotreesitter.ExtractImports(tree)
	if got, want := len(refs), 3; got != want {
		t.Fatalf("ExtractImports len = %d, want %d: %#v", got, want, refs)
	}
	if refs[0].Kind != "package" || refs[0].Path != "example.app" {
		t.Fatalf("package ref = %#v, want example.app package", refs[0])
	}
	assertImportRef(t, refs[1], "java", "import", "java.util.List", "List", "")
	assertImportRef(t, refs[2], "java", "import", "java.util.Collections", "Collections", "")
	if !refs[2].Static || !refs[2].Wildcard {
		t.Fatalf("static wildcard ref = %#v, want static wildcard", refs[2])
	}

	sourceRefs := gotreesitter.ExtractImportsFromSource(lang, source)
	assertImportRefsEqualShape(t, sourceRefs, refs)
}

func TestExtractImportsPython(t *testing.T) {
	source := []byte(`import os, sys as system
from ..pkg.sub import name as alias, other
from pkg import *
`)
	lang := grammars.PythonLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	refs := gotreesitter.ExtractImports(tree)
	if got, want := len(refs), 5; got != want {
		t.Fatalf("ExtractImports len = %d, want %d: %#v", got, want, refs)
	}
	assertImportRef(t, refs[0], "python", "import", "os", "os", "")
	assertImportRef(t, refs[1], "python", "import", "sys", "sys", "system")
	assertImportRef(t, refs[2], "python", "from_import", "pkg.sub.name", "name", "alias")
	if refs[2].From != "pkg.sub" || refs[2].Relative != 2 {
		t.Fatalf("from ref = %#v, want relative pkg.sub", refs[2])
	}
	assertImportRef(t, refs[3], "python", "from_import", "pkg.sub.other", "other", "")
	assertImportRef(t, refs[4], "python", "from_import", "pkg", "*", "")
	if !refs[4].Wildcard {
		t.Fatalf("wildcard ref = %#v, want wildcard", refs[4])
	}

	sourceRefs := gotreesitter.ExtractImportsFromSource(lang, source)
	assertImportRefsEqualShape(t, sourceRefs, refs)
}

func TestExtractImportsStarlark(t *testing.T) {
	source := []byte(`load("@rules_python//python:defs.bzl", "py_library", py_binary_alias = "py_binary")

py_library(name = "lib")
`)
	lang := grammars.StarlarkLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	refs := gotreesitter.ExtractImports(tree)
	if got, want := len(refs), 2; got != want {
		t.Fatalf("ExtractImports len = %d, want %d: %#v", got, want, refs)
	}
	assertImportRef(t, refs[0], "starlark", "load", "@rules_python//python:defs.bzl:py_library", "py_library", "")
	if refs[0].From != "@rules_python//python:defs.bzl" {
		t.Fatalf("load ref = %#v, want module in From", refs[0])
	}
	assertImportRef(t, refs[1], "starlark", "load", "@rules_python//python:defs.bzl:py_binary", "py_binary", "py_binary_alias")

	sourceRefs := gotreesitter.ExtractImportsFromSource(lang, source)
	assertImportRefsEqualShape(t, sourceRefs, refs)
}

func TestExtractImportsSourceParityFixtures(t *testing.T) {
	cases := []struct {
		name   string
		lang   *gotreesitter.Language
		source string
		want   []gotreesitter.ImportRef
	}{
		{
			name: "go_aliases_and_comments",
			lang: grammars.GoLanguage(),
			source: `package main

// import "fake/comment"
import (
	alias "example.com/aliased"
	_ "example.com/sideeffect"
	. "example.com/dot"
	"example.com/plain"
)

const s = "import \"fake/string\""
`,
			want: []gotreesitter.ImportRef{
				{Lang: "go", Kind: "package", Name: "main"},
				{Lang: "go", Kind: "import", Path: "example.com/aliased", Name: "aliased", Alias: "alias"},
				{Lang: "go", Kind: "import", Path: "example.com/sideeffect", Name: "sideeffect", Alias: "_"},
				{Lang: "go", Kind: "import", Path: "example.com/dot", Name: "dot", Alias: "."},
				{Lang: "go", Kind: "import", Path: "example.com/plain", Name: "plain"},
			},
		},
		{
			name: "java_comments_strings_static_wildcard",
			lang: grammars.JavaLanguage(),
			source: `package example.app;

// import fake.Comment;
import java.util.List;
import static java.util.Collections.*;

class A {
  String s = "import fake.String;";
}
`,
			want: []gotreesitter.ImportRef{
				{Lang: "java", Kind: "package", Path: "example.app", Name: "app"},
				{Lang: "java", Kind: "import", Path: "java.util.List", Name: "List"},
				{Lang: "java", Kind: "import", Path: "java.util.Collections", Name: "Collections", Static: true, Wildcard: true},
			},
		},
		{
			name: "python_multiline_relative_and_comments",
			lang: grammars.PythonLanguage(),
			source: `# import fake_comment
text = "from fake import string"
import os, sys as system
from . import local
from ..pkg import (
    thing,
    other as alias,
)
from pkg import *
`,
			want: []gotreesitter.ImportRef{
				{Lang: "python", Kind: "import", Path: "os", Name: "os"},
				{Lang: "python", Kind: "import", Path: "sys", Name: "sys", Alias: "system"},
				{Lang: "python", Kind: "from_import", Path: "local", Name: "local", Relative: 1},
				{Lang: "python", Kind: "from_import", Path: "pkg.thing", From: "pkg", Name: "thing", Relative: 2},
				{Lang: "python", Kind: "from_import", Path: "pkg.other", From: "pkg", Name: "other", Alias: "alias", Relative: 2},
				{Lang: "python", Kind: "from_import", Path: "pkg", From: "pkg", Name: "*", Wildcard: true},
			},
		},
		{
			name: "python_import_after_preamble_comments",
			lang: grammars.PythonLanguage(),
			source: `# Copyright 2026 Example
#
# from fake import comment
from foo.bar import baz
`,
			want: []gotreesitter.ImportRef{
				{Lang: "python", Kind: "from_import", Path: "foo.bar.baz", From: "foo.bar", Name: "baz"},
			},
		},
		{
			name: "python_future_import_and_triple_string",
			lang: grammars.PythonLanguage(),
			source: `"""module docstring"""
from __future__ import annotations
template = """
import fake_template
from fake import template
"""
from real.pkg import thing
`,
			want: []gotreesitter.ImportRef{
				{Lang: "python", Kind: "from_import", Path: "__future__.annotations", From: "__future__", Name: "annotations"},
				{Lang: "python", Kind: "from_import", Path: "real.pkg.thing", From: "real.pkg", Name: "thing"},
			},
		},
		{
			name: "starlark_skip_comments_and_strings",
			lang: grammars.StarlarkLanguage(),
			source: `# load("//fake:comment.bzl", "x")
s = 'load("//fake:string.bzl", "y")'
load(
    '@repo//pkg:file.bzl',
    'sym',
    alias = 'other',
)
`,
			want: []gotreesitter.ImportRef{
				{Lang: "starlark", Kind: "load", Path: "@repo//pkg:file.bzl:sym", From: "@repo//pkg:file.bzl", Name: "sym"},
				{Lang: "starlark", Kind: "load", Path: "@repo//pkg:file.bzl:other", From: "@repo//pkg:file.bzl", Name: "other", Alias: "alias"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			treeRefs := extractImportsFromTree(t, tc.lang, source)
			sourceReport := gotreesitter.ExtractImportsFromSourceWithReport(tc.lang, source)
			assertImportExtractOK(t, sourceReport)
			assertImportRefsEqualShape(t, sourceReport.Imports, treeRefs)
			assertImportRefsEqualShape(t, treeRefs, tc.want)
		})
	}
}

func TestExtractImportsFromSourceReportsFallback(t *testing.T) {
	cases := []struct {
		name       string
		lang       *gotreesitter.Language
		source     string
		status     gotreesitter.ImportExtractStatus
		reason     string
		wantImport int
	}{
		{
			name:   "python_malformed_multiline",
			lang:   grammars.PythonLanguage(),
			source: "from pkg import (\n    a\n",
			status: gotreesitter.ImportExtractScannerError,
			reason: "malformed_python_import",
		},
		{
			name:   "starlark_malformed_load",
			lang:   grammars.StarlarkLanguage(),
			source: `load("//pkg:file.bzl", "x"`,
			status: gotreesitter.ImportExtractScannerError,
			reason: "malformed_starlark_load",
		},
		{
			name:   "starlark_nonliteral_module",
			lang:   grammars.StarlarkLanguage(),
			source: `load(label, "x")`,
			status: gotreesitter.ImportExtractUnsupportedConstruct,
			reason: "non_literal_starlark_load_module",
		},
		{
			name:   "starlark_nonliteral_symbol",
			lang:   grammars.StarlarkLanguage(),
			source: `load("//pkg:file.bzl", sym)`,
			status: gotreesitter.ImportExtractUnsupportedConstruct,
			reason: "non_literal_starlark_load_symbol",
		},
		{
			name:   "unsupported_language",
			lang:   &gotreesitter.Language{Name: "ruby"},
			source: `require "x"`,
			status: gotreesitter.ImportExtractUnsupportedConstruct,
			reason: "unsupported_language",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := gotreesitter.ExtractImportsFromSourceWithReport(tc.lang, []byte(tc.source))
			if report.Status != tc.status || report.Reason != tc.reason || !report.FallbackRecommended {
				t.Fatalf("report = %#v, want status=%q reason=%q fallback=true", report, tc.status, tc.reason)
			}
			if len(report.Imports) != tc.wantImport {
				t.Fatalf("imports len = %d, want %d: %#v", len(report.Imports), tc.wantImport, report.Imports)
			}
		})
	}
}

func extractImportsFromTree(t *testing.T, lang *gotreesitter.Language, source []byte) []gotreesitter.ImportRef {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	return gotreesitter.ExtractImports(tree)
}

func assertImportExtractOK(t *testing.T, report gotreesitter.ImportExtractResult) {
	t.Helper()
	if report.Status != gotreesitter.ImportExtractOK || report.FallbackRecommended {
		t.Fatalf("source import report = %#v, want ok without fallback", report)
	}
}

func assertImportRef(t *testing.T, ref gotreesitter.ImportRef, lang, kind, path, name, alias string) {
	t.Helper()
	if ref.Lang != lang || ref.Kind != kind || ref.Path != path || ref.Name != name || ref.Alias != alias {
		t.Fatalf("ref = %#v, want lang=%s kind=%s path=%s name=%s alias=%s", ref, lang, kind, path, name, alias)
	}
}

func assertImportRefsEqualShape(t *testing.T, got, want []gotreesitter.ImportRef) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("source refs len = %d, want %d: got=%#v want=%#v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i].Lang != want[i].Lang ||
			got[i].Kind != want[i].Kind ||
			got[i].Path != want[i].Path ||
			got[i].From != want[i].From ||
			got[i].Name != want[i].Name ||
			got[i].Alias != want[i].Alias ||
			got[i].Static != want[i].Static ||
			got[i].Wildcard != want[i].Wildcard ||
			got[i].Relative != want[i].Relative {
			t.Fatalf("source ref[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
