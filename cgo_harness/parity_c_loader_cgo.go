//go:build cgo && treesitter_c_parity

package cgoharness

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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
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

const (
	parityMinLanguageVersion = 13
	parityMaxLanguageVersion = 15
	parityGenerateABI        = 15
	parityRepoRootEnv        = "GTS_PARITY_REPO_ROOT"
)

var languageVersionPattern = regexp.MustCompile(`(?m)^#define\s+LANGUAGE_VERSION\s+(\d+)`)

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
		lockPath, err := findParityLockPath()
		if err != nil {
			parityCRefState.err = err
			return
		}
		lock, err := loadParityLock(lockPath)
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

func findParityLockPath() (string, error) {
	candidates := []string{
		filepath.Join("grammars", "languages.lock"),
		filepath.Join("..", "grammars", "languages.lock"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not find grammars/languages.lock from cgo_harness")
}

func buildParityCRef(rootDir string, entry parityLockEntry) (*parityCRef, error) {
	repoDir, ok := parityLocalRepoDir(entry)
	if !ok {
		// Compute a temp clone destination under rootDir.
		repoDir = filepath.Join(rootDir, "repos", paritySafeName(entry.Name))
		commitShort := entry.Commit
		if len(commitShort) > 12 {
			commitShort = commitShort[:12]
		}
		if cacheDir := parityRepoCacheDir(); cacheDir != "" {
			cachedRepo, cacheErr := findCachedParityRepo(cacheDir, entry.Name, commitShort)
			if cacheErr != nil {
				return nil, fmt.Errorf("%s: parity repo cache miss: %w", entry.Name, cacheErr)
			}
			if err := clonePinnedRepoFromLocalCache(cachedRepo, entry.Commit, repoDir); err != nil {
				return nil, fmt.Errorf("%s: clone pinned repo from local cache: %w", entry.Name, err)
			}
		} else if err := clonePinnedRepo(entry.RepoURL, entry.Commit, repoDir); err != nil {
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

	var loadErrs []string
	for _, symbol := range parityLanguageSymbols(entry) {
		ref, err := loadParitySharedLanguage(soPath, symbol)
		if err == nil {
			return ref, nil
		}
		loadErrs = append(loadErrs, fmt.Sprintf("%s: %v", symbol, err))
	}
	return nil, fmt.Errorf("%s: load language symbol failed: %s", entry.Name, strings.Join(loadErrs, "; "))
}

func parityLocalRepoDir(entry parityLockEntry) (string, bool) {
	root := strings.TrimSpace(os.Getenv(parityRepoRootEnv))
	if root == "" {
		return "", false
	}

	var candidates []string
	add := func(path string) {
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	add(filepath.Join(root, entry.Name))

	repo := strings.TrimSuffix(strings.TrimSpace(entry.RepoURL), "/")
	repo = strings.TrimSuffix(repo, ".git")
	if idx := strings.LastIndex(repo, "/"); idx >= 0 && idx+1 < len(repo) {
		base := repo[idx+1:]
		add(filepath.Join(root, parityRepoBaseDir(base)))
	}

	switch entry.Name {
	case "gitcommit":
		add(filepath.Join(root, "gitcommit_gbprod"))
	case "tsx", "typescript":
		add(filepath.Join(root, "typescript"))
	case "xml", "dtd":
		add(filepath.Join(root, "xml"))
	case "markdown", "markdown_inline":
		add(filepath.Join(root, "markdown"))
	case "php":
		add(filepath.Join(root, "php"))
	case "ocaml":
		add(filepath.Join(root, "ocaml"))
	case "csv":
		add(filepath.Join(root, "csv"))
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func parityRepoBaseDir(base string) string {
	base = strings.TrimSpace(base)
	base = strings.TrimSuffix(base, ".git")
	base = strings.TrimPrefix(base, "tree-sitter-")
	base = strings.TrimPrefix(base, "tree_sitter_")
	return paritySafeName(base)
}

func parityLanguageSymbols(entry parityLockEntry) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(sym string) {
		if sym == "" || seen[sym] {
			return
		}
		seen[sym] = true
		out = append(out, sym)
	}

	add("tree_sitter_" + paritySafeName(entry.Name))
	add("tree_sitter_" + strings.ToUpper(strings.TrimSpace(entry.Name)))

	repo := strings.TrimSuffix(strings.TrimSpace(entry.RepoURL), "/")
	repo = strings.TrimSuffix(repo, ".git")
	if idx := strings.LastIndex(repo, "/"); idx >= 0 && idx+1 < len(repo) {
		base := repo[idx+1:]
		add("tree_sitter_" + paritySafeName(base))
		if strings.HasPrefix(base, "tree-sitter-") {
			add("tree_sitter_" + paritySafeName(strings.TrimPrefix(base, "tree-sitter-")))
		}
		if strings.HasPrefix(base, "tree_sitter_") {
			add("tree_sitter_" + paritySafeName(strings.TrimPrefix(base, "tree_sitter_")))
		}
	}

	return out
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

func parityRepoCacheDir() string {
	return strings.TrimSpace(os.Getenv("GTS_PARITY_REPO_CACHE"))
}

func findCachedParityRepo(cacheDir, name, commitShort string) (string, error) {
	candidates := []string{
		filepath.Join(cacheDir, name+"-"+commitShort),
		filepath.Join(cacheDir, paritySafeName(name)+"-"+commitShort),
		filepath.Join(cacheDir, paritySafeName(name+"-"+commitShort)),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("checked %s", strings.Join(candidates, ", "))
}

func clonePinnedRepoFromLocalCache(cacheRepoDir, commit, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	// The host cache is already pinned to the requested commit, so copying it is
	// enough and avoids Git safe.directory checks on the bind mount.
	return runCommand("", "cp", "-a", cacheRepoDir+string(filepath.Separator)+".", dest)
}

func clonePinnedRepo(repoURL, commit, dest string) error {
	if err := runCommandRetry("", 4, "git", "clone", "--depth=1", repoURL, dest); err != nil {
		// Fallback to a full clone if shallow clone repeatedly fails with a
		// transient GitHub transport error.
		if retryableCommandError(err) {
			if fullErr := runCommandRetry("", 2, "git", "clone", repoURL, dest); fullErr == nil {
				goto checkout
			} else {
				return fullErr
			}
		}
		return err
	}

checkout:
	return clonePinnedCheckout(commit, dest)
}

func clonePinnedCheckout(commit, dest string) error {
	if commit == "" {
		return nil
	}
	if err := runCommand("", "git", "-C", dest, "checkout", "--detach", commit); err == nil {
		return nil
	}
	if err := runCommandRetry("", 4, "git", "-C", dest, "fetch", "--depth=1", "origin", commit); err != nil {
		return err
	}
	return runCommandRetry("", 2, "git", "-C", dest, "checkout", "--detach", "FETCH_HEAD")
}

func compileParserShared(entry parityLockEntry, repoDir, outSO, objDir string) error {
	srcDir, parserPath, err := resolveParserSource(entry, repoDir)
	if err != nil {
		return err
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

func resolveParserSource(entry parityLockEntry, repoDir string) (string, string, error) {
	srcDir := filepath.Join(repoDir, entry.Subdir)
	parserPath := filepath.Join(srcDir, "parser.c")

	if _, err := os.Stat(parserPath); err != nil {
		// Some repos don't commit parser.c. Try regenerating first, then fall
		// back to a repo-wide search.
		_ = regenerateParserSource(repoDir, srcDir)
		if _, err := os.Stat(parserPath); err != nil {
			found, findErr := findParserC(repoDir)
			if findErr != nil {
				return "", "", fmt.Errorf("parser.c not found in %s", repoDir)
			}
			parserPath = found
			srcDir = filepath.Dir(found)
		}
	}

	if version, ok := readParserLanguageVersion(parserPath); ok &&
		(version < parityMinLanguageVersion || version > parityMaxLanguageVersion) {
		if err := regenerateParserSource(repoDir, srcDir); err != nil {
			return "", "", fmt.Errorf(
				"parser.c ABI %d incompatible (need %d..%d) and regeneration failed: %w",
				version, parityMinLanguageVersion, parityMaxLanguageVersion, err,
			)
		}
		if _, err := os.Stat(parserPath); err != nil {
			found, findErr := findParserC(repoDir)
			if findErr != nil {
				return "", "", fmt.Errorf("parser.c not found after regeneration in %s", repoDir)
			}
			parserPath = found
			srcDir = filepath.Dir(found)
		}
		if regeneratedVersion, ok := readParserLanguageVersion(parserPath); ok &&
			(regeneratedVersion < parityMinLanguageVersion || regeneratedVersion > parityMaxLanguageVersion) {
			return "", "", fmt.Errorf(
				"parser.c ABI %d still incompatible after regeneration (need %d..%d)",
				regeneratedVersion, parityMinLanguageVersion, parityMaxLanguageVersion,
			)
		}
	}

	return srcDir, parserPath, nil
}

func regenerateParserSource(repoDir, hintDir string) error {
	grammarRoot, err := findGrammarRoot(repoDir, hintDir)
	if err != nil {
		return err
	}

	abis := make([]int, 0, parityMaxLanguageVersion-parityMinLanguageVersion+1)
	for abi := parityGenerateABI; abi >= parityMinLanguageVersion; abi-- {
		abis = append(abis, abi)
	}

	tryGenerate := func(cmd string, args ...string) error {
		var lastErr error
		for _, abi := range abis {
			abiArgs := append(args, "--abi", strconv.Itoa(abi))
			if err := runCommand(grammarRoot, cmd, abiArgs...); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
		if lastErr == nil {
			return fmt.Errorf("all ABI attempts failed")
		}
		return fmt.Errorf("all ABI attempts failed; last error: %w", lastErr)
	}

	if _, err := exec.LookPath("tree-sitter"); err == nil {
		if err := tryGenerate("tree-sitter", "generate"); err == nil {
			return nil
		}
	}
	return tryGenerate("npx", "--yes", "tree-sitter-cli", "generate")
}

func findGrammarRoot(repoDir, hintDir string) (string, error) {
	repoDir = filepath.Clean(repoDir)
	dir := filepath.Clean(hintDir)
	if dir == "." || dir == "" || !strings.HasPrefix(dir, repoDir) {
		dir = repoDir
	}

	for {
		if hasGrammarDefinition(dir) {
			return dir, nil
		}
		if dir == repoDir {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if hasGrammarDefinition(repoDir) {
		return repoDir, nil
	}
	return "", fmt.Errorf("grammar root not found under %s", repoDir)
}

func hasGrammarDefinition(dir string) bool {
	candidates := []string{"grammar.js", "grammar.ts"}
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func readParserLanguageVersion(parserPath string) (int, bool) {
	source, err := os.ReadFile(parserPath)
	if err != nil {
		return 0, false
	}
	match := languageVersionPattern.FindSubmatch(source)
	if len(match) != 2 {
		return 0, false
	}
	version, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return 0, false
	}
	return version, true
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
	if cmdName == "git" {
		cmd.Env = append(
			os.Environ(),
			"GIT_HTTP_VERSION=HTTP/1.1",
			"GIT_TERMINAL_PROMPT=0",
		)
	}
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

func runCommandRetry(dir string, attempts int, cmdName string, args ...string) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		err := runCommand(dir, cmdName, args...)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryableCommandError(err) || i == attempts-1 {
			break
		}
		delay := time.Duration(i+1) * time.Second
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
		time.Sleep(delay)
	}
	return lastErr
}

func retryableCommandError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "The requested URL returned error: 500") ||
		strings.Contains(msg, "remote: Internal Server Error") ||
		strings.Contains(msg, "expected flush after ref listing") ||
		strings.Contains(msg, "expected 'packfile'") ||
		strings.Contains(msg, "Could not resolve host") ||
		strings.Contains(msg, "Temporary failure in name resolution") ||
		strings.Contains(msg, "Name or service not known") ||
		strings.Contains(msg, "TLS handshake timeout") ||
		strings.Contains(msg, "operation timed out") ||
		strings.Contains(msg, "Operation timed out") ||
		strings.Contains(msg, "Connection reset by peer") ||
		strings.Contains(msg, "connection reset by peer")
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
