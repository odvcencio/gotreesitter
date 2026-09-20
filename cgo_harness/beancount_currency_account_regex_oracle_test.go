//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestBeancountAccountUnicodeFirstLetterCOracle pins
// polarmutex/tree-sitter-beancount@c8a97806's grammar.js account rule fix
// against the real C tree-sitter oracle. The old rule required an ASCII
// "[A-Z]" first letter; the new rule widens it to "\p{Lu}" (any Unicode
// uppercase letter), so an account name starting with a non-ASCII uppercase
// letter now parses cleanly instead of splitting into an error node.
func TestBeancountAccountUnicodeFirstLetterCOracle(t *testing.T) {
	source := []byte("2024-01-01 open Аssets:Bank\n")

	cLanguage, err := ParityCLanguage("beancount")
	if err != nil {
		t.Skipf("C beancount oracle unavailable: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("beancount C oracle returned no root")
	}
	if cRoot.HasError() {
		t.Fatalf("beancount C oracle root has error:\n%s", dumpCTree(cRoot, 0))
	}

	goLanguage := grammars.BeancountLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("beancount Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("beancount Go parse returned no root")
	}
	if goRoot.HasError() {
		t.Fatalf("beancount Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}

	var errs []string
	compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
	if len(errs) != 0 {
		t.Fatalf(
			"beancount Go/C account tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
			joinTopErrors(errs),
			goRoot.SExpr(goLanguage),
			dumpGoTree(goRoot, goLanguage, 0),
			cRoot.ToSexp(),
			dumpCTree(cRoot, 0),
		)
	}
}

// TestBeancountCurrencyBeyondOldLimitCOracle pins
// polarmutex/tree-sitter-beancount@c8a97806's grammar.js currency rule fix
// against the real C tree-sitter oracle. The old rule capped the currency
// token at 24 characters; the new rule drops the cap, so a longer currency
// name now parses as a single currency token instead of splitting into a
// currency token plus a trailing error node.
func TestBeancountCurrencyBeyondOldLimitCOracle(t *testing.T) {
	const currency = "AVERYLONGCURRENCYNAMETHATEXCEEDSOLDLIMIT"
	source := []byte("2024-01-01 * \"Test\"\n  Assets:Bank  10.00 " + currency + "\n  Expenses:Food\n")

	cLanguage, err := ParityCLanguage("beancount")
	if err != nil {
		t.Skipf("C beancount oracle unavailable: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("beancount C oracle returned no root")
	}
	if cRoot.HasError() {
		t.Fatalf("beancount C oracle root has error:\n%s", dumpCTree(cRoot, 0))
	}

	goLanguage := grammars.BeancountLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("beancount Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("beancount Go parse returned no root")
	}
	if goRoot.HasError() {
		t.Fatalf("beancount Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}

	var errs []string
	compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
	if len(errs) != 0 {
		t.Fatalf(
			"beancount Go/C currency tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
			joinTopErrors(errs),
			goRoot.SExpr(goLanguage),
			dumpGoTree(goRoot, goLanguage, 0),
			cRoot.ToSexp(),
			dumpCTree(cRoot, 0),
		)
	}
}
