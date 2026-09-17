package grammargen

import (
	"slices"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestSupertypeMapUsesPublicAlias(t *testing.T) {
	g := NewGrammar("supertype_alias")
	// The first identifier stays unaliased. The literal shares the alias name.
	g.Define("source_file", Seq(Sym("identifier"), Str(":"), Str("type_identifier"), Sym("_simple_type")))
	g.Define("_simple_type", Choice(
		Alias(Sym("identifier"), "type_identifier", true),
		Sym("builtin_type"),
	))
	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))
	g.Define("builtin_type", Str("int"))
	g.SetExtras(Pat(`[ \t]+`))
	g.SetWord("identifier")
	g.SetSupertypes("_simple_type")

	blob, err := Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	lang, err := gotreesitter.LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("named_alias_does_not_resolve_to_anonymous_literal", func(t *testing.T) {
		supertype, ok := lang.SymbolByName("_simple_type")
		if !ok {
			t.Fatal("missing supertype symbol")
		}
		var namedAlias, anonymousLiteral gotreesitter.Symbol
		for i, name := range lang.SymbolNames {
			if name != "type_identifier" {
				continue
			}
			if lang.SymbolMetadata[i].Named {
				namedAlias = gotreesitter.Symbol(i)
			} else {
				anonymousLiteral = gotreesitter.Symbol(i)
			}
		}
		if namedAlias == 0 || anonymousLiteral == 0 || namedAlias == anonymousLiteral {
			t.Fatal("fixture requires distinct named and anonymous symbols with the alias name")
		}
		children := lang.SupertypeChildren(supertype)
		if !slices.Contains(children, namedAlias) || slices.Contains(children, anonymousLiteral) {
			t.Fatalf("supertype children=%v; want named alias %d and no anonymous literal %d", children, namedAlias, anonymousLiteral)
		}
	})

	for _, tc := range []struct {
		name   string
		source string
		query  string
		text   string
	}{
		{"aliased_subtype_capture", "x: type_identifier T", `(_simple_type/type_identifier) @type`, "T"},
		{"unaliased_subtype_capture", "x: type_identifier int", `(_simple_type/builtin_type) @type`, "int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query, err := gotreesitter.NewQuery(tc.query, lang)
			if err != nil {
				t.Fatalf("compile subtype query: %v", err)
			}
			source := []byte(tc.source)
			tree, err := gotreesitter.NewParser(lang).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("fixture returned no tree")
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() || root.StartByte() != 0 || int(root.EndByte()) != len(source) {
				t.Fatalf("fixture parse is incomplete or erroneous: %s", root.SExpr(lang))
			}
			cursor := query.Exec(root, lang, source)
			match, ok := cursor.NextMatch()
			if !ok || len(match.Captures) != 1 {
				t.Fatalf("want one subtype capture, got match=%+v present=%t", match, ok)
			}
			capture := match.Captures[0]
			if capture.Name != "type" || capture.Text(source) != tc.text || !capture.Node.IsNamed() {
				t.Fatalf("unexpected subtype capture: %+v text=%q", capture, capture.Text(source))
			}
			if _, ok := cursor.NextMatch(); ok {
				t.Fatal("subtype query returned an extra match")
			}
		})
	}

	t.Run("underlying_identifier_is_not_a_subtype", func(t *testing.T) {
		if _, err := gotreesitter.NewQuery(`(_simple_type/identifier) @type`, lang); err == nil {
			t.Fatal("subtype query accepted the underlying symbol instead of the public alias")
		}
	})
}

func TestSupertypeMapDeduplicatesPublicAliases(t *testing.T) {
	g := NewGrammar("supertype_alias_duplicates")
	// Both underlying symbols also appear without an alias.
	g.Define("source_file", Seq(Sym("identifier"), Sym("number"), Str(":"), Sym("_type")))
	g.Define("_type", Choice(
		Alias(Sym("identifier"), "public_type", true),
		Sym("builtin_type"),
		Alias(Sym("number"), "public_type", true),
	))
	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))
	g.Define("number", Pat(`[0-9]+`))
	g.Define("builtin_type", Str("int"))
	g.SetExtras(Pat(`[ \t]+`))
	g.SetWord("identifier")
	g.SetSupertypes("_type")

	blob, err := Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	lang, err := gotreesitter.LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	supertype, ok := lang.SymbolByName("_type")
	if !ok {
		t.Fatal("missing supertype symbol")
	}
	alias, ok := lang.SymbolByName("public_type")
	if !ok {
		t.Fatal("missing public alias symbol")
	}
	builtin, ok := lang.SymbolByName("builtin_type")
	if !ok {
		t.Fatal("missing unaliased subtype symbol")
	}
	if children := lang.SupertypeChildren(supertype); !slices.Equal(children, []gotreesitter.Symbol{alias, builtin}) {
		t.Fatalf("supertype children=%v; want unique public symbols [%d %d] in first-occurrence order", children, alias, builtin)
	}

	query, err := gotreesitter.NewQuery(`(_type/public_type) @type`, lang)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"T", "42"} {
		t.Run(suffix, func(t *testing.T) {
			source := []byte("x 1: " + suffix)
			tree, err := gotreesitter.NewParser(lang).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("fixture returned no tree")
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() || int(root.EndByte()) != len(source) {
				t.Fatalf("fixture parse is incomplete or erroneous: %s", root.SExpr(lang))
			}
			cursor := query.Exec(root, lang, source)
			match, ok := cursor.NextMatch()
			if !ok || len(match.Captures) != 1 || match.Captures[0].Text(source) != suffix {
				t.Fatalf("want one %q capture; got match=%+v present=%t", suffix, match, ok)
			}
			if _, ok := cursor.NextMatch(); ok {
				t.Fatal("subtype query returned an extra match")
			}
		})
	}
}
