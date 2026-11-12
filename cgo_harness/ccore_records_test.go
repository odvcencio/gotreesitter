//go:build cgo && treesitter_c_parity

package cgoharness

// Record equality with pinned C: every subtree record, hidden nodes included,
// hashed field by field (lines from internal/ccore/records.go and
// cgo_harness/internal/corecords/records.go). Uses ccoreInputs and ccoreLanguages of
// ccore_c_test.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func ccoreRecFields(line string) map[string]string {
	m := map[string]string{}
	for _, tok := range strings.Fields(line) {
		if k, v, ok := strings.Cut(tok, "="); ok {
			m[k] = v
		}
	}
	return m
}

// ccoreRecOffsets gives the byte offset (after padding) of line idx's node.
func ccoreRecOffset(lines []string, idx int) int {
	next := make([]int, 4096)
	off := 0
	for i := 0; i <= idx && i < len(lines); i++ {
		f := ccoreRecFields(lines[i])
		d, _ := strconv.Atoi(f["d"])
		pad, _ := strconv.Atoi(strings.Split(f["pad"], ",")[0])
		size, _ := strconv.Atoi(strings.Split(f["size"], ",")[0])
		start := 0
		if d > 0 {
			start = next[d-1]
			next[d-1] = start + pad + size
		}
		if d < len(next) {
			next[d] = start
		}
		off = start + pad
	}
	return off
}

