package grammargen

import (
	"slices"
	"testing"
)

func TestApplyImportGrammarShapeHintsPowerShellBinaryRepeat(t *testing.T) {
	for _, name := range []string{"d", "objc", "perl", "powershell"} {
		t.Run(name, func(t *testing.T) {
			g := NewGrammar(name)
			applyImportGrammarShapeHints(g)
			if !g.BinaryRepeatMode {
				t.Fatalf("%s import should use binary repeat mode", name)
			}
		})
	}
}

func TestApplyImportGrammarShapeHintsElixirPreciseExternalLexStates(t *testing.T) {
	g := NewGrammar("elixir")
	applyImportGrammarShapeHints(g)
	if !g.PreferPreciseExternalLexStates {
		t.Fatalf("elixir import should prefer precise external lex states")
	}
	if !g.PreferRemoteCallOperatorReduces {
		t.Fatalf("elixir import should prefer remote-call operator reduces")
	}
	if !slices.Equal(g.PreserveHiddenChoicePassthrough, []string{"_capture_expression"}) {
		t.Fatalf("elixir preserved hidden passthrough = %v, want [_capture_expression]", g.PreserveHiddenChoicePassthrough)
	}
}

func TestApplyImportGrammarPostShapeHintsPerlHeredocContent(t *testing.T) {
	g := NewGrammar("perl")
	g.Define("heredoc_content", Seq(
		Sym("_heredoc_start"),
		Repeat(Choice(
			Sym("_heredoc_middle"),
			Sym("escape_sequence"),
			Sym("_interpolations"),
			Sym("_interpolation_fallbacks"),
		)),
		Sym("heredoc_end"),
	))

	applyImportGrammarPostShapeHints(g)

	rule := g.Rules["heredoc_content"]
	if rule == nil || rule.Kind != RuleSeq || len(rule.Children) != 3 {
		t.Fatalf("heredoc_content rule = %#v, want compact seq", rule)
	}
	repeat := rule.Children[1]
	if repeat == nil || repeat.Kind != RuleRepeat || len(repeat.Children) != 1 {
		t.Fatalf("middle rule = %#v, want repeat", repeat)
	}
	choice := repeat.Children[0]
	if choice == nil || choice.Kind != RuleChoice || len(choice.Children) != 2 {
		t.Fatalf("repeat content = %#v, want compact two-way choice", choice)
	}
	if got := []string{choice.Children[0].Value, choice.Children[1].Value}; got[0] != "_heredoc_middle" || got[1] != "escape_sequence" {
		t.Fatalf("compact heredoc alternatives = %v, want [_heredoc_middle escape_sequence]", got)
	}
}

// TestApplyImportGrammarPostShapeHintsRubyHeredocBody mirrors
// TestApplyImportGrammarPostShapeHintsPerlHeredocContent for ruby's
// heredoc_body, which has the identical pathology: a nonterminal grammar
// extra whose REPEAT(CHOICE(...)) body includes a visible `interpolation`
// alternative that re-enters `_statements` (the entire statement grammar).
// See GEN_COST_RCA (wave7, "ruby - memory in add_nonterminal_extra_chains")
// and the perl precedent this rewrite is modeled on.
func TestApplyImportGrammarPostShapeHintsRubyHeredocBody(t *testing.T) {
	g := NewGrammar("ruby")
	g.Define("heredoc_body", Seq(
		Sym("_heredoc_body_start"),
		Repeat(Choice(
			Sym("heredoc_content"),
			Sym("interpolation"),
			Sym("escape_sequence"),
		)),
		Sym("heredoc_end"),
	))

	applyImportGrammarPostShapeHints(g)

	rule := g.Rules["heredoc_body"]
	if rule == nil || rule.Kind != RuleSeq || len(rule.Children) != 3 {
		t.Fatalf("heredoc_body rule = %#v, want compact seq", rule)
	}
	if rule.Children[0].Value != "_heredoc_body_start" {
		t.Fatalf("heredoc_body start = %#v, want _heredoc_body_start", rule.Children[0])
	}
	if rule.Children[2].Value != "heredoc_end" {
		t.Fatalf("heredoc_body end = %#v, want heredoc_end", rule.Children[2])
	}
	repeat := rule.Children[1]
	if repeat == nil || repeat.Kind != RuleRepeat || len(repeat.Children) != 1 {
		t.Fatalf("middle rule = %#v, want repeat", repeat)
	}
	choice := repeat.Children[0]
	if choice == nil || choice.Kind != RuleChoice || len(choice.Children) != 2 {
		t.Fatalf("repeat content = %#v, want compact two-way choice", choice)
	}
	if got := []string{choice.Children[0].Value, choice.Children[1].Value}; got[0] != "heredoc_content" || got[1] != "escape_sequence" {
		t.Fatalf("compact heredoc alternatives = %v, want [heredoc_content escape_sequence]", got)
	}
	// The recursive interpolation -> _statements path must be gone: it is the
	// nonterminal-extra chain that never converges (GEN_COST_RCA).
	for _, child := range choice.Children {
		if child.Value == "interpolation" {
			t.Fatalf("rewritten heredoc_body must not reference interpolation, got %#v", rule)
		}
	}
}

