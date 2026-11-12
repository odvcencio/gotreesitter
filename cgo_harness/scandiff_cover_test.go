//go:build cgo && treesitter_c_parity

package cgoharness

// Coverage side of the scanner differential test: build the pinned scanner
// source with gcc --coverage into a shared object, run the same calls through
// it, and read line and branch counts with gcov -b. Also reads the scanner's
// own string and character literals for the input generator.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/scandiff"
)

type sdCover struct {
	t        *testing.T
	lang     string
	out      string
	srcDir   string
	repoRoot string
	scanner  string
	objDir   string
	so       string
	symName  string
	literals [][]byte
	best     int
	files    map[string]*sdGcovFile // last measurement
}

type sdGcovFile struct {
	lines    map[int]bool // line -> executed
	branches map[string]bool
}

// sdRepoOf finds the pinned checkout of a grammar in the parity repo cache.
func sdRepoOf(lang string) (repoDir string, entry parityLockEntry, ok bool) {
	parityCRefState.mu.Lock()
	entry, found := parityCRefState.lock[lang]
	parityCRefState.mu.Unlock()
	if !found {
		return "", entry, false
	}
	short := entry.Commit
	if len(short) > 12 {
		short = short[:12]
	}
	root := parityRepoCacheDir()
	for _, base := range []string{root, filepath.Join(root, "repos")} {
		if base == "" {
			continue
		}
		if dir, err := findCachedParityRepo(base, lang, short); err == nil {
			return dir, entry, true
		}
	}
	return "", entry, false
}

// sdScannerPath finds the pinned scanner source of a grammar.
func sdScannerPath(lang string) (string, bool) {
	dir, entry, ok := sdRepoOf(lang)
	if !ok {
		return "", false
	}
	return sdPickScanner(filepath.Join(dir, entry.Subdir))
}

func sdPickScanner(dir string) (string, bool) {
	for _, n := range []string{"scanner.c", "scanner.cc", "scanner.cpp", "scanner.cxx"} {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

func newSdCover(t *testing.T, lang, out string) (*sdCover, error) {
	repo, entry, ok := sdRepoOf(lang)
	if !ok {
		return nil, fmt.Errorf("no pinned checkout for %s (set GTS_PARITY_REPO_CACHE)", lang)
	}
	// The coverage object is built from a different compile of the scanner source: check that the source is
	// the pinned commit and unmodified before it replaces the pinned object.
	if err := verifyPinnedRepo(repo, entry.Commit); err != nil {
		return nil, fmt.Errorf("the checkout of %s is not the pinned commit: %w", lang, err)
	}
	p, ok := sdPickScanner(filepath.Join(repo, entry.Subdir))
	if !ok {
		return nil, fmt.Errorf("no scanner source in %s", filepath.Join(repo, entry.Subdir))
	}
	src, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	m := regexp.MustCompile(`tree_sitter_(\w+?)_external_scanner_create`).FindSubmatch(src)
	if m == nil {
		return nil, fmt.Errorf("%s: no tree_sitter_*_external_scanner_create in the source", p)
	}
	c := &sdCover{t: t, lang: lang, out: out, srcDir: filepath.Dir(p), repoRoot: repo, scanner: p, symName: string(m[1]), literals: sdLiterals(p)}
	c.objDir, err = os.MkdirTemp(os.Getenv("TMPDIR"), "sdcov-"+lang+"-")
	if err != nil {
		return nil, err
	}
	cc, std, link := "cc", "-std=c11", "cc"
	if ext := filepath.Ext(p); ext != ".c" {
		cc, std, link = "c++", "-std=c++17", "c++"
	}
	obj := filepath.Join(c.objDir, "scanner.o")
	run := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = c.objDir
		if o, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s %v: %v\n%s", name, args, err, o)
		}
		return nil
	}
	if err := run(cc, std, "-fPIC", "-O0", "-g", "--coverage", "-I", c.srcDir, "-c", p, "-o", obj); err != nil {
		return nil, err
	}
	// libgcov's dump and reset are hidden in the shared object: export one hook that calls both.
	hook := filepath.Join(c.objDir, "hook.c")
	if err := os.WriteFile(hook, []byte("void __gcov_dump(void); void __gcov_reset(void);\nvoid sd_gcov_hook(void) { __gcov_dump(); __gcov_reset(); }\n"), 0o644); err != nil {
		return nil, err
	}
	hobj := filepath.Join(c.objDir, "hook.o")
	if err := run("cc", "-fPIC", "-O0", "-c", hook, "-o", hobj); err != nil {
		return nil, err
	}
	c.so = filepath.Join(c.objDir, "scan.so")
	if err := run(link, "--coverage", "-shared", "-o", c.so, obj, hobj); err != nil {
		return nil, err
	}
	return c, nil
}

// sdParseGcov reads one .gcov file made by `gcov -b -c`. A branch is taken when its integer count is above
// zero; a percentage (what gcov -b prints without -c) is refused, because "taken 0%" is not taken.
func sdParseGcov(text string) (*sdGcovFile, error) {
	f := &sdGcovFile{lines: map[int]bool{}, branches: map[string]bool{}}
	cur := 0
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "branch") {
			fs := strings.Fields(l)
			if len(fs) < 3 {
				return nil, fmt.Errorf("bad branch line %q", l)
			}
			key := fmt.Sprintf("%d:%s", cur, fs[1])
			switch {
			case fs[2] == "never":
				f.branches[key] = false
			case fs[2] == "taken" && len(fs) >= 4:
				n, err := strconv.ParseUint(fs[3], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("branch count %q is not an integer (was gcov run with -c?)", fs[3])
				}
				f.branches[key] = n > 0
			default:
				return nil, fmt.Errorf("bad branch line %q", l)
			}
			continue
		}
		if strings.HasPrefix(l, "call") || strings.HasPrefix(l, "function") || strings.HasPrefix(l, "unconditional") {
			continue
		}
		p := strings.SplitN(l, ":", 3)
		if len(p) < 3 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(p[1]))
		if err != nil || n == 0 {
			continue
		}
		cur = n
		cnt := strings.TrimSpace(p[0])
		if cnt == "-" {
			continue
		}
		f.lines[n] = cnt != "#####" && cnt != "====="
	}
	return f, nil
}

