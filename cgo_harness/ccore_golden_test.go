//go:build cgo && treesitter_c_parity

package cgoharness

// Golden traces for internal/ccore's own tests. Each golden holds a source
// text, the trace pinned C's logger gives for the same source, the visible-tree
// digest, and the hash and line count of C's subtree records. The language's
// tables are written once to testdata/tables/json.json. The test checks that
// ccore equals C on the source (trace, digest, records), then compares the
// goldens with the committed files. CCORE_UPDATE_GOLDEN=1 rewrites the
// committed files.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const ccoreGoldenMaxBytes = 200000

func TestCcoreGolden(t *testing.T) {
	gl, cl := ccoreLanguages(t, "json")
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var clog []string
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	inputs := []struct{ name, src string }{
		{"json-object", "{\"a\": [1, 2.5, true, null], \"b\": {\"c\": \"d\"}}\n"},
		{"json-empty-array", "[]\n"},
		{"json-broken", "{\"a\": [1, 2,, }\n"},
		{"json-open-string", "{\"a"},
		{"json-at-sign", "[1, @]"},
		{"json-invalid-byte", "[\xff]\n"},
		{"json-bom", "\xef\xbb\xbf[]"},
		{"json-bom-invalid-byte", "\xef\xbb\xbf[\xff]\n"},
		{"json-invalid-byte-end", "[1, \xff"},
	}
	root := filepath.Join("..", "internal", "ccore", "testdata")
	dir := filepath.Join(root, "golden")
	tablesDir := filepath.Join(root, "tables")
	update := os.Getenv("CCORE_UPDATE_GOLDEN") != ""
	if update {
		for _, d := range []string{dir, tablesDir} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	total := 0
	writeOrCheck := func(path string, raw []byte) {
		t.Helper()
		total += len(raw)
		if update {
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		old, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run with CCORE_UPDATE_GOLDEN=1)", path, err)
		}
		if !bytes.Equal(old, raw) {
			t.Fatalf("%s: committed file differs from a fresh recording (run with CCORE_UPDATE_GOLDEN=1)", path)
		}
	}
	tablesWritten := false
	for _, in := range inputs {
		clog = clog[:0]
		ct := cp.Parse([]byte(in.src), nil)
		ct.Close()
		golden, err := bindtest.RecordGolden(in.name, gl, []byte(in.src))
		if err != nil {
			t.Fatalf("%s: %v", in.name, err)
		}
		golden.Trace = append([]string(nil), clog...)
		trace, digest, stop, rp, gtree := ccoretest.Replay(golden)
		defer func() {
			if gtree != nil {
				gtree.Release()
			}
		}()
		if stop != golden.Stop || digest != golden.Digest {
			t.Fatalf("%s: replay differs from the recorded parse (stop %q/%q)", in.name, stop, golden.Stop)
		}
		if len(trace) != len(clog) {
			t.Fatalf("%s: trace has %d lines, C has %d", in.name, len(trace), len(clog))
		}
		for i := range trace {
			if trace[i] != clog[i] {
				t.Fatalf("%s: trace differs from C at line %d: %q against %q", in.name, i+1, trace[i], clog[i])
			}
		}
		// C's subtree records: the hash and the line count go into the golden.
		ct = cp.Parse([]byte(in.src), nil)
		ch, cn, rc := corecords.Hash(ct, "")
		ct.Close()
		if rc != 0 {
			t.Fatalf("%s: C records failed (%d)", in.name, rc)
		}
		if gh, gn := rp.Records(gtree, nil); gh != ch || gn != cn {
			t.Fatalf("%s: records hash %016x over %d lines, C has %016x over %d", in.name, gh, gn, ch, cn)
		}
		golden.RecordHash, golden.RecordCount = ch, cn
		if !tablesWritten {
			traw, err := json.Marshal(golden.Tables)
			if err != nil {
				t.Fatal(err)
			}
			writeOrCheck(filepath.Join(tablesDir, "json.json"), traw)
			tablesWritten = true
		}
		golden.TablesRef, golden.Tables = "json", nil
		raw, err := json.Marshal(golden)
		if err != nil {
			t.Fatal(err)
		}
		writeOrCheck(filepath.Join(dir, in.name+".json"), raw)
	}
	ccoreCount("TestCcoreGolden", "json", len(inputs))
	if total > ccoreGoldenMaxBytes {
		t.Fatalf("golden files total %d bytes, limit %d", total, ccoreGoldenMaxBytes)
	}
}