// TestApplyImportGrammarPostShapeHintsRubyHeredocBodyIsGatedToRuby confirms
// the ruby rewrite is scoped to lang name "ruby" only, as required, and that
// crystal's own (distinct) heredoc_body rewrite - see
// TestApplyImportGrammarPostShapeHintsCrystalHeredocBody - does not bleed
// into ruby's gate or vice versa. elixir shares the same pathology class as
// ruby's and crystal's pre-rewrite heredoc_body (a nonterminal-extra body
// whose CHOICE includes an `interpolation` alternative that re-enters the
// recursive statement/expression grammar) but has no heredoc_body rule at
// all in its real grammar; the fixture below deliberately reuses ruby's
// exact shape under the grammar name "elixir" to prove the rewrite's gate
// (applyImportGrammarPostShapeHints, g.Name == "ruby") is name-based, not
// shape-based - applying ruby's identical shape under a third, unrelated
// grammar name and confirming it is left untouched is precisely how to
// prove the gate keys off the name, and that conclusion holds regardless of
// elixir's actual grammar shape. elixir is out of scope for this change.
func TestApplyImportGrammarPostShapeHintsRubyHeredocBodyIsGatedToRuby(t *testing.T) {
	original := Seq(
		Sym("_heredoc_body_start"),
		Repeat(Choice(
			Sym("heredoc_content"),
			Sym("interpolation"),
			Sym("escape_sequence"),
		)),
		Sym("heredoc_end"),
	)

	g := NewGrammar("elixir")
	g.Define("heredoc_body", cloneRule(original))

	applyImportGrammarPostShapeHints(g)

	rule := g.Rules["heredoc_body"]
	repeat := rule.Children[1]
	choice := repeat.Children[0]
	if len(choice.Children) != 3 {
		t.Fatalf("elixir's heredoc_body should be left untouched (3-way choice), got %#v", choice)
	}
	found := false
	for _, child := range choice.Children {
		if child.Value == "interpolation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("elixir's heredoc_body should still reference interpolation, got %#v", choice)
	}
}

