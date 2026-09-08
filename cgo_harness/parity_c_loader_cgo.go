//go:build cgo && (treesitter_c_parity || treesitter_c_bench)

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

static int tsParityClose(void* handle) {
	return dlclose(handle);
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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/odvcencio/gotreesitter/internal/grammarpatch"
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
	raw    unsafe.Pointer // the TSLanguage the shared object returned
	handle unsafe.Pointer
	soPath string
}

const (
	parityMinLanguageVersion = 13
	parityMaxLanguageVersion = 15
	parityGenerateABI        = 15
	parityRepoRootEnv        = "GTS_PARITY_REPO_ROOT"
	parityCBuildCacheEnv     = "GTS_PARITY_C_REF_BUILD_CACHE"
	parityCBuildJobsEnv      = "GTS_PARITY_C_REF_BUILD_JOBS"
)

// The C oracle contract is intentionally explicit. The Go binding release is
// pinned by cgo_harness/go.mod; its repository commit pins the upstream
// tree-sitter runtime submodule below. Grammar commits come from
// grammars/languages.lock.
const (
	COracleContractVersion = "tree-sitter-c-v1"
	COracleBindingModule   = "github.com/tree-sitter/go-tree-sitter"
	COracleBindingVersion  = "v0.25.0"
	COracleBindingCommit   = "adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3"
	COracleRuntimeVersion  = "0.25.1"
	COracleRuntimeCommit   = "f5afe475deb7c0bae6407fb776c76824f717bb61"
	COracleGrammarCFlags   = "-std=c11 -fPIC -O2 -I ."
)

// COracleBuildIdentity describes the in-process cgo parity transport. The
// tree-sitter runtime is statically included by go-tree-sitter in the Go test
// binary. The exact locked grammar is an -O2 shared object loaded with dlopen.
// Publication throughput uses pure_c/run_go_benchmark.sh instead, which links
// both the same runtime and grammar statically into a standalone executable.
type COracleBuildIdentity struct {
	Contract              string `json:"contract"`
	Transport             string `json:"transport"`
	BindingModule         string `json:"binding_module"`
	BindingVersion        string `json:"binding_version"`
	BindingCommit         string `json:"binding_commit"`
	RuntimeVersion        string `json:"runtime_version"`
	RuntimeCommit         string `json:"runtime_commit"`
	RuntimeLinkage        string `json:"runtime_linkage"`
	Language              string `json:"language"`
	GrammarRepo           string `json:"grammar_repo"`
	GrammarCommit         string `json:"grammar_commit"`
	GrammarLinkage        string `json:"grammar_linkage"`
	GrammarCompileFlags   string `json:"grammar_compile_flags"`
	CompilerPath          string `json:"compiler_path"`
	CompilerVersion       string `json:"compiler_version"`
	GrammarArtifactPath   string `json:"grammar_artifact_path"`
	GrammarArtifactSHA256 string `json:"grammar_artifact_sha256"`
}

var languageVersionPattern = regexp.MustCompile(`(?m)^#define\s+LANGUAGE_VERSION\s+(\d+)`)

type parityCRefBuild struct {
	done chan struct{}
	ref  *parityCRef
	err  error
}

var parityCRefState = struct {
	once          sync.Once
	lock          map[string]parityLockEntry
	rootDir       string
	buildCacheDir string
	buildSem      chan struct{}

	mu       sync.Mutex
	refs     map[string]*parityCRef
	inflight map[string]*parityCRefBuild
	err      error
}{}

