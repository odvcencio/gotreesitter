//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

var lengthNeutralReuseLanguages = []string{"lua", "nickel", "starlark", "properties", "firrtl"}

// Keep focused runs in the current process and ordinary tagged runs isolated
// to one grammar per child process, as in the scanner certificate tests.
func lengthNeutralReuseLanguage(t *testing.T) string {
	t.Helper()
	if name := os.Getenv("GTS_ZERO_REUSE_LANGUAGE"); name != "" {
		return name
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := "^" + regexp.QuoteMeta(t.Name()) + "$"
	for _, grammar := range lengthNeutralReuseLanguages {
		t.Run(grammar, func(t *testing.T) {
			cmd := exec.Command(binary, "-test.run="+run, "-test.count=1", "-test.timeout=2m")
			cmd.Env = append(os.Environ(), "GTS_ZERO_REUSE_LANGUAGE="+grammar)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("scanner reuse process: %v\n%s", err, output)
			}
		})
	}
	return ""
}

func lengthNeutralReuseSample(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile("../internal/benchfixtures/real_corpus.json")
	if err != nil {
		tb.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language string `json:"language"`
			Role     string `json:"role"`
			Path     string `json:"committed_path"`
			SHA256   string `json:"sha256"`
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		tb.Fatal(err)
	}
	for _, row := range manifest.Entries {
		if row.Language != name || row.Role != "sample" {
			continue
		}
		source, err := os.ReadFile(filepath.Join("../internal/benchfixtures", row.Path))
		if err != nil {
			tb.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(source)) != row.SHA256 {
			tb.Fatal("sample digest changed")
		}
		return source
	}
	tb.Fatalf("no pinned sample for %s", name)
	return nil
}

// Run each subtest in its own process. These are the exact first edits used
// by the counter ledger, including their locked sample hashes.
func TestLengthNeutralScannerReusePinnedSample(t *testing.T) {
	for _, name := range lengthNeutralReuseLanguages {
		t.Run(name, func(t *testing.T) {
			source := lengthNeutralReuseSample(t, name)
			step := benchfixtures.EditingSession(source)[0]
			lang := grammars.DetectLanguageByName(name).Language()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			cp := lengthNeutralReuseCParser(t, name)
			defer cp.Close()
			cOld := cp.Parse(source, nil)
			defer cOld.Close()
			assertLockedCTreeExact(t, "initial", old, lang, cOld)
			old.Edit(step.Edit)
			cEdit := realCorpusCInputEdit(step.Edit)
			cOld.Edit(&cEdit)
			next, profile, err := p.ParseIncrementalProfiled(step.Source, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			fresh, err := p.Parse(step.Source)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			ct := cp.Parse(step.Source, cOld)
			defer ct.Close()
			assertLockedCTreeExact(t, "incremental", next, lang, ct)
			assertLockedCTreeExact(t, "fresh", fresh, lang, ct)
			if profile.ReusedBytes == 0 || profile.ReusedSubtrees == 0 || profile.ReuseUnsupported {
				t.Fatalf("no certified reuse: %+v", profile)
			}
			t.Logf("SCANNER_REUSE language=%s source=%x edit=%d reused_bytes=%d reused_subtrees=%d tokens=%d nodes=%d", name, sha256.Sum256(source), step.Edit.StartByte, profile.ReusedBytes, profile.ReusedSubtrees, profile.TokensConsumed, profile.NewNodesAllocated)
		})
	}
}

func lengthNeutralReuseCParser(tb testing.TB, name string) *sitter.Parser {
	tb.Helper()
	cl, err := COracleLanguage(name)
	if err != nil {
		tb.Fatal(err)
	}
	cp := sitter.NewParser()
	if err := cp.SetLanguage(cl); err != nil {
		cp.Close()
		tb.Fatal(err)
	}
	return cp
}

// This workload includes Tree.Edit, parsing, and release. Each iteration
// toggles the same pinned insertion and its inverse; setup stays untimed.
func BenchmarkLengthNeutralScannerCompleteEdit(b *testing.B) {
	name := os.Getenv("GTS_ZERO_REUSE_LANGUAGE")
	if name == "" {
		b.Fatal("set GTS_ZERO_REUSE_LANGUAGE to one grammar")
	}
	source, step := lengthNeutralReuseWorkload(b, name)
	restore := canonicalGoInputEdit(step.Source, source, int(step.Edit.StartByte), int(step.Edit.NewEndByte), int(step.Edit.OldEndByte))
	for _, impl := range []string{"Go1", "C1", "C2", "Go2"} {
		b.Run(impl, func(b *testing.B) {
			b.ReportAllocs()
			if impl == "Go1" || impl == "Go2" {
				p := gts.NewParser(grammars.DetectLanguageByName(name).Language())
				p.SetAdmissionCandidateRoute(false)
				old, err := p.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					after, edit := step.Source, step.Edit
					if i&1 != 0 {
						after, edit = source, restore
					}
					old.Edit(edit)
					next, err := p.ParseIncremental(after, old)
					if err != nil {
						b.Fatal(err)
					}
					old.Release()
					old = next
				}
				b.StopTimer()
				old.Release()
			} else {
				p := lengthNeutralReuseCParser(b, name)
				defer p.Close()
				old := p.Parse(source, nil)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					after, edit := step.Source, step.Edit
					if i&1 != 0 {
						after, edit = source, restore
					}
					cEdit := realCorpusCInputEdit(edit)
					old.Edit(&cEdit)
					next := p.Parse(after, old)
					old.Close()
					old = next
				}
				b.StopTimer()
				old.Close()
			}
		})
	}
}

