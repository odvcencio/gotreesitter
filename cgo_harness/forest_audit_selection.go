package cgoharness

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/realcorpus"
)

type forestCorpusCandidate struct {
	path string
	size int64
}

func parseForestCorpusLock(lockPath string, data []byte) (map[string]forestCorpusLockEntry, error) {
	entries := map[string]forestCorpusLockEntry{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("%s:%d: expected language repository revision", lockPath, lineNo)
		}
		if err := forestManifestLanguage(fields[0]); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", lockPath, lineNo, err)
		}
		if _, exists := entries[fields[0]]; exists {
			return nil, fmt.Errorf("%s:%d: duplicate language %q", lockPath, lineNo, fields[0])
		}
		if err := forestManifestRevision(fields[2]); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", lockPath, lineNo, err)
		}
		entry := forestCorpusLockEntry{Language: fields[0], Repository: fields[1], Revision: fields[2], Subdir: "."}
		if len(fields) >= 4 {
			entry.Subdir = fields[3]
		}
		if _, err := forestManifestPath("/", "corpus subdir", entry.Subdir); err != nil && entry.Subdir != "." {
			return nil, fmt.Errorf("%s:%d: %w", lockPath, lineNo, err)
		}
		if len(fields) >= 5 {
			entry.Matchers = strings.Split(fields[4], ",")
		}
		entries[entry.Language] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("corpus lock %s has no entries", lockPath)
	}
	return entries, nil
}

func selectForestCorpusLanguages(entries map[string]forestCorpusLockEntry, requested []string) ([]string, error) {
	if len(requested) == 0 {
		requested = make([]string, 0, len(entries))
		for language := range entries {
			requested = append(requested, language)
		}
	}
	seen := map[string]bool{}
	var languages []string
	for _, raw := range requested {
		language := strings.TrimSpace(raw)
		if language == "" || seen[language] {
			continue
		}
		if _, ok := entries[language]; !ok {
			return nil, fmt.Errorf("language %q absent from corpus lock", language)
		}
		seen[language] = true
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages, nil
}

func normalizeForestCorpusSelection(selection ForestCorpusSelection) (ForestCorpusSelection, error) {
	selection.Order = strings.TrimSpace(selection.Order)
	if selection.Order == "" {
		selection.Order = "largest"
	}
	switch selection.Order {
	case "largest", "smallest", "path":
	default:
		return selection, fmt.Errorf("invalid forest corpus order %q", selection.Order)
	}
	if selection.MaxFiles < 0 || selection.MinFileBytes < 0 || selection.MaxFileBytes < 0 {
		return selection, fmt.Errorf("forest corpus selection limits must be non-negative")
	}
	selection.ExcludePaths = normalizeForestExcludePaths(selection.ExcludePaths)
	return selection, nil
}

func selectForestCorpusFiles(root string, entry forestCorpusLockEntry, matcher realcorpus.FileMatcher, selection ForestCorpusSelection) ([]ForestCorpusManifestFile, error) {
	checkoutRoot, walkRoot := forestCorpusRoots(root, entry)
	candidates, err := walkForestCorpusCandidates(checkoutRoot, walkRoot, entry, matcher, selection)
	if err != nil {
		return nil, fmt.Errorf("walk %s corpus: %w", entry.Language, err)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no corpus files selected for %s", entry.Language)
	}
	sortForestCorpusCandidates(candidates, selection.Order)
	if selection.MaxFiles > 0 && len(candidates) > selection.MaxFiles {
		candidates = candidates[:selection.MaxFiles]
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })
	return materializeForestCorpusFiles(checkoutRoot, entry, candidates)
}

func forestCorpusRoots(root string, entry forestCorpusLockEntry) (string, string) {
	checkoutRoot := filepath.Join(root, entry.Language)
	walkRoot := checkoutRoot
	if entry.Subdir != "" && entry.Subdir != "." {
		walkRoot = filepath.Join(checkoutRoot, filepath.FromSlash(entry.Subdir))
	}
	return checkoutRoot, walkRoot
}

func walkForestCorpusCandidates(checkoutRoot, walkRoot string, entry forestCorpusLockEntry, matcher realcorpus.FileMatcher, selection ForestCorpusSelection) ([]forestCorpusCandidate, error) {
	var candidates []forestCorpusCandidate
	err := realcorpus.WalkMatchingFiles(walkRoot, func(relativePath string) bool {
		return matcher.Matches(relativePath) && !forestCorpusPathExcluded(entry.Language, relativePath, selection.ExcludePaths)
	}, func(file realcorpus.File) error {
		candidate, eligible, err := forestCorpusCandidateForFile(checkoutRoot, file, selection)
		if err != nil {
			return err
		}
		if eligible {
			candidates = append(candidates, candidate)
		}
		return nil
	})
	return candidates, err
}