// measure flushes the counters and reads gcov; it returns branches taken at least once, branches in
// total, lines executed and lines in total. Any failure, or a result without a single branch or line, is an error.
func (c *sdCover) measure(d *scandiff.Driver) (hit, tot, lines, ltot int, err error) {
	if rc := d.GcovFlush(); rc != 0 {
		return 0, 0, 0, 0, fmt.Errorf("gcov flush failed: %d", rc)
	}
	cmd := exec.Command("gcov", "-b", "-c", "-o", c.objDir, c.scanner)
	cmd.Dir = c.objDir
	o, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("gcov: %v %s", err, o)
	}
	c.files = map[string]*sdGcovFile{}
	for _, m := range regexp.MustCompile(`File '([^']+)'`).FindAllStringSubmatch(string(o), -1) {
		path := m[1]
		// the scanner and the project headers it includes (also above the source folder, as typescript's do)
		if !strings.HasPrefix(path, c.repoRoot) || strings.Contains(path, "/tree_sitter/") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(c.objDir, filepath.Base(path)+".gcov"))
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("gcov output for %s: %v", path, err)
		}
		f, err := sdParseGcov(string(b))
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("%s: %v", path, err)
		}
		c.files[path] = f
	}
	for _, f := range c.files {
		for _, v := range f.lines {
			ltot++
			if v {
				lines++
			}
		}
		for _, v := range f.branches {
			tot++
			if v {
				hit++
			}
		}
	}
	if ltot == 0 {
		return 0, 0, 0, 0, fmt.Errorf("gcov reported no executable line of %s", c.scanner)
	}
	return
}

func (c *sdCover) writeUncovered() error {
	if c.out == "" {
		return nil
	}
	var sb strings.Builder
	var names []string
	for n := range c.files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f := c.files[n]
		fmt.Fprintf(&sb, "file %s\n", strings.TrimPrefix(n, c.repoRoot+"/"))
		var ls []int
		for l, v := range f.lines {
			if !v {
				ls = append(ls, l)
			}
		}
		sort.Ints(ls)
		fmt.Fprintf(&sb, "  lines never executed: %v\n", ls)
		set := map[int]bool{}
		for k, v := range f.branches {
			if !v {
				l, _ := strconv.Atoi(strings.SplitN(k, ":", 2)[0])
				set[l] = true
			}
		}
		var bs []int
		for l := range set {
			bs = append(bs, l)
		}
		sort.Ints(bs)
		fmt.Fprintf(&sb, "  lines with a branch never taken: %v\n", bs)
	}
	return os.WriteFile(filepath.Join(c.out, "cov-"+c.lang+"-uncovered.txt"), []byte(sb.String()), 0o644)
}

// sdLiterals reads the string and character constants of a C or C++ source.
func sdLiterals(path string) [][]byte {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out [][]byte
	add := func(b []byte) {
		if len(b) == 0 || len(b) > 40 || seen[string(b)] {
			return
		}
		seen[string(b)] = true
		out = append(out, b)
	}
	i := 0
	for i < len(src) {
		switch {
		case bytes.HasPrefix(src[i:], []byte("//")):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case bytes.HasPrefix(src[i:], []byte("/*")):
			j := bytes.Index(src[i+2:], []byte("*/"))
			if j < 0 {
				i = len(src)
			} else {
				i += j + 4
			}
		case src[i] == '#' && bytes.HasPrefix(bytes.TrimLeft(src[i+1:], " \t"), []byte("include")):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case src[i] == '"' || src[i] == '\'':
			q := src[i]
			i++
			var b []byte
			for i < len(src) && src[i] != q && src[i] != '\n' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
					switch src[i] {
					case 'n':
						b = append(b, '\n')
					case 't':
						b = append(b, '\t')
					case 'r':
						b = append(b, '\r')
					case '0':
						b = append(b, 0)
					case 'x':
						j := i + 1
						for j < len(src) && j < i+3 && strings.IndexByte("0123456789abcdefABCDEF", src[j]) >= 0 {
							j++
						}
						if v, err := strconv.ParseUint(string(src[i+1:j]), 16, 8); err == nil {
							b = append(b, byte(v))
						}
						i = j - 1
					case 'u':
						if i+4 < len(src) {
							if v, err := strconv.ParseUint(string(src[i+1:i+5]), 16, 32); err == nil {
								b = utf8.AppendRune(b, rune(v))
							}
							i += 4
						}
					default:
						b = append(b, src[i])
					}
				} else {
					b = append(b, src[i])
				}
				i++
			}
			i++
			add(b)
		default:
			i++
		}
	}
	return out
}

// sdPlateauAllowed says whether branch counts may be used to stop a run: a file with no branch at all has
// no branch coverage, so there is nothing to reach a plateau on ("branch coverage unavailable").
func sdPlateauAllowed(branchTotal int) bool {
	if sdMut("zerobranch") {
		return true // the earlier behaviour: 0/0 counted as a plateau
	}
	return branchTotal > 0
}

// sdBranchText prints branch coverage, or says it is unavailable.
func sdBranchText(hit, tot int) string {
	if tot == 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%d/%d", hit, tot)
}
