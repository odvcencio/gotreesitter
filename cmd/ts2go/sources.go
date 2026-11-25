package main

// -fetch-sources writes the pinned, patched parser.c of every lock entry that
// ts2go converts, for the blob freshness check (blob_freshness_test.go). The
// check itself reads only this cache and needs no network.
//
// Cache layout, one folder per lock entry:
//
//	<dir>/<name>/parser.c    the parser.c that batch mode would convert
//	<dir>/<name>/source.txt  line 1: the entry's identity (freshnessSourceIdentity)
//	                         line 2: parser_sha256=<hex digest of parser.c>
//
// An entry whose source.txt matches the lock is kept; any other entry is
// fetched again.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/odvcencio/gotreesitter/internal/grammarpatch"
)

const freshnessSourceFile = "source.txt"

// freshnessExempt names the lock entries that ts2go converts but that the blob
// freshness check leaves out, with the reason. Each one needs a decision on
// how its parser.c is generated before the check can cover it.
var freshnessExempt = map[string]string{
	// The shipped Less blob was converted from a parser.c regenerated from the
	// pinned grammar.js. The parser.c committed at the pinned commit was made
	// by an older generator (language version 11), so converting it does not
	// reproduce the blob's source.
	"less": "the shipped blob comes from a regenerated parser.c, not the committed one",
	// The pinned Perl commit has no committed parser.c.
	"perl": "the pinned commit has no committed parser.c",
}

