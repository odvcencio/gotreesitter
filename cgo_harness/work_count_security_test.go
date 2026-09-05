//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	workCountAllowDirtyEnv     = "GTS_WORK_COUNT_ALLOW_DIRTY"
	workCountColdCAdmissionEnv = "GTS_WORK_COUNT_COLD_C_ADMISSION_TEST"
	workCountBuildTimeout      = 3 * time.Minute
	workCountGitTimeout        = 30 * time.Second
	workCountUnsetEnvValue     = "<unset>"
)

type workCountGitProvenance struct {
	Authoritative     bool   `json:"authoritative"`
	GitClean          bool   `json:"git_clean"`
	Head              string `json:"head"`
	HeadTree          string `json:"head_tree"`
	SnapshotSHA256    string `json:"snapshot_sha256"`
	SnapshotFiles     int    `json:"snapshot_files"`
	SnapshotMode      string `json:"snapshot_mode"`
	DirtyStatusSHA256 string `json:"dirty_status_sha256,omitempty"`
}

type workCountSourceSnapshot struct {
	Root       string
	Provenance workCountGitProvenance
	Recheck    func() error
}

type workCountEnvironment struct {
	Build   map[string]string `json:"build"`
	Runtime map[string]string `json:"runtime"`
}

func workCountChosenEnvironment() workCountEnvironment {
	return workCountEnvironment{
		Build: map[string]string{
			"CGO_ENABLED":  "0",
			"GOAMD64":      "v1",
			"GOARCH":       runtime.GOARCH,
			"GODEBUG":      "",
			"GOENV":        "off",
			"GOEXPERIMENT": "",
			"GOFLAGS":      "-mod=readonly -trimpath -buildvcs=false -p=1",
			"GOGC":         "100",
			"GOMEMLIMIT":   "off",
			"GOMAXPROCS":   "1",
			"GOOS":         runtime.GOOS,
			"GOTOOLCHAIN":  "local",
			"GONOPROXY":    "",
			"GONOSUMDB":    "",
			"GOPRIVATE":    "",
			"GOPROXY":      "off",
			"GOSUMDB":      "sum.golang.org",
			"GOT_*":        "unset",
			"GOVCS":        "*:off",
			"GOWORK":       "off",
			"LANG":         "C",
			"LC_ALL":       "C",
			"TZ":           "UTC",
		},
		Runtime: map[string]string{
			"GOAMD64":      "v1",
			"GODEBUG":      "",
			"GOENV":        "off",
			"GOEXPERIMENT": "",
			"GOFLAGS":      "",
			"GOGC":         "100",
			"GOMEMLIMIT":   "off",
			"GOMAXPROCS":   "1",
			"GOTOOLCHAIN":  "local",
			"GOT_*":        "unset",
			"LANG":         "C",
			"LC_ALL":       "C",
			"TZ":           "UTC",
		},
	}
}

func workCountCBuildEnvironment() map[string]string {
	return map[string]string{
		"CFLAGS":              "",
		"CPPFLAGS":            "",
		"CPATH":               "",
		"CPLUS_INCLUDE_PATH":  "",
		"CXXFLAGS":            "",
		"COMPILER_PATH":       workCountUnsetEnvValue,
		"GCC_EXEC_PREFIX":     workCountUnsetEnvValue,
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_DIR":             workCountUnsetEnvValue,
		"GIT_WORK_TREE":       workCountUnsetEnvValue,
		"LANG":                "C",
		"LC_ALL":              "C",
		"LDFLAGS":             "",
		"LIBRARY_PATH":        "",
		"LD_LIBRARY_PATH":     "",
		"SOURCE_DATE_EPOCH":   "0",
		"TZ":                  "UTC",
	}
}

func workCountSanitizedEnv(base []string, chosen map[string]string, extra map[string]string) []string {
	removed := map[string]bool{
		"CGO_ENABLED": true, "GOAMD64": true, "GOARCH": true,
		"GODEBUG": true, "GOENV": true, "GOEXPERIMENT": true,
		"GOFLAGS": true, "GOGC": true, "GOMEMLIMIT": true,
		"GOMAXPROCS": true, "GOOS": true, "GOTOOLCHAIN": true,
	}
	for key := range chosen {
		removed[key] = true
	}
	for key := range extra {
		removed[key] = true
	}
	out := make([]string, 0, len(base)+len(chosen)+len(extra))
	for _, item := range base {
		key := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			key = item[:index]
		}
		if removed[key] || strings.HasPrefix(key, "GOT_") {
			continue
		}
		out = append(out, item)
	}
	appendMap := func(values map[string]string) {
		keys := make([]string, 0, len(values))
		for key := range values {
			if key == "GOT_*" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if values[key] == workCountUnsetEnvValue {
				continue
			}
			out = append(out, key+"="+values[key])
		}
	}
	appendMap(chosen)
	appendMap(extra)
	return out
}

