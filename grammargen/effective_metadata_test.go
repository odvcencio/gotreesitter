package grammargen

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// The authored YAML root's alias and inherited key/value rows distinguish
// completion order from declaration order. Both preserve the visible tree.
func TestEffectiveProductionMetadataYAML(t *testing.T) {
	b, err := Generate(YAMLCoreGrammar())
	if err != nil {
		t.Fatal(err)
	}
	l, err := gts.LoadLanguage(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.FieldMapSlices) != 12 || len(l.AliasSequences) != 12 {
		t.Fatalf("metadata rows: %d fields, %d aliases", len(l.FieldMapSlices), len(l.AliasSequences))
	}
	for revision, want := range map[int]string{6: "flow_pair", 7: "flow_pair", 8: "document"} {
		alias := l.AliasSequences[revision]
		if len(alias) == 0 || int(alias[0]) >= len(l.SymbolNames) || l.SymbolNames[alias[0]] != want {
			t.Fatalf("production %d aliases %v, want child 0 %q", revision, alias, want)
		}
	}
	row := l.FieldMapSlices[1]
	if row[1] != 2 {
		t.Fatalf("first inherited field row: %v", row)
	}
	for i, name := range []string{"key", "value"} {
		f := l.FieldMapEntries[int(row[0])+i]
		if !f.Inherited || f.ChildIndex != 0 || l.FieldNames[f.FieldID] != name {
			t.Fatalf("first inherited field row entry %d: %+v", i, f)
		}
	}
}
