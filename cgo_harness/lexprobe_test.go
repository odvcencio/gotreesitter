//go:build cgo && treesitter_c_parity

package cgoharness

// The lex probe asks the compiled C lexer of a grammar what it does in each
// lex state at the end of input and on an ill-formed byte (lookahead -1), and
// compares the answers with the exact-lexer record (internal/lexexact).
//
// How C is asked without parsing: the grammar's parser.c is compiled
// unchanged, next to a copy of the grammar's own tree_sitter/parser.h in which
// the one statement `lexer->advance(lexer, skip);` of START_LEXER is replaced
// by "store the state and the skip flag, then return". ts_lex(lexer, state)
// then runs one state of the generated code up to its first step. The test
// builds a small stub TSLexer (lookahead, eof flag) around it.
//
//	LEXPROBE_LANG=json,scss      languages to probe (the test skips if unset)
//	LEXPROBE_FULL=1              also ask C about all 1,114,112 code points of every state
//	LEXPROBE_MUTATE=eof:44       alter one entry of the main record before comparing:
//	                             eof:<state> or invalid:<state>; the test must then fail
//	LEXPROBE_MUTATE=blobhash     zero bytes 8 to 39 of the committed record (its blob identity) before
//	                             it is decoded; the test must then fail
//	LEXPROBE_REQUIRE_RECORD=1    a grammar without a registered record fails (cohort runs)
//	LEXPROBE_SRC_OVERRIDE=dir    use dir/<language>-*/src/parser.c (with its tree_sitter folder) instead of
//	                             the pinned checkout, for a grammar whose patch needs tools the
//	                             container lacks; the folder is made with the pinned generator
//	LEXPROBE_BLOB_STRICT=1       a difference between the shipped blob and C on a valid
//	                             code point fails the test (otherwise it is reported)

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"text/template"

	sitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

func TestLexProbe(t *testing.T) {
	langs := os.Getenv("LEXPROBE_LANG")
	if langs == "" {
		t.Skip("set LEXPROBE_LANG")
	}
	failed := false
	for _, name := range strings.Split(langs, ",") {
		if !lexProbeLanguage(t, strings.TrimSpace(name)) {
			failed = true
		}
	}
	if failed {
		t.Fatal("the record differs from C")
	}
}

// lexProbeSource finds the parser.c that the C oracle of this language is
// built from: the pinned checkout, with the grammar patch applied (or the folder
// named by LEXPROBE_SRC_OVERRIDE).
func lexProbeSource(t *testing.T, name string) (srcDir, parserPath string) {
	t.Helper()
	if dir := os.Getenv("LEXPROBE_SRC_OVERRIDE"); dir != "" {
		if m, _ := filepath.Glob(filepath.Join(dir, name+"-*", "src")); len(m) == 1 {
			return m[0], filepath.Join(m[0], "parser.c")
		}
	}
	lockPath, err := findParityLockPath()
	if err != nil {
		t.Fatal(err)
	}
	lock, err := loadParityLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := lock[name]
	if !ok {
		t.Fatalf("parity lock has no entry for %q", name)
	}
	repoDir, local := parityLocalRepoDir(entry)
	if !local {
		repoDir = filepath.Join(t.TempDir(), "repo")
		short := entry.Commit
		if len(short) > 12 {
			short = short[:12]
		}
		if cache := parityRepoCacheDir(); cache != "" {
			cached, err := findCachedParityRepo(cache, entry.Name, short)
			if err != nil {
				t.Fatalf("%s: parity repo cache miss: %v", name, err)
			}
			if err := clonePinnedRepoFromLocalCache(cached, entry.Commit, repoDir); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		} else if err := clonePinnedRepo(entry.RepoURL, entry.Commit, repoDir); err != nil {
			t.Fatalf("%s: clone: %v", name, err)
		}
	}
	if err := verifyPinnedRepo(repoDir, entry.Commit); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := applyParityGrammarPatch(entry, repoDir); err != nil {
		t.Fatalf("%s: grammar patch: %v", name, err)
	}
	// This probe reads the lexer only. It uses the committed (and, where a patch
	// applies, patched) parser.c as it is: the ABI check of the parser runtime,
	// which would regenerate a parser.c of an old ABI, does not apply here, and
	// a regenerated file would have another hash than the one in the record.
	srcDir = filepath.Join(repoDir, entry.Subdir)
	parserPath = filepath.Join(srcDir, "parser.c")
	if _, err := os.Stat(parserPath); err == nil {
		return srcDir, parserPath
	}
	srcDir, parserPath, err = resolveParserSource(entry, repoDir)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return srcDir, parserPath
}