// COracleLanguage loads a C reference language compiled from the pinned
// grammars/languages.lock commit. It is the single in-process binding used by
// both treesitter_c_parity and treesitter_c_bench.
func COracleLanguage(name string) (*sitter.Language, error) {
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
		parityCRefState.buildCacheDir = parityDefaultCBuildCacheDir(lockPath)
		parityCRefState.buildSem = make(chan struct{}, parityCBuildJobs())
		parityCRefState.refs = make(map[string]*parityCRef)
		parityCRefState.inflight = make(map[string]*parityCRefBuild)
	})
	if parityCRefState.err != nil {
		return nil, parityCRefState.err
	}

	parityCRefState.mu.Lock()
	if ref, ok := parityCRefState.refs[name]; ok {
		parityCRefState.mu.Unlock()
		return ref.lang, nil
	}
	entry, ok := parityCRefState.lock[name]
	if !ok {
		parityCRefState.mu.Unlock()
		return nil, fmt.Errorf("parity lock has no entry for %q", name)
	}
	if build := parityCRefState.inflight[name]; build != nil {
		parityCRefState.mu.Unlock()
		<-build.done
		if build.err != nil {
			return nil, build.err
		}
		return build.ref.lang, nil
	}

	build := &parityCRefBuild{done: make(chan struct{})}
	parityCRefState.inflight[name] = build
	rootDir := parityCRefState.rootDir
	cacheDir := parityCRefState.buildCacheDir
	buildSem := parityCRefState.buildSem
	parityCRefState.mu.Unlock()

	buildSem <- struct{}{}
	ref, err := func() (*parityCRef, error) {
		defer func() { <-buildSem }()
		return buildParityCRef(rootDir, cacheDir, entry)
	}()

	parityCRefState.mu.Lock()
	build.ref = ref
	build.err = err
	if err != nil {
		delete(parityCRefState.inflight, name)
		close(build.done)
		parityCRefState.mu.Unlock()
		return nil, err
	}
	parityCRefState.refs[name] = ref
	delete(parityCRefState.inflight, name)
	close(build.done)
	parityCRefState.mu.Unlock()
	return ref.lang, nil
}

// ParityCLanguage is retained as the parity-facing name for the shared oracle
// binding. Benchmark code must use COracleLanguage rather than another C
// runtime or grammar package.
func ParityCLanguage(name string) (*sitter.Language, error) {
	return COracleLanguage(name)
}

// COracleIdentity loads the language if necessary and returns the exact
// runtime, grammar, compiler and grammar-artifact identity used by the cgo
// transport.
func COracleIdentity(name string) (COracleBuildIdentity, error) {
	if _, err := COracleLanguage(name); err != nil {
		return COracleBuildIdentity{}, err
	}

	parityCRefState.mu.Lock()
	entry := parityCRefState.lock[name]
	ref := parityCRefState.refs[name]
	parityCRefState.mu.Unlock()
	if ref == nil {
		return COracleBuildIdentity{}, fmt.Errorf("C oracle %q has no loaded reference", name)
	}

	artifactSHA, err := fileSHA256(ref.soPath)
	if err != nil {
		return COracleBuildIdentity{}, fmt.Errorf("hash C oracle grammar artifact: %w", err)
	}
	compilerPath, compilerVersion := cOracleCompilerIdentity()
	return COracleBuildIdentity{
		Contract:              COracleContractVersion,
		Transport:             "cgo_parity_binding",
		BindingModule:         COracleBindingModule,
		BindingVersion:        COracleBindingVersion,
		BindingCommit:         COracleBindingCommit,
		RuntimeVersion:        COracleRuntimeVersion,
		RuntimeCommit:         COracleRuntimeCommit,
		RuntimeLinkage:        "static_cgo_test_binary",
		Language:              name,
		GrammarRepo:           entry.RepoURL,
		GrammarCommit:         entry.Commit,
		GrammarLinkage:        "shared_dlopen",
		GrammarCompileFlags:   COracleGrammarCFlags,
		CompilerPath:          compilerPath,
		CompilerVersion:       compilerVersion,
		GrammarArtifactPath:   ref.soPath,
		GrammarArtifactSHA256: artifactSHA,
	}, nil
}

// COracleDeepDigest returns the gts-deep-tree-v1 structural digest emitted by the
// static publication artifact's --dump mode. It covers cross-runtime symbol
// identity through type+named, byte and point spans, extra/missing/error
// flags, child order and incoming field identity.
func COracleDeepDigest(tree *sitter.Tree) (string, error) {
	return cOracleDeepDigest(tree, time.Time{})
}

var errCOracleDeepDigestTimeout = errors.New("C oracle deep digest exceeded wall timeout")

// cOracleDeepDigestWithin applies a wall bound to digest materialization. The
// parser timeout has already ended when admission reaches this walk, so the
// digest needs its own independent bound.
func cOracleDeepDigestWithin(tree *sitter.Tree, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		return "", errCOracleDeepDigestTimeout
	}
	return cOracleDeepDigest(tree, time.Now().Add(timeout))
}