func BenchmarkLengthNeutralScannerFull(b *testing.B) {
	name := os.Getenv("GTS_ZERO_REUSE_LANGUAGE")
	if name == "" {
		b.Fatal("set GTS_ZERO_REUSE_LANGUAGE to one grammar")
	}
	source, _ := lengthNeutralReuseWorkload(b, name)
	source = bytes.Clone(source)
	p := gts.NewParser(grammars.DetectLanguageByName(name).Language())
	p.SetAdmissionCandidateRoute(false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := p.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}

// Preserve the proof across alternating edits, including scanner opt-outs.
func TestLengthNeutralScannerInverseEdits(t *testing.T) {
	name := lengthNeutralReuseLanguage(t)
	if name == "" {
		return
	}
	source := lengthNeutralReuseSample(t, name)
	step := benchfixtures.EditingSession(source)[0]
	restore := canonicalGoInputEdit(step.Source, source, int(step.Edit.StartByte), int(step.Edit.NewEndByte), int(step.Edit.OldEndByte))
	lang := grammars.DetectLanguageByName(name).Language()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	tree, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { tree.Release() }()
	cp := lengthNeutralReuseCParser(t, name)
	defer cp.Close()
	ct := cp.Parse(source, nil)
	defer func() { ct.Close() }()
	for i := 0; i < 96; i++ {
		after, edit := step.Source, step.Edit
		if i%2 != 0 {
			after, edit = source, restore
		}
		tree.Edit(edit)
		ce := realCorpusCInputEdit(edit)
		ct.Edit(&ce)
		next, profile, err := p.ParseIncrementalProfiled(after, tree)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := p.Parse(after)
		if err != nil {
			t.Fatal(err)
		}
		cn := cp.Parse(after, ct)
		assertLockedCTreeExact(t, fmt.Sprintf("inverse-%d incremental", i), next, lang, cn)
		assertLockedCTreeExact(t, fmt.Sprintf("inverse-%d fresh", i), fresh, lang, cn)
		if profile.ReusedBytes == 0 || (profile.TokensConsumed != 0 || profile.NewNodesAllocated != 0) {
			t.Fatalf("edit %d lost reuse: %+v", i, profile)
		}
		tree.Release()
		tree = next
		fresh.Release()
		ct.Close()
		ct = cn
	}
	for _, added := range []byte{'=', 'e', 'm', 's'} {
		nextSource := append(append(append([]byte{}, source[:step.Edit.StartByte]...), added), source[step.Edit.StartByte:]...)
		edit := canonicalGoInputEdit(source, nextSource, int(step.Edit.StartByte), int(step.Edit.StartByte), int(step.Edit.StartByte)+1)
		old, err := p.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		old.Edit(edit)
		next, err := p.ParseIncremental(nextSource, old)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := p.Parse(nextSource)
		if err != nil {
			t.Fatal(err)
		}
		cn := cp.Parse(nextSource, nil)
		assertLockedCTreeExactWithErrors(t, fmt.Sprintf("changed-byte-%q incremental", added), next, lang, cn)
		assertLockedCTreeExactWithErrors(t, fmt.Sprintf("changed-byte-%q fresh", added), fresh, lang, cn)
		old.Release()
		next.Release()
		fresh.Release()
		cn.Close()
	}
}

func BenchmarkLengthNeutralScannerNoEdit(b *testing.B) {
	name := os.Getenv("GTS_ZERO_REUSE_LANGUAGE")
	source := lengthNeutralReuseSample(b, name)
	p := gts.NewParser(grammars.DetectLanguageByName(name).Language())
	p.SetAdmissionCandidateRoute(false)
	tree, err := p.Parse(source)
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, err := p.ParseIncremental(source, tree)
		if err != nil {
			b.Fatal(err)
		}
		next.Release()
	}
}

