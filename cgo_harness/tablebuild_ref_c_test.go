//go:build cgo && treesitter_c_parity

package cgoharness

// The table-build reference (internal/tablebuild/ref) against the unmodified
// tree-sitter generator. TABLEBUILD_REF names a folder that
// cgo_harness/docker/tablebuild_reference.sh filled, one subfolder per
// grammar; TABLEBUILD_GRAMMARS lists the grammars (default json).
//
//	TestTablebuildEligibility        does the generator rewrite the pinned parser.c?
//	TestTablebuildReferenceDigests   the committed digests equal the reference
//	TestTablebuildJSONBlobAgainstPinned  the comparer's self-test on json.bin
//
// TABLEBUILD_UPDATE=1 writes internal/tablebuild/testdata/<grammar>/ instead
// of comparing with it.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/tablebuild/ref"
)

func tablebuildRefRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("TABLEBUILD_REF")
	if root == "" {
		t.Skip("TABLEBUILD_REF is not set: no reference set (cgo_harness/docker/tablebuild_reference.sh)")
	}
	return root
}

func tablebuildGrammars() []string {
	if s := os.Getenv("TABLEBUILD_GRAMMARS"); s != "" {
		return strings.Split(s, ",")
	}
	return []string{"json"}
}

func tablebuildRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tablebuildRecords runs cmd/ts2go with the strict reader on one parser.c.
func tablebuildRecords(t *testing.T, path, name, tables string) []string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "records.txt")
	cmd := exec.Command("go", "run", "./cmd/ts2go", "-input", path, "-name", name,
		"-tablebuild-records", out, "-tablebuild-tables", tables)
	cmd.Dir = ".." // cmd/ts2go is in the top-level module.
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ts2go -tablebuild-records %s: %v\n%s", path, err, b)
	}
	return strings.Split(strings.TrimSuffix(string(tablebuildRead(t, out)), "\n"), "\n")
}

func TestTablebuildEligibility(t *testing.T) {
	root := tablebuildRefRoot(t)
	for _, name := range tablebuildGrammars() {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			pinned := tablebuildRead(t, filepath.Join(dir, "parser-pinned.c"))
			files, _ := filepath.Glob(filepath.Join(dir, "parser-[0-9]*.c"))
			files = append([]string{filepath.Join(dir, "parser.c")}, files...)
			for _, f := range files {
				e := ref.CompareGenerated(pinned, tablebuildRead(t, f))
				t.Logf("ELIGIBILITY %s %s: %s", name, filepath.Base(f), e)
			}
		})
	}
}

// tablebuildReference returns the reference records of one grammar: L0 and
// the node types from the pinned inputs, L3 from parser-noopt.c, L1, L2, L4
// and L5 from parser.c, L6 from the pinned parser.c; and the L1, L2, L4 and
// L5 records of the pinned parser.c.
func tablebuildReference(t *testing.T, dir, name string) (gen, pinned []string, eligible bool) {
	l0, err := ref.GrammarRecords(tablebuildRead(t, filepath.Join(dir, "src", "grammar.json")))
	if err != nil {
		t.Fatal(err)
	}
	nt, err := ref.NodeTypeRecords(tablebuildRead(t, filepath.Join(dir, "src", "node-types.json")))
	if err != nil {
		t.Fatal(err)
	}
	gen = append(l0, nt...)
	if _, err := os.Stat(filepath.Join(dir, "parser-noopt.c")); err == nil {
		gen = append(gen, tablebuildRecords(t, filepath.Join(dir, "parser-noopt.c"), name, "L3")...)
	} else {
		t.Logf("%s: no parser-noopt.c; L3 not written", name)
	}
	isL6 := func(s string) bool { return strings.HasPrefix(s, "L6 ") }
	gen = append(gen, ref.Without(tablebuildRecords(t, filepath.Join(dir, "parser.c"), name, "L4"), isL6)...)
	all := tablebuildRecords(t, filepath.Join(dir, "parser-pinned.c"), name, "L4")
	gen = append(gen, ref.Without(all, func(s string) bool { return !isL6(s) })...)
	pinned = ref.Without(all, isL6)
	eligible = ref.CompareGenerated(tablebuildRead(t, filepath.Join(dir, "parser-pinned.c")), tablebuildRead(t, filepath.Join(dir, "parser.c"))).Equal
	return gen, pinned, eligible
}

func tablebuildManifest(t *testing.T, dir string) (commit, generator string, sources map[string]string) {
	sources = map[string]string{}
	for _, l := range strings.Split(string(tablebuildRead(t, filepath.Join(dir, "MANIFEST"))), "\n") {
		f := strings.Fields(l)
		switch {
		case len(f) >= 2 && f[0] == "commit":
			commit = f[1]
		case len(f) >= 3 && f[0] == "cli" && f[1] == "0.27.1":
			generator = strings.Join(f[2:], " ")
		case len(f) == 2 && len(f[0]) == 64:
			name := strings.TrimPrefix(f[1], "./")
			if name == "src/grammar.json" || name == "src/node-types.json" || strings.HasPrefix(name, "parser") {
				sources[name] = f[0]
			}
		}
	}
	return commit, generator, sources
}