func workCountRunBounded(dir string, env []string, timeout time.Duration, name string, args ...string) ([]byte, error) {
	stdout, stderr, err := workCountRunCaptured(dir, env, timeout, name, args...)
	output := append(append([]byte(nil), stdout...), stderr...)
	return output, err
}

func workCountRunCaptured(dir string, env []string, timeout time.Duration, name string, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err, stop, _ := perfScanRunChild(ctx, cmd, 0)
	if stop != nil && stop.Class == perfScanStopWallTimeout {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("%s timed out after %s", filepath.Base(name), timeout)
	}
	if err != nil {
		message := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), message)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func workCountPrepareSourceSnapshot(t *testing.T, repoRoot, tempRoot string) workCountSourceSnapshot {
	t.Helper()
	gitEnv := workCountSanitizedEnv(os.Environ(), nil, map[string]string{
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
	})
	gitRun := func(args ...string) ([]byte, error) {
		gitArgs := append([]string{"-c", "safe.directory=" + repoRoot}, args...)
		return workCountRunBounded(repoRoot, gitEnv, workCountGitTimeout, "git", gitArgs...)
	}
	gitOutput := func(args ...string) []byte {
		output, err := gitRun(args...)
		if err != nil {
			t.Fatal(err)
		}
		return output
	}
	head := strings.TrimSpace(string(gitOutput("rev-parse", "--verify", "HEAD^{commit}")))
	headTree := strings.TrimSpace(string(gitOutput("rev-parse", "--verify", "HEAD^{tree}")))
	if !staticCHashIdentity(head) || !staticCHashIdentity(headTree) {
		t.Fatalf("invalid source provenance head=%q tree=%q", head, headTree)
	}
	status := gitOutput("status", "--porcelain=v1", "-z", "--untracked-files=all")
	clean := len(status) == 0
	authoritative := clean
	if !clean && os.Getenv(workCountAllowDirtyEnv) != "1" {
		t.Fatalf("authoritative work-count source must be clean; set %s=1 only for a non-authoritative pre-commit test", workCountAllowDirtyEnv)
	}

	staging := filepath.Join(tempRoot, "go-source-staging")
	if err := os.Mkdir(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	mode := "git-archive-head"
	if clean {
		archivePath := filepath.Join(tempRoot, "go-source.tar")
		if _, err := gitRun("archive", "--format=tar", "-o", archivePath, head); err != nil {
			t.Fatal(err)
		}
		if err := workCountExtractTar(archivePath, staging); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(archivePath); err != nil {
			t.Fatal(err)
		}
	} else {
		mode = "dirty-worktree-test-only"
		paths := gitOutput("ls-files", "-z", "--cached", "--others", "--exclude-standard")
		for _, raw := range bytes.Split(paths, []byte{0}) {
			if len(raw) == 0 {
				continue
			}
			if err := workCountCopySnapshotPath(repoRoot, staging, string(raw)); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				t.Fatal(err)
			}
		}
	}
	snapshotSHA, files, err := workCountTreeSHA(staging)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tempRoot, "go-source-"+snapshotSHA)
	if err := os.Rename(staging, root); err != nil {
		t.Fatal(err)
	}
	if err := workCountMakeTreeReadOnly(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workCountMakeTreeWritable(root) })
	if got, gotFiles, err := workCountTreeSHA(root); err != nil || got != snapshotSHA || gotFiles != files {
		t.Fatalf("private Go source snapshot drift after sealing: sha=%s files=%d want=%s/%d err=%v", got, gotFiles, snapshotSHA, files, err)
	}
	statusHash := ""
	if !clean {
		sum := sha256.Sum256(status)
		statusHash = hex.EncodeToString(sum[:])
	}
	provenance := workCountGitProvenance{
		Authoritative:     authoritative,
		GitClean:          clean,
		Head:              head,
		HeadTree:          headTree,
		SnapshotSHA256:    snapshotSHA,
		SnapshotFiles:     files,
		SnapshotMode:      mode,
		DirtyStatusSHA256: statusHash,
	}
	recheck := func() error {
		gotSHA, gotFiles, err := workCountTreeSHA(root)
		if err != nil || gotSHA != snapshotSHA || gotFiles != files {
			return fmt.Errorf("private source snapshot sha=%s files=%d want=%s/%d err=%v", gotSHA, gotFiles, snapshotSHA, files, err)
		}
		if !authoritative {
			return nil
		}
		gotHead, err := gitRun("rev-parse", "--verify", "HEAD^{commit}")
		if err != nil || strings.TrimSpace(string(gotHead)) != head {
			return fmt.Errorf("Git HEAD drift: got=%q want=%q err=%v", strings.TrimSpace(string(gotHead)), head, err)
		}
		gotTree, err := gitRun("rev-parse", "--verify", "HEAD^{tree}")
		if err != nil || strings.TrimSpace(string(gotTree)) != headTree {
			return fmt.Errorf("Git tree drift: got=%q want=%q err=%v", strings.TrimSpace(string(gotTree)), headTree, err)
		}
		gotStatus, err := gitRun("status", "--porcelain=v1", "-z", "--untracked-files=all")
		if err != nil || len(gotStatus) != 0 {
			return fmt.Errorf("Git worktree ceased to be clean: status=%q err=%v", gotStatus, err)
		}
		return nil
	}
	return workCountSourceSnapshot{Root: root, Provenance: provenance, Recheck: recheck}
}