func cOracleDeepDigest(tree *sitter.Tree, deadline time.Time) (string, error) {
	if tree == nil || tree.RootNode() == nil {
		return "", fmt.Errorf("cannot digest nil C oracle tree")
	}
	checkDeadline := func() error {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return errCOracleDeepDigestTimeout
		}
		return nil
	}
	if err := checkDeadline(); err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write([]byte("gts-deep-tree-v1\x00"))
	root := tree.RootNode()
	cursor := root.Walk()
	defer cursor.Close()
	writeCOracleDeepNodeHeader(h, root, "")
	for {
		if err := checkDeadline(); err != nil {
			return "", err
		}
		if cursor.GotoFirstChild() {
			writeCOracleDeepNodeHeader(h, cursor.Node(), cursor.FieldName())
			continue
		}
		for !cursor.GotoNextSibling() {
			if err := checkDeadline(); err != nil {
				return "", err
			}
			if !cursor.GotoParent() {
				return hex.EncodeToString(h.Sum(nil)), nil
			}
		}
		writeCOracleDeepNodeHeader(h, cursor.Node(), cursor.FieldName())
	}
}

func writeCOracleDeepNodeHeader(h hash.Hash, node *sitter.Node, field string) {
	writeCOracleDeepString(h, node.Kind())
	writeCOracleDeepString(h, field)
	writeCOracleDeepU32(h, uint32(node.StartByte()))
	writeCOracleDeepU32(h, uint32(node.EndByte()))
	start := node.StartPosition()
	end := node.EndPosition()
	writeCOracleDeepU32(h, uint32(start.Row))
	writeCOracleDeepU32(h, uint32(start.Column))
	writeCOracleDeepU32(h, uint32(end.Row))
	writeCOracleDeepU32(h, uint32(end.Column))
	var flags byte
	if node.IsNamed() {
		flags |= 1 << 0
	}
	if node.IsExtra() {
		flags |= 1 << 1
	}
	if node.IsMissing() {
		flags |= 1 << 2
	}
	if node.IsError() {
		flags |= 1 << 3
	}
	if node.HasError() {
		flags |= 1 << 4
	}
	_, _ = h.Write([]byte{flags})
	childCount := node.ChildCount()
	writeCOracleDeepU32(h, uint32(childCount))
}

func writeCOracleDeepU32(h hash.Hash, value uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], value)
	_, _ = h.Write(buf[:])
}