func TestTablebuildReferenceDigests(t *testing.T) {
	root := tablebuildRefRoot(t)
	for _, name := range tablebuildGrammars() {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			gen, pinned, eligible := tablebuildReference(t, dir, name)
			commit, generator, sources := tablebuildManifest(t, dir)
			f := &ref.DigestFile{Grammar: name, Commit: commit, Generator: generator, Eligible: eligible, Sources: sources, Sections: ref.SectionDigests(gen)}
			genTables := ref.Without(gen, func(s string) bool { return ref.InputSections(s) || strings.HasPrefix(s, "L6 ") })
			if d := ref.Compare(pinned, genTables); d != nil {
				t.Logf("%s: the pinned parser.c's records differ from the generator's (pinned shown as port):\n%s", name, d)
				f.Pinned = ref.SectionDigests(pinned)
			}
			secs := ref.Sections(gen)
			var keys []string
			for k := range secs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				t.Logf("RECORDS %s %s: %d", name, k, len(secs[k]))
			}
			out := f.Marshal()
			t.Logf("DIGESTS %s: %d records, %d bytes, eligible %v", name, len(gen), len(out), eligible)
			testdata := filepath.Join("..", "internal", "tablebuild", "testdata", name)
			if os.Getenv("TABLEBUILD_UPDATE") == "1" {
				if err := os.MkdirAll(testdata, 0o755); err != nil {
					t.Fatal(err)
				}
				write := map[string][]byte{
					"digests.json": out,
					"grammar.json": tablebuildRead(t, filepath.Join(dir, "src", "grammar.json")),
					"LICENSE":      tablebuildRead(t, filepath.Join(dir, "LICENSE")),
				}
				for file, b := range write {
					if err := os.WriteFile(filepath.Join(testdata, file), b, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				t.Logf("wrote %s", testdata)
				return
			}
			committed, err := os.ReadFile(filepath.Join(testdata, "digests.json"))
			if os.IsNotExist(err) {
				t.Logf("%s has no committed digests", name)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(committed, out) {
				return
			}
			c, err := ref.ReadDigestFile(filepath.Join(testdata, "digests.json"))
			if err != nil {
				t.Fatal(err)
			}
			if d := ref.CompareDigests(gen, c.Sections); d != nil {
				t.Fatalf("committed digests differ from the reference (reference side shown as port):\n%s", d)
			}
			t.Fatalf("committed digests.json differs from the reference outside the generator sections (header or pinned sections); rerun with TABLEBUILD_UPDATE=1 and review")
		})
	}
}

// TestTablebuildJSONBlobAgainstPinned is the comparer's self-test with both
// sides as text: the records of the shipped json.bin equal those of JSON's
// pinned parser.c, and one changed cell is named.
func TestTablebuildJSONBlobAgainstPinned(t *testing.T) {
	root := tablebuildRefRoot(t)
	pinned := tablebuildRecords(t, filepath.Join(root, "json", "parser-pinned.c"), "json", "L4")
	omitted := func(s string) bool { _, ok := ref.BlobOmits[s]; return ok }
	for s, why := range ref.BlobOmits {
		t.Logf("not compared: %s (%s)", s, why)
	}
	want := ref.Without(pinned, omitted)
	lang, err := gotreesitter.LoadLanguage(tablebuildRead(t, filepath.Join("..", "grammars", "grammar_blobs", "json.bin")))
	if err != nil {
		t.Fatal(err)
	}
	port, err := ref.LanguageRecords(lang, ref.NoExtra, "L4")
	if err != nil {
		t.Fatal(err)
	}
	// The one known difference: json.bin was last written on 19 Sep 2026,
	// before ts2go kept the end-token accept (ref.RestoreEndAccept).
	const endState = 21
	d := ref.Compare(port, want)
	if d == nil || d.Section != "L5 lex" || d.Index != endState || d.PortField != "end=false" || d.RefField != "end=true" {
		t.Fatalf("json.bin: want the known difference at L5 lex %d first, got:\n%v", endState, d)
	}
	t.Logf("known difference:\n%s", d)
	if err := ref.RestoreEndAccept(lang, endState); err != nil {
		t.Fatal(err)
	}
	if port, err = ref.LanguageRecords(lang, ref.NoExtra, "L4"); err != nil {
		t.Fatal(err)
	}
	if d := ref.Compare(port, want); d != nil {
		t.Fatalf("json.bin differs from the pinned parser.c beyond the known end accept:\n%s", d)
	}
	t.Logf("json.bin equals the pinned parser.c: %d records in %d sections", len(want), len(ref.Sections(want)))

	state, field, before, after, err := ref.ChangeOneCell(lang)
	if err != nil {
		t.Fatal(err)
	}
	port, err = ref.LanguageRecords(lang, ref.NoExtra, "L4")
	if err != nil {
		t.Fatal(err)
	}
	d = ref.Compare(port, want)
	if d == nil {
		t.Fatalf("a changed cell (state %d: %s to %s) was not found", state, before, after)
	}
	if d.Section != "L4 state" || d.Index != state || d.Field != field || d.PortField != after || d.RefField != before {
		t.Fatalf("the changed cell (state %d, field %d: %s to %s) was not named:\n%s", state, field, before, after, d)
	}
	t.Logf("one changed cell is named:\n%s", d)
}