// TestApplyImportGrammarPostShapeHintsCrystalHeredocBody mirrors
// TestApplyImportGrammarPostShapeHintsRubyHeredocBody for crystal's
// heredoc_body, which has the identical pathology (a nonterminal grammar
// extra whose REPEAT(CHOICE(...)) body includes a visible `interpolation`
// alternative that re-enters `_expression`) but a different concrete shape:
// crystal's real heredoc_body is a 4-way CHOICE(heredoc_content,
// interpolation, string_escape_sequence, ignored_backslash) - it does not
// use ruby's `escape_sequence` symbol name. See
// SHAPE_RCA_CRYSTAL_TLAPLUS_RESCRIPT ("crystal | heredoc_body ->
// interpolation -> _expression (173304 >= cap 173304) | REWRITABLE-LIKE-RUBY
// ... validated: drop `interpolation` from heredoc_body ... generates a
// 217KB blob cleanly").
func TestApplyImportGrammarPostShapeHintsCrystalHeredocBody(t *testing.T) {
	g := NewGrammar("crystal")
	g.Define("heredoc_body", Seq(
		Sym("_heredoc_body_start"),
		Repeat(Choice(
			Sym("heredoc_content"),
			Sym("interpolation"),
			Sym("string_escape_sequence"),
			Sym("ignored_backslash"),
		)),
		Sym("heredoc_end"),
	))

	applyImportGrammarPostShapeHints(g)

	rule := g.Rules["heredoc_body"]
	if rule == nil || rule.Kind != RuleSeq || len(rule.Children) != 3 {
		t.Fatalf("heredoc_body rule = %#v, want compact seq", rule)
	}
	if rule.Children[0].Value != "_heredoc_body_start" {
		t.Fatalf("heredoc_body start = %#v, want _heredoc_body_start", rule.Children[0])
	}
	if rule.Children[2].Value != "heredoc_end" {
		t.Fatalf("heredoc_body end = %#v, want heredoc_end", rule.Children[2])
	}
	repeat := rule.Children[1]
	if repeat == nil || repeat.Kind != RuleRepeat || len(repeat.Children) != 1 {
		t.Fatalf("middle rule = %#v, want repeat", repeat)
	}
	choice := repeat.Children[0]
	if choice == nil || choice.Kind != RuleChoice || len(choice.Children) != 3 {
		t.Fatalf("repeat content = %#v, want compact three-way choice", choice)
	}
	if got := []string{choice.Children[0].Value, choice.Children[1].Value, choice.Children[2].Value}; got[0] != "heredoc_content" || got[1] != "string_escape_sequence" || got[2] != "ignored_backslash" {
		t.Fatalf("compact heredoc alternatives = %v, want [heredoc_content string_escape_sequence ignored_backslash]", got)
	}
	// ignored_backslash must survive the rewrite: unlike ruby, crystal keeps
	// two scanner-delimited alternatives alongside heredoc_content.
	found := false
	for _, child := range choice.Children {
		if child.Value == "ignored_backslash" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rewritten crystal heredoc_body must keep ignored_backslash, got %#v", choice)
	}
	// The recursive interpolation -> _expression path must be gone: it is
	// the nonterminal-extra chain that never converges (SHAPE_RCA).
	for _, child := range choice.Children {
		if child.Value == "interpolation" {
			t.Fatalf("rewritten heredoc_body must not reference interpolation, got %#v", rule)
		}
	}
}

// TestApplyImportGrammarPostShapeHintsCrystalHeredocBodyIsGatedFromRuby
// confirms the crystal rewrite does not touch ruby's heredoc_body (and thus
// does not accidentally require crystal's string_escape_sequence /
// ignored_backslash symbol names to be present in ruby's grammar): ruby
// keeps its own 3-way CHOICE(heredoc_content, interpolation,
// escape_sequence) input, and only ruby's escape_sequence-shaped rewrite
// applies to it, exactly as TestApplyImportGrammarPostShapeHintsRubyHeredocBody
// already verifies. This test instead proves the converse direction: running
// the switch against ruby's exact original (pre-rewrite) shape under grammar
// name "crystal" produces crystal's rewrite (string_escape_sequence /
// ignored_backslash), not ruby's (escape_sequence) - the two cases are
// independent rewrites keyed strictly on g.Name, not on which symbols the
// input happens to contain.
func TestApplyImportGrammarPostShapeHintsCrystalHeredocBodyIsGatedFromRuby(t *testing.T) {
	g := NewGrammar("ruby")
	g.Define("heredoc_body", Seq(
		Sym("_heredoc_body_start"),
		Repeat(Choice(
			Sym("heredoc_content"),
			Sym("interpolation"),
			Sym("escape_sequence"),
		)),
		Sym("heredoc_end"),
	))

	applyImportGrammarPostShapeHints(g)

	rule := g.Rules["heredoc_body"]
	repeat := rule.Children[1]
	choice := repeat.Children[0]
	if got := []string{choice.Children[0].Value, choice.Children[1].Value}; got[0] != "heredoc_content" || got[1] != "escape_sequence" {
		t.Fatalf("ruby's heredoc_body should keep escape_sequence (ruby's rewrite, not crystal's), got %v", got)
	}
	for _, child := range choice.Children {
		if child.Value == "string_escape_sequence" || child.Value == "ignored_backslash" {
			t.Fatalf("ruby's heredoc_body should not pick up crystal's symbol names, got %#v", choice)
		}
	}
}
