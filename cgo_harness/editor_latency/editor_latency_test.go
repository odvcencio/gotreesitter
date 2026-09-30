//go:build cgo && treesitter_c_parity

// Package editorlatency is a benchmark driver, not parser engine code. The
// campaign compiles this exact driver against both revisions before timing.
package editorlatency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	oracle "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type fixture struct {
	Language          string `json:"language"`
	Repo              string `json:"repo"`
	Commit            string `json:"commit"`
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	Bytes             int    `json:"bytes"`
	OneByteOffset     int    `json:"one_byte_offset"`
	OneByteOld        string `json:"one_byte_old"`
	OneByteNew        string `json:"one_byte_new"`
	HundredByteOffset int    `json:"hundred_byte_offset"`
	HundredByteText   string `json:"hundred_byte_text"`
	TypingText        string `json:"typing_text"`
	TypingPrefix      string `json:"typing_prefix"`
}

type manifest struct {
	Schema                string    `json:"schema"`
	Languages             []string  `json:"languages"`
	SupplementalLanguages []string  `json:"supplemental_languages"`
	Fixtures              []fixture `json:"fixtures"`
}

type step struct {
	source []byte
	edit   gts.InputEdit
}

type workload struct {
	name    string
	initial []byte
	steps   []step
}

type driver struct {
	fixture fixture
	source  []byte
	entry   grammars.LangEntry
	lang    *gts.Language
	cLang   *sitter.Language
}

type counters struct {
	Tokens         uint64 `json:"tokens"`
	Nodes          uint64 `json:"nodes"`
	MaxStacks      uint64 `json:"max_stacks"`
	ReusedSubtrees uint64 `json:"reused_subtrees"`
	ReusedBytes    uint64 `json:"reused_bytes"`
}

type stepReceipt struct {
	SourceSHA256 string   `json:"source_sha256"`
	TreeSHA256   string   `json:"tree_sha256"`
	HasError     bool     `json:"has_error"`
	Counters     counters `json:"counters"`
}

type cellReceipt struct {
	Workload string        `json:"workload"`
	Steps    []stepReceipt `json:"steps"`
}

type receipt struct {
	Schema            string                      `json:"schema"`
	Revision          string                      `json:"revision"`
	Language          string                      `json:"language"`
	FixtureSHA256     string                      `json:"fixture_sha256"`
	ManifestSHA256    string                      `json:"manifest_sha256"`
	InitialTreeSHA256 string                      `json:"initial_tree_sha256"`
	NoEditAllocs      float64                     `json:"no_edit_allocs"`
	Oracle            oracle.COracleBuildIdentity `json:"oracle"`
	Cells             []cellReceipt               `json:"cells"`
}

func TestMain(m *testing.M) {
	// The oracle locates its lock relative to the working directory. Both
	// revision binaries use the campaign's one pinned oracle checkout.
	if root := os.Getenv("GTS_EDIT_REPO_ROOT"); root != "" {
		if err := os.Chdir(root); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadDriver(tb testing.TB) (driver, string) {
	tb.Helper()
	manifestBytes, err := os.ReadFile(os.Getenv("GTS_EDIT_MANIFEST"))
	if err != nil {
		tb.Fatal(err)
	}
	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		tb.Fatal(err)
	}
	if m.Schema != "w5-editor-fixtures-v1" {
		tb.Fatalf("unknown manifest schema %q", m.Schema)
	}
	name := os.Getenv("GTS_EDIT_LANGUAGE")
	var f fixture
	count := 0
	for _, candidate := range m.Fixtures {
		if candidate.Language == name {
			f = candidate
			count++
		}
	}
	if count != 1 {
		tb.Fatalf("language %q has %d fixtures, want one", name, count)
	}
	source, err := os.ReadFile(filepath.Join(os.Getenv("GTS_EDIT_FIXTURES"), name))
	if err != nil {
		tb.Fatal(err)
	}
	if digest(source) != f.SHA256 || len(source) != f.Bytes {
		tb.Fatal("fixture hash/size mismatch")
	}
	if len(f.OneByteOld) != 1 || len(f.OneByteNew) != 1 || f.OneByteOld == f.OneByteNew ||
		f.OneByteOffset < 0 || f.OneByteOffset >= len(source) || string(source[f.OneByteOffset]) != f.OneByteOld {
		tb.Fatal("one-byte edit is not a pinned, changed byte")
	}
	if f.HundredByteOffset < 0 || f.HundredByteOffset > len(source) || len(f.HundredByteText) != 100 {
		tb.Fatal("hundred-byte edit must insert exactly 100 pinned bytes")
	}
	if len(f.TypingText) < 8 {
		tb.Fatal("typing needs at least eight EOF keystrokes")
	}
	var entry grammars.LangEntry
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
			break
		}
	}
	if entry.Language == nil {
		tb.Fatalf("no grammar for %q", name)
	}
	cLang, err := oracle.ParityCLanguage(name)
	if err != nil {
		tb.Fatal(err)
	}
	return driver{fixture: f, source: source, entry: entry, lang: entry.Language(), cLang: cLang}, digest(manifestBytes)
}