// Generated files contain short tokens, so the lexer proof has the same bound
// at 32 KiB, 137 KiB, and 1 MiB. A giant single token deliberately falls back
// when authenticating it would exceed the existing proof budget.
func lengthNeutralReuseWorkload(tb testing.TB, name string) ([]byte, benchfixtures.EditStep) {
	tb.Helper()
	raw := os.Getenv("GTS_ZERO_REUSE_BYTES")
	if raw == "" {
		source := lengthNeutralReuseSample(tb, name)
		return source, benchfixtures.EditingSession(source)[0]
	}
	size, err := strconv.Atoi(raw)
	if err != nil || size < 1024 {
		tb.Fatal("invalid generated byte count")
	}
	var source []byte
	if name == "nickel" {
		source = append(source, '{', '\n')
	}
	if name == "firrtl" {
		source = []byte("circuit Example:\n  module Example:\n")
	}
	for i := 0; len(source) < size; i++ {
		var line string
		switch name {
		case "lua":
			line = fmt.Sprintf("local value%06d = \"alpha beta gamma delta\"\n", i)
		case "starlark":
			line = fmt.Sprintf("value%06d = \"alpha beta gamma delta\"\n", i)
		case "nickel":
			line = fmt.Sprintf("value%06d = \"alpha beta gamma delta\",\n", i)
		case "properties":
			line = fmt.Sprintf("# alpha beta gamma delta\nvalue%06d=example\n", i)
		case "firrtl":
			line = fmt.Sprintf("    wire value%06d: UInt<4> ; alpha beta gamma delta\n", i)
		default:
			tb.Fatal("unsupported generated grammar")
		}
		source = append(source, line...)
	}
	if name == "nickel" {
		source = append(source, '}', '\n')
	}
	half := len(source) / 2
	needle, interior := "alpha", 2
	if name == "starlark" {
		// Its visible string content is a nonterminal over scanner tokens;
		// identifiers satisfy the unchanged-terminal-tape premise.
		needle, interior = "value", 3
	}
	rel := strings.Index(string(source[half:]), needle)
	if rel < 0 {
		tb.Fatal("missing generated edit run")
	}
	at := half + rel + interior
	next := append(append(append([]byte{}, source[:at]...), 'x'), source[at:]...)
	edit := canonicalGoInputEdit(source, next, at, at, at+1)
	return source, benchfixtures.EditStep{Source: next, Edit: edit}
}

