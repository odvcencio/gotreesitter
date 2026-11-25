package cquery

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const corpusPath = "testdata/cases.jsonl"

// TestCorpusFile checks corpus 1 as a file: every case is classified and
// carries pinned C's records, and the file is in the form WriteCorpus writes.
// The harness test TestCQueryCorpus in cgo_harness checks the records
// themselves against pinned C.
func TestCorpusFile(t *testing.T) {
	cases, err := ReadCorpus(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(cases); err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]int)
	for i := range cases {
		c := &cases[i]
		for _, id := range c.IDs {
			ids[IDSource(id)]++
		}
		if !strings.HasPrefix(c.Tree, "sha256:") {
			t.Errorf("%s has no tree digest: run TestCQueryCorpus with GTS_CQUERY_UPDATE=1", c.ID())
		}
		accepted := c.Accepted()
		switch {
		case accepted && len(c.L1) == 0:
			t.Errorf("%s: C accepts the query but the corpus has no L1 records", c.ID())
		case !accepted && !strings.HasPrefix(c.Verdict(), "reject "):
			t.Errorf("%s has no verdict", c.ID())
		case !accepted && (len(c.L1) > 0 || len(c.L2) != 1 || len(c.L3) > 0 || len(c.L4) > 0):
			t.Errorf("%s: C rejects the query but the corpus has records after the verdict", c.ID())
		}
	}
	// The review of PR 1734 measured 133 cases; two of them are one case.
	if ids["review"] != 133 {
		t.Errorf("corpus holds %d review ids, want 133", ids["review"])
	}
	t.Logf("%d cases; ids by source: %v", len(cases), ids)

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := filepath.Join(t.TempDir(), "cases.jsonl")
	if err := WriteCorpus(rewritten, cases); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("cases.jsonl is not in the form WriteCorpus writes; rewrite it with TestCQueryCorpus")
	}
}

func TestReadCorpusRejectsDuplicates(t *testing.T) {
	dir := t.TempDir()
	for name, lines := range map[string]string{
		"id":     `{"ids":["review:A"],"language":"json","class":"c","query":"(a)","input":"1"}` + "\n" + `{"ids":["review:A"],"language":"json","class":"c","query":"(b)","input":"1"}`,
		"key":    `{"ids":["review:A"],"language":"json","class":"c","query":"(a)","input":"1"}` + "\n" + `{"ids":["guard:B"],"language":"json","class":"c","query":"(a)","input":"1"}`,
		"source": `{"ids":["A"],"language":"json","class":"c","query":"(a)","input":"1"}`,
		"field":  `{"ids":["review:A"],"language":"json","class":"c","query":"(a)","input":"1","extra":1}`,
	} {
		path := filepath.Join(dir, name+".jsonl")
		if err := os.WriteFile(path, []byte(lines+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadCorpus(path); err == nil {
			t.Errorf("ReadCorpus accepted a corpus with a bad %s", name)
		}
	}
	cases := []Case{{IDs: []string{"guard:A"}, Language: "json", Class: Unclassified, Query: "(a)"}}
	if err := checkCases(cases, false); err != nil {
		t.Fatalf("an unclassified case must read: %v", err)
	}
	if err := Validate(cases); err == nil {
		t.Fatal("Validate accepted an unclassified case")
	}
}