func point(source []byte, offset int) gts.Point {
	var p gts.Point
	for _, ch := range source[:offset] {
		if ch == '\n' {
			p.Row++
			p.Column = 0
		} else {
			p.Column++
		}
	}
	return p
}

func editStep(source []byte, start, end int, replacement []byte) step {
	out := make([]byte, 0, len(source)+len(replacement)-(end-start))
	out = append(out, source[:start]...)
	out = append(out, replacement...)
	out = append(out, source[end:]...)
	return step{source: out, edit: gts.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(start + len(replacement)),
		StartPoint: point(source, start), OldEndPoint: point(source, end), NewEndPoint: point(out, start+len(replacement)),
	}}
}

func (d driver) workloads() []workload {
	f := d.fixture
	one := editStep(d.source, f.OneByteOffset, f.OneByteOffset+1, []byte(f.OneByteNew))
	oneUndo := editStep(one.source, f.OneByteOffset, f.OneByteOffset+1, []byte(f.OneByteOld))
	hundred := editStep(d.source, f.HundredByteOffset, f.HundredByteOffset, []byte(f.HundredByteText))
	hundredUndo := editStep(hundred.source, f.HundredByteOffset, f.HundredByteOffset+100, nil)
	source := append(append([]byte(nil), d.source...), []byte(f.TypingPrefix)...)
	typing := workload{name: "typing", initial: source}
	for _, ch := range []byte(f.TypingText) {
		s := editStep(source, len(source), len(source), []byte{ch})
		typing.steps = append(typing.steps, s)
		source = s.source
	}
	return []workload{{name: "one_byte", initial: d.source, steps: []step{one, oneUndo}}, {name: "hundred_byte", initial: d.source, steps: []step{hundred, hundredUndo}}, typing}
}

func (d driver) parse(tb testing.TB, p *gts.Parser, source []byte, old *gts.Tree, profiled bool) (*gts.Tree, gts.IncrementalParseProfile) {
	tb.Helper()
	var tree *gts.Tree
	var profile gts.IncrementalParseProfile
	var err error
	if d.entry.TokenSourceFactory != nil {
		ts := d.entry.TokenSourceFactory(source, d.lang)
		if old == nil {
			tree, err = p.ParseWithTokenSource(source, ts)
		} else if profiled {
			tree, profile, err = p.ParseIncrementalWithTokenSourceProfiled(source, old, ts)
		} else {
			tree, err = p.ParseIncrementalWithTokenSource(source, old, ts)
		}
	} else if old == nil {
		tree, err = p.Parse(source)
	} else if profiled {
		tree, profile, err = p.ParseIncrementalProfiled(source, old)
	} else {
		tree, err = p.ParseIncremental(source, old)
	}
	if err != nil {
		tb.Fatal(err)
	}
	if tree == nil || tree.RootNode() == nil {
		tb.Fatal("nil Go tree")
	}
	rt := tree.ParseRuntime()
	r := tree.RootNode()
	if rt.StopReason != gts.ParseStopAccepted || rt.Truncated || rt.TokenSourceEOFEarly || r.EndByte() != uint32(len(source)) {
		tb.Fatalf("incomplete Go parse: end=%d bytes=%d runtime=%s", r.EndByte(), len(source), rt.Summary())
	}
	if r.Type(d.lang) == "ERROR" && !r.HasError() {
		tb.Fatal("ERROR root without HasError")
	}
	return tree, profile
}

func (d driver) cParser(tb testing.TB) *sitter.Parser {
	tb.Helper()
	p := sitter.NewParser()
	if err := p.SetLanguage(d.cLang); err != nil {
		p.Close()
		tb.Fatal(err)
	}
	return p
}

func cEdit(e gts.InputEdit) sitter.InputEdit {
	return sitter.InputEdit{StartByte: uint(e.StartByte), OldEndByte: uint(e.OldEndByte), NewEndByte: uint(e.NewEndByte),
		StartPosition:  sitter.Point{Row: uint(e.StartPoint.Row), Column: uint(e.StartPoint.Column)},
		OldEndPosition: sitter.Point{Row: uint(e.OldEndPoint.Row), Column: uint(e.OldEndPoint.Column)},
		NewEndPosition: sitter.Point{Row: uint(e.NewEndPoint.Row), Column: uint(e.NewEndPoint.Column)}}
}

func cParse(tb testing.TB, p *sitter.Parser, source []byte, old *sitter.Tree) *sitter.Tree {
	tb.Helper()
	tree := p.Parse(source, old)
	if tree == nil || tree.RootNode() == nil || tree.RootNode().EndByte() != uint(len(source)) {
		tb.Fatal("nil or incomplete C parse")
	}
	return tree
}

