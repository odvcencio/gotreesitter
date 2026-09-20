//go:build !grammar_subset || grammar_subset_beancount

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestBeancountAccountAcceptsNonASCIIUppercaseFirstLetter pins
// polarmutex/tree-sitter-beancount@c8a97806's grammar.js account rule fix.
// The old rule required an ASCII "[A-Z]" first letter ("\p{L}\p{N}..." was
// only allowed after that first letter), so an account name starting with a
// non-ASCII uppercase letter (for example Cyrillic "А") produced an error
// node. The new rule widens the first letter to "\p{Lu}" (any Unicode
// uppercase letter), matching every letter class the rest of the token
// already accepted.
func TestBeancountAccountAcceptsNonASCIIUppercaseFirstLetter(t *testing.T) {
	language := BeancountLanguage()
	source := []byte("2024-01-01 open Аssets:Bank\n")
	tree, err := gotreesitter.NewParser(language).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("root = %v, want a clean tree: %s", root, root.SExpr(language))
	}
	account := findFirstNamedDescendantWhere(root, language, "account", func(*gotreesitter.Node) bool { return true })
	if account == nil {
		t.Fatalf("no account node found: %s", root.SExpr(language))
	}
	if got, want := string(source[account.StartByte():account.EndByte()]), "Аssets:Bank"; got != want {
		t.Fatalf("account text = %q, want %q", got, want)
	}
}

// TestBeancountCurrencyAcceptsNameLongerThanOldLimit pins
// polarmutex/tree-sitter-beancount@c8a97806's grammar.js currency rule fix.
// The old rule capped the currency token at 24 characters
// ("[A-Z]([A-Z0-9'._-]{0,22}[A-Z0-9])?"); the new rule drops the cap
// ("[A-Z][A-Z0-9'._-]*[A-Z0-9]?"), so a longer currency name now parses as a
// single currency token instead of splitting into a currency token plus a
// trailing error node.
func TestBeancountCurrencyAcceptsNameLongerThanOldLimit(t *testing.T) {
	language := BeancountLanguage()
	const currency = "AVERYLONGCURRENCYNAMETHATEXCEEDSOLDLIMIT"
	source := []byte("2024-01-01 * \"Test\"\n  Assets:Bank  10.00 " + currency + "\n  Expenses:Food\n")
	tree, err := gotreesitter.NewParser(language).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("root = %v, want a clean tree: %s", root, root.SExpr(language))
	}
	node := findFirstNamedDescendantWhere(root, language, "currency", func(*gotreesitter.Node) bool { return true })
	if node == nil {
		t.Fatalf("no currency node found: %s", root.SExpr(language))
	}
	if got := string(source[node.StartByte():node.EndByte()]); got != currency {
		t.Fatalf("currency text = %q, want %q", got, currency)
	}
}
