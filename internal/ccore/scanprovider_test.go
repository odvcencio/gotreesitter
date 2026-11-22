package ccore_test

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// scanRecord keeps each request a provider gets and the answer it returned.
type scanRecord struct {
	state    uint32
	pos      int
	row, col uint32
	ok       bool
	tok      ccore.Token
	fail     [2]ccore.Length
}

// recordingScan answers from the table walk and records every request.
type recordingScan struct {
	inner *ccore.TableScan
	log   []scanRecord
}

func (r *recordingScan) Scan(state uint32, pos int, row, col uint32, out *ccore.Token, fail *[2]ccore.Length) bool {
	ok := r.inner.Scan(state, pos, row, col, out, fail)
	r.log = append(r.log, scanRecord{state, pos, row, col, ok, *out, *fail})
	return ok
}

func replayWith(t *testing.T, tables *ccore.Tables, src []byte, prov ccore.ScanProvider) (trace []string, digest, stop string, hash uint64, lines uint64) {
	t.Helper()
	p, err := ccore.NewParser(ccore.Config{Tables: tables})
	if err != nil {
		t.Fatal(err)
	}
	p.SetLogger(func(s string) { trace = append(trace, s) })
	tr, stop := p.ParseWithScan(src, nil, prov)
	p.SetLogger(nil)
	digest = p.Digest(tr)
	if tr != nil {
		hash, lines = p.Records(tr, nil)
		tr.Release()
	}
	return
}

// The table-backed provider and the nil path give the same trace, digest and
// records on every golden. The goldens hold two lex modes at one byte (the
// retry in the error state's mode), a failed scan followed by an error-mode
// success, input that ends mid-token, a byte order mark and the end of input.
func TestScanProviderEqualsNilPath(t *testing.T) {
	served := 0
	for _, g := range loadGoldens(t) {
		nilTrace, nilDigest, nilStop, nilHash, nilLines := replayWith(t, g.Tables, g.Src, nil)
		prov := ccore.NewTableScan(g.Tables, g.Src)
		trace, digest, stop, hash, lines := replayWith(t, g.Tables, g.Src, prov)
		if prov.Calls() == 0 {
			t.Errorf("%s: the core never asked the provider", g.Name)
		}
		served += prov.Calls()
		if digest != nilDigest || stop != nilStop || hash != nilHash || lines != nilLines {
			t.Errorf("%s: provider digest/stop/records differ from the nil path", g.Name)
		}
		if strings.Join(trace, "\n") != strings.Join(nilTrace, "\n") {
			t.Errorf("%s: provider trace differs from the nil path", g.Name)
		}
		if nilDigest != g.Digest {
			t.Errorf("%s: nil path digest %s, golden %s", g.Name, nilDigest, g.Digest)
		}
		if prov.Identity() != ccore.TableScanID || ccore.TableScanID != "table-v1" {
			t.Errorf("provider identity %q", prov.Identity())
		}
	}
	if served == 0 {
		t.Fatal("no provider call over all goldens")
	}
}

// A request log shows the modes: some golden asks the same byte in two lex
// states, and some golden has a failed answer followed by an answer in another
// state at the same byte (the error-mode retry), and one asks at the end of the
// input. Each answer equals the walk's answer for that request.
func TestScanProviderRequests(t *testing.T) {
	var twoModes, failThenOK, atEnd bool
	for _, g := range loadGoldens(t) {
		rec := &recordingScan{inner: ccore.NewTableScan(g.Tables, g.Src)}
		replayWith(t, g.Tables, g.Src, rec)
		for i, r := range rec.log {
			var tok ccore.Token
			var fail [2]ccore.Length
			walk := ccore.NewTableScan(g.Tables, g.Src)
			ok := walk.Scan(r.state, r.pos, r.row, r.col, &tok, &fail)
			if ok != r.ok || (ok && tok != r.tok) || (!ok && (fail != r.fail || tok.LookaheadEnd != r.tok.LookaheadEnd)) { // fail is read only after a failure
				t.Errorf("%s: request %d is not repeatable", g.Name, i)
			}
			if r.pos >= len(g.Src) {
				atEnd = true
			}
			if i > 0 {
				q := rec.log[i-1]
				if q.pos == r.pos && q.state != r.state {
					twoModes = true
					if !q.ok && r.ok {
						failThenOK = true
					}
				}
			}
		}
	}
	if !twoModes || !failThenOK || !atEnd {
		t.Errorf("the goldens did not reach: two modes at one byte %v, failure then success %v, end of input %v", twoModes, failThenOK, atEnd)
	}
}

// failFirst refuses the first two scans (the lex mode and the error mode) and answers the rest from the table walk,
// so a parse that differs from the nil path proves the core used the answer.
type failFirst struct {
	inner *ccore.TableScan
	n     int
}

func (f *failFirst) Scan(state uint32, pos int, row, col uint32, out *ccore.Token, fail *[2]ccore.Length) bool {
	f.n++
	if f.n <= 2 {
		*out = ccore.Token{LookaheadEnd: uint32(pos)}
		start := ccore.NewLength(uint32(pos), row, col)
		fail[0], fail[1] = start, start
		return false
	}
	return f.inner.Scan(state, pos, row, col, out, fail)
}

func TestScanProviderAnswerIsUsed(t *testing.T) {
	for _, g := range loadGoldens(t) {
		if g.Name != "json-object" && !strings.Contains(g.Name, "object") {
			continue
		}
		_, nilDigest, _, _, _ := replayWith(t, g.Tables, g.Src, nil)
		f := &failFirst{inner: ccore.NewTableScan(g.Tables, g.Src)}
		_, digest, _, _, _ := replayWith(t, g.Tables, g.Src, f)
		if f.n == 0 || digest == nilDigest {
			t.Errorf("%s: a provider that refuses the first two scans left the tree unchanged (calls %d)", g.Name, f.n)
		}
		return
	}
	t.Skip("no object golden")
}

// A provider bound for one parse is not bound for the next.
func TestScanProviderBindingEnds(t *testing.T) {
	g := loadGoldens(t)[0]
	p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
	if err != nil {
		t.Fatal(err)
	}
	prov := ccore.NewTableScan(g.Tables, g.Src)
	tr, stop := p.ParseWithScan(g.Src, nil, prov)
	if stop != "" {
		t.Fatal(stop)
	}
	tr.Release()
	calls := prov.Calls()
	tr, _ = p.Parse(g.Src)
	tr.Release()
	if prov.Calls() != calls {
		t.Fatalf("the provider was asked by a parse that bound none: %d then %d", calls, prov.Calls())
	}
}
