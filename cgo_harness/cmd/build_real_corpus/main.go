package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultSmallMin  = 256
	defaultMediumMin = 2 * 1024
	defaultLargeMin  = 16 * 1024
	defaultMaxBytes  = 512 * 1024
)

type lockEntry struct {
	Name    string
	RepoURL string
	Commit  string
	Subdir  string
	Exts    []string
}

type profileSource struct {
	Language string `json:"language"`
	RepoURL  string `json:"repo_url"`
	Commit   string `json:"commit"`
	Subdir   string `json:"subdir,omitempty"`
}

type profileFile struct {
	Name      string          `json:"name"`
	Languages []string        `json:"languages"`
	Sources   []profileSource `json:"sources,omitempty"`
}

type corpusFile struct {
	AbsPath    string
	RelPath    string
	Size       int64
	SourceRoot string
}

type corpusManifest struct {
	GeneratedAt       string                `json:"generated_at"`
	LockPath          string                `json:"lock_path"`
	ProfilePath       string                `json:"profile_path,omitempty"`
	WorkDir           string                `json:"work_dir"`
	Languages         []string              `json:"languages"`
	IncludeFixtures   bool                  `json:"include_fixtures"`
	MinSmallBytes     int                   `json:"min_small_bytes"`
	MinMediumBytes    int                   `json:"min_medium_bytes"`
	MinLargeBytes     int                   `json:"min_large_bytes"`
	MaxBytes          int                   `json:"max_bytes"`
	MaxFilesPerBucket int                   `json:"max_files_per_bucket"`
	Entries           []corpusManifestEntry `json:"entries"`
	Missing           []string              `json:"missing_languages,omitempty"`
}