func forestCorpusCandidateForFile(checkoutRoot string, file realcorpus.File, selection ForestCorpusSelection) (forestCorpusCandidate, bool, error) {
	if !forestCorpusSizeEligible(file.Size, selection) {
		return forestCorpusCandidate{}, false, nil
	}
	relCheckout, err := filepath.Rel(checkoutRoot, file.Path)
	if err != nil {
		return forestCorpusCandidate{}, false, err
	}
	return forestCorpusCandidate{path: filepath.ToSlash(relCheckout), size: file.Size}, true, nil
}

func forestCorpusSizeEligible(size int64, selection ForestCorpusSelection) bool {
	if selection.MinFileBytes > 0 && size < selection.MinFileBytes {
		return false
	}
	return selection.MaxFileBytes == 0 || size <= selection.MaxFileBytes
}

func sortForestCorpusCandidates(candidates []forestCorpusCandidate, order string) {
	sort.Slice(candidates, func(i, j int) bool {
		if order == "path" {
			return candidates[i].path < candidates[j].path
		}
		if candidates[i].size != candidates[j].size {
			if order == "smallest" {
				return candidates[i].size < candidates[j].size
			}
			return candidates[i].size > candidates[j].size
		}
		return candidates[i].path < candidates[j].path
	})
}

func materializeForestCorpusFiles(checkoutRoot string, entry forestCorpusLockEntry, candidates []forestCorpusCandidate) ([]ForestCorpusManifestFile, error) {
	files := make([]ForestCorpusManifestFile, 0, len(candidates))
	for _, candidate := range candidates {
		filePath := filepath.Join(checkoutRoot, filepath.FromSlash(candidate.path))
		digest, err := forestFileSHA256(filePath)
		if err != nil {
			return nil, err
		}
		files = append(files, ForestCorpusManifestFile{
			Language: entry.Language, Repository: entry.Repository, Revision: entry.Revision,
			Path: candidate.path, Bytes: candidate.size, SHA256: digest,
		})
	}
	return files, nil
}

func verifyForestCorpusCheckout(root string, entry forestCorpusLockEntry) error {
	checkout := filepath.Join(root, entry.Language)
	git := func(args ...string) ([]byte, error) {
		prefix := []string{"-c", "safe.directory=" + checkout, "-C", checkout}
		return exec.Command("git", append(prefix, args...)...).Output()
	}
	out, err := git("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("verify %s corpus checkout: %w", entry.Language, err)
	}
	head := strings.TrimSpace(string(out))
	if head != entry.Revision {
		return fmt.Errorf("%s corpus checkout revision %s, lock requires %s", entry.Language, head, entry.Revision)
	}
	status, err := git("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("verify %s corpus checkout cleanliness: %w", entry.Language, err)
	}
	gitLFSObjectCurrent := func(rel string) bool { return forestGitLFSObjectCurrent(checkout, rel, git) }
	if err := verifyForestCorpusCheckoutStatus(entry, status, gitLFSObjectCurrent); err != nil {
		return err
	}
	remote, err := git("remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("verify %s corpus checkout origin: %w", entry.Language, err)
	}
	if !forestSameRepository(strings.TrimSpace(string(remote)), entry.Repository) {
		return fmt.Errorf("%s corpus checkout origin %q, lock requires %q", entry.Language, strings.TrimSpace(string(remote)), entry.Repository)
	}
	return nil
}

func verifyForestCorpusCheckoutStatus(entry forestCorpusLockEntry, status []byte, gitLFSObjectCurrent func(rel string) bool) error {
	for len(status) > 0 {
		end := bytes.IndexByte(status, 0)
		if end < 0 {
			return fmt.Errorf("%s corpus checkout returned malformed git status", entry.Language)
		}
		record := status[:end]
		status = status[end+1:]
		if len(record) < 4 || record[2] != ' ' {
			return fmt.Errorf("%s corpus checkout returned malformed git status", entry.Language)
		}
		state, rel := string(record[:2]), string(record[3:])
		if state == "??" && forestCorpusGeneratedPathAllowed(entry, rel) {
			continue
		}
		if state == " M" && gitLFSObjectCurrent(rel) {
			continue
		}
		return fmt.Errorf("%s corpus checkout is dirty (%s %s)", entry.Language, state, rel)
	}
	return nil
}

