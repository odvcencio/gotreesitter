//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// gradCleanC checks the entire C tree, including missing and error nodes.
func gradCleanC(c *gradCtx, text []byte) bool {
	p := sitter.NewParser()
	defer p.Close()
	if p.SetLanguage(c.cLang) != nil {
		return false
	}
	tree := p.Parse(text, nil)
	if tree == nil {
		return false
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.StartByte() != 0 || root.EndByte() != uint(len(text)) || root.HasError() {
		return false
	}
	var visit func(*sitter.Node) bool
	visit = func(n *sitter.Node) bool {
		if n == nil || n.IsError() || n.IsMissing() {
			return false
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			if !visit(n.Child(i)) {
				return false
			}
		}
		return true
	}
	return visit(root)
}

// gradTemplateMarkers split a template input in cgo_harness/testdata/
// graduation_inputs: the lines before "@@repeat" are the head, the lines
// between it and "@@end" are the body, and the lines after "@@end" are the tail.
const gradTemplateRepeat, gradTemplateEnd = "@@repeat\n", "@@end\n"

// gradTemplateParts splits a template input. It reports false for an input
// without both markers, in that order and each on a line of its own.
func gradTemplateParts(src []byte) (head, body, tail []byte, ok bool) {
	s := string(src)
	i := strings.Index(s, "\n"+gradTemplateRepeat)
	if i < 0 && strings.HasPrefix(s, gradTemplateRepeat) {
		i = -1
	} else if i < 0 {
		return nil, nil, nil, false
	}
	bodyStart := i + 1 + len(gradTemplateRepeat)
	j := strings.Index(s[bodyStart:], "\n"+gradTemplateEnd)
	if j < 0 {
		return nil, nil, nil, false
	}
	bodyEnd := bodyStart + j + 1
	if bodyEnd <= bodyStart {
		return nil, nil, nil, false
	}
	return []byte(s[:i+1]), []byte(s[bodyStart:bodyEnd]), []byte(s[bodyEnd+len(gradTemplateEnd):]), true
}

// gradTemplate repeats body between head and tail until the text is at least
// target bytes long.
func gradTemplate(head, body, tail []byte, target int) []byte {
	out := append(make([]byte, 0, target+len(body)+len(tail)), head...)
	for len(out)+len(tail) < target {
		out = append(out, body...)
	}
	return append(out, tail...)
}

type gradSourceResult struct {
	text []byte
	name string
}

var gradSourceCache = map[string]gradSourceResult{}

// gradCleanSource tries existing sized text, samples, corpus and dedicated inputs.
func gradCleanSource(t *testing.T, c *gradCtx, target int) ([]byte, string) {
	t.Helper()
	key := fmt.Sprintf("%s:%d", c.name, target)
	if result, ok := gradSourceCache[key]; ok {
		return result.text, result.name
	}
	var tried []string
	var smaller gradSourceResult
	try := func(name string, src []byte) bool {
		tried = append(tried, name)
		if len(src) < 512 || !gradCleanC(c, src) {
			return false
		}
		if len(src) < target {
			if target == 4<<10 && len(src) > len(smaller.text) {
				smaller = gradSourceResult{src, name}
			}
			return false
		}
		gradSourceCache[key] = gradSourceResult{src, name}
		return true
	}
	repeat := func(src []byte, separator string) []byte {
		if len(src) == 0 {
			return nil
		}
		out := make([]byte, 0, target+len(src))
		for len(out) < target {
			if len(out) > 0 {
				out = append(out, separator...)
			}
			out = append(out, src...)
		}
		return out
	}
	sample, _ := gradSample(t, c.name)
	if len(sample) > 0 && try("gradSized", gradSized(t, c.name, target)) {
		r := gradSourceCache[key]
		return r.text, r.name
	}
	if target == 32<<10 {
		var safety []byte
		if len(sample) > 0 {
			for len(safety) < target {
				safety = append(safety, sample...)
				if safety[len(safety)-1] != '\n' {
					safety = append(safety, '\n')
				}
			}
		}
		if try("gradSafetySource", safety) {
			r := gradSourceCache[key]
			return r.text, r.name
		}
		// These constructions match the root package's store-bound inputs.
		var b strings.Builder
		switch c.name {
		case "json":
			b.WriteString("[\n")
			for i := 0; i < target/12; i++ {
				if i > 0 {
					b.WriteString(",\n")
				}
				fmt.Fprintf(&b, "  {\"k\": %d, \"v\": [1, 2, \"x\"]}", i)
			}
			b.WriteString("\n]\n")
		case "python":
			for i := 0; b.Len() < target; i++ {
				fmt.Fprintf(&b, "def f%d(a):\n    return a + %d\n\n", i, i)
			}
		case "c":
			for i := 0; b.Len() < target; i++ {
				fmt.Fprintf(&b, "int f%d(int a) { return a + %d; }\n", i, i)
			}
		case "typescript":
			for i := 0; b.Len() < target; i++ {
				fmt.Fprintf(&b, "function f%d(a: number): number { return a + %d; }\n", i, i)
			}
		}
		if b.Len() > 0 && try("ccoreGateSource", []byte(b.String())) {
			r := gradSourceCache[key]
			return r.text, r.name
		}
	}
	for _, sep := range []string{"\n", "\n\n"} {
		name := "sample_newline"
		if len(sep) == 2 {
			name = "sample_blankline"
		}
		if try(name, repeat(sample, sep)) {
			r := gradSourceCache[key]
			return r.text, r.name
		}
	}
	type input struct {
		name string
		data []byte
	}
	var corpus []input
	var largest []byte
	largestName := ""
	if dir := os.Getenv("GRAD_CORPUS_DIR"); dir != "" {
		path := filepath.Join(dir, c.name)
		err := filepath.WalkDir(path, func(file string, entry os.DirEntry, walkErr error) error {
			if os.IsNotExist(walkErr) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(path, file)
			if err != nil {
				return err
			}
			corpus = append(corpus, input{rel, data})
			if len(data) > len(largest) {
				largest, largestName = data, rel
			}
			return nil
		})
		if err != nil {
			t.Fatalf("corpus directory %s: %v", path, err)
		}
	}
	dedicatedPath := filepath.Join("testdata", "graduation_inputs", c.name)
	dedicated, err := os.ReadFile(dedicatedPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("graduation input %s: %v", dedicatedPath, err)
	}
	if head, body, tail, ok := gradTemplateParts(dedicated); ok {
		// A grammar whose file is one document (one graph, one object) cannot
		// be scaled by repeating the whole file, so its input repeats a body
		// between a fixed head and tail.
		if try("dedicated_template", gradTemplate(head, body, tail, target)) {
			r := gradSourceCache[key]
			return r.text, r.name
		}
		dedicated = nil
	}
	for _, entry := range []struct {
		name string
		data []byte
	}{{"corpus_" + largestName, largest}, {"dedicated", dedicated}} {
		if try(entry.name, entry.data) {
			r := gradSourceCache[key]
			return r.text, r.name
		}
		for _, sep := range []string{"\n", "\n\n"} {
			name := entry.name + "_newline"
			if len(sep) == 2 {
				name = entry.name + "_blankline"
			}
			if try(name, repeat(entry.data, sep)) {
				r := gradSourceCache[key]
				return r.text, r.name
			}
		}
	}
	for _, entry := range corpus {
		for _, candidate := range []struct {
			name string
			data []byte
		}{
			{"corpus_" + entry.name, entry.data},
			{"corpus_" + entry.name + "_newline", repeat(entry.data, "\n")},
			{"corpus_" + entry.name + "_blankline", repeat(entry.data, "\n\n")},
		} {
			if try(candidate.name, candidate.data) {
				r := gradSourceCache[key]
				return r.text, r.name
			}
		}
	}
	if try("sample", sample) {
		r := gradSourceCache[key]
		return r.text, r.name
	}
	for _, entry := range append(corpus, input{"sample", sample}) {
		var ends []int
		for i, b := range entry.data {
			if b == '\n' && i+1 >= 512 {
				ends = append(ends, i+1)
			}
		}
		stride := (len(ends) + 199) / 200
		if stride == 0 {
			continue
		}
		for i := len(ends) - 1; i >= 0; i -= stride {
			prefix := entry.data[:ends[i]]
			if !gradCleanC(c, prefix) {
				continue
			}
			name := fmt.Sprintf("corpus_prefix:%s:%d", entry.name, len(prefix))
			for _, candidate := range [][]byte{prefix, repeat(prefix, "\n"), repeat(prefix, "\n\n")} {
				if try(name, candidate) {
					r := gradSourceCache[key]
					return r.text, r.name
				}
			}
			break
		}
	}
	if len(smaller.text) > 0 {
		gradSourceCache[key] = smaller
		return smaller.text, smaller.name
	}
	result := gradSourceResult{nil, "tried=" + strings.Join(tried, ",")}
	gradSourceCache[key] = result
	return result.text, result.name
}

// gradCleanEdits accepts only proposals whose resulting text C parses cleanly.
func gradCleanEdits(t *testing.T, c *gradCtx, src []byte, n int, seed uint64) []gradStep {
	t.Helper()
	if src == nil || !gradCleanC(c, src) {
		return nil
	}
	var gen benchfixtures.CleanEditGenerator
	kinds := []string{"space+", "space-", "same", "type", "del"}
	current := src
	var steps []gradStep
	for step := 0; step < n; step++ {
		found := false
		tries := 0
		for k := 0; k < len(kinds) && tries < 40; k++ {
			kind := kinds[(step+k)%len(kinds)]
			for _, candidate := range gen.Candidates(current, kind, 8, seed+uint64(step)*41+uint64(k)) {
				tries++
				proposal, err := gen.Apply(current, candidate)
				if err != nil || !gradCleanC(c, proposal.Source) {
					continue
				}
				gen.Accept(candidate)
				e := proposal.Edit
				steps = append(steps, gradStep{id: fmt.Sprintf("clean%d:%s@%d", step+1, kind, candidate.At), src: proposal.Source, edit: &e})
				current = proposal.Source
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			break
		}
	}
	return steps
}

// TestGraduationStoreSource passes a C-validated source to the root safety test.
func TestGraduationStoreSource(t *testing.T) {
	c := gradLoad(t)
	path := os.Getenv("GRAD_SOURCE_FILE")
	if path == "" {
		t.Skip("set GRAD_SOURCE_FILE")
	}
	editsPath := os.Getenv("GRAD_EDITS_FILE")
	if editsPath == "" {
		t.Fatal("GRAD_EDITS_FILE is required")
	}
	var edits strings.Builder
	edits.WriteString("v1\n")
	small, _ := gradSample(t, c.name)
	if len(small) > 0 && small[len(small)-1] != '\n' {
		small = append(append([]byte(nil), small...), '\n')
	}
	var gen benchfixtures.CleanEditGenerator
	writeSmallEdit := func(input []byte, label string) bool {
		if !gradCleanC(c, input) {
			return false
		}
		candidates := gen.Candidates(input, "space+", len(input), 0)
		distance := func(at int) int {
			d := at - len(input)/3
			if d < 0 {
				return -d
			}
			return d
		}
		sort.SliceStable(candidates, func(i, j int) bool { return distance(candidates[i].At) < distance(candidates[j].At) })
		for _, candidate := range candidates {
			step, err := gen.Apply(input, candidate)
			if err == nil && gradCleanC(c, step.Source) {
				fmt.Fprintf(&edits, "%s %d %d %d\n", label, candidate.At, input[candidate.At], byte(' '))
				return true
			}
		}
		return false
	}
	source, name := gradCleanSource(t, c, 32<<10)
	if !writeSmallEdit(small, "small") && !writeSmallEdit(source, "small-source") {
		edits.WriteString("small-na no-C-clean-space-edit\n")
	}
	if source == nil {
		edits.WriteString("store-na no-C-clean-source\n")
		if err := os.WriteFile(editsPath, []byte(edits.String()), 0600); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("GRADSOURCE status=na source=%q bytes=0 reason=%q\n", name, "no C-clean source")
		return
	}
	current := source
	var store strings.Builder
	for i := 0; i < 300; i++ {
		found := false
		for _, candidate := range gen.Candidates(current, "same", 40, uint64(i)*7919) {
			step, err := gen.Apply(current, candidate)
			if err != nil || !gradCleanC(c, step.Source) {
				continue
			}
			fmt.Fprintf(&store, "same %d %d %d\n", candidate.At, current[candidate.At], step.Source[candidate.At])
			gen.Accept(candidate)
			current = step.Source
			found = true
			break
		}
		if !found {
			fmt.Fprintf(&edits, "store-na no-C-clean-edit-at-%d\n", i)
			break
		}
		if i == 299 {
			edits.WriteString(store.String())
		}
	}
	if err := os.WriteFile(editsPath, []byte(edits.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("GRADSOURCE status=pass source=%q bytes=%d\n", name, len(source))
}

// gradCleanNoShortcut compares core reuse with C with the shortcut disabled.
func gradCleanNoShortcut(t *testing.T, c *gradCtx, src []byte, steps []gradStep) (int, string) {
	if src == nil {
		return -1, "no clean source"
	}
	if len(steps) == 0 {
		return -1, "no clean steps"
	}
	core, err := ccoreNewE(c.lang)
	if err != nil {
		return -1, err.Error()
	}
	core.SetShortcut(false)
	if stop := core.Parse(src, nil); stop != "" {
		return -1, stop
	}
	tree := core.Tree()
	defer func() { core.Release(tree) }()
	var log []string
	cp := gradCParser(t, c, &log)
	defer cp.Close()
	ct := cp.Parse(src, nil)
	if ct == nil {
		return -1, "C parse failed"
	}
	defer func() {
		if ct != nil {
			ct.Close()
		}
	}()
	differing := 0
	for _, st := range steps {
		log = log[:0]
		ct.Edit(gradSitterEdit(*st.edit))
		nextC := cp.Parse(st.src, ct)
		ct.Close()
		ct = nextC
		if ct == nil {
			return -1, "C reparse failed"
		}
		e := st.edit
		ce := bindtest.Edit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, StartRow: e.StartPoint.Row, StartCol: e.StartPoint.Column, OldEndRow: e.OldEndPoint.Row, OldEndCol: e.OldEndPoint.Column, NewEndRow: e.NewEndPoint.Row, NewEndCol: e.NewEndPoint.Column}
		edited := core.Edit(tree, ce)
		next, stop := core.ParseIncremental(st.src, edited, nil)
		core.Release(edited)
		core.Release(tree)
		tree = next
		if stop != "" || !tree.Valid() {
			return -1, "core reparse stopped: " + stop
		}
		nodes, _, _ := core.ReuseStats()
		if nodes != gradCount(log, "reuse_node ") {
			differing++
		}
	}
	return differing, ""
}

func gradCleanShortReason(source []byte, steps, served int) string {
	if source == nil {
		return fmt.Sprintf("clean_incremental=%d clean_steps=%d: no C-clean source", served, steps)
	}
	if steps < 48 {
		return fmt.Sprintf("clean_incremental=%d clean_steps=%d: no accepted candidate", served, steps)
	}
	return fmt.Sprintf("clean_incremental=%d clean_steps=%d: route did not serve enough edits incrementally", served, steps)
}

func TestGradTemplateParts(t *testing.T) {
	head, body, tail, ok := gradTemplateParts([]byte("{\n@@repeat\n  a;\n@@end\n}\n"))
	if !ok || string(head) != "{\n" || string(body) != "  a;\n" || string(tail) != "}\n" {
		t.Fatalf("parts = %q %q %q %v", head, body, tail, ok)
	}
	if got := string(gradTemplate(head, body, tail, 12)); got != "{\n  a;\n  a;\n}\n" {
		t.Fatalf("template = %q", got)
	}
	head, body, tail, ok = gradTemplateParts([]byte("@@repeat\nx +\n@@end\nx\n"))
	if !ok || string(head) != "" || string(body) != "x +\n" || string(tail) != "x\n" {
		t.Fatalf("leading marker parts = %q %q %q %v", head, body, tail, ok)
	}
	for _, bad := range []string{"a\nb\n", "{\n@@repeat\n  a;\n}\n", "{\n@@end\n  a;\n@@repeat\n}\n", "x @@repeat\na\n@@end\n"} {
		if _, _, _, ok := gradTemplateParts([]byte(bad)); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}
