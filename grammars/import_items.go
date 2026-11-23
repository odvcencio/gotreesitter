package grammars

import gts "github.com/odvcencio/gotreesitter"

// ImportItem describes one written item in an import or re-export statement.
// Range spans the item. NameRange spans the original name (or '*') and is
// empty when no imported name is written. BindingRange spans the local name
// or the exported alias of a re-export. Wildcards without an alias have no binding.
type ImportItem struct {
	Kind                           string // "module", "named", "default", "namespace", or "wildcard"
	Range, NameRange, BindingRange gts.Range
	TypeOnly                       bool
}

// ImportItems returns Python and ECMAScript import and re-export items in source
// order. TypeScript import-equals with require binds the whole module ("module").
// Bare side-effect imports, local exports and internal aliases have no import items.
// All ranges address the original source, including repeated names and aliases.
func ImportItems(statement *gts.Node, lang *gts.Language) []ImportItem {
	if statement == nil || lang == nil {
		return nil
	}
	switch lang.Name {
	case "python":
		switch statement.Type(lang) {
		case "import_statement", "import_from_statement", "future_import_statement":
			return pythonImportItems(statement, lang)
		}
	case "javascript", "typescript", "tsx":
		switch statement.Type(lang) {
		case "import_statement", "export_statement":
			if statement.ChildByFieldName("source", lang) != nil {
				return ecmaImportItems(statement, lang)
			}
			if clause := importToken(statement, lang, "import_require_clause"); clause != nil {
				binding := importToken(clause, lang, "identifier")
				source := clause.ChildByFieldName("source", lang)
				if binding == nil || binding.IsMissing() || source == nil || source.IsMissing() {
					return nil
				}
				return []ImportItem{{
					Kind: "module", Range: clause.Range(), BindingRange: binding.Range(),
					TypeOnly: importToken(statement, lang, "type") != nil,
				}}
			}
		}
	}
	return nil
}

func pythonImportItems(statement *gts.Node, lang *gts.Language) []ImportItem {
	module := statement.ChildByFieldName("module_name", lang)
	kind := "named"
	if statement.Type(lang) == "import_statement" {
		kind = "module"
	}
	var items []ImportItem
	for i := 0; i < statement.NamedChildCount(); i++ {
		n := statement.NamedChild(i)
		if n == module {
			continue
		}
		var name, binding *gts.Node
		switch n.Type(lang) {
		case "wildcard_import":
			items = append(items, ImportItem{Kind: "wildcard", Range: n.Range(), NameRange: n.Range()})
			continue
		case "aliased_import":
			name, binding = n.ChildByFieldName("name", lang), n.ChildByFieldName("alias", lang)
		case "dotted_name":
			name, binding = n, n
			// import a.b introduces a in the local scope.
			if kind == "module" {
				binding = n.NamedChild(0)
			}
		default:
			continue
		}
		if name != nil && binding != nil && !name.IsMissing() && !binding.IsMissing() {
			items = append(items, ImportItem{Kind: kind, Range: n.Range(), NameRange: name.Range(), BindingRange: binding.Range()})
		}
	}
	return items
}

func ecmaImportItems(statement *gts.Node, lang *gts.Language) []ImportItem {
	var items []ImportItem
	var visit func(*gts.Node, bool)
	visit = func(n *gts.Node, typeOnly bool) {
		switch n.Type(lang) {
		case "import_statement", "export_statement", "import_clause", "named_imports", "export_clause":
			typeOnly = typeOnly || importToken(n, lang, "type") != nil
			// Some TS grammars recover the modifier of "export type *" as
			// an ERROR node containing just the type token.
			if n.Type(lang) == "export_statement" {
				if e := importToken(n, lang, "ERROR"); e != nil && e.ChildCount() == 1 && e.Child(0).Type(lang) == "type" {
					typeOnly = true
				}
			}
			for i := 0; i < n.ChildCount(); i++ {
				visit(n.Child(i), typeOnly)
			}
		case "identifier":
			items = append(items, ImportItem{Kind: "default", Range: n.Range(), BindingRange: n.Range(), TypeOnly: typeOnly})
		case "import_specifier", "export_specifier":
			name := n.ChildByFieldName("name", lang)
			if name == nil || name.IsMissing() {
				return
			}
			binding := n.ChildByFieldName("alias", lang)
			if binding == nil {
				binding = name
			}
			items = append(items, ImportItem{Kind: "named", Range: n.Range(), NameRange: name.Range(), BindingRange: binding.Range(), TypeOnly: typeOnly || importToken(n, lang, "type") != nil})
		case "namespace_import", "namespace_export":
			star := importToken(n, lang, "*")
			binding := importToken(n, lang, "identifier")
			if binding == nil {
				binding = importToken(n, lang, "string")
			}
			if star != nil && binding != nil {
				items = append(items, ImportItem{Kind: "namespace", Range: n.Range(), NameRange: star.Range(), BindingRange: binding.Range(), TypeOnly: typeOnly})
			}
		case "*":
			items = append(items, ImportItem{Kind: "wildcard", Range: n.Range(), NameRange: n.Range(), TypeOnly: typeOnly})
		}
	}
	visit(statement, false)
	return items
}

func importToken(n *gts.Node, lang *gts.Language, typ string) *gts.Node {
	for i := 0; i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Type(lang) == typ {
			return c
		}
	}
	return nil
}
