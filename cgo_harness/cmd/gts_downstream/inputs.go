//go:build cgo && treesitter_c_parity

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func grammarForShape(shape string) string {
	if strings.HasSuffix(shape, "-comments") {
		return strings.TrimSuffix(shape, "-comments")
	}
	switch shape {
	case "scala_report":
		return "scala"
	case "make-report":
		return "make"
	case "dart-report", "dart-report-class", "dart-report-single":
		return "dart"
	case "sh":
		return "bash"
	case "ps1":
		return "powershell"
	case "md":
		return "markdown"
	}
	return shape
}

func querySource(j job, e grammars.LangEntry) ([]byte, string) {
	if j.Query != "" {
		return []byte(j.Query), "authenticated-parent-query"
	}
	if j.Workflow == "index" {
		return []byte(grammars.ResolveTagsQuery(e)), "registry-tags"
	}
	if strings.TrimSpace(e.HighlightQuery) != "" {
		return []byte(e.HighlightQuery), "registry-highlight"
	}
	// Grammars without a highlight file still exercise a real query over the
	// whole tree. Record the fallback so it cannot masquerade as a shipped query.
	return []byte("(_) @variable"), "generic-highlight-no-registry-query"
}

func git(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-c", "safe.directory=*", "-C", root}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, b)
	}
	return b, nil
}

func plan(o options) ([]job, error) {
	if o.file != "" && o.workflow != "index" {
		return nil, errors.New("-file requires -workflow index")
	}
	if o.workflow == "fixtures" {
		return fixturePlan(o)
	}
	if o.workflow != "editor" && o.workflow != "index" {
		return nil, errors.New("unknown workflow")
	}
	lock, err := readCorpusLock(o)
	if err != nil {
		return nil, err
	}
	var corpus struct {
		Entries []struct {
			Language string `json:"language"`
			Role     string `json:"role"`
			Path     string `json:"path"`
			SHA256   string `json:"sha256"`
			Commit   string `json:"commit"`
		} `json:"entries"`
	}
	if o.workflow == "editor" {
		if err := readJSON(filepath.Join(o.root, "internal/benchfixtures/real_corpus.json"), &corpus); err != nil {
			return nil, err
		}
	}
	var jobs []job
	for _, lang := range downstreamLanguages {
		if o.language != "" && o.language != lang {
			continue
		}
		pin, ok := lock[lang]
		if !ok {
			return nil, fmt.Errorf("corpus lock missing %s", lang)
		}
		dir := filepath.Join(o.corpus, lang)
		head, err := git(dir, "rev-parse", "HEAD")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(head)) != pin.commit {
			return nil, fmt.Errorf("%s checkout is not corpus commit %s", lang, pin.commit)
		}
		if _, err := git(dir, "diff", "--quiet", "HEAD", "--"); err != nil {
			return nil, fmt.Errorf("%s tracked corpus differs from pinned commit: %w", lang, err)
		}
		j := job{Workflow: o.workflow, Language: lang, Commit: pin.commit}
		if o.workflow == "editor" {
			for _, f := range corpus.Entries {
				if f.Language == lang && f.Role == "largest" {
					if f.Commit != pin.commit {
						return nil, fmt.Errorf("%s large-file manifest and corpus lock differ", lang)
					}
					j.Path = f.Path
					j.SourceHash = f.SHA256
					break
				}
			}
			if j.Path == "" {
				return nil, fmt.Errorf("no pinned large file for %s", lang)
			}
			j.Files = []string{j.Path}
		} else {
			paths, err := git(dir, "ls-files", "-z")
			if err != nil {
				return nil, err
			}
			for _, p := range strings.Split(string(paths), "\x00") {
				for _, ext := range pin.extensions {
					if p != "" && strings.HasSuffix(p, ext) {
						j.Files = append(j.Files, p)
						break
					}
				}
			}
			sort.Strings(j.Files)
			if o.file != "" {
				wanted := filepath.ToSlash(filepath.Clean(o.file))
				found := false
				for _, path := range j.Files {
					if path == wanted {
						found = true
						break
					}
				}
				if !found {
					return nil, fmt.Errorf("%s is not a tracked %s source file", wanted, lang)
				}
				j.Files = []string{wanted}
				j.Path = wanted
			}
			if len(j.Files) == 0 {
				return nil, fmt.Errorf("empty indexing checkout for %s", lang)
			}
		}
		jobs = append(jobs, j)
	}
	if len(jobs) == 0 {
		return nil, errors.New("no matching downstream jobs")
	}
	return jobs, nil
}