func workCountPrepareBoardReceiptPath(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(workCountBoardReceiptEnv))
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		t.Fatal(err)
	}
	absolute = filepath.Join(resolvedDirectory, filepath.Base(absolute))
	if err := os.Remove(absolute); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return absolute
}

func TestWorkCountPrepareBoardReceiptPathRemovesStaleOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workCountBoardReceiptEnv, path)
	if got := workCountPrepareBoardReceiptPath(t); got != path {
		t.Fatalf("prepared receipt path=%q want=%q", got, path)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale receipt remains: %v", err)
	}
}

func workCountClearAmbientGitRouting(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE"} {
		value, present := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		key, value, present := key, value, present
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func workCountExtractTar(path, destination string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		// Git archives may carry a global PAX record containing the source
		// commit identity. It is archive metadata, not a filesystem entry.
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}
		target, err := workCountSafeSnapshotTarget(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fs.FileMode(header.Mode).Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, reader)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported Git archive entry %q type=%d", header.Name, header.Typeflag)
		}
	}
}

func workCountCopySnapshotPath(sourceRoot, destinationRoot, relative string) error {
	target, err := workCountSafeSnapshotTarget(destinationRoot, relative)
	if err != nil {
		return err
	}
	source, err := workCountSafeSnapshotTarget(sourceRoot, relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(link, target)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported worktree path %s mode=%s", relative, info.Mode())
	}
	return workCountCopyFile(source, target, info.Mode().Perm())
}

func workCountSafeSnapshotTarget(root, relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe snapshot path %q", relative)
	}
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("snapshot path escapes root: %q", relative)
	}
	return target, nil
}

func workCountCopyFile(source, destination string, mode fs.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		_ = in.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeInErr := in.Close()
	closeOutErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeInErr != nil {
		return closeInErr
	}
	return closeOutErr
}

func workCountTreeSHA(root string) (string, int, error) {
	hash := sha256.New()
	files := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		workCountHashField(hash, filepath.ToSlash(relative))
		var mode [4]byte
		// Content identity retains type and executable intent but deliberately
		// ignores writable/readable permission bits changed by source sealing.
		normalizedMode := info.Mode().Type() | (info.Mode().Perm() & 0o111)
		binary.LittleEndian.PutUint32(mode[:], uint32(normalizedMode))
		_, _ = hash.Write(mode[:])
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			workCountHashField(hash, link)
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported snapshot entry %s mode=%s", relative, info.Mode())
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), files, nil
}

func workCountHashField(writer io.Writer, value string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = io.WriteString(writer, value)
}

func workCountMakeTreeReadOnly(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		return os.Chmod(path, 0o444|(info.Mode().Perm()&0o111))
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err := os.Chmod(directories[i], 0o555); err != nil {
			return err
		}
	}
	return nil
}

func workCountMakeTreeWritable(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644|(info.Mode().Perm()&0o111))
	})
}