type corpusManifestEntry struct {
	Language     string `json:"language"`
	Bucket       string `json:"bucket"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	SourceRepo   string `json:"source_repo"`
	SourceCommit string `json:"source_commit"`
	SourcePath   string `json:"source_path"`
	OutputPath   string `json:"output_path"`
}

func main() {
	var (
		lockPath          string
		profilePath       string
		langsRaw          string
		outDir            string
		workDir           string
		repoCachePath     string
		keepWorkDir       bool
		includeFixtures   bool
		maxFilesPerBucket int
		minSmallBytes     int
		minMediumBytes    int
		minLargeBytes     int
		maxBytes          int
	)

	flag.StringVar(&lockPath, "lock", "", "path to grammars/languages.lock (auto-detected when empty)")
	flag.StringVar(&profilePath, "profile", "", "optional profile JSON path (e.g. cgo_harness/testdata/top50_manifest.json)")
	flag.StringVar(&langsRaw, "langs", "top50", "language list: top50 or comma-separated names")
	flag.StringVar(&outDir, "out", "cgo_harness/corpus_real", "output corpus directory")
	flag.StringVar(&workDir, "work-dir", "", "temporary clone work directory (default: temp dir)")
	flag.StringVar(&repoCachePath, "repo-cache", os.Getenv("GTS_PARITY_REPO_CACHE"), "optional root of cached git repos to mine for missing buckets")
	flag.BoolVar(&keepWorkDir, "keep-work-dir", false, "keep work directory after command exits")
	flag.BoolVar(&includeFixtures, "include-fixtures", false, "include upstream grammar corpus/tests/fixtures instead of restricting selection to real-world/example-style files")
	flag.IntVar(&maxFilesPerBucket, "max-files-per-bucket", 1, "max files selected per bucket per language")
	flag.IntVar(&minSmallBytes, "min-small-bytes", defaultSmallMin, "minimum bytes for small bucket")
	flag.IntVar(&minMediumBytes, "min-medium-bytes", defaultMediumMin, "minimum bytes for medium bucket")
	flag.IntVar(&minLargeBytes, "min-large-bytes", defaultLargeMin, "minimum bytes for large bucket")
	flag.IntVar(&maxBytes, "max-bytes", defaultMaxBytes, "maximum bytes for selected files")
	flag.Parse()

	if maxFilesPerBucket <= 0 {
		fatalf("-max-files-per-bucket must be > 0")
	}
	if !(minSmallBytes > 0 && minSmallBytes < minMediumBytes && minMediumBytes < minLargeBytes && minLargeBytes < maxBytes) {
		fatalf("invalid size thresholds: require 0 < small < medium < large < max")
	}

	resolvedLockPath, err := resolveLockPath(lockPath)
	if err != nil {
		fatalf("resolve lock path: %v", err)
	}
	lockEntries, err := parseLockFile(resolvedLockPath)
	if err != nil {
		fatalf("parse lock: %v", err)
	}

	resolvedProfilePath, profile, languages, err := resolveLanguageList(profilePath, langsRaw)
	if err != nil {
		fatalf("resolve languages: %v", err)
	}
	if len(languages) == 0 {
		fatalf("no languages selected")
	}
	profileSources := groupProfileSources(profile.Sources)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatalf("create out dir: %v", err)
	}

	createdWorkDir := false
	if strings.TrimSpace(workDir) == "" {
		workDir, err = os.MkdirTemp("", "gotreesitter-real-corpus-*")
		if err != nil {
			fatalf("create work dir: %v", err)
		}
		createdWorkDir = true
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		fatalf("create work dir: %v", err)
	}
	if createdWorkDir && !keepWorkDir {
		defer os.RemoveAll(workDir)
	}

	manifest := corpusManifest{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		LockPath:          resolvedLockPath,
		ProfilePath:       resolvedProfilePath,
		WorkDir:           workDir,
		Languages:         append([]string(nil), languages...),
		IncludeFixtures:   includeFixtures,
		MinSmallBytes:     minSmallBytes,
		MinMediumBytes:    minMediumBytes,
		MinLargeBytes:     minLargeBytes,
		MaxBytes:          maxBytes,
		MaxFilesPerBucket: maxFilesPerBucket,
		Entries:           make([]corpusManifestEntry, 0, len(languages)*3*maxFilesPerBucket),
	}

	repoRoot := filepath.Join(workDir, "repos")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		fatalf("create repo root: %v", err)
	}
	repoMetaCache := map[string]repoMetadata{}

	for _, lang := range languages {
		entry, ok := lockEntries[lang]
		if !ok {
			manifest.Missing = append(manifest.Missing, lang)
			fmt.Fprintf(os.Stderr, "[warn] language %q missing from lock; skipping\n", lang)
			continue
		}

		repoDir := filepath.Join(repoRoot, safeName(lang+"-"+shortCommit(entry.Commit)))
		if err := checkoutRepoAtCommit(entry.RepoURL, entry.Commit, repoDir); err != nil {
			manifest.Missing = append(manifest.Missing, lang)
			fmt.Fprintf(os.Stderr, "[warn] checkout failed for %q: %v\n", lang, err)
			continue
		}

		candidateExts, candidateNames := candidateMatchersForLanguage(lang, entry.Exts)
		candidates, err := collectCandidatesWithNames(repoDir, candidateExts, candidateNames, maxBytes, includeFixtures)
		if err != nil {
			manifest.Missing = append(manifest.Missing, lang)
			fmt.Fprintf(os.Stderr, "[warn] collect candidates failed for %q: %v\n", lang, err)
			continue
		}
		if needsRepoCacheFallback(candidates, minMediumBytes, minLargeBytes) {
			extra, err := collectCandidatesFromProfileSources(profileSources[lang], repoRoot, candidateExts, candidateNames, maxBytes, includeFixtures)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[warn] profile-source fallback failed for %q: %v\n", lang, err)
			} else if len(extra) > 0 {
				candidates = appendUniqueCorpusFiles(candidates, extra)
			}
		}
		if needsRepoCacheFallback(candidates, minMediumBytes, minLargeBytes) {
			extra, err := collectCandidatesFromRepoCache(repoCachePath, repoDir, candidateExts, candidateNames, maxBytes, includeFixtures)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[warn] repo-cache fallback failed for %q: %v\n", lang, err)
			} else if len(extra) > 0 {
				candidates = appendUniqueCorpusFiles(candidates, extra)
			}
		}
		if len(candidates) == 0 {
			manifest.Missing = append(manifest.Missing, lang)
			fmt.Fprintf(os.Stderr, "[warn] no corpus candidates for %q\n", lang)
			continue
		}

		selected := selectFilesByBucket(candidates, maxFilesPerBucket, minSmallBytes, minMediumBytes, minLargeBytes)
		if len(selected) == 0 {
			manifest.Missing = append(manifest.Missing, lang)
			fmt.Fprintf(os.Stderr, "[warn] no selected corpus files for %q\n", lang)
			continue
		}

		langOutDir := filepath.Join(outDir, lang)
		if err := os.MkdirAll(langOutDir, 0o755); err != nil {
			fatalf("create output directory for %q: %v", lang, err)
		}
		for _, sf := range selected {
			content, err := os.ReadFile(sf.AbsPath)
			if err != nil {
				fatalf("read selected file %q: %v", sf.AbsPath, err)
			}
			outputs := materializeCorpusOutputs(sf, content)
			for _, out := range outputs {
				sum := sha256.Sum256(out.Content)
				outputPath := filepath.Join(langOutDir, out.Name)
				if err := os.WriteFile(outputPath, out.Content, 0o644); err != nil {
					fatalf("write %s: %v", outputPath, err)
				}
				manifest.Entries = append(manifest.Entries, corpusManifestEntry{
					Language:     lang,
					Bucket:       sf.Bucket,
					Bytes:        int64(len(out.Content)),
					SHA256:       hex.EncodeToString(sum[:]),
					SourceRepo:   sourceRepoURL(sf.corpusFile, repoDir, entry, repoMetaCache),
					SourceCommit: sourceRepoCommit(sf.corpusFile, repoDir, entry, repoMetaCache),
					SourcePath:   sf.RelPath,
					OutputPath:   outputPath,
				})
			}
		}
		fmt.Printf("[ok] %s: selected=%d candidates=%d\n", lang, len(selected), len(candidates))
	}

	sort.Slice(manifest.Entries, func(i, j int) bool {
		a, b := manifest.Entries[i], manifest.Entries[j]
		if a.Language != b.Language {
			return a.Language < b.Language
		}
		if a.Bucket != b.Bucket {
			return a.Bucket < b.Bucket
		}
		return a.SourcePath < b.SourcePath
	})
	sort.Strings(manifest.Missing)

	manifestPath := filepath.Join(outDir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		fatalf("write manifest: %v", err)
	}

	fmt.Printf("\nWrote real corpus: %s\n", outDir)
	fmt.Printf("Manifest: %s\n", manifestPath)
	fmt.Printf("Languages requested: %d\n", len(languages))
	fmt.Printf("Entries selected: %d\n", len(manifest.Entries))
	if len(manifest.Missing) > 0 {
		fmt.Printf("Missing/failed: %d (%s)\n", len(manifest.Missing), strings.Join(manifest.Missing, ", "))
	}
}

type repoMetadata struct {
	URL    string
	Commit string
}

func sourceRepoURL(cf corpusFile, primaryRoot string, entry lockEntry, cache map[string]repoMetadata) string {
	if cf.SourceRoot == "" || sameDir(cf.SourceRoot, primaryRoot) {
		return entry.RepoURL
	}
	return repoMetadataForRoot(cf.SourceRoot, cache).URL
}

func sourceRepoCommit(cf corpusFile, primaryRoot string, entry lockEntry, cache map[string]repoMetadata) string {
	if cf.SourceRoot == "" || sameDir(cf.SourceRoot, primaryRoot) {
		return entry.Commit
	}
	return repoMetadataForRoot(cf.SourceRoot, cache).Commit
}

func repoMetadataForRoot(root string, cache map[string]repoMetadata) repoMetadata {
	root = filepath.Clean(root)
	if meta, ok := cache[root]; ok {
		return meta
	}
	base := filepath.Base(root)
	meta := repoMetadata{
		URL:    strings.TrimSpace(gitOutput(root, "config", "--get", "remote.origin.url")),
		Commit: strings.TrimSpace(gitOutput(root, "rev-parse", "HEAD")),
	}
	if meta.URL == "" {
		meta.URL = "local:" + base
	}
	if meta.Commit == "" {
		meta.Commit = inferredLocalCacheCommit(base)
	}
	cache[root] = meta
	return meta
}

func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func resolveLanguageList(profilePath, langsRaw string) (string, profileFile, []string, error) {
	if strings.TrimSpace(profilePath) != "" {
		p, err := resolvePath(profilePath, []string{profilePath})
		if err != nil {
			return "", profileFile{}, nil, err
		}
		profile, err := loadProfile(p)
		if err != nil {
			return "", profileFile{}, nil, err
		}
		return p, profile, dedupe(profile.Languages), nil
	}
	value := strings.TrimSpace(langsRaw)
	switch value {
	case "", "top50", "top":
		listPath, err := resolveTop50Path()
		if err != nil {
			return "", profileFile{}, nil, err
		}
		langs, err := loadTop50List(listPath)
		if err != nil {
			return "", profileFile{}, nil, err
		}
		profile := profileFile{Name: "top50", Languages: dedupe(langs)}
		return "", profile, profile.Languages, nil
	default:
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			name := strings.TrimSpace(part)
			if name == "" {
				continue
			}
			out = append(out, name)
		}
		profile := profileFile{Name: "inline", Languages: dedupe(out)}
		return "", profile, profile.Languages, nil
	}
}

func loadProfile(path string) (profileFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return profileFile{}, err
	}
	var profile profileFile
	if err := json.Unmarshal(b, &profile); err != nil {
		return profileFile{}, fmt.Errorf("decode profile json: %w", err)
	}
	if len(profile.Languages) == 0 {
		return profileFile{}, fmt.Errorf("profile %s has no languages", path)
	}
	for i, src := range profile.Sources {
		if strings.TrimSpace(src.Language) == "" {
			return profileFile{}, fmt.Errorf("profile %s source[%d] missing language", path, i)
		}
		if strings.TrimSpace(src.RepoURL) == "" {
			return profileFile{}, fmt.Errorf("profile %s source[%d] missing repo_url", path, i)
		}
		if strings.TrimSpace(src.Commit) == "" {
			return profileFile{}, fmt.Errorf("profile %s source[%d] missing commit", path, i)
		}
	}
	return profile, nil
}

func resolveLockPath(raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return resolvePath(raw, []string{raw})
	}
	return resolvePath("grammars/languages.lock", []string{
		"grammars/languages.lock",
		filepath.Join("..", "grammars", "languages.lock"),
	})
}

func resolveTop50Path() (string, error) {
	return resolvePath("grammars/update_tier1_top50.txt", []string{
		"grammars/update_tier1_top50.txt",
		filepath.Join("..", "grammars", "update_tier1_top50.txt"),
	})
}

func resolvePath(label string, candidates []string) (string, error) {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find %s (tried: %s)", label, strings.Join(candidates, ", "))
}

func parseLockFile(path string) (map[string]lockEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]lockEntry{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("%s:%d: invalid lock row", path, lineNo)
		}
		entry := lockEntry{
			Name:    fields[0],
			RepoURL: fields[1],
			Commit:  fields[2],
			Subdir:  "src",
		}
		if len(fields) >= 4 {
			entry.Subdir = fields[3]
		}
		if len(fields) >= 5 {
			rawExts := strings.Split(fields[4], ",")
			entry.Exts = make([]string, 0, len(rawExts))
			for _, ext := range rawExts {
				ext = strings.TrimSpace(ext)
				if ext == "" {
					continue
				}
				entry.Exts = append(entry.Exts, strings.ToLower(ext))
			}
		}
		out[entry.Name] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func loadTop50List(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func checkoutRepoAtCommit(repoURL, commit, dstDir string) error {
	if strings.TrimSpace(repoURL) == "" || strings.TrimSpace(commit) == "" {
		return errors.New("empty repo URL or commit")
	}
	if _, err := os.Stat(dstDir); err == nil {
		return nil
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	run := func(args ...string) error {
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	runRetry := func(attempts int, args ...string) error {
		if attempts < 1 {
			attempts = 1
		}
		var lastErr error
		for i := 0; i < attempts; i++ {
			if err := run(args...); err == nil {
				return nil
			} else {
				lastErr = err
				if !retryableGitCheckoutError(err) || i == attempts-1 {
					break
				}
			}
			delay := time.Duration(i+1) * time.Second
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
			time.Sleep(delay)
		}
		return lastErr
	}
	if err := run("-C", dstDir, "init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := run("-C", dstDir, "remote", "add", "origin", repoURL); err != nil {
		return fmt.Errorf("git remote add: %w", err)
	}
	if err := runRetry(5, "-C", dstDir, "fetch", "--depth=1", "origin", commit); err != nil {
		return fmt.Errorf("git fetch %s: %w", shortCommit(commit), err)
	}
	if err := run("-C", dstDir, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}
	return nil
}

func collectCandidates(repoDir string, exts []string, maxBytes int, includeFixtures bool) ([]corpusFile, error) {
	return collectCandidatesWithNames(repoDir, exts, nil, maxBytes, includeFixtures)
}

func collectCandidatesWithNames(repoDir string, exts, names []string, maxBytes int, includeFixtures bool) ([]corpusFile, error) {
	return collectCandidatesWithNamesFromRoot(repoDir, repoDir, exts, names, maxBytes, includeFixtures)
}

func collectCandidatesWithNamesFromRoot(sourceRoot, walkRoot string, exts, names []string, maxBytes int, includeFixtures bool) ([]corpusFile, error) {
	seen := map[string]struct{}{}
	out := make([]corpusFile, 0, 256)
	extSet := map[string]struct{}{}
	for _, ext := range exts {
		extSet[strings.ToLower(strings.TrimSpace(ext))] = struct{}{}
	}
	nameSet := map[string]struct{}{}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		nameSet[name] = struct{}{}
	}

	addFile := func(absPath string, d fs.DirEntry, requireKnownExt bool) {
		if d.IsDir() {
			return
		}
		base := strings.ToLower(filepath.Base(absPath))
		if requireKnownExt && len(extSet) > 0 {
			ext := strings.ToLower(filepath.Ext(absPath))
			if _, ok := extSet[ext]; !ok {
				if _, ok := nameSet[base]; !ok {
					return
				}
			}
		}
		if requireKnownExt && len(extSet) == 0 && len(nameSet) > 0 {
			if _, ok := nameSet[base]; !ok {
				return
			}
		}
		rel, err := filepath.Rel(sourceRoot, absPath)
		if err != nil {
			return
		}
		if !looksCorpusCandidatePath(rel, includeFixtures, nameSet) {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		info, err := d.Info()
		if err != nil {
			return
		}
		size := info.Size()
		if size > int64(maxBytes) {
			return
		}
		if size < defaultSmallMin && !allowUndersizedSourceCandidate(rel, extSet, nameSet) {
			return
		}
		if !looksText(absPath) {
			return
		}
		seen[rel] = struct{}{}
		out = append(out, corpusFile{
			AbsPath:    absPath,
			RelPath:    rel,
			Size:       size,
			SourceRoot: sourceRoot,
		})
	}

	type priorityDir struct {
		path            string
		requireKnownExt bool
	}
	priorityDirs := []priorityDir{
		{path: "examples", requireKnownExt: true},
		{path: "example", requireKnownExt: true},
		{path: "samples", requireKnownExt: true},
		{path: "spec", requireKnownExt: false},
	}
	if includeFixtures {
		priorityDirs = append([]priorityDir{
			{path: "test/corpus", requireKnownExt: false},
			{path: "tests/corpus", requireKnownExt: false},
			{path: "corpus", requireKnownExt: false},
			{path: "test/fixtures", requireKnownExt: false},
			{path: "tests/fixtures", requireKnownExt: false},
			{path: "fixtures", requireKnownExt: false},
		}, priorityDirs...)
	}
	for _, dir := range priorityDirs {
		root := filepath.Join(walkRoot, filepath.FromSlash(dir.path))
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			addFile(path, d, dir.requireKnownExt)
			return nil
		})
	}
	_ = filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := strings.ToLower(d.Name())
			switch base {
			case ".git", "node_modules", "vendor", "target", "build", "dist", "queries":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if len(extSet) > 0 {
			ext := strings.ToLower(filepath.Ext(path))
			if _, ok := extSet[ext]; !ok {
				base := strings.ToLower(filepath.Base(path))
				if _, ok := nameSet[base]; !ok {
					return nil
				}
			}
		} else if len(nameSet) > 0 {
			base := strings.ToLower(filepath.Base(path))
			if _, ok := nameSet[base]; !ok {
				return nil
			}
		} else {
			rel, err := filepath.Rel(sourceRoot, path)
			if err != nil {
				return nil
			}
			if !looksGenericSourcePath(rel, includeFixtures) {
				return nil
			}
		}
		addFile(path, d, false)
		return nil
	})

	sort.Slice(out, func(i, j int) bool {
		if out[i].Size != out[j].Size {
			return out[i].Size < out[j].Size
		}
		return out[i].RelPath < out[j].RelPath
	})
	return out, nil
}

func collectCandidatesFromProfileSources(sources []profileSource, checkoutRoot string, exts []string, names []string, maxBytes int, includeFixtures bool) ([]corpusFile, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	out := make([]corpusFile, 0, 64)
	var failures []string
	for _, src := range sources {
		repoDir := filepath.Join(checkoutRoot, safeName(profileSourceCheckoutKey(src)))
		if err := checkoutRepoAtCommit(src.RepoURL, src.Commit, repoDir); err != nil {
			failures = append(failures, fmt.Sprintf("%s@%s: %v", src.RepoURL, shortCommit(src.Commit), err))
			continue
		}
		walkRoot := repoDir
		if strings.TrimSpace(src.Subdir) != "" {
			walkRoot = filepath.Join(repoDir, filepath.FromSlash(src.Subdir))
			if st, err := os.Stat(walkRoot); err != nil || !st.IsDir() {
				failures = append(failures, fmt.Sprintf("%s@%s: missing subdir %q", src.RepoURL, shortCommit(src.Commit), src.Subdir))
				continue
			}
		}
		candidates, err := collectCandidatesWithNamesFromRoot(repoDir, walkRoot, exts, names, maxBytes, includeFixtures)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s@%s: %v", src.RepoURL, shortCommit(src.Commit), err))
			continue
		}
		out = appendUniqueCorpusFiles(out, candidates)
	}
	if len(out) == 0 && len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "; "))
	}
	return out, nil
}

func collectCandidatesFromRepoCache(repoCachePath, primaryRepoDir string, exts []string, names []string, maxBytes int, includeFixtures bool) ([]corpusFile, error) {
	if strings.TrimSpace(repoCachePath) == "" {
		return nil, nil
	}
	repoRoots, err := discoverRepoRoots(repoCachePath)
	if err != nil {
		return nil, err
	}
	out := make([]corpusFile, 0, 64)
	primaryBase := filepath.Base(filepath.Clean(primaryRepoDir))
	for _, root := range repoRoots {
		if sameDir(root, primaryRepoDir) || filepath.Base(filepath.Clean(root)) == primaryBase {
			continue
		}
		candidates, err := collectCandidatesWithNames(root, exts, names, maxBytes, includeFixtures)
		if err != nil {
			continue
		}
		out = append(out, candidates...)
	}
	return out, nil
}

func discoverRepoRoots(root string) ([]string, error) {
	root = filepath.Clean(root)
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	if isGitRepo(root) {
		return []string{root}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Join(root, ent.Name())
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func isGitRepo(path string) bool {
	return strings.TrimSpace(gitOutput(path, "rev-parse", "--show-toplevel")) != ""
}

func needsRepoCacheFallback(candidates []corpusFile, minMedium, minLarge int) bool {
	var hasMedium, hasLarge bool
	for _, cf := range candidates {
		switch {
		case cf.Size >= int64(minLarge):
			hasLarge = true
		case cf.Size >= int64(minMedium):
			hasMedium = true
		}
		if hasMedium && hasLarge {
			return false
		}
	}
	return !hasMedium || !hasLarge
}

func appendUniqueCorpusFiles(dst, extra []corpusFile) []corpusFile {
	seen := make(map[string]struct{}, len(dst)+len(extra))
	for _, cf := range dst {
		seen[corpusFileKey(cf)] = struct{}{}
	}
	for _, cf := range extra {
		key := corpusFileKey(cf)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dst = append(dst, cf)
	}
	return dst
}

func groupProfileSources(sources []profileSource) map[string][]profileSource {
	if len(sources) == 0 {
		return nil
	}
	out := make(map[string][]profileSource, len(sources))
	for _, src := range sources {
		lang := strings.TrimSpace(src.Language)
		if lang == "" {
			continue
		}
		out[lang] = append(out[lang], src)
	}
	return out
}

func corpusFileKey(cf corpusFile) string {
	if strings.TrimSpace(cf.AbsPath) != "" {
		return filepath.Clean(cf.AbsPath)
	}
	return filepath.ToSlash(cf.RelPath)
}

func inferredLocalCacheCommit(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return "unknown"
	}
	if idx := strings.LastIndexByte(base, '-'); idx >= 0 && idx+1 < len(base) {
		suffix := base[idx+1:]
		if len(suffix) >= 7 && len(suffix) <= 40 && isHexString(suffix) {
			return suffix
		}
	}
	return "unknown"
}

func isHexString(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return s != ""
}

func profileSourceCheckoutKey(src profileSource) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(src.RepoURL)), ".git")
	if base == "" {
		base = "source"
	}
	return base + "-" + shortCommit(src.Commit)
}

func candidateMatchersForLanguage(lang string, exts []string) ([]string, []string) {
	outExts := append([]string(nil), exts...)
	names := make([]string, 0, 2)
	if len(outExts) > 0 {
		return outExts, names
	}
	switch lang {
	case "awk":
		outExts = append(outExts, ".awk", ".gawk")
	case "cmake":
		outExts = append(outExts, ".cmake")
		names = append(names, "cmakelists.txt")
	case "d":
		outExts = append(outExts, ".d", ".di")
	case "dart":
		outExts = append(outExts, ".dart")
	case "erlang":
		outExts = append(outExts, ".erl", ".hrl")
	case "gomod":
		names = append(names, "go.mod")
	case "make":
		outExts = append(outExts, ".mk")
		names = append(names, "makefile")
	case "markdown":
		outExts = append(outExts, ".md")
		names = append(names, "readme.md")
	}
	return outExts, names
}

func allowUndersizedSourceCandidate(rel string, extSet, nameSet map[string]struct{}) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	if !isAllowedSourceTestPath(rel) {
		return false
	}
	if strings.HasSuffix(rel, ".cmake") {
		_, hasExt := extSet[".cmake"]
		_, hasName := nameSet["cmakelists.txt"]
		return hasExt || hasName
	}
	return false
}

type materializedCorpusOutput struct {
	Name    string
	Content []byte
}

type treeSitterCorpusCase struct {
	Title  string
	Source []byte
}

func materializeCorpusOutputs(sf selectedCorpusFile, content []byte) []materializedCorpusOutput {
	baseName := fmt.Sprintf("%s__%s", sf.Bucket, safeName(filepath.Base(sf.RelPath)))
	cases, ok := splitTreeSitterCorpusSources(content)
	if !ok {
		return []materializedCorpusOutput{{Name: baseName, Content: content}}
	}

	out := make([]materializedCorpusOutput, 0, len(cases))
	for i, c := range cases {
		name := fmt.Sprintf("%s__case%d", baseName, i+1)
		if title := safeName(c.Title); title != "" {
			name += "__" + title
		}
		out = append(out, materializedCorpusOutput{
			Name:    name,
			Content: c.Source,
		})
	}
	return out
}

func splitTreeSitterCorpusSources(content []byte) ([]treeSitterCorpusCase, bool) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.SplitAfter(text, "\n")
	cases := make([]treeSitterCorpusCase, 0, 4)

	for i := 0; i < len(lines); {
		if !isRepeatedLine(lines[i], '=') {
			i++
			continue
		}
		if i+2 >= len(lines) || !isRepeatedLine(lines[i+2], '=') {
			return nil, false
		}
		title := strings.TrimSpace(lines[i+1])
		i += 3
		if i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		start := i
		for i < len(lines) && !isRepeatedLine(lines[i], '-') {
			i++
		}
		if i >= len(lines) {
			return nil, false
		}
		source := strings.Join(lines[start:i], "")
		if strings.TrimSpace(source) == "" {
			return nil, false
		}
		cases = append(cases, treeSitterCorpusCase{
			Title:  title,
			Source: []byte(source),
		})
		i++
		for i < len(lines) && !isRepeatedLine(lines[i], '=') {
			i++
		}
	}

	return cases, len(cases) > 0
}

func isRepeatedLine(line string, want rune) bool {
	s := strings.TrimSpace(line)
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if r != want {
			return false
		}
	}
	return true
}

func looksCorpusCandidatePath(relPath string, includeFixtures bool, specialNames map[string]struct{}) bool {
	rel := strings.ToLower(filepath.ToSlash(relPath))
	base := strings.ToLower(filepath.Base(rel))
	if strings.HasPrefix(base, ".") {
		return false
	}
	switch base {
	case "readme", "readme.md", "readme.txt",
		"license", "license.md", "copying",
		"go.sum",
		"cargo.lock", "cargo.toml",
		"package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"pipfile.lock", "poetry.lock", "composer.lock",
		"gemfile.lock", "mix.lock":
		if _, ok := specialNames[base]; !ok {
			return false
		}
	case "go.mod":
		if _, ok := specialNames[base]; !ok {
			return false
		}
	}
	if strings.HasPrefix(rel, ".github/") ||
		strings.Contains(rel, "/.github/") ||
		strings.HasPrefix(rel, "bindings/") ||
		strings.Contains(rel, "/bindings/") ||
		strings.Contains(rel, "/node_modules/") ||
		strings.Contains(rel, "/vendor/") ||
		strings.Contains(rel, "/dist/") ||
		strings.Contains(rel, "/build/") {
		if !isAllowedSourceMetadataPath(rel) && !isAllowedSourceBindingPath(rel) {
			return false
		}
	}
	if !includeFixtures {
		if strings.HasPrefix(rel, "test/") ||
			strings.HasPrefix(rel, "tests/") ||
			strings.Contains(rel, "/test/") ||
			strings.Contains(rel, "/tests/") {
			if !isAllowedSourceTestPath(rel) {
				return false
			}
		}
		if strings.Contains(rel, "/corpus/") ||
			strings.Contains(rel, "/fixture") {
			return false
		}
	}
	return true
}

func isAllowedSourceTestPath(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	return strings.HasPrefix(rel, "test/highlight/") ||
		strings.HasPrefix(rel, "tests/highlight/") ||
		strings.HasPrefix(rel, "test/tags/") ||
		strings.HasPrefix(rel, "tests/tags/") ||
		strings.Contains(rel, "/test/highlight/") ||
		strings.Contains(rel, "/tests/highlight/") ||
		strings.Contains(rel, "/test/tags/") ||
		strings.Contains(rel, "/tests/tags/")
}

func isAllowedSourceBindingPath(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	return strings.HasPrefix(rel, "bindings/r/r/") ||
		strings.Contains(rel, "/bindings/r/r/") ||
		rel == "bindings/r/bootstrap.r" ||
		strings.HasSuffix(rel, "/bindings/r/bootstrap.r")
}

func isAllowedSourceMetadataPath(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	if !strings.HasSuffix(rel, ".yml") && !strings.HasSuffix(rel, ".yaml") {
		return false
	}
	return strings.HasPrefix(rel, ".github/workflows/") ||
		strings.Contains(rel, "/.github/workflows/") ||
		strings.HasPrefix(rel, ".github/issue_template/") ||
		strings.Contains(rel, "/.github/issue_template/")
}

func looksGenericSourcePath(relPath string, includeFixtures bool) bool {
	if !looksCorpusCandidatePath(relPath, includeFixtures, nil) {
		return false
	}
	rel := strings.ToLower(filepath.ToSlash(relPath))
	if includeFixtures {
		if strings.Contains(rel, "/test/") ||
			strings.Contains(rel, "/tests/") ||
			strings.Contains(rel, "/corpus/") ||
			strings.Contains(rel, "/fixture") ||
			strings.Contains(rel, "/example") ||
			strings.Contains(rel, "/sample") {
			return true
		}
		return false
	}
	if strings.Contains(rel, "/example") ||
		strings.Contains(rel, "/sample") {
		return true
	}
	return false
}

func selectFilesByBucket(candidates []corpusFile, maxPerBucket, minSmall, minMedium, minLarge int) []selectedCorpusFile {
	small, medium, large := make([]corpusFile, 0), make([]corpusFile, 0), make([]corpusFile, 0)
	for _, cf := range candidates {
		switch {
		case cf.Size >= int64(minSmall) && cf.Size < int64(minMedium):
			small = append(small, cf)
		case cf.Size >= int64(minMedium) && cf.Size < int64(minLarge):
			medium = append(medium, cf)
		case cf.Size >= int64(minLarge):
			large = append(large, cf)
		}
	}

	pick := func(bucket string, pool []corpusFile) []selectedCorpusFile {
		if len(pool) == 0 {
			return nil
		}
		result := make([]selectedCorpusFile, 0, maxPerBucket)
		switch bucket {
		case "small":
			sort.Slice(pool, func(i, j int) bool {
				if pool[i].Size != pool[j].Size {
					return pool[i].Size < pool[j].Size
				}
				return pool[i].AbsPath < pool[j].AbsPath
			})
		case "medium":
			target := int64((minMedium + minLarge) / 2)
			sort.Slice(pool, func(i, j int) bool {
				di := abs64(pool[i].Size - target)
				dj := abs64(pool[j].Size - target)
				if di != dj {
					return di < dj
				}
				return pool[i].AbsPath < pool[j].AbsPath
			})
		case "large":
			sort.Slice(pool, func(i, j int) bool {
				if pool[i].Size != pool[j].Size {
					return pool[i].Size > pool[j].Size
				}
				return pool[i].AbsPath < pool[j].AbsPath
			})
		}
		for i := 0; i < len(pool) && len(result) < maxPerBucket; i++ {
			result = append(result, selectedCorpusFile{
				corpusFile: pool[i],
				Bucket:     bucket,
			})
		}
		return result
	}

	all := append([]corpusFile(nil), candidates...)
	out := make([]selectedCorpusFile, 0, maxPerBucket*3)
	used := map[string]struct{}{}
	for _, bucketPick := range [][]selectedCorpusFile{
		pick("small", small),
		pick("medium", medium),
		pick("large", large),
	} {
		for _, sf := range bucketPick {
			if _, ok := used[corpusFileKey(sf.corpusFile)]; ok {
				continue
			}
			used[corpusFileKey(sf.corpusFile)] = struct{}{}
			out = append(out, sf)
		}
	}

	target := maxPerBucket * 3
	if len(out) < target {
		sort.Slice(all, func(i, j int) bool {
			if all[i].Size != all[j].Size {
				return all[i].Size > all[j].Size
			}
			return all[i].RelPath < all[j].RelPath
		})
		for _, cf := range all {
			if len(out) >= target {
				break
			}
			if _, ok := used[corpusFileKey(cf)]; ok {
				continue
			}
			used[corpusFileKey(cf)] = struct{}{}
			out = append(out, selectedCorpusFile{
				corpusFile: cf,
				Bucket:     classifyBucket(cf.Size, minSmall, minMedium, minLarge),
			})
		}
	}
	return out
}

type selectedCorpusFile struct {
	corpusFile
	Bucket string
}

func looksText(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	if n == 0 {
		return false
	}
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return false
		}
	}
	return true
}

func dedupe(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func safeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "file"
	}
	return out
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func retryableGitCheckoutError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "could not resolve host") ||
		strings.Contains(msg, "temporary failure in name resolution") ||
		strings.Contains(msg, "name or service not known") ||
		strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "operation timed out") ||
		strings.Contains(msg, "connection reset by peer")
}

func classifyBucket(size int64, minSmall, minMedium, minLarge int) string {
	switch {
	case size >= int64(minLarge):
		return "large"
	case size >= int64(minMedium):
		return "medium"
	case size >= int64(minSmall):
		return "small"
	default:
		return "small"
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "build_real_corpus: "+format+"\n", args...)
	os.Exit(2)
}
