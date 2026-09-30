//go:build cgo && treesitter_c_parity

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"sort"
	"time"
	"unicode/utf8"

	ts "github.com/odvcencio/gotreesitter"
	harness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Each keystroke inserts one ASCII character. Three cursors advance at the
// initial start, midpoint, and end. Only coordinates are prepared before
// timing; retaining 200 full source copies would inflate peak memory.
func typingEdits(initial []byte, count int) []ts.InputEdit {
	middle := len(initial) / 2
	for middle > 0 && middle < len(initial) && !utf8.RuneStart(initial[middle]) {
		middle--
	}
	cursors := []int{0, middle, len(initial)}
	points := []ts.Point{point(initial, 0), point(initial, middle), point(initial, len(initial))}
	edits := make([]ts.InputEdit, 0, count)
	for i := 0; i < count; i++ {
		at, start := cursors[i%3], points[i%3]
		end := start
		end.Column++
		edits = append(edits, ts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1), StartPoint: start, OldEndPoint: start, NewEndPoint: end})
		for k := range cursors {
			if cursors[k] >= at {
				cursors[k]++
				if points[k].Row == start.Row {
					points[k].Column++
				}
			}
		}
	}
	return edits
}

func insertByte(current []byte, e ts.InputEdit) []byte {
	at := int(e.StartByte)
	next := make([]byte, len(current)+1)
	copy(next, current[:at])
	next[at] = 'x'
	copy(next[at+1:], current[at:])
	return next
}