func TestLengthNeutralScannerGenerated(t *testing.T) {
	name := lengthNeutralReuseLanguage(t)
	if name == "" {
		return
	}
	source, step := lengthNeutralReuseWorkload(t, name)
	lang := grammars.DetectLanguageByName(name).Language()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	cp := lengthNeutralReuseCParser(t, name)
	defer cp.Close()
	ct := cp.Parse(source, nil)
	defer ct.Close()
	assertLockedCTreeExact(t, "generated initial", old, lang, ct)
	old.Edit(step.Edit)
	ce := realCorpusCInputEdit(step.Edit)
	ct.Edit(&ce)
	next, profile, err := p.ParseIncrementalProfiled(step.Source, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	fresh, err := p.Parse(step.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	cn := cp.Parse(step.Source, ct)
	defer cn.Close()
	assertLockedCTreeExact(t, "generated incremental", next, lang, cn)
	assertLockedCTreeExact(t, "generated fresh", fresh, lang, cn)
	if profile.ReusedBytes == 0 || next.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(step.Source)) {
		t.Fatalf("incomplete generated reuse: %+v", profile)
	}
	t.Logf("GENERATED language=%s bytes=%d reused_bytes=%d tokens=%d nodes=%d arena_bytes=%d max_stacks=%d", name, len(source), profile.ReusedBytes, profile.TokensConsumed, profile.NewNodesAllocated, profile.ArenaBytesAllocated, profile.MaxStacksSeen)
}

func TestLengthNeutralScannerBudgetFallback(t *testing.T) {
	name := lengthNeutralReuseLanguage(t)
	if name == "" {
		return
	}
	text := strings.Repeat("x", 70000)
	var source []byte
	switch name {
	case "lua":
		source = []byte("local value = \"" + text + "\"\n")
	case "nickel":
		source = []byte("{ value = \"" + text + "\" }\n")
	case "starlark":
		source = []byte(text + " = \"value\"\n")
	case "properties":
		source = []byte("# " + text + "\nvalue=example\n")
	case "firrtl":
		source = []byte("; " + text + "\ncircuit Example:\n  module Example:\n    wire value: UInt<4>\n")
	default:
		t.Fatal("set GTS_ZERO_REUSE_LANGUAGE to one certified grammar")
	}
	at := len(source) / 2
	nextSource := append(append(append([]byte{}, source[:at]...), 'x'), source[at:]...)
	edit := canonicalGoInputEdit(source, nextSource, at, at, at+1)
	lang := grammars.DetectLanguageByName(name).Language()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(edit)
	next, profile, err := p.ParseIncrementalProfiled(nextSource, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	fresh, err := p.Parse(nextSource)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	cp := lengthNeutralReuseCParser(t, name)
	defer cp.Close()
	ct := cp.Parse(nextSource, nil)
	defer ct.Close()
	assertLockedCTreeExact(t, "budget fallback incremental", next, lang, ct)
	assertLockedCTreeExact(t, "budget fallback fresh", fresh, lang, ct)
	if profile.ReusedBytes != 0 || profile.NewNodesAllocated == 0 {
		t.Fatalf("unbounded token was reused: %+v", profile)
	}
}

func TestLengthNeutralScannerNonterminalFallback(t *testing.T) {
	source := []byte("value = \"alpha beta gamma\"\n")
	at := bytes.Index(source, []byte("alpha")) + 2
	nextSource := append(append(append([]byte{}, source[:at]...), 'x'), source[at:]...)
	edit := canonicalGoInputEdit(source, nextSource, at, at, at+1)
	lang := grammars.DetectLanguageByName("starlark").Language()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(edit)
	next, profile, err := p.ParseIncrementalProfiled(nextSource, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	fresh, err := p.Parse(nextSource)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	cp := lengthNeutralReuseCParser(t, "starlark")
	defer cp.Close()
	ct := cp.Parse(nextSource, nil)
	defer ct.Close()
	assertLockedCTreeExact(t, "nonterminal fallback incremental", next, lang, ct)
	assertLockedCTreeExact(t, "nonterminal fallback fresh", fresh, lang, ct)
	if profile.ReusedBytes != 0 {
		t.Fatalf("nonterminal reused as a terminal: %+v", profile)
	}
}