func forestCorpusGeneratedPathAllowed(entry forestCorpusLockEntry, rel string) bool {
	if err := forestManifestLanguage(entry.Language); err != nil {
		return false
	}
	generatedSubdir := path.Join(".gts-extracted", entry.Language)
	if entry.Subdir != generatedSubdir {
		return false
	}
	if rel == "" || strings.ContainsRune(rel, '\\') || filepath.IsAbs(rel) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	return clean == rel && strings.HasPrefix(rel, generatedSubdir+"/")
}

// forestGitLFSObjectCurrent reports whether the file at rel holds the Git LFS
// object that the pointer committed at HEAD names.
//
// A checkout made with git-lfs replaces each pointer with its object. A git
// without git-lfs, such as the one in the harness image, cannot run the clean
// filter that maps the object back to the pointer. It hashes the object, finds
// a blob that differs from the pointer, and reports the file as modified. The
// Odin corpus stores its vendored Windows libraries this way. Matching the file
// against the size and SHA-256 in the pointer makes the content check that git
// status cannot make here, so it forgives no change to the bytes.
func forestGitLFSObjectCurrent(checkout, rel string, git func(args ...string) ([]byte, error)) bool {
	attribute, err := git("check-attr", "-z", "filter", "--", rel)
	if err != nil {
		return false
	}
	// The -z form is "<path> NUL <attribute> NUL <value> NUL".
	if fields := bytes.Split(attribute, []byte{0}); len(fields) < 3 || string(fields[2]) != "lfs" {
		return false
	}
	blob := "HEAD:" + rel
	blobSize, err := git("cat-file", "-s", blob)
	if err != nil {
		return false
	}
	if n, err := strconv.Atoi(strings.TrimSpace(string(blobSize))); err != nil || n > forestGitLFSPointerMaxBytes {
		return false
	}
	pointer, err := git("cat-file", "blob", blob)
	if err != nil {
		return false
	}
	oid, size, ok := parseForestGitLFSPointer(pointer)
	if !ok {
		return false
	}
	filePath, err := forestManifestPath(checkout, "git LFS object", rel)
	if err != nil {
		return false
	}
	return forestVerifyManifestFile(filePath, size, oid, rel) == nil
}

// forestGitLFSPointerMaxBytes is the largest pointer the Git LFS specification allows.
const forestGitLFSPointerMaxBytes = 1024

// parseForestGitLFSPointer reads a Git LFS version 1 pointer: the lines
// "version https://git-lfs.github.com/spec/v1", "oid sha256:<lowercase hex>"
// and "size <bytes>", in that order. It rejects every other form, so a file
// that only resembles a pointer never names an object.
func parseForestGitLFSPointer(data []byte) (oid string, size int64, ok bool) {
	text, found := strings.CutSuffix(string(data), "\n")
	if !found {
		return "", 0, false
	}
	lines := strings.Split(text, "\n")
	if len(lines) != 3 || lines[0] != "version https://git-lfs.github.com/spec/v1" {
		return "", 0, false
	}
	oid, found = strings.CutPrefix(lines[1], "oid sha256:")
	if !found {
		return "", 0, false
	}
	if decoded, err := hex.DecodeString(oid); err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != oid {
		return "", 0, false
	}
	sizeText, found := strings.CutPrefix(lines[2], "size ")
	if !found {
		return "", 0, false
	}
	size, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil || size < 0 || strconv.FormatInt(size, 10) != sizeText {
		return "", 0, false
	}
	return oid, size, true
}

func forestSameRepository(left, right string) bool {
	normalize := func(value string) string {
		return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(value), "/"), ".git")
	}
	return normalize(left) == normalize(right)
}

func normalizeForestExcludePaths(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range raw {
		item = strings.Trim(strings.ReplaceAll(strings.TrimSpace(item), "\\", "/"), "/")
		if item != "" && !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func forestCorpusPathExcluded(language, rel string, patterns []string) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	for _, pattern := range patterns {
		for _, candidate := range []string{rel, language + "/" + rel} {
			if pattern == candidate || strings.HasSuffix(pattern, "/") && strings.HasPrefix(candidate, pattern) {
				return true
			}
			if strings.ContainsAny(pattern, "*?[") {
				if ok, err := path.Match(pattern, candidate); err == nil && ok {
					return true
				}
			}
		}
	}
	return false
}
