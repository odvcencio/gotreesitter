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
	type streamCase struct {
		name         string
		documents    int
		owned        bool
		prefix       string
		sampleSource bool
	}
	tests := []streamCase{
		{"one owned document", 1, true, "", false},
		{"four ordinary documents", 4, false, "", false},
		{"four owned documents", 4, true, "", false},
		{"sixteen owned documents", 16, true, "", false},
		{"owned sample with leading padding", 1, true, "  ", true},
		{"owned sample with leading newline", 1, true, "\n  ", true},
	}
	if os.Getenv("GTS_YAML_LARGE_PROBE") == "1" {
		tests = append(tests,
			streamCase{"large ordinary documents", 524, false, "", false},
			streamCase{"large owned documents", 524, true, "", false},
		)
	}
	for _, test := range tests {
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

	t.Run("scanner_cursor_transitions", func(t *testing.T) {
		sources := []string{
			"key: value\n",
			"key: value\r\nnext: ok\r\n",
			"key:\tvalue\n",
			"# comment with : and #\nkey: value # tail\n",
			"key: \"value: # \\t end\"\n",
			"key: 'value it''s quoted'\n",
			"key: |+\n  value\n  another line\n\n",
			"key: >-\n  value\n  continued\n",
			"[value, {nested: [one, two]}, three]\n",
			"{key: value, empty: null}\n",
			"key: &anchor value\nref: *anchor\n",
			"key: !!str value\n",
			"key: value café Ελληνικά 日本語 😀\n",
			"---\nkey: value\n...\n---\nnext: other\n",
			"key:\n  - value\n  - nested:\n      child: other\n",
			"key: value",
		}
		for index, text := range sources {
			t.Run(fmt.Sprint(index), func(t *testing.T) {
				source := []byte(text)
				ct := compactT3ParseC(t, cLang, source)
				defer ct.Close()
				if ct.RootNode().HasError() {
					t.Fatal("locked C rejected cursor fixture")
				}
				for _, route := range []struct {
					name             string
					candidate, owned bool
				}{
					{"legacy", false, false}, {"candidate", true, false}, {"owned", true, true},
				} {
					t.Run(route.name, func(t *testing.T) {
						t.Cleanup(core.SetExternalLexerOwnershipProbeForTest(route.owned))
						parser := gts.NewParser(lang)
						parser.SetAdmissionCandidateRoute(route.candidate)
						tree, err := parser.Parse(source)
						if err != nil || tree == nil {
							t.Fatalf("initial parse: %v", err)
						}
						defer tree.Release()
						assertLockedCTreeExactWithErrors(t, "scanner cursor initial", tree, lang, ct)
						at := bytes.Index(source, []byte("value"))
						if at < 0 {
							t.Fatal("fixture lacks edit marker")
						}
						edited := bytes.Clone(source)
						edited[at] = 'w'
						point := gts.Point{Row: uint32(bytes.Count(source[:at], []byte{'\n'})), Column: uint32(at - bytes.LastIndexByte(source[:at], '\n') - 1)}
						end := point
						end.Column++
						tree.Edit(gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1), StartPoint: point, OldEndPoint: end, NewEndPoint: end})
						incremental, err := parser.ParseIncremental(edited, tree)
						if err != nil || incremental == nil {
							t.Fatalf("incremental parse: %v", err)
						}
						defer incremental.Release()
						editedC := compactT3ParseC(t, cLang, edited)
						defer editedC.Close()
						assertLockedCTreeExactWithErrors(t, "scanner cursor edited", incremental, lang, editedC)
						freshParser := gts.NewParser(lang)
						freshParser.SetAdmissionCandidateRoute(route.candidate)
						fresh, err := freshParser.Parse(edited)
						if err != nil || fresh == nil {
							t.Fatalf("edited fresh parse: %v", err)
						}
						defer fresh.Release()
						assertLockedCTreeExactWithErrors(t, "scanner cursor edited fresh", fresh, lang, editedC)
					})
				}
			})
		}
	})
}
