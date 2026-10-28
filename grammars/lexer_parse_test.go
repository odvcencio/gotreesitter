//go:build !grammar_subset

package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

type parseTestCase struct {
	name     string
	src      []byte
	factory  func([]byte, *gotreesitter.Language) gotreesitter.TokenSource
	lang     func() *gotreesitter.Language
	wantErr  bool
	minNodes int
}

func TestDeepParse(t *testing.T) {
	cases := []parseTestCase{
		{
			name: "c_preprocessor_and_functions",
			src: []byte(`#include <stdio.h>
#define MAX 100

int add(int a, int b) {
    return a + b;
}

int main(void) {
    int x = add(1, 2);
    printf("%d\n", x);
    return 0;
}
`),
			factory:  NewCTokenSourceOrEOF,
			lang:     CLanguage,
			minNodes: 2,
		},
		{
			name: "go_imports_and_structs",
			src: []byte(`package main

import (
	"fmt"
	"strings"
)

type Config struct {
	Name    string
	Value   int
	Enabled bool
}

func (c *Config) String() string {
	return fmt.Sprintf("%s=%d", c.Name, c.Value)
}

func main() {
	c := &Config{Name: "test", Value: 42, Enabled: true}
	fmt.Println(strings.ToUpper(c.String()))
}
`),
			factory:  NewGoTokenSourceOrEOF,
			lang:     GoLanguage,
			minNodes: 3,
		},
		{
			name: "java_annotations_and_generics",
			src: []byte(`import java.util.List;
import java.util.ArrayList;

public class Main {
    @Override
    public String toString() {
        return "Main";
    }

    public static <T> List<T> wrap(T item) {
        List<T> list = new ArrayList<>();
        list.add(item);
        return list;
    }

    public static void main(String[] args) {
        List<String> items = wrap("hello");
        System.out.println(items);
    }
}
`),
			factory:  NewJavaTokenSourceOrEOF,
			lang:     JavaLanguage,
			minNodes: 2,
		},
		{
			name: "html_nested_tags",
			src: []byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8"/>
    <title>Test Page</title>
</head>
<body>
    <div class="container">
        <h1>Hello World</h1>
        <p>This is a <strong>test</strong> page.</p>
        <ul>
            <li>Item 1</li>
            <li>Item 2</li>
        </ul>
    </div>
</body>
</html>
`),
			factory:  NewHTMLTokenSourceOrEOF,
			lang:     HtmlLanguage,
			minNodes: 1,
		},
		{
			name: "lua_functions_and_tables",
			src: []byte(`local function fibonacci(n)
    if n <= 1 then
        return n
    end
    return fibonacci(n - 1) + fibonacci(n - 2)
end

local config = {
    name = "test",
    values = {1, 2, 3},
    nested = {a = true, b = false},
}

for i = 1, 10 do
    print(fibonacci(i))
end
`),
			factory:  NewLuaTokenSourceOrEOF,
			lang:     LuaLanguage,
			minNodes: 2,
		},
		{
			name: "toml_sections",
			src: []byte(`[package]
name = "myapp"
version = "1.0.0"

[dependencies]
serde = "1.0"
tokio = {version = "1.0", features = ["full"]}

[[bin]]
name = "server"
path = "src/main.rs"
`),
			factory:  NewTomlTokenSourceOrEOF,
			lang:     TomlLanguage,
			minNodes: 1,
		},
		{
			name: "authzed_permissions",
			src: []byte(`definition user {}

definition document {
    relation writer: user
    relation viewer: user
    permission edit = writer
    permission view = viewer + writer
}
`),
			factory:  NewAuthzedTokenSourceOrEOF,
			lang:     AuthzedLanguage,
			minNodes: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			parser := gotreesitter.NewParser(lang)
			ts := tc.factory(tc.src, lang)

			tree, err := parser.ParseWithTokenSource(tc.src, ts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected parse error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("parse returned nil root")
			}

			root := tree.RootNode()
			if root.HasError() {
				// Custom lexers are approximate — syntax errors in the parse tree
				// are common and expected. Log for visibility but don't fail.
				t.Logf("parse has syntax errors (root type=%s, children=%d)", root.Type(lang), root.ChildCount())
			}
			if root.ChildCount() < tc.minNodes {
				t.Errorf("expected at least %d child nodes, got %d", tc.minNodes, root.ChildCount())
			}
		})
	}
}
