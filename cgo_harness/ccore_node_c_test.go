//go:build cgo && treesitter_c_parity

package cgoharness

// Value-node readers of internal/ccore against pinned C (C/node.c): for every
// node of every input of one language, symbol name, named, extra, missing,
// error, has_error, has_changes, start and end (byte and point), child count,
// named child count, ts_node_child and ts_node_named_child for every index (for
// the first and last 64 indices and every 97th one in between when a node has
// more than 256 children), the index one past the end, and the field name of
// every child (ts_node_field_name_for_child and _for_named_child). The one-pass
// Children reader must equal the Child reader node for node, and a named child
// must be the child at its ordinal. CCORE_LANG=<language>.

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type ccoreNodeCheck struct {
	t            *testing.T
	rd           ccore.NodeReader
	names        []string
	fields       []string
	cur          *sitter.TreeCursor
	nodes        int
	checks       int
	diffs        int
	idDiff       int
	pastEndNodes int
	indirect     int // named results whose C visible parent is not the queried node
	maxPhysical  int // most physical children of one record read
	nonzeroMasks int // children whose supertype mask is not zero
	cSuperKids   int // cursor positions where C reported at least one supertype symbol
	largeLists   int // records whose child list has a page of its own
	rng          *rand.Rand
	shown        int
	input        string
}

func (c *ccoreNodeCheck) fail(path string, format string, args ...any) {
	c.diffs++
	if c.shown < 12 {
		c.shown++
		c.t.Logf("CCNODEDIFF input=%s path=%s %s", c.input, path, fmt.Sprintf(format, args...))
	}
}

func (c *ccoreNodeCheck) eq(path, what string, got, want any) {
	c.checks++
	if got != want {
		c.fail(path, "%s: core %v, C %v", what, got, want)
	}
}