func point(b []byte, at int) ts.Point {
	p := b[:at]
	return ts.Point{Row: uint32(bytes.Count(p, []byte{'\n'})), Column: uint32(at - bytes.LastIndexByte(p, '\n') - 1)}
}
func cEdit(e ts.InputEdit) *sitter.InputEdit {
	return &sitter.InputEdit{StartByte: uint(e.StartByte), OldEndByte: uint(e.OldEndByte), NewEndByte: uint(e.NewEndByte), StartPosition: sitter.Point{Row: uint(e.StartPoint.Row), Column: uint(e.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(e.OldEndPoint.Row), Column: uint(e.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(e.NewEndPoint.Row), Column: uint(e.NewEndPoint.Column)}}
}

func parseGo(p *ts.Parser, e grammars.LangEntry, b []byte, old *ts.Tree) (*ts.Tree, error) {
	if e.TokenSourceFactory == nil {
		if old == nil {
			return p.Parse(b)
		}
		return p.ParseIncremental(b, old)
	}
	factory := func(input []byte) (ts.TokenSource, error) {
		token := e.TokenSourceFactory(input, e.Language())
		if token == nil {
			return nil, errors.New("nil grammar token source")
		}
		return token, nil
	}
	if old == nil {
		return p.ParseWithTokenSourceFactory(b, factory)
	}
	return p.ParseIncrementalWithTokenSourceFactory(b, old, factory)
}

func goCaptures(q *ts.Query, t *ts.Tree, lang *ts.Language, b []byte) ([]capture, error) {
	cur := q.Exec(t.RootNode(), lang, b)
	var out []capture
	for {
		m, ok := cur.NextMatch()
		if !ok {
			break
		}
		for _, c := range m.Captures {
			start, end := c.Node.StartByte(), c.Node.EndByte()
			if end > uint32(len(b)) || start > end {
				return nil, errors.New("Go query capture outside source")
			}
			out = append(out, capture{Name: c.Name, Start: start, End: end, Text: string(b[start:end])})
		}
	}
	if cur.DidExceedMatchLimit() {
		return nil, errors.New("Go query exceeded match/work budget")
	}
	sortCaptures(out)
	return out, nil
}

func cCaptures(q *sitter.Query, t *sitter.Tree, b []byte) ([]capture, error) {
	cur := sitter.NewQueryCursor()
	defer cur.Close()
	names := q.CaptureNames()
	matches := cur.Matches(q, t.RootNode(), b)
	var out []capture
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		for _, c := range m.Captures {
			start, end := c.Node.StartByte(), c.Node.EndByte()
			if end > uint(len(b)) || start > end {
				return nil, errors.New("C query capture outside source")
			}
			out = append(out, capture{Name: names[c.Index], Start: uint32(start), End: uint32(end), Text: string(b[start:end])})
		}
	}
	if cur.DidExceedMatchLimit() {
		return nil, errors.New("C query exceeded match limit")
	}
	sortCaptures(out)
	return out, nil
}

func sortCaptures(c []capture) {
	sort.Slice(c, func(i, j int) bool {
		a, b := c[i], c[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.End != b.End {
			return a.End < b.End
		}
		return a.Text < b.Text
	})
}
func captureDigest(c []capture) string { b, _ := json.Marshal(c); return sha(b) }
func recordOutput(h hash.Hash, path string, step int, c []capture) {
	fmt.Fprintf(h, "%s\x00%d\x00", path, step)
	_ = json.NewEncoder(h).Encode(c)
}

func measure(o options, j job, engine string) (result measurement) {
	// Setup is outside the operation: grammar loading and C compilation are
	// separately authenticated, rather than charged to one engine's parse.
	entry := grammars.DetectLanguageByName(j.Language)
	if entry == nil {
		result.Error = "unknown grammar"
		return
	}
	qsrc, _ := querySource(j, *entry)
	if len(bytes.TrimSpace(qsrc)) == 0 {
		result.Error = "empty query"
		return
	}
	var lang *ts.Language
	var gp *ts.Parser
	var gq *ts.Query
	var cp *sitter.Parser
	var cq *sitter.Query
	var err error
	if engine == "go" {
		lang = entry.Language()
		gp = ts.NewParser(lang)
		gq, err = ts.NewQuery(string(qsrc), lang)
	} else if engine == "c" {
		cl, e := harness.COracleLanguage(j.Language)
		if e != nil {
			result.Error = e.Error()
			return
		}
		cp = sitter.NewParser()
		defer cp.Close()
		err = cp.SetLanguage(cl)
		if err == nil {
			var qe *sitter.QueryError
			cq, qe = sitter.NewQuery(cl, string(qsrc))
			if qe != nil {
				err = qe
			}
		}
		if cq != nil {
			defer cq.Close()
		}
	} else {
		result.Error = "invalid engine"
		return
	}
	if err != nil {
		result.Error = err.Error()
		return
	}
	paths := j.Files
	if j.Workflow == "fixtures" {
		paths = []string{j.Shape}
	}
	var initial []byte
	var edits []ts.InputEdit
	if j.Workflow != "index" {
		initial, err = source(o, j, paths[0])
		if err != nil {
			result.Error = err.Error()
			return
		}
		count := 3
		if j.Workflow == "editor" {
			count = 200
		}
		edits = typingEdits(initial, count)
	}
	h := sha256.New()
	durations := make([]int64, 0, len(edits))
	started := time.Now()
	for _, path := range paths {
		fmt.Fprintf(os.Stderr, "input %q\n", path)
		b := initial
		if j.Workflow == "index" {
			b, err = source(o, j, path)
			if err != nil {
				result.Error = err.Error()
				break
			}
		}
		var gt *ts.Tree
		var ct *sitter.Tree
		for step := 0; step <= len(edits); step++ {
			at := time.Now()
			if step > 0 {
				e := edits[step-1]
				b = insertByte(b, e)
				if gt != nil {
					gt.Edit(e)
				}
				if ct != nil {
					ct.Edit(cEdit(e))
				}
			}
			var caps []capture
			if engine == "go" {
				old := gt
				gt, err = parseGo(gp, *entry, b, old)
				if old != nil && old != gt {
					old.Release()
				}
				if err == nil && gt == nil {
					err = errors.New("nil Go tree")
				}
				if err == nil {
					caps, err = goCaptures(gq, gt, lang, b)
				}
			} else {
				old := ct
				ct = cp.Parse(b, old)
				if old != nil {
					old.Close()
				}
				if ct == nil {
					err = errors.New("nil C tree")
				} else {
					caps, err = cCaptures(cq, ct, b)
				}
			}
			if err != nil {
				result.Error = fmt.Sprintf("%s step %d: %v", path, step, err)
				break
			}
			recordOutput(h, path, step, caps)
			result.Captures += len(caps)
			result.Operations++
			if step > 0 {
				durations = append(durations, time.Since(at).Nanoseconds())
			}
		}
		if gt != nil {
			gt.Release()
		}
		if ct != nil {
			ct.Close()
		}
		if result.Error != "" {
			break
		}
	}
	result.WallNS = time.Since(started).Nanoseconds()
	if result.Error == "" {
		result.OutputSHA256 = hex.EncodeToString(h.Sum(nil))
	}
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		result.P95NS = durations[(95*len(durations)+99)/100-1]
	}
	return
}

func (v *validation) add(w witness) {
	if v.Mismatches == nil {
		v.Mismatches = map[string]int{}
	}
	v.Mismatches[w.Kind]++
	// Preserve one deterministic witness per kind, without retaining every tree.
	for _, old := range v.Witnesses {
		if old.Kind == w.Kind {
			return
		}
	}
	v.Witnesses = append(v.Witnesses, w)
}

func goDigest(t *ts.Tree, lang *ts.Language) (string, error) {
	d, err := benchfixtures.InspectGoTree(t.RootNode(), lang)
	return d.SHA256, err
}

func validate(o options, j job) (v validation) {
	entry := grammars.DetectLanguageByName(j.Language)
	if entry == nil {
		v.Error = "unknown grammar"
		return
	}
	lang := entry.Language()
	cl, err := harness.COracleLanguage(j.Language)
	if err != nil {
		v.Error = err.Error()
		return
	}
	qsrc, _ := querySource(j, *entry)
	file := j.Path
	if j.Workflow == "fixtures" {
		file = j.Shape
	}
	if file == "" && len(j.Files) > 0 {
		file = j.Files[0]
	}
	if len(bytes.TrimSpace(qsrc)) == 0 {
		v.add(witness{Kind: "empty-query", File: file, Step: 0, Detail: "workflow has no query patterns"})
	}
	gq, gerr := ts.NewQuery(string(qsrc), lang)
	if gerr != nil {
		v.add(witness{Kind: "go-query-compile", File: file, Step: 0, Detail: gerr.Error()})
	}
	cq, cerr := sitter.NewQuery(cl, string(qsrc))
	if cerr != nil {
		v.add(witness{Kind: "c-query-compile", File: file, Step: 0, Detail: cerr.Error()})
	}
	if cq != nil {
		defer cq.Close()
	}

	gp := ts.NewParser(lang)
	freshParser := ts.NewParser(lang)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		v.Error = err.Error()
		return
	}
	cf := sitter.NewParser()
	defer cf.Close()
	if err := cf.SetLanguage(cl); err != nil {
		v.Error = err.Error()
		return
	}
	paths := j.Files
	if j.Workflow == "fixtures" {
		paths = []string{j.Shape}
	}
	for _, path := range paths {
		initial, err := source(o, j, path)
		if err != nil {
			v.Error = err.Error()
			return
		}
		var edits []ts.InputEdit
		if j.Workflow != "index" {
			count := 3
			if j.Workflow == "editor" {
				count = 200
			}
			edits = typingEdits(initial, count)
		}
		b := initial
		var gt *ts.Tree
		var ct *sitter.Tree
		for step := 0; step <= len(edits); step++ {
			fmt.Fprintf(os.Stderr, "input %q step %d\n", path, step)
			w := witness{File: path, Step: step}
			if step > 0 {
				e := edits[step-1]
				b = insertByte(b, e)
				w.Edit = &e
				gt.Edit(e)
				ct.Edit(cEdit(e))
			}
			old := gt
			gt, err = parseGo(gp, *entry, b, old)
			if old != nil && old != gt {
				old.Release()
			}
			cold := ct
			ct = cp.Parse(b, cold)
			if cold != nil {
				cold.Close()
			}
			if err != nil || gt == nil || ct == nil {
				v.Error = fmt.Sprintf("%s step %d: parse failed: %v", path, step, err)
				if gt != nil {
					gt.Release()
				}
				if ct != nil {
					ct.Close()
				}
				return
			}
			gd, e1 := goDigest(gt, lang)
			cd, e2 := harness.COracleDeepDigest(ct)
			if e1 != nil || e2 != nil {
				v.Error = fmt.Sprintf("digest: %v / %v", e1, e2)
				gt.Release()
				ct.Close()
				return
			}
			v.Checks++
			w.GoDigest = gd
			w.CDigest = cd
			if gd != cd {
				w.Kind = "go-c-tree"
				if d := harness.FirstDivergenceDumpV1(gt.RootNode(), lang, ct.RootNode()); d != nil {
					detail, _ := json.Marshal(d)
					w.Detail = string(detail)
				}
				v.add(w)
			}
			root := gt.RootNode()
			if root.IsError() && !root.HasError() {
				w.Kind = "error-root-without-haserror"
				w.Detail = "ERROR root does not report HasError"
				v.add(w)
			}
			if gt.ParseStoppedEarly() {
				w.Kind = "stopped-early"
				w.Detail = gt.ParseRuntime().Summary()
				v.add(w)
			}
			if root.EndByte() < uint32(len(b)) && !gt.ParseStoppedEarly() {
				// Tree-sitter can omit trailing whitespace. A missing non-whitespace
				// suffix must have an explicit stop reason.
				if len(bytes.TrimSpace(b[root.EndByte():])) > 0 {
					w.Kind = "unexplained-root-gap"
					w.Detail = fmt.Sprintf("root end %d, input bytes %d", root.EndByte(), len(b))
					v.add(w)
				}
			}
			if gq != nil && cq != nil && gerr == nil && cerr == nil {
				gc, ge := goCaptures(gq, gt, lang, b)
				cc, ce := cCaptures(cq, ct, b)
				if ge != nil || ce != nil {
					w.Kind = "query-error"
					w.Detail = fmt.Sprintf("Go: %v; C: %v", ge, ce)
					v.add(w)
				} else if captureDigest(gc) != captureDigest(cc) {
					w.Kind = "query-output"
					w.GoDigest = captureDigest(gc)
					w.CDigest = captureDigest(cc)
					w.Detail = firstCaptureDifference(gc, cc)
					v.add(w)
				}
			}
			if step > 0 {
				fresh, fe := parseGo(freshParser, *entry, b, nil)
				if fe != nil || fresh == nil {
					v.Error = fmt.Sprintf("fresh Go: %v", fe)
					gt.Release()
					ct.Close()
					return
				}
				fd, de := goDigest(fresh, lang)
				fresh.Release()
				if de != nil {
					v.Error = de.Error()
					gt.Release()
					ct.Close()
					return
				}
				v.Checks++
				if fd != gd {
					w.Kind = "d8-incremental-fresh"
					w.GoDigest = gd
					w.CDigest = cd
					w.FreshDigest = fd
					w.Detail = "incremental Go differs from fresh Go"
					v.add(w)
				}
				cFresh := cf.Parse(b, nil)
				if cFresh == nil {
					v.Error = "nil fresh C tree"
					gt.Release()
					ct.Close()
					return
				}
				cfd, cde := harness.COracleDeepDigest(cFresh)
				cFresh.Close()
				if cde != nil {
					v.Error = cde.Error()
					gt.Release()
					ct.Close()
					return
				}
				v.Checks++
				if cfd != cd {
					w.Kind = "c-incremental-fresh"
					w.GoDigest = gd
					w.CDigest = cd
					w.FreshDigest = cfd
					w.Detail = "incremental C differs from fresh C"
					v.add(w)
				}
				if fd != cfd {
					w.Kind = "fresh-go-c-tree"
					w.GoDigest = fd
					w.CDigest = cfd
					w.FreshDigest = ""
					w.Detail = "fresh Go differs from fresh C"
					v.add(w)
				}
			}
		}
		gt.Release()
		ct.Close()
	}
	return
}

func firstCaptureDifference(a, b []capture) string {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			x, _ := json.Marshal(a[i])
			y, _ := json.Marshal(b[i])
			return fmt.Sprintf("capture %d: Go %s; C %s", i, x, y)
		}
	}
	return fmt.Sprintf("Go captures %d; C captures %d", len(a), len(b))
}