func writeCOracleDeepString(h hash.Hash, value string) {
	writeCOracleDeepU32(h, uint32(len(value)))
	_, _ = h.Write([]byte(value))
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

func buildParityCRef(rootDir, cacheDir string, entry parityLockEntry) (*parityCRef, error) {
	if ref, ok := loadCachedParityCRef(cacheDir, entry); ok {
		return ref, nil
	}

	repoDir, localRepo := parityLocalRepoDir(entry)
	if !localRepo {
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
	repoKind := "pinned parity repository"
	if localRepo {
		repoKind = "local parity repository"
	}
	if err := verifyPinnedRepo(repoDir, entry.Commit); err != nil {
		return nil, fmt.Errorf("%s: %s %s rejected: %w", entry.Name, repoKind, repoDir, err)
	}
	if err := applyParityGrammarPatch(entry, repoDir); err != nil {
		return nil, fmt.Errorf("%s: apply grammar patch: %w", entry.Name, err)
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
			storeCachedParityCRef(cacheDir, entry, soPath)
			return ref, nil
		}
		loadErrs = append(loadErrs, fmt.Sprintf("%s: %v", symbol, err))
	}
	return nil, fmt.Errorf("%s: load language symbol failed: %s", entry.Name, strings.Join(loadErrs, "; "))
}

func parityCBuildJobs() int {
	raw := strings.TrimSpace(os.Getenv(parityCBuildJobsEnv))
	if raw == "" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func parityDefaultCBuildCacheDir(lockPath string) string {
	raw := strings.TrimSpace(os.Getenv(parityCBuildCacheEnv))
	switch strings.ToLower(raw) {
	case "0", "false", "off", "none", "no":
		return ""
	}
	if raw != "" {
		if abs, err := filepath.Abs(raw); err == nil {
			return abs
		}
		return raw
	}

	repoRoot := filepath.Dir(filepath.Dir(lockPath))
	if abs, err := filepath.Abs(repoRoot); err == nil {
		repoRoot = abs
	}
	return filepath.Join(repoRoot, "harness_out", "parity_c_ref_cache", runtime.GOOS+"_"+runtime.GOARCH)
}

func parityCachedSOPath(cacheDir string, entry parityLockEntry) string {
	if strings.TrimSpace(cacheDir) == "" {
		return ""
	}
	compilerPath, compilerVersion := cOracleCompilerIdentity()
	keyInput := strings.Join([]string{
		entry.Name,
		entry.RepoURL,
		entry.Commit,
		entry.Subdir,
		parityGrammarPatchIdentity(entry),
		COracleGrammarCFlags,
		compilerPath,
		compilerVersion,
		strconv.Itoa(parityMinLanguageVersion),
		strconv.Itoa(parityMaxLanguageVersion),
		strconv.Itoa(parityGenerateABI),
		runtime.GOOS,
		runtime.GOARCH,
	}, "\x00")
	sum := sha256.Sum256([]byte(keyInput))
	return filepath.Join(cacheDir, paritySafeName(entry.Name)+"-"+hex.EncodeToString(sum[:])[:16]+".so")
}

func parityGrammarPatchPath(entry parityLockEntry) string {
	spec, ok := grammarpatch.Lookup(entry.Name)
	if !ok {
		return ""
	}
	for _, path := range []string{
		filepath.Join("grammars", "patches", spec.File),
		filepath.Join("..", "grammars", "patches", spec.File),
	} {
		if _, err := os.Stat(path); err == nil {
			abs, err := filepath.Abs(path)
			if err == nil {
				return abs
			}
			return path
		}
	}
	return ""
}

func parityGrammarPatchIdentity(entry parityLockEntry) string {
	patchPath := parityGrammarPatchPath(entry)
	if patchPath == "" {
		if spec, ok := grammarpatch.Lookup(entry.Name); ok {
			return "missing-" + spec.File
		}
		return "none"
	}
	sum, err := fileSHA256(patchPath)
	if err != nil {
		return "unreadable:" + patchPath
	}
	return sum
}

func applyParityGrammarPatch(entry parityLockEntry, repoDir string) error {
	spec, hasPatch := grammarpatch.Lookup(entry.Name)
	if !hasPatch {
		return nil
	}
	patchPath := parityGrammarPatchPath(entry)
	if patchPath == "" {
		return fmt.Errorf("missing grammar patch %s", spec.File)
	}
	for _, args := range [][]string{{"apply", "--check", patchPath}, {"apply", patchPath}} {
		if err := runCommand(repoDir, "git", args...); err != nil {
			return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
	}
	if spec.RegenerateParser {
		if err := regeneratePatchedTypeScriptCParser(repoDir, entry.Subdir); err != nil {
			return err
		}
	}
	return nil
}

func regeneratePatchedTypeScriptCParser(repoDir, subdir string) error {
	// tree-sitter-cli's executable is installed by its package lifecycle
	// scripts, so this must remain a normal lockfile-pinned install.
	install := exec.Command("npm", "ci")
	install.Dir = repoDir
	if out, err := install.CombinedOutput(); err != nil {
		return fmt.Errorf("npm ci: %v: %s", err, strings.TrimSpace(string(out)))
	}
	generator := filepath.Join(repoDir, "node_modules", ".bin", "tree-sitter")
	generate := exec.Command(generator, "generate")
	// The lock points to generated parser.c under <grammar>/src, while
	// tree-sitter generate reads grammar.js from the parent grammar directory.
	generate.Dir = filepath.Join(repoDir, filepath.Dir(subdir))
	if out, err := generate.CombinedOutput(); err != nil {
		return fmt.Errorf("tree-sitter generate: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func loadCachedParityCRef(cacheDir string, entry parityLockEntry) (*parityCRef, bool) {
	soPath := parityCachedSOPath(cacheDir, entry)
	if soPath == "" {
		return nil, false
	}
	if _, err := os.Stat(soPath); err != nil {
		return nil, false
	}
	ref, err := loadParitySharedLanguageAny(soPath, parityLanguageSymbols(entry))
	if err == nil {
		return ref, true
	}
	_ = os.Remove(soPath)
	return nil, false
}

func storeCachedParityCRef(cacheDir string, entry parityLockEntry, soPath string) {
	cacheSO := parityCachedSOPath(cacheDir, entry)
	if cacheSO == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(cacheSO), 0o755); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.tmp.%d.%d", cacheSO, os.Getpid(), time.Now().UnixNano())
	if err := runCommand("", "cp", soPath, tmp); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, cacheSO); err != nil {
		_ = os.Remove(tmp)
	}
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
	if err := verifyPinnedRepo(cacheRepoDir, commit); err != nil {
		return fmt.Errorf("cached parity repository %s rejected: %w", cacheRepoDir, err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	// The verified host cache can be copied directly. Both the bind-mounted
	// source and copied destination are verified with a repository-scoped
	// safe.directory setting before compilation.
	return runCommand("", "cp", "-a", cacheRepoDir+string(filepath.Separator)+".", dest)
}

func verifyPinnedRepo(repoDir, commit string) error {
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return fmt.Errorf("pinned commit is empty")
	}

	head, err := parityGitOutput(repoDir, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	want, err := parityGitOutput(repoDir, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolve pinned commit %s: %w", commit, err)
	}
	head = strings.TrimSpace(head)
	want = strings.TrimSpace(want)
	if head != want {
		return fmt.Errorf("HEAD mismatch: got %s, want pinned commit %s", head, want)
	}

	status, err := parityGitOutput(repoDir, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return fmt.Errorf("inspect working tree: %w", err)
	}
	if status = strings.TrimSpace(status); status != "" {
		// Some upstream grammar commits contain blobs whose line endings conflict
		// with their own .gitattributes. Git reports those fresh checkouts as
		// modified even though the worktree bytes still exactly match HEAD. Keep
		// the fail-closed provenance gate, but distinguish that filter artifact
		// from a real source mutation by comparing the reported paths without
		// applying Git's clean filters.
		if err := verifyPinnedRepoStatusBytes(repoDir); err != nil {
			return fmt.Errorf("working tree is not clean (tracked and untracked files must match the pinned commit):\n%s\nexact-byte verification: %w", status, err)
		}
	}
	return nil
}

func verifyPinnedRepoStatusBytes(repoDir string) error {
	untracked, err := parityGitStdout(repoDir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return fmt.Errorf("inspect untracked files: %w", err)
	}
	if untracked != "" {
		return fmt.Errorf("untracked path %q", strings.Split(untracked, "\x00")[0])
	}

	changed, err := parityGitStdout(repoDir, "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return fmt.Errorf("list changed paths: %w", err)
	}
	for _, rel := range strings.Split(changed, "\x00") {
		if rel == "" {
			continue
		}
		entry, err := parityGitStdout(repoDir, "ls-tree", "-z", "HEAD", "--", rel)
		if err != nil {
			return fmt.Errorf("read pinned tree entry %q: %w", rel, err)
		}
		entry = strings.TrimSuffix(entry, "\x00")
		fields := strings.Fields(strings.SplitN(entry, "\t", 2)[0])
		if len(fields) != 3 || fields[1] != "blob" {
			return fmt.Errorf("path %q is not a pinned blob", rel)
		}

		info, err := os.Lstat(filepath.Join(repoDir, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("stat %q: %w", rel, err)
		}
		if !pinnedRepoModeMatches(fields[0], info.Mode()) {
			return fmt.Errorf("path %q mode differs from pinned mode %s", rel, fields[0])
		}
		got, err := parityGitStdout(repoDir, "hash-object", "--no-filters", "--", rel)
		if err != nil {
			return fmt.Errorf("hash raw bytes for %q: %w", rel, err)
		}
		if strings.TrimSpace(got) != fields[2] {
			return fmt.Errorf("path %q bytes differ from pinned blob", rel)
		}
	}
	return nil
}

func pinnedRepoModeMatches(want string, got os.FileMode) bool {
	switch want {
	case "100644":
		return got.IsRegular() && got.Perm()&0o111 == 0
	case "100755":
		return got.IsRegular() && got.Perm()&0o111 != 0
	case "120000":
		return got&os.ModeSymlink != 0
	default:
		return false
	}
}

func parityGitStdout(repoDir string, args ...string) (string, error) {
	gitArgs := append([]string{"-c", "safe.directory=" + repoDir, "-C", repoDir}, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Env = append(
		os.Environ(),
		"GIT_HTTP_VERSION=HTTP/1.1",
		"GIT_TERMINAL_PROMPT=0",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return string(out), nil
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = err.Error()
	}
	return "", fmt.Errorf("git %s: %s", strings.Join(gitArgs, " "), msg)
}

func parityGitOutput(repoDir string, args ...string) (string, error) {
	// A cache mounted into a root-owned Docker container can legitimately have a
	// different owner. Scope safe.directory to the exact repository being
	// verified instead of mutating global Git configuration or weakening the
	// check for arbitrary paths.
	gitArgs := append([]string{"-c", "safe.directory=" + repoDir, "-C", repoDir}, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Env = append(
		os.Environ(),
		"GIT_HTTP_VERSION=HTTP/1.1",
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return "", fmt.Errorf("git %s: %s", strings.Join(gitArgs, " "), msg)
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
		// Use a relative source name so __FILE__ does not contain a temp path.
		sourceName := filepath.Base(source)
		switch ext {
		case ".cc", ".cpp", ".cxx":
			hasCXX = true
			if err := runCommand(
				srcDir,
				"c++",
				"-std=c++17",
				"-fPIC",
				"-O2",
				"-I",
				".",
				"-c",
				sourceName,
				"-o",
				obj,
			); err != nil {
				return err
			}
		default:
			if err := runCommand(
				srcDir,
				"cc",
				"-std=c11",
				"-fPIC",
				"-O2",
				"-I",
				".",
				"-c",
				sourceName,
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

func loadParitySharedLanguageAny(soPath string, symbols []string) (*parityCRef, error) {
	var loadErrs []string
	for _, symbol := range symbols {
		ref, err := loadParitySharedLanguage(soPath, symbol)
		if err == nil {
			return ref, nil
		}
		loadErrs = append(loadErrs, fmt.Sprintf("%s: %v", symbol, err))
	}
	return nil, fmt.Errorf("load language symbol failed: %s", strings.Join(loadErrs, "; "))
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
		C.tsParityClose(handle)
		return nil, fmt.Errorf("dlsym %s: %s", symbol, parityDLError())
	}

	langPtr := C.tsParityCall(sym)
	if langPtr == nil {
		C.tsParityClose(handle)
		return nil, fmt.Errorf("%s returned nil TSLanguage", symbol)
	}

	lang := sitter.NewLanguage(unsafe.Pointer(langPtr))
	if lang == nil {
		C.tsParityClose(handle)
		return nil, fmt.Errorf("NewLanguage(%s) returned nil", symbol)
	}

	return &parityCRef{
		lang:   lang,
		raw:    unsafe.Pointer(langPtr),
		handle: handle,
		soPath: soPath,
	}, nil
}

// cOracleRawLanguage returns the TSLanguage pointer of a loaded C oracle
// grammar for direct runtime calls the binding does not expose.
func cOracleRawLanguage(name string) (unsafe.Pointer, error) {
	if _, err := COracleLanguage(name); err != nil {
		return nil, err
	}
	parityCRefState.mu.Lock()
	ref := parityCRefState.refs[name]
	parityCRefState.mu.Unlock()
	if ref == nil || ref.raw == nil {
		return nil, fmt.Errorf("C oracle %q has no loaded reference", name)
	}
	return ref.raw, nil
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

func cOracleCompilerIdentity() (string, string) {
	path, err := exec.LookPath("cc")
	if err != nil {
		return "cc", "unknown"
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return path, "unknown"
	}
	version := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(version, '\n'); idx >= 0 {
		version = version[:idx]
	}
	if version == "" {
		version = "unknown"
	}
	return path, version
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
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