// typeName is the symbol's name as C prints it, with backslashes removed on
// both sides (the Go table escapes a few names; CCORE_NORM_NAMES does the same
// in the other equality tests). The NUL token's name holds a NUL byte, which
// C's string ends at; the error symbol is named ERROR.
func (c *ccoreNodeCheck) typeName(sym ccore.Symbol) string {
	if sym == 0xFFFF {
		return "ERROR"
	}
	if int(sym) >= len(c.names) {
		return fmt.Sprintf("?%d", sym)
	}
	if c.names[sym] == `\0` {
		return ""
	}
	return strings.ReplaceAll(c.names[sym], `\`, "")
}

func (c *ccoreNodeCheck) fieldName(id uint16) string {
	if id == 0 {
		return ""
	}
	if int(id) < len(c.fields) {
		return c.fields[id]
	}
	return fmt.Sprintf("?%d", id)
}

// scalars compares what one node answers by itself.
func (c *ccoreNodeCheck) scalars(path string, cn *sitter.Node, n ccore.Node) {
	rd := c.rd
	c.eq(path, "type", c.typeName(rd.Symbol(n)), strings.ReplaceAll(cn.Kind(), `\`, ""))
	sym := rd.Symbol(n)
	c.checks++
	if uint16(cn.KindId()) != uint16(sym) {
		c.idDiff++ // the public symbol map merges duplicate names; the name is equal, so this is not a difference
	}
	c.eq(path, "named", rd.Named(n), cn.IsNamed())
	c.eq(path, "extra", rd.Extra(n), cn.IsExtra())
	c.eq(path, "missing", rd.Missing(n), cn.IsMissing())
	c.eq(path, "error", rd.IsError(n), cn.IsError())
	c.eq(path, "has_error", rd.HasError(n), cn.HasError())
	c.eq(path, "has_changes", rd.HasChanges(n), cn.HasChanges())
	c.eq(path, "start_byte", uint(rd.StartByte(n)), cn.StartByte())
	c.eq(path, "end_byte", uint(rd.EndByte(n)), cn.EndByte())
	sr, sc := rd.StartPoint(n)
	sp := cn.StartPosition()
	c.eq(path, "start_row", uint(sr), sp.Row)
	c.eq(path, "start_col", uint(sc), sp.Column)
	e := rd.End(n)
	ep := cn.EndPosition()
	c.eq(path, "end_row", uint(e.Row()), ep.Row)
	c.eq(path, "end_col", uint(e.Col()), ep.Column)
	c.eq(path, "parse_state", uint(rd.ParseState(n)), uint(cn.ParseState()))
	c.eq(path, "grammar_symbol", uint(rd.GrammarSymbol(n)), uint(cn.GrammarId()))
	c.eq(path, "not null", n.Null(), false)
	c.eq(path, "child_count", uint(rd.ChildCount(n)), cn.ChildCount())
	c.eq(path, "named_child_count", uint(rd.NamedChildCount(n)), cn.NamedChildCount())
}

// supertypeMask maps the supertype symbols C reports at the cursor to the core's mask bits.
func (c *ccoreNodeCheck) supertypeMask(cur *sitter.TreeCursor) uint32 {
	var m uint32
	syms := corecords.Supertypes(cur)
	if len(syms) > 0 {
		c.cSuperKids++
	}
	for _, sym := range syms {
		// C reports a supertype symbol the core's table has no bit for, or no entry at
		// all (a table shorter than the symbol): the table, not the reader, is wrong.
		if int(sym) >= len(c.rd.T.SuperBit) {
			c.fail("", "supertype symbol %d reported by C is beyond the core's bit table (length %d)", sym, len(c.rd.T.SuperBit))
			continue
		}
		if c.rd.T.SuperBit[sym] == 0 {
			c.fail("", "supertype symbol %d reported by C has no bit in the core's table", sym)
		}
		m |= c.rd.T.SuperBit[sym]
	}
	return m
}

// meta checks the ordinal, the cursor field and the supertype mask of a node
// that a Child or NamedChild call returned, against C's cursor placed on the
// same node among the children of its visible parent (ts_node_parent).
func (c *ccoreNodeCheck) meta(path, what string, got ccore.Node, ccn *sitter.Node) {
	par := ccn.Parent()
	if par == nil {
		return
	}
	cur := par.Walk()
	defer cur.Close()
	cur.GotoFirstChild()
	for i := uint(0); ; i++ {
		if cur.Node().Id() == ccn.Id() {
			c.eq(path, what+" ordinal", uint(got.Ordinal), i)
			c.eq(path, what+" cursor field", uint(got.CField), uint(cur.FieldId()))
			c.eq(path, what+" supertype mask", got.Mask, c.supertypeMask(cur))
			return
		}
		if !cur.GotoNextSibling() {
			c.fail(path, "%s: the node is not among its parent's children in C", what)
			return
		}
	}
}

// pastEnd checks the index one past the end: C finds no node, except where an
// anonymous visible node holds named children that its parent's count leaves
// out (hurl); the core must give C's answer, with the metadata, either way.
func (c *ccoreNodeCheck) pastEnd(path, what string, got ccore.Node, ok bool, want *sitter.Node, fieldWant string) {
	if want == nil {
		c.eq(path, what+" is none", ok, false)
		c.eq(path, what+" none is the null node", got.Null(), true)
		return
	}
	c.pastEndNodes++
	if c.same(path, what, got, ok, want) {
		c.meta(path, what, got, want)
		c.eq(path, what+" field name", c.fieldName(got.Field), fieldWant)
	}
}

// shape compares two nodes that must be the same node (scalars and position).
func (c *ccoreNodeCheck) same(path, what string, got ccore.Node, ok bool, want *sitter.Node) bool {
	c.checks++
	if want == nil {
		if ok {
			c.fail(path, "%s: core has a node, C has none", what)
		}
		return false
	}
	if !ok {
		c.fail(path, "%s: C has a node, core has none", what)
		return false
	}
	rd := c.rd
	c.eq(path, what+" start_byte", uint(rd.StartByte(got)), want.StartByte())
	c.eq(path, what+" end_byte", uint(rd.EndByte(got)), want.EndByte())
	c.eq(path, what+" type", c.typeName(rd.Symbol(got)), strings.ReplaceAll(want.Kind(), `\`, ""))
	return true
}

func (c *ccoreNodeCheck) walk(path string, cn *sitter.Node, n ccore.Node) {
	c.nodes++
	c.scalars(path, cn, n)
	rd := c.rd
	count := uint(rd.ChildCount(n))
	named := uint(rd.NamedChildCount(n))
	kids, hiddenField := rd.ChildrenHidden(n, nil)
	c.eq(path, "len(Children)", uint(len(kids)), count)
	_ = hiddenField // no C counterpart: it is checked through the field answers of the views
	if _, phys := rd.S.ChildSlots(n.H); int(phys) > c.maxPhysical {
		c.maxPhysical = int(phys)
	}
	if list, _ := rd.S.ChildSlots(n.H); ccore.LargeSlot(list) {
		c.largeLists++
	}
	// the one-past-the-end index
	// The index one past the end: C finds no node, except where an anonymous
	// visible node holds named children that its parent's count leaves out
	// (hurl); the core must give C's answer either way.
	pg, ok := rd.Child(n, uint32(count))
	c.pastEnd(path, "child(count)", pg, ok, cn.Child(count), cn.FieldNameForChild(uint32(count)))
	pg, ok = rd.NamedChild(n, uint32(named))
	c.pastEnd(path, "named_child(count)", pg, ok, cn.NamedChild(named), cn.FieldNameForNamedChild(uint32(named)))
	// The field of every child as the tree cursor reports it
	// (ts_tree_cursor_current_field_id), which differs from the node answer below an extra hidden node.
	if count > 0 {
		c.cur.Reset(*cn)
		c.cur.GotoFirstChild()
		for i := uint(0); i < count && i < uint(len(kids)); i++ {
			c.eq(path, fmt.Sprintf("cursor field id of child %d", i), uint(kids[i].CField), uint(c.cur.FieldId()))
			c.eq(path, fmt.Sprintf("supertype mask of child %d", i), kids[i].Mask, c.supertypeMask(c.cur))
			if kids[i].Mask != 0 {
				c.nonzeroMasks++
			}
			c.cur.GotoNextSibling()
		}
	}
	// C's children, in order
	var ckids []sitter.Node
	if count > 8192 {
		ckids = cn.Children(c.cur)
	}
	sample := func(i, total uint) bool { return total <= 8192 || i < 64 || i+64 >= total || i%97 == 0 }
	for i := uint(0); i < count && i < uint(len(kids)); i++ {
		var ccn *sitter.Node
		if ckids != nil {
			if i >= uint(len(ckids)) {
				c.fail(path, "C Children has %d nodes, child_count %d", len(ckids), count)
				break
			}
			ccn = &ckids[i]
		} else {
			ccn = cn.Child(i)
		}
		kid := kids[i]
		if sample(i, count) {
			got, ok := rd.Child(n, uint32(i))
			if c.same(path, fmt.Sprintf("child(%d)", i), got, ok, ccn) {
				c.eq(path, fmt.Sprintf("child(%d) equals Children[%d]", i, i), got, kid)
			}
			cf := cn.FieldNameForChild(uint32(i))
			c.eq(path, fmt.Sprintf("field_name_for_child(%d)", i), c.fieldName(kid.Field), cf)
		}
		if ccn == nil {
			continue
		}
		c.eq(path, fmt.Sprintf("Children[%d] ordinal", i), uint(kid.Ordinal), i)
		c.walk(fmt.Sprintf("%s/%d", path, i), ccn, kid)
	}
	forwardNamed := map[uint]ccore.Node{}
	for j := uint(0); j < named; j++ {
		if !sample(j, named) {
			continue
		}
		got, ok := rd.NamedChild(n, uint32(j))
		ccn := cn.NamedChild(j)
		if c.same(path, fmt.Sprintf("named_child(%d)", j), got, ok, ccn) {
			forwardNamed[j] = got
			// A named result is a direct visible child only when C's visible parent of it is
			// the queried node; below an anonymous visible node it is not.
			if par := ccn.Parent(); par != nil && par.Id() == cn.Id() {
				if int(got.Ordinal) < len(kids) {
					c.eq(path, fmt.Sprintf("named_child(%d) equals Children[ordinal]", j), got, kids[got.Ordinal])
				} else {
					c.fail(path, "named_child(%d) ordinal %d past %d children", j, got.Ordinal, len(kids))
				}
			} else {
				c.indirect++
			}
			c.meta(path, fmt.Sprintf("named_child(%d)", j), got, ccn)
			c.eq(path, fmt.Sprintf("field_name_for_named_child(%d)", j), c.fieldName(got.Field), cn.FieldNameForNamedChild(uint32(j)))
		}
	}
	// The readers keep no state: the same answers in reverse and in a shuffled order.
	if count <= 256 && named <= 256 {
		for _, order := range [][]int{reversed(int(count)), c.rng.Perm(int(count))} {
			for _, i := range order {
				got, ok := rd.Child(n, uint32(i))
				c.eq(path, fmt.Sprintf("replayed child(%d)", i), ok && got == kids[i], true)
			}
		}
		for _, order := range [][]int{reversed(int(named)), c.rng.Perm(int(named))} {
			for _, j := range order {
				got, ok := rd.NamedChild(n, uint32(j))
				c.eq(path, fmt.Sprintf("replayed named_child(%d)", j), ok && got == forwardNamed[uint(j)], true)
			}
		}
	}
}

func reversed(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = n - 1 - i
	}
	return out
}

// TestCcoreNodes compares the core's node readers with pinned C.
func TestCcoreNodes(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	gl, cl := ccoreLanguages(t, name)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	spike := ccoreNew(gl)
	var total ccoreNodeCheck
	inputs, skipped, treeDiffers := 0, 0, 0
	all := ccoreInputs(t, name, false)
	switch name {
	case "c":
		var b strings.Builder
		b.WriteString("int a[] = {")
		for i := 0; i < 6000; i++ {
			fmt.Fprintf(&b, "%d,", i)
		}
		b.WriteString("};\n")
		all = append(all, ccoreInput{"c/wide_initializer", []byte(b.String())})
	case "json":
		var b strings.Builder
		b.WriteString("[")
		for i := 0; i < 6000; i++ {
			fmt.Fprintf(&b, "%d,", i)
		}
		b.WriteString("0]\n")
		all = append(all, ccoreInput{"json/wide_array", []byte(b.String())})
		all = append(all, ccoreInput{"json/wide_error_20000", []byte(strings.Repeat("} ", 20000))})
	}
	if name == "json" {
		// One bounded wide fixture: 5,000 stray tokens that recovery gathers under one ERROR
		// node, so that a record has more than 4,096 children. Every index is checked.
		all = append(all, ccoreInput{"json/wide_error", []byte(strings.Repeat("} ", 5000))})
	}
	for _, in := range all {
		ct := cp.Parse(in.Src, nil)
		stop := spike.Parse(in.Src, nil)
		// The readers are compared on trees that equal C's record for record: on
		// another tree the two sides read different data.
		ch, cn, rc := corecords.Hash(ct, "")
		gh, gn := spike.RecordsHash("")
		if stop != "" || rc != 0 || ch != gh || cn != gn {
			if stop != "" {
				skipped++
			} else {
				treeDiffers++
				t.Logf("CCNODESKIP input=%s: the core's tree differs from C's (records)", in.ID)
			}
			ct.Close()
			spike.ReleaseLast()
			continue
		}
		rd, root := spike.NodeReader(spike.Tree())
		rd = ccoreNodesMaybeTruncated(rd)
		c := ccoreNodeCheck{t: t, rd: rd, names: gl.SymbolNames, fields: gl.FieldNames, cur: ct.Walk(), input: in.ID, rng: rand.New(rand.NewSource(int64(inputs) + 1))}
		c.walk("", ct.RootNode(), rd.RootNode(root))
		c.cur.Close()
		inputs++
		total.nodes += c.nodes
		total.checks += c.checks
		total.diffs += c.diffs
		total.idDiff += c.idDiff
		total.pastEndNodes += c.pastEndNodes
		total.indirect += c.indirect
		total.nonzeroMasks += c.nonzeroMasks
		total.cSuperKids += c.cSuperKids
		total.largeLists += c.largeLists
		total.maxPhysical = max(total.maxPhysical, c.maxPhysical)
		ct.Close()
		spike.ReleaseLast()
	}
	fmt.Printf("CCNODE lang=%s inputs=%d skipped=%d tree_differs=%d nodes=%d checks=%d differences=%d public_id_differs=%d past_end_nodes=%d indirect_named=%d max_physical_children=%d large_lists=%d nonzero_masks=%d c_supertype_children=%d\n",
		name, inputs, skipped, treeDiffers, total.nodes, total.checks, total.diffs, total.idDiff, total.pastEndNodes, total.indirect, total.maxPhysical, total.largeLists, total.nonzeroMasks, total.cSuperKids)
	if inputs == 0 || total.nodes == 0 {
		// Nothing was compared: this language is not covered by the check. It is
		// reported as skipped, never as passing.
		fmt.Printf("CCNODENOTCOVERED lang=%s inputs_skipped=%d inputs_tree_differs=%d\n", name, skipped, treeDiffers)
		if os.Getenv("CCORE_NODES_SWEEP") == "" {
			t.Fatalf("%s: selected, but no input could be compared (set CCORE_NODES_SWEEP=1 in a sweep over all languages to report it as not covered)", name)
		}
		t.Skipf("%s: no input could be compared (the core's tree differs from C's, or the core stopped)", name)
	}
	if total.diffs != 0 {
		t.Errorf("%s: %d differences from C", name, total.diffs)
	}
	// A language in which C reports supertypes must show a child with a non-zero mask in
	// the core, or the mask check proved nothing. TypeScript must show one in any case.
	if (total.cSuperKids > 0 || name == "typescript") && total.nonzeroMasks == 0 {
		t.Errorf("%s: no child had a supertype mask; the mask check compared only zeros", name)
	}
	ccoreCount("TestCcoreNodes", name, total.nodes)
	if total.cSuperKids > 0 {
		ccoreCount("TestCcoreNodes", name+"-masks", total.nonzeroMasks)
	}
}

// ccoreNodesMaybeTruncated returns rd with an empty supertype bit table when
// CCORE_NODES_TRUNCATE_SUPERBIT is set. It exists only so that the exit-status test
// can show that a missing table fails the comparison.
func ccoreNodesMaybeTruncated(rd ccore.NodeReader) ccore.NodeReader {
	if os.Getenv("CCORE_NODES_TRUNCATE_SUPERBIT") == "" {
		return rd
	}
	nt := *rd.T
	nt.SuperBit = nt.SuperBit[:0]
	rd.T = &nt
	return rd
}

// TestCcoreNodesNotCoveredExitStatus runs the checked runner on a language whose inputs cannot
// be compared (agda: the core's records differ from C's): selected on its own it must exit
// non-zero; in a sweep it reports CCNODENOTCOVERED and exits zero.
func TestCcoreNodesNotCoveredExitStatus(t *testing.T) {
	if os.Getenv("CCORE_NODES_EXIT_CHECK") == "" {
		t.Skip("set CCORE_NODES_EXIT_CHECK=1 (the sub-runs need the C oracle)")
	}
	run := func(test, lang string, sweep bool, extra ...string) (string, error) {
		cmd := exec.Command(os.Args[0], "-test.run", "^"+test+"$", "-test.v", "-test.count=1")
		cmd.Env = append(os.Environ(), "CCORE_LANG="+lang, "CCORE_NODES_EXIT_CHECK=", "CCORE_NODES_SWEEP=")
		if sweep {
			cmd.Env = append(cmd.Env, "CCORE_NODES_SWEEP=1")
		}
		cmd.Env = append(cmd.Env, extra...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	for _, c := range []struct{ test, marker string }{
		{"TestCcoreNodes", "CCNODENOTCOVERED"},
		{"TestCcoreNodesEdited", "CCNODEEDITNOTCOVERED"},
	} {
		out, err := run(c.test, "agda", false)
		if err == nil || !strings.Contains(out, c.marker) {
			t.Fatalf("%s: a selected language with nothing compared must fail and print %s: err %v\n%s", c.test, c.marker, err, out)
		}
		out, err = run(c.test, "agda", true)
		if err != nil || !strings.Contains(out, c.marker) {
			t.Fatalf("%s: in a sweep the same language must be reported and exit zero: err %v\n%s", c.test, err, out)
		}
	}
	// A core whose supertype bit table is empty must not pass for a language that has supertypes.
	out, err := run("TestCcoreNodes", "json", false, "CCORE_NODES_TRUNCATE_SUPERBIT=1", "CCORE_NORM_NAMES=")
	if err == nil || !strings.Contains(out, "is beyond the core's bit table") {
		t.Fatalf("an empty supertype bit table must fail the comparison: err %v\n%s", err, out)
	}
	t.Logf("truncated bit table (json), exit error %v; first lines:\n%s", err, ccoreNodesFirstLines(out, 12))
}

func ccoreNodesFirstLines(s string, n int) string {
	l := strings.SplitAfter(s, "\n")
	if len(l) > n {
		l = l[:n]
	}
	return strings.Join(l, "")
}

// TestCcoreNodesEdited compares the readers on trees after ts_tree_edit, so that
// has_changes is true on the nodes the edit touched and the positions moved. Four
// edits of the committed sample (insert in the middle, delete, replace, a byte
// inserted at the start), each on a fresh tree. CCORE_LANG=<language>.
func TestCcoreNodesEdited(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	gl, cl := ccoreLanguages(t, name)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	spike := ccoreNew(gl)
	src := ccoreInputs(t, name, true)[0].Src
	type edit struct {
		what             string
		start, oldE, new int
	}
	n := len(src)
	edits := []edit{{"insert", n / 2, n / 2, n/2 + 1}, {"delete", n / 3, n/3 + 1, n / 3}, {"replace", n / 5, n/5 + 1, n/5 + 1}, {"insert at the start", 0, 0, 1}}
	var total ccoreNodeCheck
	changed := 0
	notCovered := func(why string) {
		fmt.Printf("CCNODEEDITNOTCOVERED lang=%s %s\n", name, why)
		if os.Getenv("CCORE_NODES_SWEEP") == "" {
			t.Fatalf("%s: selected, but no edited tree could be compared (%s; set CCORE_NODES_SWEEP=1 in a sweep over all languages to report it as not covered)", name, why)
		}
		t.Skipf("%s: no edited tree could be compared (%s)", name, why)
	}
	for _, e := range edits {
		ct := cp.Parse(src, nil)
		spike.Parse(src, nil)
		ch, cn, rc := corecords.Hash(ct, "")
		gh, gn := spike.RecordsHash("")
		if rc != 0 || ch != gh || cn != gn {
			ct.Close()
			spike.ReleaseLast()
			notCovered("the core's tree differs from C's before the edit")
		}
		sr, sc := ccorePointAt(src, e.start)
		or, oc := ccorePointAt(src, e.oldE)
		after := append(append([]byte(nil), src[:e.start]...), make([]byte, e.new-e.start)...)
		after = append(after, src[e.oldE:]...)
		nr, nc := ccorePointAt(after, e.new)
		ct.Edit(&sitter.InputEdit{StartByte: uint(e.start), OldEndByte: uint(e.oldE), NewEndByte: uint(e.new),
			StartPosition: sitter.Point{Row: uint(sr), Column: uint(sc)}, OldEndPosition: sitter.Point{Row: uint(or), Column: uint(oc)}, NewEndPosition: sitter.Point{Row: uint(nr), Column: uint(nc)}})
		spike.EditInPlace(spike.Tree(), bindtest.Edit{StartByte: uint32(e.start), OldEndByte: uint32(e.oldE), NewEndByte: uint32(e.new),
			StartRow: sr, StartCol: sc, OldEndRow: or, OldEndCol: oc, NewEndRow: nr, NewEndCol: nc})
		rd, root := spike.NodeReader(spike.Tree())
		rd = ccoreNodesMaybeTruncated(rd)
		c := ccoreNodeCheck{t: t, rd: rd, names: gl.SymbolNames, fields: gl.FieldNames, cur: ct.Walk(), input: name + "/" + e.what, rng: rand.New(rand.NewSource(7))}
		c.walk("", ct.RootNode(), rd.RootNode(root))
		c.cur.Close()
		if !rd.HasChanges(rd.RootNode(root)) && e.oldE != e.new {
			c.fail("", "the root does not report the edit")
		}
		total.nodes += c.nodes
		total.checks += c.checks
		total.diffs += c.diffs
		if rd.HasChanges(rd.RootNode(root)) {
			changed++
		}
		ct.Close()
		spike.ReleaseLast()
	}
	fmt.Printf("CCNODEEDIT lang=%s edits=%d roots_with_changes=%d nodes=%d checks=%d differences=%d\n", name, len(edits), changed, total.nodes, total.checks, total.diffs)
	if total.nodes == 0 {
		notCovered("no node was compared")
	}
	if total.diffs != 0 {
		t.Errorf("%s: %d differences from C after an edit", name, total.diffs)
	}
	ccoreCount("TestCcoreNodesEdited", name, total.nodes)
}
