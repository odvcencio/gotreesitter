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

type profileFile struct {
	Name      string   `json:"name"`
	Languages []string `json:"languages"`
}

type corpusFile struct {
	AbsPath string
	RelPath string
	Size    int64
}

type corpusManifest struct {
	GeneratedAt       string                `json:"generated_at"`
	LockPath          string                `json:"lock_path"`
	ProfilePath       string                `json:"profile_path,omitempty"`
	WorkDir           string                `json:"work_dir"`
	Languages         []string              `json:"languages"`
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
		keepWorkDir       bool
		maxFilesPerBucket int
		minSmallBytes     int
		minMediumBytes    int
		minLargeBytes     int
		maxBytes          int
	)

	flag.StringVar(&lockPath, "lock", "", "path to grammars/languages.lock (auto-detected when empty)")
	flag.StringVar(&profilePath, "profile", "", "optional profile JSON path (e.g. cmd/build_real_corpus/top50_manifest.json)")
	flag.StringVar(&langsRaw, "langs", "top50", "language list: top50 or comma-separated names")
	flag.StringVar(&outDir, "out", "cgo_harness/corpus_real", "output corpus directory")
	flag.StringVar(&workDir, "work-dir", "", "temporary clone work directory (default: temp dir)")
	flag.BoolVar(&keepWorkDir, "keep-work-dir", false, "keep work directory after command exits")
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

	resolvedProfilePath, languages, err := resolveLanguageList(profilePath, langsRaw)
	if err != nil {
		fatalf("resolve languages: %v", err)
	}
	if len(languages) == 0 {
		fatalf("no languages selected")
	}

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

		candidates, err := collectCandidates(repoDir, entry.Exts, maxBytes)
		if err != nil {
			manifest.Missing = append(manifest.Missing, lang)
			fmt.Fprintf(os.Stderr, "[warn] collect candidates failed for %q: %v\n", lang, err)
			continue
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
			sum := sha256.Sum256(content)
			outputName := fmt.Sprintf("%s__%s", sf.Bucket, safeName(filepath.Base(sf.RelPath)))
			outputPath := filepath.Join(langOutDir, outputName)
			if err := os.WriteFile(outputPath, content, 0o644); err != nil {
				fatalf("write %s: %v", outputPath, err)
			}
			manifest.Entries = append(manifest.Entries, corpusManifestEntry{
				Language:     lang,
				Bucket:       sf.Bucket,
				Bytes:        sf.Size,
				SHA256:       hex.EncodeToString(sum[:]),
				SourceRepo:   entry.RepoURL,
				SourceCommit: entry.Commit,
				SourcePath:   sf.RelPath,
				OutputPath:   outputPath,
			})
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

func resolveLanguageList(profilePath, langsRaw string) (string, []string, error) {
	if strings.TrimSpace(profilePath) != "" {
		p, err := resolvePath(profilePath, []string{profilePath})
		if err != nil {
			return "", nil, err
		}
		profile, err := loadProfile(p)
		if err != nil {
			return "", nil, err
		}
		return p, dedupe(profile.Languages), nil
	}
	value := strings.TrimSpace(langsRaw)
	switch value {
	case "", "top50", "top":
		listPath, err := resolveTop50Path()
		if err != nil {
			return "", nil, err
		}
		langs, err := loadTop50List(listPath)
		if err != nil {
			return "", nil, err
		}
		return "", dedupe(langs), nil
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
		return "", dedupe(out), nil
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

func collectCandidates(repoDir string, exts []string, maxBytes int) ([]corpusFile, error) {
	seen := map[string]struct{}{}
	out := make([]corpusFile, 0, 256)
	extSet := map[string]struct{}{}
	for _, ext := range exts {
		extSet[strings.ToLower(strings.TrimSpace(ext))] = struct{}{}
	}

	addFile := func(absPath string, d fs.DirEntry, requireKnownExt bool) {
		if d.IsDir() {
			return
		}
		if requireKnownExt && len(extSet) > 0 {
			ext := strings.ToLower(filepath.Ext(absPath))
			if _, ok := extSet[ext]; !ok {
				return
			}
		}
		rel, err := filepath.Rel(repoDir, absPath)
		if err != nil {
			return
		}
		if !looksCorpusCandidatePath(rel) {
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
		if size < defaultSmallMin || size > int64(maxBytes) {
			return
		}
		if !looksText(absPath) {
			return
		}
		seen[rel] = struct{}{}
		out = append(out, corpusFile{
			AbsPath: absPath,
			RelPath: rel,
			Size:    size,
		})
	}

	type priorityDir struct {
		path            string
		requireKnownExt bool
	}
	priorityDirs := []priorityDir{
		{path: "test/corpus", requireKnownExt: false},
		{path: "tests/corpus", requireKnownExt: false},
		{path: "corpus", requireKnownExt: false},
		{path: "test/fixtures", requireKnownExt: false},
		{path: "tests/fixtures", requireKnownExt: false},
		{path: "fixtures", requireKnownExt: false},
		{path: "examples", requireKnownExt: true},
		{path: "example", requireKnownExt: true},
		{path: "samples", requireKnownExt: true},
		{path: "spec", requireKnownExt: false},
	}
	for _, dir := range priorityDirs {
		root := filepath.Join(repoDir, filepath.FromSlash(dir.path))
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
	_ = filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
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
				return nil
			}
		} else {
			rel, err := filepath.Rel(repoDir, path)
			if err != nil {
				return nil
			}
			if !looksGenericSourcePath(rel) {
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

func looksCorpusCandidatePath(relPath string) bool {
	rel := strings.ToLower(filepath.ToSlash(relPath))
	base := strings.ToLower(filepath.Base(rel))
	if strings.HasPrefix(base, ".") {
		return false
	}
	switch base {
	case "readme", "readme.md", "readme.txt",
		"license", "license.md", "copying",
		"go.mod", "go.sum",
		"cargo.lock", "cargo.toml",
		"package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"pipfile.lock", "poetry.lock", "composer.lock",
		"gemfile.lock", "mix.lock":
		return false
	}
	if strings.HasPrefix(rel, ".github/") ||
		strings.Contains(rel, "/.github/") ||
		strings.HasPrefix(rel, "bindings/") ||
		strings.Contains(rel, "/bindings/") ||
		strings.Contains(rel, "/node_modules/") ||
		strings.Contains(rel, "/vendor/") ||
		strings.Contains(rel, "/dist/") ||
		strings.Contains(rel, "/build/") {
		return false
	}
	return true
}

func looksGenericSourcePath(relPath string) bool {
	if !looksCorpusCandidatePath(relPath) {
		return false
	}
	rel := strings.ToLower(filepath.ToSlash(relPath))
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

	pick := func(bucket string, pool []corpusFile, fallback []corpusFile) []selectedCorpusFile {
		if len(pool) == 0 {
			pool = fallback
		}
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
				return pool[i].RelPath < pool[j].RelPath
			})
		case "medium":
			target := int64((minMedium + minLarge) / 2)
			sort.Slice(pool, func(i, j int) bool {
				di := abs64(pool[i].Size - target)
				dj := abs64(pool[j].Size - target)
				if di != dj {
					return di < dj
				}
				return pool[i].RelPath < pool[j].RelPath
			})
		case "large":
			sort.Slice(pool, func(i, j int) bool {
				if pool[i].Size != pool[j].Size {
					return pool[i].Size > pool[j].Size
				}
				return pool[i].RelPath < pool[j].RelPath
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
		pick("small", small, all),
		pick("medium", medium, all),
		pick("large", large, all),
	} {
		for _, sf := range bucketPick {
			if _, ok := used[sf.RelPath]; ok {
				continue
			}
			used[sf.RelPath] = struct{}{}
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
			if _, ok := used[cf.RelPath]; ok {
				continue
			}
			used[cf.RelPath] = struct{}{}
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
