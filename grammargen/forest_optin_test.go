package grammargen

import (
	"strings"
	"testing"
)

// minimalForestOptInGrammarJSON is a minimal valid grammar.json body — just
// enough for ImportGrammarJSON to succeed (a single STRING rule) — used to
// exercise the declarative "gotreesitter" extension object independent of any
// other grammar feature.
const minimalForestOptInGrammarJSON = `{
	"name": "pawn",
	"rules": {
		"source_file": {"type": "STRING", "value": "a"}
	},
	"extras": [],
	"conflicts": [],
	"externals": [],
	"inline": [],
	"supertypes": [],
	"word": "",
	"reserved": {},
	"precedences": [],
	"gotreesitter": {"wantsForest": true}
}`

// TestImportGrammarJSONGotreesitterWantsForest verifies that ImportGrammarJSON
// reads the namespaced "gotreesitter" extension object's "wantsForest" field
// into Grammar.WantsForest — letting an existing grammar opt into the
// GSS-forest fast path declaratively via grammar.json, with no Go code or
// fork required.
func TestImportGrammarJSONGotreesitterWantsForest(t *testing.T) {
	g, err := ImportGrammarJSON([]byte(minimalForestOptInGrammarJSON))
	if err != nil {
		t.Fatalf("ImportGrammarJSON failed: %v", err)
	}
	if !g.WantsForest {
		t.Errorf("Grammar.WantsForest = false, want true")
	}
}

// TestImportGrammarJSONWithoutGotreesitterDefaultsFalse verifies that a
// standard grammar.json with no "gotreesitter" object leaves WantsForest
// false — the extension is purely additive and does not affect existing
// grammars.
func TestImportGrammarJSONWithoutGotreesitterDefaultsFalse(t *testing.T) {
	data := []byte(`{
		"name": "pawn",
		"rules": {
			"source_file": {"type": "STRING", "value": "a"}
		},
		"extras": [],
		"conflicts": [],
		"externals": [],
		"inline": [],
		"supertypes": [],
		"word": "",
		"reserved": {},
		"precedences": []
	}`)

	g, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON failed: %v", err)
	}
	if g.WantsForest {
		t.Errorf("Grammar.WantsForest = true, want false (no gotreesitter object present)")
	}
}

// TestGrammarJSONGotreesitterWantsForestRoundTrip verifies the full
// Import -> Export -> Import cycle preserves WantsForest, and that
// ExportGrammarJSON mirrors it back into a "gotreesitter" object.
func TestGrammarJSONGotreesitterWantsForestRoundTrip(t *testing.T) {
	g, err := ImportGrammarJSON([]byte(minimalForestOptInGrammarJSON))
	if err != nil {
		t.Fatalf("ImportGrammarJSON failed: %v", err)
	}
	if !g.WantsForest {
		t.Fatalf("precondition failed: Grammar.WantsForest = false, want true")
	}

	data, err := ExportGrammarJSON(g)
	if err != nil {
		t.Fatalf("ExportGrammarJSON failed: %v", err)
	}
	if !strings.Contains(string(data), `"gotreesitter"`) {
		t.Errorf("exported grammar.json missing \"gotreesitter\" key: %s", data)
	}
	if !strings.Contains(string(data), `"wantsForest": true`) {
		t.Errorf("exported grammar.json missing wantsForest:true: %s", data)
	}

	g2, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("re-ImportGrammarJSON failed: %v", err)
	}
	if !g2.WantsForest {
		t.Errorf("round-tripped Grammar.WantsForest = false, want true")
	}
}

// TestExportGrammarJSONOmitsGotreesitterWhenUnset verifies that
// ExportGrammarJSON's "gotreesitter" key is entirely absent (via omitempty)
// when WantsForest is false, so standard grammars' exported JSON is
// byte-identical to before this feature existed.
func TestExportGrammarJSONOmitsGotreesitterWhenUnset(t *testing.T) {
	g := CalcGrammar()
	if g.WantsForest {
		t.Fatalf("precondition failed: CalcGrammar().WantsForest = true, want false")
	}

	data, err := ExportGrammarJSON(g)
	if err != nil {
		t.Fatalf("ExportGrammarJSON failed: %v", err)
	}
	if strings.Contains(string(data), `"gotreesitter"`) {
		t.Errorf("exported grammar.json unexpectedly contains \"gotreesitter\" key (should be omitted when unset): %s", data)
	}
}

// TestGrammarWantsForestBlobRoundTrip verifies that Grammar.WantsForest is
// plumbed onto the assembled Language and survives gob-blob encode/decode
// (grammargen.Generate / decodeLanguageBlob) — consumer grammars set this
// flag to opt a grammargen-generated Language into the GSS-forest fast path
// without forking gotreesitter's curated builtinForestDefaults allowlist.
func TestGrammarWantsForestBlobRoundTrip(t *testing.T) {
	g := CalcGrammar()
	g.WantsForest = true

	lang, blob, err := GenerateLanguageAndBlob(g)
	if err != nil {
		t.Fatalf("GenerateLanguageAndBlob failed: %v", err)
	}
	if !lang.WantsForest {
		t.Errorf("directly generated Language.WantsForest = false, want true")
	}

	decoded, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("decodeLanguageBlob failed: %v", err)
	}
	if !decoded.WantsForest {
		t.Errorf("blob round-trip Language.WantsForest = false, want true")
	}
}

// TestGrammarWantsForestDefaultFalse verifies the flag defaults to false when
// unset, so existing consumer grammars are unaffected until they opt in.
func TestGrammarWantsForestDefaultFalse(t *testing.T) {
	g := CalcGrammar()

	lang, blob, err := GenerateLanguageAndBlob(g)
	if err != nil {
		t.Fatalf("GenerateLanguageAndBlob failed: %v", err)
	}
	if lang.WantsForest {
		t.Errorf("unset Grammar.WantsForest should default to false, got true")
	}

	decoded, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("decodeLanguageBlob failed: %v", err)
	}
	if decoded.WantsForest {
		t.Errorf("blob round-trip of unset WantsForest should default to false, got true")
	}
}

// TestExtendGrammarPreservesWantsForest verifies ExtendGrammar inherits the base
// grammar's WantsForest opt-in, the same way it carries every other Grammar-level
// config flag. Without this, extending a forest-enabled base grammar would
// silently drop the opt-in to false.
func TestExtendGrammarPreservesWantsForest(t *testing.T) {
	base := CalcGrammar()
	base.WantsForest = true

	ext := ExtendGrammar("calc_ext", base, func(g *Grammar) {})
	if !ext.WantsForest {
		t.Errorf("ExtendGrammar dropped base.WantsForest; extended grammar should inherit it")
	}

	// A base without the opt-in stays false.
	ext2 := ExtendGrammar("calc_ext2", CalcGrammar(), func(g *Grammar) {})
	if ext2.WantsForest {
		t.Errorf("ExtendGrammar set WantsForest when base had none")
	}
}