func (d driver) goDigest(tb testing.TB, tree *gts.Tree) string {
	tb.Helper()
	i, err := benchfixtures.InspectGoTree(tree.RootNode(), d.lang)
	if err != nil {
		tb.Fatal(err)
	}
	return i.SHA256
}

func cDigest(tb testing.TB, tree *sitter.Tree) string {
	tb.Helper()
	h, err := oracle.COracleDeepDigest(tree)
	if err != nil {
		tb.Fatal(err)
	}
	return h
}

// TestW5RealCodeEdits admits every step before timing. No known-difference,
// candidate-search, or timing-eligibility skip can remove a gate cell.
func TestW5RealCodeEdits(t *testing.T) {
	d, manifestSHA := loadDriver(t)
	identity, err := oracle.COracleIdentity(d.fixture.Language)
	if err != nil {
		t.Fatal(err)
	}
	// The receipt is public: retain content identities, not machine paths.
	identity.GrammarArtifactPath = filepath.Base(identity.GrammarArtifactPath)
	identity.CompilerPath = filepath.Base(identity.CompilerPath)
	r := receipt{Schema: "w5-edit-admission-v1", Revision: os.Getenv("GTS_EDIT_REVISION"), Language: d.fixture.Language,
		FixtureSHA256: d.fixture.SHA256, ManifestSHA256: manifestSHA, Oracle: identity}
	gp := gts.NewParser(d.lang)
	cp := d.cParser(t)
	defer cp.Close()
	gInitial, _ := d.parse(t, gp, d.source, nil, false)
	cInitial := cParse(t, cp, d.source, nil)
	r.InitialTreeSHA256 = d.goDigest(t, gInitial)
	if r.InitialTreeSHA256 != cDigest(t, cInitial) {
		t.Fatal("initial fresh Go != fresh C")
	}
	if gInitial.RootNode().HasError() || cInitial.RootNode().HasError() {
		t.Fatal("initial real-code fixture is not clean")
	}
	cInitial.Close()
	r.NoEditAllocs = testing.AllocsPerRun(10, func() {
		unchanged, err := gp.ParseIncremental(d.source, gInitial)
		if err != nil || unchanged != gInitial {
			t.Fatal("no-edit reparse did not preserve tree identity")
		}
	})
	gInitial.Release()
	if r.NoEditAllocs != 0 {
		t.Fatalf("no-edit allocations=%g want zero", r.NoEditAllocs)
	}
	for _, w := range d.workloads() {
		cell := cellReceipt{Workload: w.name}
		if !t.Run(w.name, func(t *testing.T) {
			old, _ := d.parse(t, gp, w.initial, nil, false)
			plainParser := gts.NewParser(d.lang)
			plainOld, _ := d.parse(t, plainParser, w.initial, nil, false)
			cseed := cParse(t, cp, w.initial, nil)
			defer cseed.Close()
			seedDigest := cDigest(t, cseed)
			cold := cseed.Clone()
			defer func() { old.Release(); plainOld.Release(); cold.Close() }()
			if d.goDigest(t, old) != cDigest(t, cold) || old.RootNode().HasError() || cold.RootNode().HasError() {
				t.Fatalf("session seed must be clean and match fresh C: GoError=%v CError=%v Go=%s C=%s", old.RootNode().HasError(), cold.RootNode().HasError(), d.goDigest(t, old), cDigest(t, cold))
			}
			for index, s := range w.steps {
				fresh, _ := d.parse(t, gts.NewParser(d.lang), s.source, nil, false)
				cfp := d.cParser(t)
				cfresh := cParse(t, cfp, s.source, nil)
				want := d.goDigest(t, fresh)
				cwant := cDigest(t, cfresh)
				fresh.Release()
				cfresh.Close()
				cfp.Close()
				if want != cwant {
					t.Fatalf("step %d fresh Go != fresh C: %s != %s", index, want, cwant)
				}
				old.Edit(s.edit)
				next, profile := d.parse(t, gp, s.source, old, true)
				if next != old {
					old.Release()
				}
				old = next
				plainOld.Edit(s.edit)
				plainNext, _ := d.parse(t, plainParser, s.source, plainOld, false)
				if plainNext != plainOld {
					plainOld.Release()
				}
				plainOld = plainNext
				ce := cEdit(s.edit)
				cold.Edit(&ce)
				cnext := cParse(t, cp, s.source, cold)
				cold.Close()
				cold = cnext
				got := d.goDigest(t, old)
				if got != want || d.goDigest(t, plainOld) != want || cDigest(t, cold) != want {
					t.Fatalf("step %d incremental != fresh: Go=%s fresh=%s C=%s", index, got, want, cDigest(t, cold))
				}
				cell.Steps = append(cell.Steps, stepReceipt{SourceSHA256: digest(s.source), TreeSHA256: want, HasError: old.RootNode().HasError(),
					Counters: counters{Tokens: profile.TokensConsumed, Nodes: profile.NewNodesAllocated, MaxStacks: uint64(profile.MaxStacksSeen),
						ReusedSubtrees: profile.ReusedSubtrees, ReusedBytes: profile.ReusedBytes}})
			}
			// Timing restores C sessions with a cheap clone, not a full parse.
			// Prove the seed stayed immutable and replay with the same parser.
			if cDigest(t, cseed) != seedDigest {
				t.Fatal("C edit mutated its immutable seed")
			}
			replay := cseed.Clone()
			defer func() { replay.Close() }()
			for index, s := range w.steps {
				ce := cEdit(s.edit)
				replay.Edit(&ce)
				next := cParse(t, cp, s.source, replay)
				replay.Close()
				replay = next
				if cDigest(t, replay) != cell.Steps[index].TreeSHA256 {
					t.Fatalf("C restored session differs at step %d", index)
				}
			}
		}) {
			return
		}
		r.Cells = append(r.Cells, cell)
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("GTS_EDIT_ADMISSION_OUT"), append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

// Each shuffled workload runs Go-C-C-Go on one thread. An operation is a
// complete edit session, so calibration cannot silently drop late keystrokes.
// The reducer divides time and allocation metrics by edits/op.
func BenchmarkW5OneByte(b *testing.B)     { benchmarkPair(b, "one_byte") }
func BenchmarkW5HundredByte(b *testing.B) { benchmarkPair(b, "hundred_byte") }
func BenchmarkW5Typing(b *testing.B)      { benchmarkPair(b, "typing") }

func benchmarkPair(b *testing.B, name string) {
	d, _ := loadDriver(b)
	var w workload
	for _, candidate := range d.workloads() {
		if candidate.name == name {
			w = candidate
		}
	}
	for _, backend := range []string{"GoA", "CA", "CB", "GoB"} {
		b.Run(backend, func(b *testing.B) {
			if backend[0] == 'G' {
				benchmarkGo(b, d, w)
			} else {
				benchmarkC(b, d, w)
			}
		})
	}
}

func benchmarkGo(b *testing.B, d driver, w workload) {
	p := gts.NewParser(d.lang)
	old, _ := d.parse(b, p, w.initial, nil, false)
	defer func() { old.Release() }()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if w.name == "typing" && i > 0 {
			b.StopTimer()
			old.Release()
			old, _ = d.parse(b, p, w.initial, nil, false)
			b.StartTimer()
		}
		for _, s := range w.steps {
			old.Edit(s.edit)
			next, _ := d.parse(b, p, s.source, old, false)
			if next != old {
				old.Release()
			}
			old = next
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(w.steps)), "edits/op")
}

func benchmarkC(b *testing.B, d driver, w workload) {
	p := d.cParser(b)
	defer p.Close()
	seed := cParse(b, p, w.initial, nil)
	defer seed.Close()
	old := seed.Clone()
	defer func() { old.Close() }()
	var editTime time.Duration
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if w.name == "typing" && i > 0 {
			old.Close()
			old = seed.Clone()
		}
		// Repeated StopTimer/StartTimer pairs force runtime memory snapshots.
		// That untimed work can dwarf C's actual edit CPU. Time a whole typing
		// session directly; cloning is excluded from ns/op, while its tiny Go
		// binding allocation remains in C's allocation metrics.
		var started time.Time
		if w.name == "typing" {
			started = time.Now()
		}
		for _, s := range w.steps {
			ce := cEdit(s.edit)
			old.Edit(&ce)
			next := cParse(b, p, s.source, old)
			old.Close()
			old = next
		}
		if w.name == "typing" {
			editTime += time.Since(started)
		}
	}
	b.StopTimer()
	if w.name == "typing" {
		b.ReportMetric(float64(editTime.Nanoseconds())/float64(b.N), "ns/op")
	}
	b.ReportMetric(float64(len(w.steps)), "edits/op")
}

func TestW5EditConstruction(t *testing.T) {
	s := editStep([]byte("a\nbc"), 2, 3, []byte("XY\n"))
	if string(s.source) != "a\nXY\nc" || s.edit.StartPoint != (gts.Point{Row: 1}) || s.edit.NewEndPoint != (gts.Point{Row: 2}) {
		t.Fatal(s)
	}
	if fmt.Sprint(point(s.source, len(s.source))) != fmt.Sprint(gts.Point{Row: 2, Column: 1}) {
		t.Fatal("EOF point")
	}
	if strings.Contains(string(s.source), "bc") {
		t.Fatal("edit retained replaced bytes")
	}
}
