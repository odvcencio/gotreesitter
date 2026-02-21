//go:build cgo && treesitter_c_parity

package gotreesitter

/*
#cgo linux LDFLAGS: -ldl
#cgo freebsd LDFLAGS: -ldl
#cgo netbsd LDFLAGS: -ldl
#cgo openbsd LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>

typedef const void* (*ts_parity_lang_fn)(void);

static void* tsParityOpen(const char* path) {
	dlerror();
	return dlopen(path, RTLD_NOW | RTLD_LOCAL);
}

static void* tsParitySymbol(void* handle, const char* name) {
	dlerror();
	return dlsym(handle, name);
}

static const char* tsParityError(void) {
	return dlerror();
}

static const void* tsParityCall(void* symbol) {
	ts_parity_lang_fn fn = (ts_parity_lang_fn)symbol;
	return fn();
}
*/
import "C"

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type parityLockEntry struct {
	Name    string
	RepoURL string
	Commit  string
	Subdir  string
}

type parityCRef struct {
	lang   *sitter.Language
	handle unsafe.Pointer
	soPath string
}

var parityCRefState = struct {
	once    sync.Once
	lock    map[string]parityLockEntry
	rootDir string

	mu   sync.Mutex
	refs map[string]*parityCRef
	err  error
}{}

// ParityCLanguage loads a C reference language compiled from the pinned
// grammars/languages.lock commit for the given language name.
func ParityCLanguage(name string) (*sitter.Language, error) {
	parityCRefState.once.Do(func() {
		lock, err := loadParityLock(filepath.Join("grammars", "languages.lock"))
		if err != nil {
			parityCRefState.err = err
			return
		}
		rootDir, err := os.MkdirTemp("", "gotreesitter-parity-c-*")
		if err != nil {
			parityCRefState.err = fmt.Errorf("create parity temp root: %w", err)
			return
		}
		parityCRefState.lock = lock
		parityCRefState.rootDir = rootDir
		parityCRefState.refs = make(map[string]*parityCRef)
	})
	if parityCRefState.err != nil {
		return nil, parityCRefState.err
	}

	parityCRefState.mu.Lock()
	defer parityCRefState.mu.Unlock()

	if ref, ok := parityCRefState.refs[name]; ok {
		return ref.lang, nil
	}
	entry, ok := parityCRefState.lock[name]
	if !ok {
		return nil, fmt.Errorf("parity lock has no entry for %q", name)
	}

	ref, err := buildParityCRef(parityCRefState.rootDir, entry)
	if err != nil {
		return nil, err
	}
	parityCRefState.refs[name] = ref
	return ref.lang, nil
}

func buildParityCRef(rootDir string, entry parityLockEntry) (*parityCRef, error) {
	commitShort := entry.Commit
	if len(commitShort) > 12 {
		commitShort = commitShort[:12]
	}
	repoDir := filepath.Join(rootDir, "repos", paritySafeName(entry.Name+"-"+commitShort))
	if _, err := os.Stat(repoDir); err != nil {
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return nil, fmt.Errorf("%s: mkdir repo parent: %w", entry.Name, err)
		}
		if err := clonePinnedRepo(entry.RepoURL, entry.Commit, repoDir); err != nil {
			return nil, fmt.Errorf("%s: clone pinned repo: %w", entry.Name, err)
		}
	}

	buildDir := filepath.Join(rootDir, "build", paritySafeName(entry.Name))
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return nil, fmt.Errorf("%s: mkdir build dir: %w", entry.Name, err)
	}
	soPath := filepath.Join(buildDir, "parser.so")
	if err := compileParserShared(entry, repoDir, soPath, buildDir); err != nil {
		return nil, fmt.Errorf("%s: compile parser shared library: %w", entry.Name, err)
	}

	symbol := "tree_sitter_" + paritySafeName(entry.Name)
	ref, err := loadParitySharedLanguage(soPath, symbol)
	if err != nil {
		return nil, fmt.Errorf("%s: load %s: %w", entry.Name, symbol, err)
	}
	return ref, nil
}

func loadParityLock(path string) (map[string]parityLockEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}
	defer f.Close()

	entries := make(map[string]parityLockEntry)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("%s:%d: invalid lock line %q", path, lineNum, line)
		}

		entry := parityLockEntry{
			Name:    fields[0],
			RepoURL: fields[1],
			Subdir:  "src",
		}
		next := 2
		if len(fields) > next && looksLikeCommitHash(fields[next]) {
			entry.Commit = fields[next]
			next++
		}
		if len(fields) > next {
			entry.Subdir = fields[next]
		}
		entries[entry.Name] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan lock file %s: %w", path, err)
	}
	return entries, nil
}