func workCountPrepareReceiptPath(t *testing.T, repoRoot string) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(workCountReceiptEnv))
	if path == "" {
		path = filepath.Join(repoRoot, "harness_out", "work_count", workCountFixtureID+".json")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		t.Fatal(err)
	}
	absolute = filepath.Join(resolvedDirectory, filepath.Base(absolute))
	if err := os.Remove(absolute); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return absolute
}

func workCountAtomicPublish(path string, data []byte) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		_ = temp.Close()
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	syncErr := directoryHandle.Sync()
	closeErr := directoryHandle.Close()
	if syncErr != nil {
		_ = os.Remove(path)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	keep = true
	return nil
}

func TestWorkCountSanitizedEnvDropsParserAndGoOverrides(t *testing.T) {
	base := []string{
		"PATH=/bin", "GOFLAGS=-race", "GOAMD64=v4", "GOEXPERIMENT=fieldtrack",
		"GODEBUG=gctrace=1", "GOT_GLR_MAX_STACKS=99", "GOT_FAKE=1",
	}
	chosen := workCountChosenEnvironment().Build
	got := workCountSanitizedEnv(base, chosen, map[string]string{"EXTRA": "yes"})
	joined := strings.Join(got, "\n")
	for _, forbidden := range []string{"-race", "GOAMD64=v4", "fieldtrack", "gctrace=1", "GOT_GLR", "GOT_FAKE"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("sanitized environment retained %q: %s", forbidden, joined)
		}
	}
	for _, required := range []string{"GOFLAGS=-mod=readonly -trimpath -buildvcs=false -p=1", "GOAMD64=v1", "EXTRA=yes"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("sanitized environment omitted %q: %s", required, joined)
		}
	}
	cChosen := workCountCBuildEnvironment()
	cGot := strings.Join(workCountSanitizedEnv(
		[]string{"PATH=/usr/bin", "COMPILER_PATH=/host/compiler", "GCC_EXEC_PREFIX=/host/gcc", "GIT_DIR=/host/repo", "GIT_WORK_TREE=/host/tree"},
		cChosen,
		nil,
	), "\n")
	for _, forbidden := range []string{"COMPILER_PATH=", "GCC_EXEC_PREFIX=", "GIT_DIR=", "GIT_WORK_TREE="} {
		if strings.Contains(cGot, forbidden) {
			t.Fatalf("sanitized C environment retained %q: %s", forbidden, cGot)
		}
	}
	if cChosen["COMPILER_PATH"] != workCountUnsetEnvValue || cChosen["GCC_EXEC_PREFIX"] != workCountUnsetEnvValue {
		t.Fatalf("unset C environment is not recorded explicitly: %v", cChosen)
	}
}

func TestWorkCountGoChildUsesPrivateSourceDirectory(t *testing.T) {
	const fixtureID = "private-source-directory-probe"
	const witnessName = "work-count-child-directory.txt"
	if os.Getenv(workCountFixtureEnv) == fixtureID {
		data, err := os.ReadFile(filepath.Join("testdata", witnessName))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv("GTS_WORK_COUNT_RESULT"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	parentDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(sourceRoot, "testdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	witness := []byte("private snapshot fixture\n")
	sourcePath := filepath.Join(sourceRoot, "testdata", witnessName)
	if err := os.WriteFile(sourcePath, witness, 0o644); err != nil {
		t.Fatal(err)
	}
	beforeSHA, beforeFiles, err := workCountTreeSHA(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := workCountMakeTreeReadOnly(sourceRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workCountMakeTreeWritable(sourceRoot) })
	artifact, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "child-result.txt")
	workCountRunGoChild(t, sourceRoot, artifact, "^TestWorkCountGoChildUsesPrivateSourceDirectory$",
		fixtureID, sourcePath, resultPath, workCountChosenEnvironment())
	got, err := os.ReadFile(resultPath)
	if err != nil || !bytes.Equal(got, witness) {
		t.Fatalf("child fixture=%q want=%q err=%v", got, witness, err)
	}
	if got, err := os.Getwd(); err != nil || got != parentDirectory {
		t.Fatalf("parent directory=%q want=%q err=%v", got, parentDirectory, err)
	}
	if gotSHA, gotFiles, err := workCountTreeSHA(sourceRoot); err != nil || gotSHA != beforeSHA || gotFiles != beforeFiles {
		t.Fatalf("private snapshot changed: sha=%s files=%d want=%s/%d err=%v", gotSHA, gotFiles, beforeSHA, beforeFiles, err)
	}
}

func TestWorkCountRunCapturedKillsDescendantProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "descendant-escaped")
	environment := workCountSanitizedEnv(os.Environ(), nil, map[string]string{
		"LANG": "C", "LC_ALL": "C", "TZ": "UTC",
	})
	_, _, err := workCountRunCaptured(
		"",
		environment,
		40*time.Millisecond,
		"sh", "-c", `(sleep 0.2; printf escaped > "$1") & wait`, "work-count-helper", marker,
	)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("bounded process group error=%v, want timeout", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, statErr := os.Stat(marker); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("timed-out descendant escaped process-group kill: %v", statErr)
	}
}