const lexProbeAdvance = "lexer->advance(lexer, skip);"
const lexProbeStop = "lp_state = state; lp_skip = skip; return result;"

// lexProbeBuild compiles the probe for one parser.c and returns the path of
// the executable. parser.c is copied byte for byte; only the copy of
// tree_sitter/parser.h is changed, in the one statement above.
func lexProbeBuild(t *testing.T, name, srcDir, parserPath string, hasKeyword bool) string {
	t.Helper()
	dir := t.TempDir()
	parserC, err := os.ReadFile(parserPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parser.c"), parserC, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tree_sitter"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(srcDir, "tree_sitter"))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(srcDir, "tree_sitter", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if e.Name() == "parser.h" {
			if n := strings.Count(string(b), lexProbeAdvance); n != 1 {
				t.Fatalf("%s: parser.h holds %d copies of %q, want 1", name, n, lexProbeAdvance)
			}
			b = []byte(strings.Replace(string(b), lexProbeAdvance, lexProbeStop, 1))
		}
		if err := os.WriteFile(filepath.Join(dir, "tree_sitter", e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	header, _ := os.ReadFile(filepath.Join(dir, "tree_sitter", "parser.h"))
	tmpl, err := template.ParseFiles(filepath.Join("testdata", "lexprobe", "wrapper.c.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	var wrapper bytes.Buffer
	data := struct{ HasEOF, HasKeyword bool }{bytes.Contains(header, []byte("(*eof)")), hasKeyword}
	if err := tmpl.Execute(&wrapper, data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wrapper.c"), wrapper.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "probe")
	cmd := exec.Command("cc", "-std=c11", "-O1", "-w", "-I", ".", "-o", exe, "wrapper.c", "-Wl,--unresolved-symbols=ignore-all")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: compile probe: %v\n%s", name, err, out)
	}
	return exe
}

func lexProbeRun(t *testing.T, exe, fn string, states int, mode string, stdin string) []string {
	t.Helper()
	cmd := exec.Command(exe, fn, strconv.Itoa(states), mode)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("probe %s %s: %v\n%s", fn, mode, err, errb.String())
	}
	return strings.Split(strings.TrimSpace(out.String()), "\n")
}

// lexProbeMutate alters one entry of the main record: a step becomes "no step",
// and "no step" becomes a step to state 0.
func lexProbeMutate(t *testing.T, rec *lexexact.Tables, spec string) {
	t.Helper()
	kind, num, ok := strings.Cut(spec, ":")
	state, err := strconv.Atoi(num)
	if !ok || err != nil || state < 0 || state >= len(rec.Main.OnEOF) {
		t.Fatalf("LEXPROBE_MUTATE=%q: want eof:<state> or invalid:<state>", spec)
	}
	var steps []uint32
	switch kind {
	case "eof":
		steps = rec.Main.OnEOF
	case "invalid":
		steps = rec.Main.OnInvalid
	default:
		t.Fatalf("LEXPROBE_MUTATE=%q: unknown kind", spec)
	}
	if steps[state] != 0 {
		steps[state] = 0
	} else {
		steps[state] = 1 << 1
	}
}

func lexProbeLanguage(t *testing.T, name string) bool {
	srcDir, parserPath := lexProbeSource(t, name)
	parserC, err := os.ReadFile(parserPath)
	if err != nil {
		t.Fatal(err)
	}
	header, err := os.ReadFile(filepath.Join(srcDir, "tree_sitter", "parser.h"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := lexexact.Parse(string(parserC), string(header))
	if err != nil {
		t.Fatalf("%s: extract: %v", name, err)
	}
	// The probe reports every difference between blob and C itself, so it
	// uses the unchecked build.
	rec, err := lexexact.BuildUnchecked(src, string(parserC))
	if err != nil {
		t.Fatalf("%s: build: %v", name, err)
	}

	// A committed record, if the blob has one, must equal a fresh extraction.
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("unknown language %s", name)
	}
	lang := entry.Language()
	committed := "none"
	// The hash that the runtime reports for this grammar's blob is the key of
	// the registration and must be the one the record names.
	blobSHA, haveSHA := lang.GrammarBlobSHA256()
	if !haveSHA {
		t.Errorf("%s: the language reports no blob hash", name)
		return false
	}
	mutate := os.Getenv("LEXPROBE_MUTATE")
	if enc, ok := lexexact.Lookup(blobSHA); ok {
		if mutate == "blobhash" {
			enc = append([]byte(nil), enc...)
			for i := 8; i < 40 && i < len(enc); i++ {
				enc[i] = 0
			}
		}
		old, err := lexexact.Decode(enc)
		if err != nil {
			t.Fatalf("%s: committed record: %v", name, err)
		}
		if old.BlobSHA256 != blobSHA {
			t.Errorf("%s: the committed record names blob %x, the language reports %x", name, old.BlobSHA256, blobSHA)
			return false
		}
		if old.SourceSHA256 != rec.SourceSHA256 || !reflect.DeepEqual(old.Main, rec.Main) || !reflect.DeepEqual(old.Keyword, rec.Keyword) {
			t.Errorf("%s: the committed record differs from a fresh extraction of the oracle's parser.c", name)
			return false
		}
		committed = "equal"
	} else if os.Getenv("LEXPROBE_REQUIRE_RECORD") != "" {
		t.Errorf("%s: no record is registered under blob hash %x", name, blobSHA)
		return false
	}
	if mutate == "blobhash" && committed == "none" {
		t.Errorf("%s: LEXPROBE_MUTATE=blobhash needs a committed record", name)
		return false
	}
	if spec := mutate; spec != "" && spec != "blobhash" {
		lexProbeMutate(t, rec, spec)
	}

	exe := lexProbeBuild(t, name, srcDir, parserPath, src.Keyword != nil)
	ok := true
	checked := 0
	report := 0
	differ := func(format string, a ...any) {
		ok = false
		if report++; report <= 20 {
			t.Errorf("%s: %s", name, fmt.Sprintf(format, a...))
		}
	}

	type fn struct {
		name  string
		f     *lexexact.Func
		lexer *lexexact.Lexer
	}
	fns := []fn{{"main", &rec.Main, src.Main}}
	if src.Keyword != nil {
		fns = append(fns, fn{"keyword", &rec.Keyword, src.Keyword})
	}
	for _, f := range fns {
		n := len(f.f.Accept)
		for _, line := range lexProbeRun(t, exe, f.name, n, "special", "") {
			var kind string
			var s, acc, target, skip int
			if _, err := fmt.Sscanf(line, "%s %d %d %d %d", &kind, &s, &acc, &target, &skip); err != nil {
				t.Fatalf("%s: bad probe line %q", name, line)
			}
			want := f.f.OnEOF[s]
			if kind == "I" {
				want = f.f.OnInvalid[s]
			}
			var got uint32
			if target >= 0 {
				got = uint32(target+1)<<1 | uint32(skip)
			}
			if got != want {
				differ("%s state %d, %s: C steps %s, the record %s", f.name, s, map[string]string{"E": "at end of input", "I": "on lookahead -1"}[kind], stepText(got), stepText(want))
			}
			if wantAcc := int(f.f.Accept[s]) - 1; acc != wantAcc {
				differ("%s state %d: C accepts symbol %d, the record %d", f.name, s, acc, wantAcc)
			}
			checked += 2
		}
		blobStates := lang.LexStates
		if f.name == "keyword" {
			blobStates = lang.KeywordLexStates
		}
		c, d := lexProbeBlob(t, exe, f.name, n, blobStates, os.Getenv("LEXPROBE_FULL") == "1")
		fmt.Printf("CCLEXBLOB language=%s func=%s checked=%d differ=%d\n", name, f.name, c, d)
		if d > 0 && os.Getenv("LEXPROBE_BLOB_STRICT") == "1" {
			differ("%s: %d valid-code-point answers of the shipped blob differ from C", f.name, d)
		}
	}
	fmt.Printf("CCLEXPROBE language=%s main_states=%d keyword_states=%d record_bytes=%d committed_record=%s ok=%v\n",
		name, len(rec.Main.Accept), len(rec.Keyword.Accept), lexProbeRecordBytes(rec), committed, ok)
	if checked == 0 {
		t.Errorf("%s: nothing was compared", name)
		ok = false
	}
	ccoreCount("TestLexProbe", name, checked)
	return ok
}

func lexProbeRecordBytes(rec *lexexact.Tables) int {
	b, err := rec.Encode()
	if err != nil {
		return -1
	}
	return len(b)
}

func stepText(v uint32) string {
	if v == 0 {
		return "none"
	}
	s := "advance"
	if v&1 != 0 {
		s = "skip"
	}
	return fmt.Sprintf("%s to %d", s, int(v>>1)-1)
}

// lexProbeBlob compares C with the first matching range of the shipped blob
// for valid code points. In the default mode it asks C at the ends of every
// blob range and their neighbours; in full mode it sweeps all code points and
// compares at every point where either side changes its answer.
func lexProbeBlob(t *testing.T, exe, fnName string, n int, blob []sitter.LexState, full bool) (checked, differ int) {
	t.Helper()
	if len(blob) != n {
		t.Errorf("%s: the blob has %d states, C has %d", fnName, len(blob), n)
		return 0, 1
	}
	first := func(s int, c int) (int, int, bool) {
		for _, r := range blob[s].Transitions {
			if int(r.Lo) <= c && c <= int(r.Hi) {
				sk := 0
				if r.Skip {
					sk = 1
				}
				return r.NextState, sk, true
			}
		}
		return -1, 0, false
	}
	type ans struct{ target, skip int }
	cAt := map[[2]int]ans{}
	if full {
		type run struct{ lo, hi, target, skip int }
		runs := make([][]run, n)
		for _, line := range lexProbeRun(t, exe, fnName, n, "sweep", "") {
			var kind string
			var s, lo, hi, target, skip int
			if _, err := fmt.Sscanf(line, "%s %d %d %d %d %d", &kind, &s, &lo, &hi, &target, &skip); err != nil {
				t.Fatalf("bad probe line %q", line)
			}
			runs[s] = append(runs[s], run{lo, hi, target, skip})
		}
		for s := 0; s < n; s++ {
			pts := map[int]bool{0: true, 0x10FFFF: true}
			for _, r := range runs[s] {
				pts[r.lo], pts[r.hi] = true, true
				if r.lo > 0 {
					pts[r.lo-1] = true
				}
			}
			for _, r := range blob[s].Transitions {
				for _, d := range []int{-1, 0, 1} {
					for _, v := range []int{int(r.Lo), int(r.Hi)} {
						if v+d >= 0 && v+d <= 0x10FFFF {
							pts[v+d] = true
						}
					}
				}
			}
			for p := range pts {
				i := sort.Search(len(runs[s]), func(i int) bool { return runs[s][i].hi >= p })
				r := runs[s][i]
				cAt[[2]int{s, p}] = ans{r.target, r.skip}
			}
		}
	} else {
		pts := map[[2]int]bool{}
		for s := 0; s < n; s++ {
			base := []int{0, 1, 9, 10, 13, 32, 127, 128, 0x7FF, 0x800, 0xFFFF, 0x10000, 0x10FFFF}
			for _, r := range blob[s].Transitions {
				base = append(base, int(r.Lo)-1, int(r.Lo), int(r.Hi), int(r.Hi)+1)
			}
			for _, p := range base {
				if p >= 0 && p <= 0x10FFFF {
					pts[[2]int{s, p}] = true
				}
			}
		}
		var in strings.Builder
		keys := make([][2]int, 0, len(pts))
		for k := range pts {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(a, b int) bool {
			if keys[a][0] != keys[b][0] {
				return keys[a][0] < keys[b][0]
			}
			return keys[a][1] < keys[b][1]
		})
		for _, k := range keys {
			fmt.Fprintf(&in, "%d %d\n", k[0], k[1])
		}
		for _, line := range lexProbeRun(t, exe, fnName, n, "points", in.String()) {
			var kind string
			var s, c, target, skip int
			if _, err := fmt.Sscanf(line, "%s %d %d %d %d", &kind, &s, &c, &target, &skip); err != nil {
				t.Fatalf("bad probe line %q", line)
			}
			cAt[[2]int{s, c}] = ans{target, skip}
		}
	}
	shown := 0
	for k, a := range cAt {
		tg, sk, ok := first(k[0], k[1])
		checked++
		if !ok {
			tg, sk = -1, 0
		}
		if a.target != tg || a.skip != sk {
			differ++
			if shown++; shown <= 3 {
				fmt.Printf("CCLEXBLOB difference func=%s state=%d code_point=U+%04X C=%d/%d blob=%d/%d\n", fnName, k[0], k[1], a.target, a.skip, tg, sk)
			}
		}
	}
	return checked, differ
}