// freshnessEntries returns the lock entries whose shipped blob ts2go converts
// from the pinned parser.c and that the freshness check covers: every entry
// except the grammargen-owned blobs and the exempt names, sorted by name.
func freshnessEntries(entries []ManifestEntry) []ManifestEntry {
	out := make([]ManifestEntry, 0, len(entries))
	for _, entry := range entries {
		if grammargenOwnedBlobs[entry.Name] {
			continue
		}
		if _, exempt := freshnessExempt[entry.Name]; exempt {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// freshnessSourceIdentity is the first line of an entry's source.txt: the
// fields that decide which parser.c batch mode converts. patch is "none" or a
// digest of the patch files and the regeneration settings.
func freshnessSourceIdentity(entry ManifestEntry, manifestDir string) (string, error) {
	patch := "none"
	if spec, ok := grammarpatch.Lookup(entry.Name); ok {
		h := sha256.New()
		for _, path := range upstreamGrammarPatchPaths(entry.Name, manifestDir) {
			data, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("read patch: %w", err)
			}
			sum := sha256.Sum256(data)
			fmt.Fprintf(h, "%s\x00%x\x00", filepath.Base(path), sum)
		}
		fmt.Fprintf(h, "regenerate=%t\x00generator=%s\x00", spec.RegenerateParser, spec.GeneratorVersion)
		patch = hex.EncodeToString(h.Sum(nil))[:16]
	}
	return fmt.Sprintf("%s %s %s %s patch=%s", entry.Name, entry.RepoURL, entry.Commit, entry.Subdir, patch), nil
}

// RunFetchSources fills dir with the pinned, patched parser.c of every lock
// entry that the freshness check covers (or of the named ones). It fetches
// each pinned commit, applies the entry's grammar patch the way batch mode
// does (regenerating parser.c when the patch requires it), and keeps only
// parser.c. Entries already in dir with a matching identity are kept.
func RunFetchSources(manifestPath, dir string, names []string, jobs int) error {
	entries, err := ParseManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	manifestDir := filepath.Dir(manifestPath)
	selected := freshnessEntries(entries)
	if len(names) > 0 {
		want := map[string]bool{}
		for _, name := range names {
			if name = strings.TrimSpace(name); name != "" {
				want[name] = true
			}
		}
		var kept []ManifestEntry
		for _, entry := range selected {
			if want[entry.Name] {
				kept = append(kept, entry)
				delete(want, entry.Name)
			}
		}
		if len(want) > 0 {
			var missing []string
			for name := range want {
				missing = append(missing, name)
			}
			sort.Strings(missing)
			return fmt.Errorf("not a checked lock entry: %s", strings.Join(missing, ", "))
		}
		selected = kept
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmpRoot, err := os.MkdirTemp("", "ts2go-fetch-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpRoot)

	if jobs < 1 {
		jobs = 1
	}
	var (
		mu      sync.Mutex
		failed  []string
		fetched int
		cached  int
		wg      sync.WaitGroup
		slots   = make(chan struct{}, jobs)
	)
	for _, entry := range selected {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			hit, err := fetchFreshnessSource(entry, manifestDir, dir, filepath.Join(tmpRoot, safeFileBase(entry.Name)))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				failed = append(failed, entry.Name)
				fmt.Printf("FAIL %s: %v\n", entry.Name, err)
			case hit:
				cached++
				fmt.Println("cached " + entry.Name)
			default:
				fetched++
				fmt.Printf("fetched %s %s\n", entry.Name, shortCommit(entry.Commit))
			}
		}()
	}
	wg.Wait()
	fmt.Printf("fetch-sources: %d entries, %d fetched, %d cached, %d failed\n", len(selected), fetched, cached, len(failed))
	if len(failed) > 0 {
		sort.Strings(failed)
		return fmt.Errorf("%d entries failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// fetchFreshnessSource makes dir/<name> hold the entry's parser.c. It reports
// true when the folder already matched the entry's identity.
func fetchFreshnessSource(entry ManifestEntry, manifestDir, dir, workDir string) (bool, error) {
	identity, err := freshnessSourceIdentity(entry, manifestDir)
	if err != nil {
		return false, err
	}
	dest := filepath.Join(dir, safeFileBase(entry.Name))
	if _, err := readFreshnessSource(dest, identity); err == nil {
		return true, nil
	}
	defer os.RemoveAll(workDir)
	if err := fetchPinnedCommit(entry.RepoURL, entry.Commit, workDir); err != nil {
		return false, err
	}
	if err := applyUpstreamGrammarPatch(entry, workDir, manifestDir); err != nil {
		return false, fmt.Errorf("apply upstream grammar patch: %w", err)
	}
	parserPath := filepath.Join(workDir, entry.Subdir, "parser.c")
	if _, err := os.Stat(parserPath); err != nil {
		detected, derr := findParserC(workDir)
		if derr != nil {
			return false, fmt.Errorf("parser.c not found under %s", entry.Subdir)
		}
		parserPath = detected
	}
	source, err := os.ReadFile(parserPath)
	if err != nil {
		return false, err
	}
	if err := os.RemoveAll(dest); err != nil {
		return false, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(dest, "parser.c"), source, 0o644); err != nil {
		return false, err
	}
	sum := sha256.Sum256(source)
	// source.txt is written last: a folder without it is incomplete.
	record := fmt.Sprintf("%s\nparser_sha256=%x\n", identity, sum)
	return false, os.WriteFile(filepath.Join(dest, freshnessSourceFile), []byte(record), 0o644)
}

// readFreshnessSource returns the cached parser.c in folder when its
// source.txt names identity and its digest matches.
func readFreshnessSource(folder, identity string) ([]byte, error) {
	record, err := os.ReadFile(filepath.Join(folder, freshnessSourceFile))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(record), "\n"), "\n")
	if len(lines) != 2 {
		return nil, fmt.Errorf("%s: want 2 lines, have %d", freshnessSourceFile, len(lines))
	}
	if lines[0] != identity {
		return nil, fmt.Errorf("cached source %q does not match the lock entry %q", lines[0], identity)
	}
	source, err := os.ReadFile(filepath.Join(folder, "parser.c"))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(source)
	if want := fmt.Sprintf("parser_sha256=%x", sum); lines[1] != want {
		return nil, fmt.Errorf("cached parser.c digest %s, %s says %s", want, freshnessSourceFile, lines[1])
	}
	return source, nil
}

// fetchPinnedCommit checks out exactly commit in dir. It fetches only that
// commit, as cmd/grammar_updater -verify-pins does, and falls back to batch
// mode's clone when the server refuses a fetch by commit.
func fetchPinnedCommit(repoURL, commit, dir string) error {
	if commit == "" {
		return errors.New("the lock entry has no pinned commit")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	err := runFetchGit(dir, "init", "-q")
	if err == nil {
		err = runFetchGit(dir, "fetch", "--depth=1", "-q", repoURL, commit)
	}
	if err == nil {
		err = runFetchGit(dir, "checkout", "-q", "--detach", "FETCH_HEAD")
	}
	if err != nil {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			return rmErr
		}
		if cloneErr := cloneRepo(repoURL, commit, dir); cloneErr != nil {
			return fmt.Errorf("fetch %s: %v; clone: %w", shortCommit(commit), err, cloneErr)
		}
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	head := strings.TrimSpace(string(out))
	if !strings.HasPrefix(head, strings.ToLower(commit)) {
		return fmt.Errorf("checked out %s, want pinned commit %s", head, commit)
	}
	return nil
}

func runFetchGit(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