func TestWorkCountExtractTarAcceptsPAXMetadata(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "snapshot.tar")
	file, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	if err := writer.WriteHeader(&tar.Header{
		Typeflag:   tar.TypeXGlobalHeader,
		PAXRecords: map[string]string{"comment": "0123456789abcdef"},
	}); err != nil {
		t.Fatal(err)
	}
	payload := []byte("package fixture\n")
	if err := writer.WriteHeader(&tar.Header{Name: "fixture.go", Mode: 0o644, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "extracted")
	if err := workCountExtractTar(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "fixture.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("extracted payload=%q want=%q", got, payload)
	}
}

func TestWorkCountStaticCAdmissionColdArtifactCache(t *testing.T) {
	if os.Getenv(workCountColdCAdmissionEnv) != "1" {
		t.Skipf("set %s=1 for the focused cold-artifact gate", workCountColdCAdmissionEnv)
	}
	fixture := workCountLoadFixture(t)
	repoRoot := workCountRepoRoot(t)
	lock, err := loadParityLock(filepath.Join(repoRoot, "grammars", "languages.lock"))
	if err != nil {
		t.Fatal(err)
	}
	entry := lock["go"]
	warmRoot := strings.TrimSpace(os.Getenv(staticCPerfCacheEnv))
	if warmRoot == "" {
		warmRoot = filepath.Join(repoRoot, "harness_out", "c_oracle", "fleet_static")
	}
	warmRuntime := filepath.Join(warmRoot, "sources", "tree-sitter-"+COracleRuntimeCommit)
	warmGrammar := filepath.Join(warmRoot, "sources", paritySafeName("go")+"-"+entry.Commit)
	workCountEnsurePinnedRepo(t, "runtime", staticCPerfRuntimeRepo, COracleRuntimeCommit, warmRuntime, "")
	workCountEnsurePinnedRepo(t, "go grammar", entry.RepoURL, entry.Commit, warmGrammar, "go")

	tempRoot := t.TempDir()
	coldRoot := filepath.Join(tempRoot, "cold-c-oracle")
	coldRuntime := filepath.Join(coldRoot, "sources", filepath.Base(warmRuntime))
	coldGrammar := filepath.Join(coldRoot, "sources", filepath.Base(warmGrammar))
	copyTool := workCountTool(t, "cp")
	environment := workCountSanitizedEnv(os.Environ(), workCountCBuildEnvironment(), nil)
	for source, destination := range map[string]string{warmRuntime: coldRuntime, warmGrammar: coldGrammar} {
		if err := os.MkdirAll(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := workCountRunBounded("", environment, workCountBuildTimeout, copyTool.Path, "-a", source+string(filepath.Separator)+".", destination); err != nil {
			t.Fatal(err)
		}
	}
	if err := workCountVerifyPinnedRepo(coldRuntime, COracleRuntimeCommit, environment); err != nil {
		t.Fatal(err)
	}
	if err := workCountVerifyPinnedRepo(coldGrammar, entry.Commit, environment); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(coldRoot, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Join(coldRoot, "artifacts")); err != nil || len(entries) != 0 {
		t.Fatalf("cold artifact cache is not empty: entries=%d err=%v", len(entries), err)
	}
	t.Setenv(staticCPerfCacheEnv, coldRoot)
	sourcePath := filepath.Join(tempRoot, "query_compile.go")
	if err := os.WriteFile(sourcePath, fixture.Source, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := workCountUninstrumentedCAdmission(t, fixture, sourcePath, tempRoot); got != fixture.Fixture.DeepTreeSHA256 {
		t.Fatalf("cold static C admission digest=%s want=%s", got, fixture.Fixture.DeepTreeSHA256)
	}
	artifacts, err := os.ReadDir(filepath.Join(coldRoot, "artifacts"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("cold static C admission artifact count=%d want=1 err=%v", len(artifacts), err)
	}
}
