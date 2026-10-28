//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// These tests pin valid inputs that the default Go route rejected with
// HasError while the locked C parser accepted them. The committed real samples
// for agda, djot, haskell and norg are clean in locked C. The short inputs are
// the smallest witnesses found for agda, haskell and norg. Each Go tree must
// match locked C in symbols, fields, spans, flags and deep digest.
//
// There is one top-level test per language so the Docker gate can run each one
// alone (AGENTS.md section 1: one language at a time).

type acceptedLockedCCase struct {
	name string
	// source is used when file is empty.
	source string
	// file is relative to internal/benchfixtures/testdata/real.
	file string
}

func runAcceptedLockedCCases(t *testing.T, lang string, cases []acceptedLockedCCase) {
	t.Helper()
	cases = append(cases, acceptedLockedCCase{name: lang + " smoke sample", source: grammars.ParseSmokeSample(lang)})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := readAcceptedLockedCSource(t, tc)
			cLanguage, err := COracleLanguage(lang)
			if err != nil {
				t.Fatalf("load locked %s language: %v", lang, err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("set locked %s language: %v", lang, err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			if cTree.RootNode().HasError() {
				t.Fatalf("locked C reports an error for %q; the case is not a valid input", tc.name)
			}
			goTree, goLanguage, err := parseWithGo(parityCase{name: lang, source: string(source)}, source, nil)
			if err != nil {
				t.Fatalf("Go parse: %v", err)
			}
			t.Cleanup(func() { releaseGoTree(goTree) })
			assertLockedCTreeExact(t, tc.name, goTree, goLanguage, cTree)
		})
	}
}

func readAcceptedLockedCSource(t *testing.T, tc acceptedLockedCCase) []byte {
	t.Helper()
	if tc.file == "" {
		return []byte(tc.source)
	}
	raw, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", "testdata", "real", tc.file))
	if err != nil {
		t.Fatalf("read real sample: %v", err)
	}
	return raw
}

func TestRealSampleAcceptedLockedCParityAgda(t *testing.T) {
	runAcceptedLockedCCases(t, "agda", []acceptedLockedCCase{
		{name: "agda real sample", file: "agda"},
		{name: "agda parenthesized application", source: "(I U p)"},
		{name: "agda postulate", source: "postulate A : Set\n"},
	})
}

// The locked C parse of the committed djot real sample corrupts the C heap in
// the locked djot scanner artifact: a later C parse in the same process aborts
// with "malloc(): mismatching next->prev_size (unsorted)" in about half of the
// runs (7 of 10 and 13 of 15 when it is followed by the haskell real sample).
// The Go parser never touches the C heap, and the other djot inputs below do
// not trigger it. So the djot real sample is pinned by its locked-C deep
// digest instead of parsed live. The digest was recorded from a live locked-C
// parse on 2026-10-27, and the same run showed the Go tree equal to it. The
// digest also covers the HasError flag of every C node, so a C tree with an
// error would not hash to this value.
//
// To refresh the pin, parse the real sample with the locked djot language in a
// process that does nothing else, and print COracleDeepDigest of the tree.
const (
	djotRealSampleSHA256     = "47ba7463d513fbccb7245504c1eb8526d20bc313079287620ae9fd33208abfa3"
	djotRealSampleLockedCSum = "bc9d5465bb1ae3d8d2f6c179c337b6d0058afb25cdbfe98ac72b20290e737496"
)

func TestRealSampleAcceptedLockedCParityDjotRealSample(t *testing.T) {
	source := readAcceptedLockedCSource(t, acceptedLockedCCase{file: "djot"})
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != djotRealSampleSHA256 {
		t.Fatalf("djot real sample changed: sha256 %s, pinned %s; refresh djotRealSampleLockedCSum", got, djotRealSampleSHA256)
	}
	goTree, goLanguage, err := parseWithGo(parityCase{name: "djot", source: string(source)}, source, nil)
	if err != nil {
		t.Fatalf("Go parse: %v", err)
	}
	t.Cleanup(func() { releaseGoTree(goTree) })
	root := goTree.RootNode()
	if root.HasError() {
		t.Fatal("Go reports an error for the djot real sample; locked C accepts it")
	}
	if int(root.EndByte()) != len(source) {
		t.Fatalf("Go root ends at byte %d, input has %d bytes", root.EndByte(), len(source))
	}
	inspection, err := benchfixtures.InspectGoTree(root, goLanguage)
	if err != nil {
		t.Fatalf("inspect Go deep tree: %v", err)
	}
	if inspection.SHA256 != djotRealSampleLockedCSum {
		t.Fatalf("deep digest Go=%s pinned locked C=%s", inspection.SHA256, djotRealSampleLockedCSum)
	}
}

func TestRealSampleAcceptedLockedCParityDjot(t *testing.T) {
	runAcceptedLockedCCases(t, "djot", []acceptedLockedCCase{
		{name: "djot inline link", source: "[a](http://x.y/)"},
	})
}

func TestRealSampleAcceptedLockedCParityHaskell(t *testing.T) {
	runAcceptedLockedCCases(t, "haskell", []acceptedLockedCCase{
		{name: "haskell real sample", file: "haskell"},
		{name: "haskell three layouts closed at one dedent", source: "t=()where\n R=case d of\n      R->()\n      t->case s of\n        g->()\n        ()->()\nr"},
	})
}

func TestRealSampleAcceptedLockedCParityNorg(t *testing.T) {
	runAcceptedLockedCCases(t, "norg", []acceptedLockedCCase{
		{name: "norg real sample", file: "norg"},
		{name: "norg verbatim tag end at end of input", source: "@g\n@end"},
		{name: "norg verbatim tag end before newline", source: "@code\nfoo\n@end\n"},
		{name: "norg bold at line start", source: "*a b*\n"},
		{name: "norg weak paragraph delimiter", source: "* heading\n---\n"},
	})
}
