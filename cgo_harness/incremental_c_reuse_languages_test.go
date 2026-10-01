//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

var cReuseLanguages = []string{"go", "java", "javascript", "typescript", "python", "rust", "c", "cpp"}

type cReuseEditFixture struct {
	source, edited   []byte
	forward, reverse gts.InputEdit
	lang             *gts.Language
}

// Edit the middle of the document, including a full declaration insertion for
// splice. A comment gives the 100-byte replacement a clean, identical-width
// span in every grammar. The byte edit changes an identifier in a declaration.
func cReuseLanguageFixture(tb testing.TB, name string, size int, kind string) cReuseEditFixture {
	tb.Helper()
	source, marker, err := benchfixtures.GeneratedSource(name, size)
	if err != nil {
		tb.Fatal(err)
	}
	separator := []byte("\n\n")
	if name == "java" || name == "cpp" {
		separator = []byte("\n")
	}
	boundary := bytes.Index(source[len(source)/2:], separator)
	if boundary < 0 {
		tb.Fatal("missing declaration boundary")
	}
	boundary += len(source)/2 + len(separator)
	payload := bytes.Repeat([]byte{'x'}, 100)
	comment := append([]byte("// "), payload...)
	if name == "python" {
		comment = append([]byte("# "), payload...)
	}
	comment = append(comment, '\n')
	source = bytes.Join([][]byte{source[:boundary], comment, source[boundary:]}, nil)
	var at, oldEnd, newEnd int
	var edited []byte
	switch kind {
	case "byte1":
		at = bytes.Index(source[boundary+len(comment):], []byte(marker))
		if at < 0 {
			tb.Fatal("missing middle edit marker")
		}
		at += boundary + len(comment)
		edited = bytes.Clone(source)
		edited[at] = 'y'
		oldEnd, newEnd = at+1, at+1
	case "byte100":
		at = boundary + len(comment) - 101
		oldEnd, newEnd = at+100, at+100
		edited = bytes.Clone(source)
		copy(edited[at:newEnd], bytes.Repeat([]byte{'y'}, 100))
	case "splice":
		at, oldEnd = boundary+len(comment), boundary+len(comment)
		end := bytes.Index(source[at:], separator)
		if end < 0 {
			tb.Fatal("missing splice end")
		}
		end += at + len(separator)
		declaration := bytes.Clone(source[at:end])
		edited = bytes.Join([][]byte{source[:at], declaration, source[at:]}, nil)
		newEnd = at + len(declaration)
	default:
		tb.Fatalf("unknown edit kind %q", kind)
	}
	return cReuseEditFixture{source, edited,
		canonicalGoInputEdit(source, edited, at, oldEnd, newEnd),
		canonicalGoInputEdit(edited, source, at, newEnd, oldEnd),
		grammars.DetectLanguageByName(name).Language()}
}

func TestIncrementalCReuseLanguages(t *testing.T) {
	for _, name := range cReuseLanguages {
		t.Run(name, func(t *testing.T) {
			for _, size := range []int{32, 137, 1024} {
				t.Run(fmt.Sprintf("%dKiB", size), func(t *testing.T) {
					for _, kind := range []string{"byte1", "byte100", "splice"} {
						t.Run(kind, func(t *testing.T) { testCReuseLanguage(t, name, size*1024, kind) })
					}
				})
			}
		})
	}
}

func testCReuseLanguage(t *testing.T, name string, size int, kind string) {
	f := cReuseLanguageFixture(t, name, size, kind)
	testCReuseEditFixture(t, name, kind, f)
}

func TestIncrementalCReuseScannerBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, source, marker string }{
		{"cpp", "const char *s = R\"(before)\";\nint f() { return 1; }\n", "before"},
		{"cpp", "const char *s = R\"tag(before)tag\";\nint f() { return 1; }\n", "before"},
		{"cpp", "const char *s = R\"é世(before)é世\";\nint f() { return 1; }\n", "before"},
		{"rust", "const S: &str = r###\"before\"###;\nfn f() -> i32 { 1 }\n", "before"},
		{"rust", "/// before\nfn f() -> i32 { 1 }\n", "before"},
		{"rust", "/* outer /* before */ comment */\nfn f() -> f64 { 1.0 }\n", "before"},
		{"python", "def f(a):\n    s = f\"before {a}\"\n    return s\n\ndef g():\n    return 1\n", "before"},
		{"typescript", "const s = `before ${1 + 2}`;\nconst r = /before+/;\nfunction f() { return s; }\n", "before"},
		{"typescript", "function f() {\n  const before = 1\n  return before\n}\nfunction g() { return 2; }\n", "before"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			edited := bytes.Clone(source)
			at := bytes.Index(source, []byte(tc.marker))
			edited[at] = 'y'
			f := cReuseEditFixture{source, edited,
				canonicalGoInputEdit(source, edited, at, at+1, at+1),
				canonicalGoInputEdit(edited, source, at, at+1, at+1),
				grammars.DetectLanguageByName(tc.name).Language()}
			testCReuseEditFixture(t, tc.name, "scanner-boundary", f)
		})
	}
}

