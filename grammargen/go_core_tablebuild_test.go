package grammargen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/tablebuild/ref"
)

// The Go grammar is built by internal/tablebuild from GoCoreGrammar. These
// tests check its two ends against tree-sitter-go's pinned files, whose
// records internal/tablebuild/testdata/go holds:
//
//	L0: the exported grammar.json equals the pinned grammar.json, rule for
//	    rule and member for member, and in the same order.
//	L6: the blob that the build encodes decodes to a Language equal to the one
//	    the pinned parser.c gives, field by field (the L6 records), and its
//	    counts and parse and lex tables equal that parser.c's (the L1, L4 and
//	    L5 records, which are written from the same Language).

const goTablebuildDir = "../internal/tablebuild/testdata/go"

// goCoreDroppedSections are the sections that a Language cannot give: they
// read data that the pinned parser.c holds beside the tables (the symbol map,
// the longest alias sequence) or the node types file.
var goCoreDroppedSections = map[string]bool{
	"L1 node-type":                 true,
	"L2 max-alias-sequence-length": true,
	"L4 symbol-map":                true,
	"L4 primary-states":            true, // dropped from the blob; tablebuild's L4 test covers it
}

// goCoreFormOnlyFields are the Language fields that the L6 comparison leaves
// out, because the reference holds them in another form with the same
// content. The reference Language comes from cmd/ts2go, which reads the
// ranges of a lex function in the order of the C text, overlapping, with the
// last range as the fallback. The build writes disjoint ranges in ascending
// order. Layer L5 compares both in one canonical form (every state's accept
// token, end-of-input edge and the step on each range, in internal/tablebuild's
// TestL5), and its reference mode reads the lex functions of parser.c with
// internal/lexexact and compares the step on every code point where a range
// starts or ends.
var goCoreFormOnlyFields = map[string]bool{
	"LexStates":        true,
	"KeywordLexStates": true,
}

func goCoreDigests(t *testing.T) map[string][]string {
	t.Helper()
	f, _, err := ref.ReadDigestDir(goTablebuildDir)
	if err != nil {
		t.Fatal(err)
	}
	return f.Sections
}

func goCoreExport(t *testing.T) []byte {
	t.Helper()
	text, err := ExportGrammarJSON(GoCoreGrammar())
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// TestGoCoreGrammarEqualsPinnedGrammarJSON is layer L0 for Go.
func TestGoCoreGrammarEqualsPinnedGrammarJSON(t *testing.T) {
	lines, err := ref.GrammarRecords(goCoreExport(t))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for s, d := range goCoreDigests(t) {
		if len(s) > 3 && s[:3] == "L0 " {
			want[s] = d
		}
	}
	if len(want) == 0 {
		t.Fatal("the digest file has no L0 sections")
	}
	if d := ref.CompareDigests(lines, want); d != nil {
		t.Fatalf("GoCoreGrammar differs from the pinned grammar.json:\n%s", d)
	}

	// The records hide nothing that a reader of the text sees: also compare
	// the parsed documents, which includes every key the records skip.
	pinned, err := os.ReadFile(filepath.Join(goTablebuildDir, "grammar.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got, wantDoc map[string]any
	if err := json.Unmarshal(goCoreExport(t), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pinned, &wantDoc); err != nil {
		t.Fatal(err)
	}
	delete(wantDoc, "$schema")
	if !reflect.DeepEqual(got, wantDoc) {
		var gk, wk []string
		for k := range got {
			gk = append(gk, k)
		}
		for k := range wantDoc {
			wk = append(wk, k)
		}
		sort.Strings(gk)
		sort.Strings(wk)
		t.Fatalf("the exported document differs from the pinned grammar.json (keys %v against %v)", gk, wk)
	}
}

// TestGoCoreBlobEqualsPinnedTables is layer L6 for Go, through the blob.
func TestGoCoreBlobEqualsPinnedTables(t *testing.T) {
	g := GoCoreGrammar()
	if g.TableBuilder != TableBuilderTreeSitter {
		t.Fatalf("GoCoreGrammar().TableBuilder = %q, want %q", g.TableBuilder, TableBuilderTreeSitter)
	}
	_, blob, err := GenerateLanguageAndBlob(g)
	if err != nil {
		t.Fatal(err)
	}
	lang, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("go: %d states, %d symbols, %d tokens, %d production ids, %d fields; blob %d bytes",
		lang.StateCount, lang.SymbolCount, lang.TokenCount, lang.ProductionIDCount, lang.FieldCount, len(blob))
	lines, err := ref.LanguageRecords(lang, ref.NoExtra, "L4")
	if err != nil {
		t.Fatal(err)
	}
	drop := func(s string) bool { return goCoreDroppedSections[s] }
	lines = ref.Without(lines, drop)
	want := map[string][]string{}
	for s, d := range goCoreDigests(t) {
		if !drop(s) && len(s) > 3 && s[:3] != "L0 " && s[:3] != "L3 " {
			want[s] = d
		}
	}
	// The L6 record of a field is the digest of its whole value. Two fields
	// differ in form, not in content: see goCoreFormOnlyFields.
	var fields []string
	var kept []string
	for _, l := range lines {
		if ref.SectionOf(l) != "L6 field" {
			kept = append(kept, l)
			continue
		}
		name := strings.Split(l, "\t")[2]
		fields = append(fields, name)
		if !goCoreFormOnlyFields[name] {
			kept = append(kept, l)
		}
	}
	lines = kept
	if len(fields) != len(want["L6 field"]) {
		t.Fatalf("%d L6 fields, %d reference digests", len(fields), len(want["L6 field"]))
	}
	var keptDigests []string
	for i, name := range fields {
		if !goCoreFormOnlyFields[name] {
			keptDigests = append(keptDigests, want["L6 field"][i])
		}
	}
	want["L6 field"] = keptDigests
	if d := ref.CompareDigests(lines, want); d != nil {
		t.Fatalf("the Go blob differs from the pinned parser.c:\n%s", d)
	}
}
