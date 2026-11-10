//go:build cgo && treesitter_c_parity

package cgoharness

// Edit chains on the core
// against pinned C: trace equality per incremental parse, edited-old-tree
// digests with has_changes, three-way tree equality, the old tree unchanged,
// determinism.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type ccoreIncStep struct {
	id   string
	src  []byte
	edit *bindtest.Edit // nil: fresh parse (start of a chain)
}

func ccorePointAt(src []byte, at int) (uint32, uint32) {
	var row, col uint32
	for i := 0; i < at && i < len(src); i++ {
		if src[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return row, col
}

// ccoreDiffEdit computes the edit between two sources by common prefix and
// suffix, as the archive's edit-session test does.
func ccoreDiffEdit(before, after []byte) *bindtest.Edit {
	start := 0
	for start < len(before) && start < len(after) && before[start] == after[start] {
		start++
	}
	tail := 0
	for tail < len(before)-start && tail < len(after)-start && before[len(before)-1-tail] == after[len(after)-1-tail] {
		tail++
	}
	oldEnd, newEnd := len(before)-tail, len(after)-tail
	e := &bindtest.Edit{StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd)}
	e.StartRow, e.StartCol = ccorePointAt(before, start)
	e.OldEndRow, e.OldEndCol = ccorePointAt(before, oldEnd)
	e.NewEndRow, e.NewEndCol = ccorePointAt(after, newEnd)
	return e
}

func ccoreIncChains(t testing.TB, set string) [][]ccoreIncStep {
	var chains [][]ccoreIncStep
	switch set {
	case "go-session", "json-session", "typescript-session", "tsx-session", "python-session":
		lang := strings.TrimSuffix(set, "-session")
		clean := ccoreManifestSample(t, lang)
		var chain []ccoreIncStep
		chain = append(chain, ccoreIncStep{id: "step0", src: clean})
		prev := clean
		if lang != "json" {
			for i, step := range benchfixtures.EditingSession(clean) {
				chain = append(chain, ccoreIncStep{id: fmt.Sprintf("step%d", i+1), src: step.Source, edit: ccoreDiffEdit(prev, step.Source)})
				prev = step.Source
			}
		} else {
			// A seeded JSON session: half the steps replace a digit with
			// another digit (the source stays valid), half delete, insert or
			// replace one byte at a random position.
			rng := rand.New(rand.NewSource(72))
			for i := 0; i < 72; i++ {
				next := append([]byte(nil), prev...)
				if i%2 == 0 {
					var digits []int
					for k, b := range next {
						if b >= '0' && b <= '9' {
							digits = append(digits, k)
						}
					}
					if len(digits) > 0 {
						k := digits[rng.Intn(len(digits))]
						next[k] = byte('0' + (int(next[k]-'0')+1+rng.Intn(8))%10)
					}
				} else {
					at := rng.Intn(len(next))
					switch rng.Intn(3) {
					case 0:
						next = append(next[:at], next[at+1:]...)
					case 1:
						next = append(next[:at], append([]byte{"x,:]{\" "[rng.Intn(7)]}, next[at:]...)...)
					default:
						next[at] = "x,:]{\" "[rng.Intn(7)]
					}
				}
				chain = append(chain, ccoreIncStep{id: fmt.Sprintf("step%d", i+1), src: next, edit: ccoreDiffEdit(prev, next)})
				prev = next
			}
		}
		chains = append(chains, chain)
	case "go-generic":
		for ni, name := range []string{"F", "p.G"} {
			for ti, ty := range []string{"T", "int", "*T", "p.T"} {
				for ci, context := range []string{"_ = %s", "return %s", "if true { _ = %s }"} {
					var chain []ccoreIncStep
					var prev []byte
					for step, args := range []string{"x", "x,y", "", "z", "F[T](x)", "x"} {
						src := []byte("package p\nfunc f() { " + fmt.Sprintf(context, name+"["+ty+"]("+args+")") + " }\n")
						st := ccoreIncStep{id: fmt.Sprintf("n%d_t%d_c%d/step%d", ni, ti, ci, step), src: src}
						if prev != nil {
							st.edit = ccoreDiffEdit(prev, src)
						}
						chain = append(chain, st)
						prev = src
					}
					chains = append(chains, chain)
				}
			}
		}
	}
	return chains
}

func ccoreCDigestChanges(tree *sitter.Tree, changes bool) string {
	h := sha256.New()
	cursor := tree.RootNode().Walk()
	defer cursor.Close()
	var buf []byte
	write := func(n *sitter.Node) {
		s, e := n.StartPosition(), n.EndPosition()
		buf = bindtest.DigestRecord(buf[:0], n.Kind(), uint32(n.StartByte()), uint32(n.EndByte()), uint32(s.Row), uint32(s.Column), uint32(e.Row), uint32(e.Column), n.IsNamed(), n.IsExtra(), n.IsMissing(), changes && n.HasChanges(), uint32(n.ChildCount()))
		h.Write(buf)
	}
	write(cursor.Node())
	for {
		if cursor.GotoFirstChild() {
			write(cursor.Node())
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				if tree.RootNode().HasError() {
					h.Write([]byte{1})
				} else {
					h.Write([]byte{0})
				}
				return hex.EncodeToString(h.Sum(nil))
			}
		}
		write(cursor.Node())
	}
}