func ccoreRecRead(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

type ccoreRecClass struct {
	inputs   int
	smallest string
	bytes    int
}

// TestCcoreRecords compares subtree records with pinned C.
func TestCcoreRecords(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	outDir := os.Getenv("CCORE_REC_OUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	os.MkdirAll(filepath.Join(outDir, "dumps"), 0o755)
	gl, cl := ccoreLanguages(t, name)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	spike := ccoreNew(gl)
	classes := map[string]*ccoreRecClass{}
	fieldInputs := map[string]int{}
	var total, equal int
	checked := 0
	for _, in := range ccoreInputs(t, name, false) {
		checked++
		total++
		ct := cp.Parse(in.Src, nil)
		ch, cn, rc := corecords.Hash(ct, "")
		ct.Close()
		stop := spike.Parse(in.Src, nil)
		gh, gn := spike.RecordsHash("")
		spike.ReleaseLast()
		spike.Parse(in.Src, nil)
		gh2, gn2 := spike.RecordsHash("")
		spike.ReleaseLast()
		det := gh == gh2 && gn == gn2
		same := rc == 0 && stop == "" && ch == gh && cn == gn
		if same {
			equal++
		}
		if os.Getenv("CCORE_REC_COUNT") != "" {
			ct = cp.Parse(in.Src, nil)
			tmp := filepath.Join(outDir, "count.c.txt")
			corecords.Hash(ct, tmp)
			ct.Close()
			var ne, nc, nk, nl int
			for _, l := range ccoreRecRead(tmp) {
				nl++
				if strings.Contains(l, " ext=1 ") {
					ne++
				}
				if strings.Contains(l, " extchg=1 ") {
					nc++
				}
				if strings.Contains(l, " col=1 ") {
					nk++
				}
			}
			os.Remove(tmp)
			fmt.Printf("CCRECCOUNT input=%s lines=%d ext1=%d extchg1=%d col1=%d\n", in.ID, nl, ne, nc, nk)
		}
		fmt.Printf("CCREC input=%s bytes=%d c_lines=%d go_lines=%d c_hash=%016x go_hash=%016x equal=%t deterministic=%t stop=%q\n", in.ID, len(in.Src), cn, gn, ch, gh, same, det, stop)
		ccoreExpect(t, same && det, "%s: the subtree records differ from C (equal %t, deterministic %t)", in.ID, same, det)
		if same {
			continue
		}
		safe := strings.NewReplacer("/", "_", " ", "_").Replace(in.ID)
		cpath := filepath.Join(outDir, "dumps", safe+".c.txt")
		gpath := filepath.Join(outDir, "dumps", safe+".go.txt")
		ct = cp.Parse(in.Src, nil)
		corecords.Hash(ct, cpath)
		ct.Close()
		spike.Parse(in.Src, nil)
		spike.RecordsHash(gpath)
		spike.ReleaseLast()
		cLines, gLines := ccoreRecRead(cpath), ccoreRecRead(gpath)
		first := -1
		diffLines := 0
		fieldSet := map[string]int{}
		var firstKeys []string
		for i := 0; i < len(cLines) || i < len(gLines); i++ {
			if i >= len(cLines) || i >= len(gLines) {
				diffLines++
				if first < 0 {
					first, firstKeys = i, []string{"line-count"}
				}
				continue
			}
			if cLines[i] == gLines[i] {
				continue
			}
			diffLines++
			cf, gf := ccoreRecFields(cLines[i]), ccoreRecFields(gLines[i])
			var keys []string
			for k, v := range cf {
				if gf[k] != v {
					keys = append(keys, k)
				}
			}
			for k := range gf {
				if _, ok := cf[k]; !ok {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				fieldSet[k]++
			}
			if first < 0 {
				first, firstKeys = i, keys
			}
		}
		class := strings.Join(firstKeys, "+")
		if stop != "" {
			class = "go-stop:" + stop
		}
		var all []string
		for k, n := range fieldSet {
			all = append(all, fmt.Sprintf("%s:%d", k, n))
			fieldInputs[k]++
		}
		sort.Strings(all)
		fmt.Printf("CCRECDIFF input=%s class=%s first_diff_line=%d differing_lines=%d of c=%d go=%d offset=%d fields=%s\n", in.ID, class, first, diffLines, len(cLines), len(gLines), ccoreRecOffset(cLines, first), strings.Join(all, ","))
		if first >= 0 {
			for i := first - 3; i < first; i++ {
				if i >= 0 && i < len(cLines) {
					fmt.Printf("CCRECDIFF   %d  : %s\n", i, cLines[i])
				}
			}
			c, s := "<none>", "<none>"
			if first < len(cLines) {
				c = cLines[first]
			}
			if first < len(gLines) {
				s = gLines[first]
			}
			fmt.Printf("CCRECDIFF   %d C: %s\nCCRECDIFF   %d G: %s\n", first, c, first, s)
		}
		cls := classes[class]
		if cls == nil {
			cls = &ccoreRecClass{bytes: -1}
			classes[class] = cls
		}
		cls.inputs++
		if cls.bytes < 0 || len(in.Src) < cls.bytes {
			if cls.smallest != "" {
				old := strings.NewReplacer("/", "_", " ", "_").Replace(cls.smallest)
				os.Remove(filepath.Join(outDir, "dumps", old+".c.txt"))
				os.Remove(filepath.Join(outDir, "dumps", old+".go.txt"))
			}
			cls.smallest, cls.bytes = in.ID, len(in.Src)
		} else {
			os.Remove(cpath)
			os.Remove(gpath)
		}
	}
	for c, cls := range classes {
		fmt.Printf("CCRECCLASS class=%s inputs=%d smallest=%s smallest_bytes=%d\n", c, cls.inputs, cls.smallest, cls.bytes)
	}
	for k, n := range fieldInputs {
		fmt.Printf("CCRECFIELD field=%s inputs=%d\n", k, n)
	}
	fmt.Printf("CCRECTOTAL lang=%s inputs=%d equal=%d differing=%d\n", name, total, equal, total-equal)
	fmt.Printf("CCSTOREINT test=records lang=%s shared_in_tree=%d double_free=%d bad_retain=%d\n", name, spike.SharedInTree(), spike.DoubleFree(), spike.BadRetain())
	ccoreExpectStore(t, "TestCcoreRecords "+name, spike)
	ccoreCount("TestCcoreRecords", name, checked)
}
