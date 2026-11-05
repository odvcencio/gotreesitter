package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestPowerShellNativeCommandArgumentsParseCleanly(t *testing.T) {
	lang := PowershellLanguage()

	cases := []struct {
		name string
		src  string
	}{
		{"comma list", "Import-Module A,B\nImport-Module After\n"},
		{"comma list with spaces", "Select-Object Id, Name\n"},
		{"named parameter comma list", "Import-Module -Name A, B -Force\n"},
		{"requires modules", "#Requires -Modules A,B\nImport-Module After\n"},
		{"bare double dash", "git log -- path\n"},
		{"bare dash", "tar -xf -\n"},
		{"option with attached value", "git log --pretty=format:%H\n"},
		{"trailing background operator", "npm run dev &\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := ts.NewParser(lang).Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatal("missing root node")
			}
			if root.HasError() {
				t.Fatalf("expected %q to parse cleanly, got %s", tc.src, root.SExpr(lang))
			}
		})
	}
}