func ccoreSitterEdit(e *bindtest.Edit) *sitter.InputEdit {
	return &sitter.InputEdit{
		StartByte: uint(e.StartByte), OldEndByte: uint(e.OldEndByte), NewEndByte: uint(e.NewEndByte),
		StartPosition:  sitter.Point{Row: uint(e.StartRow), Column: uint(e.StartCol)},
		OldEndPosition: sitter.Point{Row: uint(e.OldEndRow), Column: uint(e.OldEndCol)},
		NewEndPosition: sitter.Point{Row: uint(e.NewEndRow), Column: uint(e.NewEndCol)},
	}
}

// TestCcoreIncremental runs the edit chains of CCORE_INC_SET.
func TestCcoreIncremental(t *testing.T) {
	set := os.Getenv("CCORE_INC_SET")
	if set == "" {
		t.Skip("set CCORE_INC_SET")
	}
	langName := "go"
	for _, l := range []string{"json", "typescript", "tsx", "python"} {
		if strings.HasPrefix(set, l) {
			langName = l
		}
	}
	gl, cl := ccoreLanguages(t, langName)
	newC := func() *sitter.Parser {
		cp := sitter.NewParser()
		if err := cp.SetLanguage(cl); err != nil {
			t.Fatal(err)
		}
		return cp
	}
	cp, cfresh := newC(), newC()
	defer cp.Close()
	defer cfresh.Close()
	var clog []string
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	steps := 0
	for _, chain := range ccoreIncChains(t, set) {
		core := ccoreNew(gl)
		fresh := ccoreNew(gl)
		var ctree *sitter.Tree
		var tree bindtest.Tree
		for _, st := range chain {
			steps++
			clog = clog[:0]
			var slog []string
			logf := func(s string) { slog = append(slog, s) }
			editEq, oldKept, det := true, true, true
			recEq, recLines, recFirst := true, uint64(0), -1
			var stop string
			var editNs, parseNs int64
			if st.edit == nil {
				if ctree != nil {
					ctree.Close()
				}
				ctree = cp.Parse(st.src, nil)
				stop = core.Parse(st.src, logf)
				tree = core.Tree()
			} else {
				ctree.Edit(ccoreSitterEdit(st.edit))
				cEdited := ccoreCDigestChanges(ctree, true)
				t0 := time.Now()
				edited := core.Edit(tree, *st.edit)
				editNs = time.Since(t0).Nanoseconds()
				sEdited := core.DigestOf(edited, true)
				editEq = cEdited == sEdited
				clog = clog[:0]
				next := cp.Parse(st.src, ctree)
				ctree.Close()
				ctree = next
				t1 := time.Now()
				tree, stop = core.ParseIncremental(st.src, edited, logf)
				parseNs = time.Since(t1).Nanoseconds()
				oldKept = core.DigestOf(edited, true) == sEdited
				// record check against C's incremental tree at this step
				ch, cn, rc := corecords.Hash(ctree, "")
				gh, gn := core.RecordsHash("")
				recEq = rc == 0 && ch == gh && cn == gn
				recLines = gn
				if !recEq && os.Getenv("CCORE_REC_OUT") != "" {
					dir := os.Getenv("CCORE_REC_OUT")
					os.MkdirAll(dir, 0o755)
					cp_, gp_ := dir+"/"+strings.ReplaceAll(set+"_"+st.id, "/", "_")+".c.txt", dir+"/"+strings.ReplaceAll(set+"_"+st.id, "/", "_")+".go.txt"
					corecords.Hash(ctree, cp_)
					core.RecordsHash(gp_)
					cl, gl2 := ccoreRecRead(cp_), ccoreRecRead(gp_)
					for i := 0; i < len(cl) || i < len(gl2); i++ {
						if i >= len(cl) || i >= len(gl2) || cl[i] != gl2[i] {
							recFirst = i
							for k := i - 3; k <= i; k++ {
								if k >= 0 && k < len(cl) && k < len(gl2) {
									fmt.Printf("CCINCREC   %d C: %s\nCCINCREC   %d G: %s\n", k, cl[k], k, gl2[k])
								}
							}
							break
						}
					}
				}
				again, againStop := core.ParseIncremental(st.src, edited, nil)
				det = againStop == stop && core.DigestOf(again, false) == core.DigestOf(tree, false)
			}
			first := -1
			for i := 0; i < len(clog) || i < len(slog); i++ {
				if i >= len(clog) || i >= len(slog) || ccoreNormLine(clog[i]) != ccoreNormLine(slog[i]) {
					first = i
					break
				}
			}
			cInc := ccoreCDigestChanges(ctree, false)
			sInc := core.DigestOf(tree, false)
			fstop := fresh.Parse(st.src, nil)
			sFresh := fresh.Digest()
			cf := cfresh.Parse(st.src, nil)
			cFresh := ccoreCDigestChanges(cf, false)
			cerr := cf.RootNode().HasError()
			cf.Close()
			nodes, bytes, largest := core.ReuseStats()
			extRej, extLex := 0, 0
			for _, l := range slog {
				if strings.HasPrefix(l, "reusable_node_has_different_external_scanner_state") {
					extRej++
				}
				if strings.HasPrefix(l, "lex_external") {
					extLex++
				}
			}
			fmt.Printf("CCINC set=%s input=%s edit=%t c_error=%t stop=%q fresh_stop=%q edit_eq=%t old_kept=%t deterministic=%t trace_eq=%t first_diff=%d inc_eq_c=%t inc_eq_fresh=%t fresh_eq_cfresh=%t cinc_eq_cfresh=%t rec_eq=%t rec_lines=%d rec_first=%d ext_rejects=%d lex_external=%d reused=%d reused_bytes=%d largest=%d edit_ns=%d parse_ns=%d\n",
				set, st.id, st.edit != nil, cerr, stop, fstop, editEq, oldKept, det, first < 0, first, sInc == cInc, sInc == sFresh, sFresh == cFresh, cInc == cFresh, recEq, recLines, recFirst, extRej, extLex, nodes, bytes, largest, editNs, parseNs)
			// Every parse of the step must finish (no stop text), the core's fresh
			// parse must equal C's fresh parse, and the incremental parse must
			// equal both C's incremental parse and the core's fresh parse.
			ccoreExpect(t, stop == "" && fstop == "" && first < 0 && len(clog) > 0 && sInc == cInc && sInc == sFresh && sFresh == cFresh && editEq && oldKept && det && recEq,
				"%s/%s: incremental parse differs (stop %q, fresh_stop %q, trace_eq %t, inc_eq_c %t, inc_eq_fresh %t, fresh_eq_cfresh %t, edit_eq %t, old_kept %t, deterministic %t, rec_eq %t, c_log_lines %d)",
				set, st.id, stop, fstop, first < 0, sInc == cInc, sInc == sFresh, sFresh == cFresh, editEq, oldKept, det, recEq, len(clog))
			if first >= 0 {
				for i := first - 2; i < first+3; i++ {
					if i < 0 {
						continue
					}
					c, s := "<none>", "<none>"
					if i < len(clog) {
						c = clog[i]
					}
					if i < len(slog) {
						s = slog[i]
					}
					fmt.Printf("CCINC   %d C: %s\nCCINC   %d S: %s\n", i, c, i, s)
				}
			}
		}
		if ctree != nil {
			ctree.Close()
			ctree = nil
		}
	}
	ccoreCount("TestCcoreIncremental", set, steps)
}