func testCReuseEditFixture(t *testing.T, name, kind string, f cReuseEditFixture) {
	p := gts.NewParser(f.lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(f.source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { old.Release() }()
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 4; step++ {
		to, edit := f.edited, f.forward
		if step%2 != 0 {
			to, edit = f.source, f.reverse
		}
		cReuseBeginWorkCount()
		started := time.Now()
		old.Edit(edit)
		editNanos := time.Since(started).Nanoseconds()
		next, profile, err := p.ParseIncrementalProfiled(to, old)
		cReuseEndWorkCount(t)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := p.Parse(to)
		if err != nil {
			t.Fatal(err)
		}
		ct := cp.Parse(to, nil)
		if ct == nil {
			t.Fatal("C parse failed")
		}
		got, err := benchfixtures.InspectGoTree(next.RootNode(), f.lang)
		if err != nil {
			t.Fatal(err)
		}
		want, err := benchfixtures.InspectGoTree(fresh.RootNode(), f.lang)
		if err != nil {
			t.Fatal(err)
		}
		oracle, err := COracleDeepDigest(ct)
		if err != nil {
			t.Fatal(err)
		}
		if got.SHA256 != want.SHA256 || got.SHA256 != oracle {
			t.Fatalf("step=%d incremental=%s fresh=%s C=%s profile=%+v", step, got.SHA256, want.SHA256, oracle, profile)
		}
		if next.ParseStopReason() != gts.ParseStopAccepted || next.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(to)) {
			t.Fatalf("clean completion lost: stop=%s error=%t end=%d bytes=%d", next.ParseStopReason(), next.RootNode().HasError(), next.RootNode().EndByte(), len(to))
		}
		t.Logf("LANGUAGE_COUNTERS language=%s size=%d kind=%s step=%d source=%x tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d edit_ns=%d profile=%+v", name, len(to), kind, step, sha256.Sum256(to), profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, editNanos, profile)
		fresh.Release()
		ct.Close()
		old.Release()
		old = next
	}
	if allocs := testing.AllocsPerRun(3, func() {
		next, err := p.ParseIncremental(f.source, old)
		if err != nil {
			t.Fatal(err)
		}
		next.Release()
	}); allocs != 0 {
		t.Fatalf("no-edit allocations=%g", allocs)
	}
}

func BenchmarkIncrementalCReuseLanguages(b *testing.B) {
	seed, _ := strconv.ParseInt(flag.Lookup("test.shuffle").Value.String(), 10, 64)
	for _, name := range cReuseLanguages {
		b.Run(name, func(b *testing.B) {
			order := rand.New(rand.NewSource(seed))
			sizes := []int{32, 137, 1024}
			order.Shuffle(len(sizes), func(i, j int) { sizes[i], sizes[j] = sizes[j], sizes[i] })
			for _, size := range sizes {
				b.Run(fmt.Sprintf("%dKiB", size), func(b *testing.B) {
					kinds := []string{"byte1", "byte100", "splice"}
					order.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
					for _, kind := range kinds {
						b.Run(kind, func(b *testing.B) { benchmarkCReuseLanguage(b, name, size*1024, kind) })
					}
				})
			}
		})
	}
}

// Paired Go-C-C-Go samples include Tree.Edit, parsing, and old-tree release.
// Fixture generation and the initial fresh parse stay outside the timer.
func benchmarkCReuseLanguage(b *testing.B, name string, size int, kind string) {
	f := cReuseLanguageFixture(b, name, size, kind)
	for _, engine := range []string{"Go1", "C1", "C2", "Go2"} {
		b.Run(engine, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(f.source)))
			if engine[0] == 'G' {
				p := gts.NewParser(f.lang)
				p.SetAdmissionCandidateRoute(false)
				tree, err := p.Parse(f.source)
				if err != nil {
					b.Fatal(err)
				}
				defer func() { tree.Release() }()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					to, edit := f.edited, f.forward
					if i%2 != 0 {
						to, edit = f.source, f.reverse
					}
					tree.Edit(edit)
					next, err := p.ParseIncremental(to, tree)
					if err != nil {
						b.Fatal(err)
					}
					tree.Release()
					tree = next
				}
			} else {
				cl, err := COracleLanguage(name)
				if err != nil {
					b.Fatal(err)
				}
				p := sitter.NewParser()
				defer p.Close()
				if err := p.SetLanguage(cl); err != nil {
					b.Fatal(err)
				}
				tree := p.Parse(f.source, nil)
				if tree == nil {
					b.Fatal("C initial parse failed")
				}
				defer func() { tree.Close() }()
				forward, reverse := realCorpusCInputEdit(f.forward), realCorpusCInputEdit(f.reverse)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					to, edit := f.edited, forward
					if i%2 != 0 {
						to, edit = f.source, reverse
					}
					tree.Edit(&edit)
					next := p.Parse(to, tree)
					if next == nil {
						b.Fatal("C incremental parse failed")
					}
					tree.Close()
					tree = next
				}
			}
		})
	}
}
