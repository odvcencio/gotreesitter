package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestOutlineOwnerRulesResolvesGoReceiverThroughAccessor proves the shipped
// Go row is not just present in the table: composed through the documented
// call shape --
//
//	gotreesitter.NewOutliner(lang, query,
//	    gotreesitter.WithOutlineOwnerRules(grammars.OutlineOwnerRules(entry)))
//
// -- it resolves a real method's receiver on a real parse.
func TestOutlineOwnerRulesResolvesGoReceiverThroughAccessor(t *testing.T) {
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Fatal("the go grammar is not registered")
	}
	lang := entry.Language()
	query := grammars.ResolveTagsQuery(*entry)

	rules := grammars.OutlineOwnerRules(*entry)
	if len(rules) == 0 {
		t.Fatal("OutlineOwnerRules(go) returned no rows; the shipped receiver row did not survive gating")
	}

	const src = `package demo

type Registry struct{}

func (r *Registry) Add(name string) {}
`
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	outliner, err := gotreesitter.NewOutliner(lang, query,
		gotreesitter.WithOutlineOwnerRules(rules))
	if err != nil {
		t.Fatalf("build outliner: %v", err)
	}
	symbols, report := outliner.OutlineTree(tree)
	if report.OwnerRuleMisses != 0 {
		t.Errorf("OwnerRuleMisses = %d, want 0", report.OwnerRuleMisses)
	}

	found := false
	for _, symbol := range symbols {
		if symbol.Kind != "method" {
			continue
		}
		found = true
		if symbol.Owner != "Registry" {
			t.Errorf("%s: Owner = %q, want %q", symbol.Name, symbol.Owner, "Registry")
		}
	}
	if !found {
		t.Fatal("the snippet produced no method symbol, so the accessor proof is vacuous")
	}
}

// TestOutlineOwnerRulesGatesOnFieldPresence proves the table is gated by
// symbol and field presence, not shipped unconditionally: a language whose
// grammar defines "method_declaration" but no "receiver" field -- java, in
// this fleet -- must never receive the Go row.
func TestOutlineOwnerRulesGatesOnFieldPresence(t *testing.T) {
	entry := grammars.DetectLanguageByName("java")
	if entry == nil {
		t.Fatal("the java grammar is not registered")
	}
	lang := entry.Language()
	if _, ok := lang.SymbolByName("method_declaration"); !ok {
		t.Fatal("java's grammar no longer defines method_declaration; this gating proof needs a different witness")
	}
	if _, ok := lang.FieldByName("receiver"); ok {
		t.Fatal("java's grammar now defines a receiver field; this gating proof needs a different witness")
	}

	rules := grammars.OutlineOwnerRules(*entry)
	for _, rule := range rules {
		if rule.NodeType == "method_declaration" {
			t.Errorf("OutlineOwnerRules(java) returned a method_declaration row despite no receiver field: %+v", rule)
		}
	}
}

// TestOutlineOwnerRulesEmptyForUnknownLanguage proves the accessor fails
// closed on a LangEntry with no name and no owner-rule row, rather than
// panicking or loading a grammar it was never asked about.
func TestOutlineOwnerRulesEmptyForUnknownLanguage(t *testing.T) {
	if got := grammars.OutlineOwnerRules(grammars.LangEntry{}); got != nil {
		t.Errorf("OutlineOwnerRules(zero value) = %v, want nil", got)
	}
}