type corpusPin struct {
	commit     string
	extensions []string
}

func readCorpusLock(o options) (map[string]corpusPin, error) {
	var data []byte
	var err error
	if o.lock != "" {
		data, err = os.ReadFile(o.lock)
	} else {
		url := os.Getenv("GTS_CORPUS_LOCK_URL")
		if url == "" {
			return nil, errors.New("set GTS_CORPUS_LOCK_URL or -corpus-lock; the lock stays outside the repository")
		}
		client := http.Client{Timeout: 30 * time.Second}
		response, e := client.Get(url)
		if e != nil {
			return nil, e
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("corpus lock HTTP %d", response.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	}
	if err != nil {
		return nil, err
	}
	if sha(data) != corpusLockDigest {
		return nil, fmt.Errorf("corpus lock digest %s, want %s", sha(data), corpusLockDigest)
	}
	pins := map[string]corpusPin{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pins[fields[0]] = corpusPin{commit: fields[2], extensions: strings.Split(fields[4], ",")}
	}
	return pins, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func fixturePlan(o options) ([]job, error) {
	var manifest struct {
		Entries []struct {
			Language    string `json:"language"`
			TargetBytes int    `json:"target_bytes"`
			SourceBytes int    `json:"source_bytes"`
			SHA256      string `json:"sha256"`
		} `json:"entries"`
	}
	if err := readJSON(filepath.Join(o.root, "internal/benchfixtures/generated.json"), &manifest); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var jobs []job
	for _, f := range manifest.Entries {
		seen[f.Language] = true
		lang := grammarForShape(f.Language)
		if o.language != "" && lang != o.language || o.shape != "" && f.Language != o.shape || o.size != 0 && f.TargetBytes != o.size {
			continue
		}
		src, _, err := benchfixtures.GeneratedSource(f.Language, f.TargetBytes)
		if err != nil {
			return nil, err
		}
		if len(src) != f.SourceBytes || sha(src) != f.SHA256 {
			return nil, fmt.Errorf("fixture %s/%d differs from generated.json", f.Language, f.TargetBytes)
		}
		jobs = append(jobs, job{Workflow: "fixtures", Language: lang, Shape: f.Language, TargetBytes: f.TargetBytes, SourceHash: f.SHA256})
	}
	for _, shape := range benchfixtures.GeneratedLanguages() {
		if !seen[shape] {
			return nil, fmt.Errorf("shape %s has no pinned sizes", shape)
		}
	}
	if len(jobs) == 0 {
		return nil, errors.New("no matching pinned fixtures")
	}
	return jobs, nil
}

func inputDigest(o options, j job) (string, int64, error) {
	h := sha256.New()
	var size int64
	if j.Workflow == "fixtures" {
		b, _, err := benchfixtures.GeneratedSource(j.Shape, j.TargetBytes)
		if err != nil {
			return "", 0, err
		}
		return sha(b), int64(len(b)), nil
	}
	for _, path := range j.Files {
		b, err := os.ReadFile(filepath.Join(o.corpus, j.Language, path))
		if err != nil {
			return "", 0, err
		}
		if j.SourceHash != "" && sha(b) != j.SourceHash {
			return "", 0, fmt.Errorf("%s differs from pinned source digest", path)
		}
		size += int64(len(b))
		fmt.Fprintf(h, "%s\x00%d\x00%s\n", path, len(b), sha(b))
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func source(o options, j job, path string) ([]byte, error) {
	if j.Workflow == "fixtures" {
		b, _, err := benchfixtures.GeneratedSource(j.Shape, j.TargetBytes)
		return b, err
	}
	return os.ReadFile(filepath.Join(o.corpus, j.Language, path))
}
