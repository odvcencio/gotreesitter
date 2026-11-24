package ref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// committedDigests reads the committed reference of a grammar; cgo_harness
// writes it (TestTablebuildReferenceDigests with TABLEBUILD_UPDATE=1).
func committedDigests(t *testing.T, grammar string) *DigestFile {
	t.Helper()
	f, _, err := ReadDigestDir(filepath.Join("..", "testdata", grammar))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func jsonDigests(t *testing.T) *DigestFile { return committedDigests(t, "json") }

// pinnedSections returns the digests of the pinned parser.c that a Language
// decoded from a ts2go blob can be compared with.
func pinnedSections(f *DigestFile) map[string][]string {
	src := f.Sections
	if len(f.Pinned) > 0 {
		src = f.Pinned
	}
	out := map[string][]string{}
	for s, d := range src {
		if _, omitted := BlobOmits[s]; !omitted && !InputSections(s) && !strings.HasPrefix(s, "L6 ") {
			out[s] = d
		}
	}
	for s, d := range f.Sections {
		if strings.HasPrefix(s, "L6 ") {
			out[s] = d
		}
	}
	return out
}

func loadJSONBlob(t *testing.T) *gts.Language {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "grammars", "grammar_blobs", "json.bin"))
	if err != nil {
		t.Fatal(err)
	}
	lang, err := gts.LoadLanguage(b)
	if err != nil {
		t.Fatal(err)
	}
	return lang
}

// TestGrammarJSONEqualsDigests: each committed grammar.json gives the L0
// digests of the committed reference, so the pure-Go tier reads the input the
// reference was built from.
func TestGrammarJSONEqualsDigests(t *testing.T) {
	for _, grammar := range []string{"json", "go"} {
		t.Run(grammar, func(t *testing.T) {
			f := committedDigests(t, grammar)
			b, err := os.ReadFile(filepath.Join("..", "testdata", grammar, "grammar.json"))
			if err != nil {
				t.Fatal(err)
			}
			lines, err := GrammarRecords(b)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string][]string{}
			for s, d := range f.Sections {
				if strings.HasPrefix(s, "L0 ") {
					want[s] = d
				}
			}
			if len(want) == 0 {
				t.Fatal("the reference has no L0 sections")
			}
			if d := CompareDigests(lines, want); d != nil {
				t.Fatalf("testdata/%s/grammar.json differs from the reference:\n%s", grammar, d)
			}
			n := 0
			for _, d := range f.Sections {
				n += len(d)
			}
			t.Logf("%s: %d L0 records equal; the reference holds %d records in %d sections (eligible %v)", grammar, len(lines), n, len(f.Sections), f.Eligible)
		})
	}
}

// jsonBlobEndState is the main lex state of the shipped json.bin that lacks
// the end-token accept of the pinned parser.c: the blob was last written on
// 19 Sep 2026, before ts2go kept that accept (see RestoreEndAccept).
const jsonBlobEndState = 21

// TestJSONBlobEqualsPinnedDigests is the comparer's self-test in the pure-Go
// tier. The shipped json.bin differs from JSON's pinned parser.c in exactly
// one record, the known missing end accept; with that field restored it gives
// the digests of the pinned parser.c in every section a blob can hold, and one
// changed cell makes the comparison fail on that cell.
func TestJSONBlobEqualsPinnedDigests(t *testing.T) {
	f := jsonDigests(t)
	want := pinnedSections(f)
	lang := loadJSONBlob(t)
	port, err := LanguageRecords(lang, NoExtra, "L4")
	if err != nil {
		t.Fatal(err)
	}
	d := CompareDigests(port, want)
	if d == nil || d.Section != "L5 lex" || d.Index != jsonBlobEndState || !strings.Contains(d.Port, "\tend=false\t") {
		t.Fatalf("json.bin: want the known difference at L5 lex %d first, got:\n%v", jsonBlobEndState, d)
	}
	if err := RestoreEndAccept(lang, jsonBlobEndState); err != nil {
		t.Fatal(err)
	}
	if port, err = LanguageRecords(lang, NoExtra, "L4"); err != nil {
		t.Fatal(err)
	}
	if d := CompareDigests(port, want); d != nil {
		t.Fatalf("json.bin differs from the pinned parser.c beyond the known end accept:\n%s", d)
	}
	t.Logf("%d records in %d sections equal; not compared: %v", len(port), len(want), BlobOmits)

	state, field, before, after, err := ChangeOneCell(lang)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := LanguageRecords(lang, NoExtra, "L4")
	if err != nil {
		t.Fatal(err)
	}
	d = CompareDigests(changed, want)
	if d == nil {
		t.Fatalf("a changed cell (state %d: %s to %s) was not found", state, before, after)
	}
	if d.Section != "L4 state" || d.Index != state {
		t.Fatalf("the changed cell (state %d) was not named:\n%s", state, d)
	}
	// The unchanged blob's line of that record has the reference digest, so
	// it is the reference line: the field comparison names the cell.
	orig := Sections(port)[d.Section][d.Index]
	if Digest(orig) != d.RefDigest {
		t.Fatalf("unchanged line has digest %s, reference %s", Digest(orig), d.RefDigest)
	}
	k, got, was := firstField(d.Port, orig)
	if k != field || got != after || was != before {
		t.Fatalf("field %d %q (was %q), want field %d %q (was %q)", k, got, was, field, after, before)
	}
	t.Logf("one changed cell is named:\n%s\n  field %d: port %q, reference %q", d, k, got, was)
}