func clonePinnedRepo(repoURL, commit, dest string) error {
	if err := runCommand("", "git", "clone", "--depth=1", repoURL, dest); err != nil {
		return err
	}
	if commit == "" {
		return nil
	}
	if err := runCommand("", "git", "-C", dest, "checkout", "--detach", commit); err == nil {
		return nil
	}
	if err := runCommand("", "git", "-C", dest, "fetch", "--depth=1", "origin", commit); err != nil {
		return err
	}
	return runCommand("", "git", "-C", dest, "checkout", "--detach", "FETCH_HEAD")
}

func compileParserShared(entry parityLockEntry, repoDir, outSO, objDir string) error {
	srcDir := filepath.Join(repoDir, entry.Subdir)
	parserPath := filepath.Join(srcDir, "parser.c")
	if _, err := os.Stat(parserPath); err != nil {
		found, findErr := findParserC(repoDir)
		if findErr != nil {
			return fmt.Errorf("parser.c not found in %s", repoDir)
		}
		parserPath = found
		srcDir = filepath.Dir(found)
	}

	sources := []string{parserPath}
	for _, scannerName := range []string{"scanner.c", "scanner.cc", "scanner.cpp", "scanner.cxx"} {
		scannerPath := filepath.Join(srcDir, scannerName)
		if _, err := os.Stat(scannerPath); err == nil {
			sources = append(sources, scannerPath)
		}
	}

	var (
		objects []string
		hasCXX  bool
	)
	for _, source := range sources {
		ext := strings.ToLower(filepath.Ext(source))
		obj := filepath.Join(objDir, paritySafeName(filepath.Base(source))+".o")
		switch ext {
		case ".cc", ".cpp", ".cxx":
			hasCXX = true
			if err := runCommand(
				"",
				"c++",
				"-std=c++17",
				"-fPIC",
				"-O2",
				"-I",
				srcDir,
				"-c",
				source,
				"-o",
				obj,
			); err != nil {
				return err
			}
		default:
			if err := runCommand(
				"",
				"cc",
				"-std=c11",
				"-fPIC",
				"-O2",
				"-I",
				srcDir,
				"-c",
				source,
				"-o",
				obj,
			); err != nil {
				return err
			}
		}
		objects = append(objects, obj)
	}

	linker := "cc"
	if hasCXX {
		linker = "c++"
	}
	args := []string{"-shared", "-fPIC", "-o", outSO}
	args = append(args, objects...)
	return runCommand("", linker, args...)
}

func loadParitySharedLanguage(soPath, symbol string) (*parityCRef, error) {
	cPath := C.CString(soPath)
	defer C.free(unsafe.Pointer(cPath))

	handle := C.tsParityOpen(cPath)
	if handle == nil {
		return nil, fmt.Errorf("dlopen %s: %s", soPath, parityDLError())
	}

	cSym := C.CString(symbol)
	defer C.free(unsafe.Pointer(cSym))

	sym := C.tsParitySymbol(handle, cSym)
	if sym == nil {
		return nil, fmt.Errorf("dlsym %s: %s", symbol, parityDLError())
	}

	langPtr := C.tsParityCall(sym)
	if langPtr == nil {
		return nil, fmt.Errorf("%s returned nil TSLanguage", symbol)
	}

	lang := sitter.NewLanguage(unsafe.Pointer(langPtr))
	if lang == nil {
		return nil, fmt.Errorf("NewLanguage(%s) returned nil", symbol)
	}

	return &parityCRef{
		lang:   lang,
		handle: handle,
		soPath: soPath,
	}, nil
}

func parityDLError() string {
	if err := C.tsParityError(); err != nil {
		return C.GoString(err)
	}
	return "unknown dynamic loader error"
}

func runCommand(dir, cmdName string, args ...string) error {
	cmd := exec.Command(cmdName, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("%s %s: %s", cmdName, strings.Join(args, " "), msg)
}

func findParserC(repoDir string) (string, error) {
	var candidates []string
	err := filepath.WalkDir(repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "parser.c" {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("parser.c not found")
	}
	for _, c := range candidates {
		if strings.Contains(c, string(filepath.Separator)+"src"+string(filepath.Separator)+"parser.c") {
			return c, nil
		}
	}
	return candidates[0], nil
}

func looksLikeCommitHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func paritySafeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "lang"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "lang"
	}
	return out
}
