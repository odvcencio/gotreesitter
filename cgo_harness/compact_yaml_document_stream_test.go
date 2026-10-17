//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Four documents reproduced the owned EOF frontier cap on 8 KiB. Keep compact
// acceptance and the complete locked-C structure pinned together.
func TestCompactYAMLDocumentStreamLockedC(t *testing.T) {
	sample, err := os.ReadFile("../internal/benchfixtures/testdata/real/yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(sample) != 1998 || fmt.Sprintf("%x", sha256.Sum256(sample)) != "5aa2c70bccfe73254f13eca40476c3f88f522c6a47aea499f9d672dbc5aae8c4" {
		t.Fatal("YAML R4 sample identity changed")
	}
	document := append([]byte("---\n"), sample...)
	document = append(document, '\n')
	lang := grammars.YamlLanguage()
	cLang, err := ParityCLanguage("yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		documents    int
		owned        bool
		prefix       string
		sampleSource bool
	}{
		{"one owned document", 1, true, "", false},
		{"four ordinary documents", 4, false, "", false},
		{"four owned documents", 4, true, "", false},
		{"sixteen owned documents", 16, true, "", false},
		{"owned sample with leading padding", 1, true, "  ", true},
		{"owned sample with leading newline", 1, true, "\n  ", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Cleanup(core.SetExternalLexerOwnershipProbeForTest(test.owned))
			source := bytes.Repeat(document, test.documents)
			if test.sampleSource {
				source = sample
			}
			source = append([]byte(test.prefix), source...)
			cTree := compactT3ParseC(t, cLang, source)
			defer cTree.Close()
			if cTree.RootNode().HasError() {
				t.Fatal("locked C rejected the document-stream fixture")
			}
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			beforeRouted, beforeFallback := gts.AdmissionCandidateCounters()
			tree, err := p.Parse(source)
			if err != nil || tree == nil || tree.RootNode() == nil {
				t.Fatalf("parse: tree=%v err=%v", tree != nil, err)
			}
			defer tree.Release()
			routed, fallback := gts.AdmissionCandidateCounters()
			routed, fallback = routed-beforeRouted, fallback-beforeFallback
			reason := ""
			if fallback != 0 {
				reason = gts.AdmissionCandidateLastFallbackReason()
			}
			t.Logf("documents=%d bytes=%d sha256=%x compact=%d fallback=%d reason=%q", test.documents, len(source), sha256.Sum256(source), routed, fallback, reason)
			if routed+fallback != 1 || routed > 1 || fallback > 1 {
				t.Fatal("document-stream route counters are inconsistent")
			}
			if test.owned && (routed != 1 || fallback != 0) {
				t.Fatal("the owned document stream lost compact acceptance")
			}
			if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
				t.Fatal("document stream is incomplete or erroneous")
			}
			if diffs := compactT3StructuralDivergences(tree.RootNode(), lang, cTree.RootNode()); len(diffs) != 0 {
				for _, diff := range diffs[:min(len(diffs), 8)] {
					t.Error(compactT3FormatDivergence(diff))
				}
				t.Fatalf("%d document-stream structural differences", len(diffs))
			}
		})
	}
}
