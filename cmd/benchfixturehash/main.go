// Command benchfixturehash generates or verifies the pinned fixture digests.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type generatedRow struct {
	Language      string `json:"language"`
	TargetBytes   int    `json:"target_bytes"`
	SourceBytes   int    `json:"source_bytes"`
	SHA256        string `json:"sha256"`
	SessionSHA256 string `json:"session_sha256,omitempty"`
}

type generatedManifest struct {
	Schema  string         `json:"schema"`
	Seed    uint32         `json:"seed"`
	Steps   int            `json:"steps"`
	Entries []generatedRow `json:"entries"`
}

type realManifest struct {
	Entries []struct {
		Language      string `json:"language"`
		Role          string `json:"role"`
		SHA256        string `json:"sha256"`
		SessionSHA256 string `json:"session_sha256,omitempty"`
		CommittedPath string `json:"committed_path"`
		SourceKey     string `json:"source_key"`
		SourcePath    string `json:"path"`
	} `json:"entries"`
}

func main() {
	write := flag.Bool("write", false, "write the reviewed digest files")
	externalRoot := flag.String("external-root", "", "verified checkout root for manifest-only source samples")
	flag.Parse()
	if err := run(*write, *externalRoot); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(write bool, externalRoot string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "internal", "benchfixtures")
	manifest := generatedManifest{Schema: "benchfixture-generated-v1", Seed: benchfixtures.EditSessionSeed, Steps: benchfixtures.EditSessionSteps}
	for _, language := range benchfixtures.GeneratedLanguages() {
		for _, size := range []int{32 << 10, 137 << 10, 1 << 20} {
			source, _, err := benchfixtures.GeneratedSource(language, size)
			if err != nil {
				return err
			}
			row := generatedRow{Language: language, TargetBytes: size, SourceBytes: len(source), SHA256: fmt.Sprintf("%x", sha256.Sum256(source))}
			if size == 137<<10 {
				row.SessionSHA256 = benchfixtures.EditingSessionSHA256(source)
			}
			manifest.Entries = append(manifest.Entries, row)
		}
	}
	if err := checkJSON(filepath.Join(path, "generated.json"), manifest, write); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(path, "real_corpus.json"))
	if err != nil {
		return err
	}
	var real realManifest
	if err := json.Unmarshal(data, &real); err != nil {
		return err
	}
	count := 0
	seen := map[string]bool{}
	for i := range real.Entries {
		row := &real.Entries[i]
		if row.Role != "sample" {
			continue
		}
		seen[row.Language] = true
		var sourcePath string
		if row.CommittedPath != "" {
			sourcePath = filepath.Join(path, row.CommittedPath)
			count++
		} else if externalRoot != "" {
			sourcePath = filepath.Join(externalRoot, row.SourceKey, row.SourcePath)
		} else if write {
			return fmt.Errorf("--write needs --external-root to pin %s", row.Language)
		} else {
			if decoded, err := hex.DecodeString(row.SessionSHA256); err != nil || len(decoded) != 32 {
				return fmt.Errorf("%s has no pinned editing session", row.Language)
			}
			continue
		}
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(source)) != row.SHA256 {
			return fmt.Errorf("%s sample digest changed", row.Language)
		}
		row.SessionSHA256 = benchfixtures.EditingSessionSHA256(source)
	}
	if count != 204 || len(seen) != 206 {
		return fmt.Errorf("sample coverage=%d committed, %d total; want 204 and 206", count, len(seen))
	}
	if write {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		entries := raw["entries"].([]any)
		for i, row := range real.Entries {
			if row.Role == "sample" {
				entries[i].(map[string]any)["session_sha256"] = row.SessionSHA256
			}
		}
		return checkJSON(filepath.Join(path, "real_corpus.json"), raw, true)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	for _, item := range stored["entries"].([]any) {
		row := item.(map[string]any)
		if row["role"] != "sample" || (row["committed_path"] == "" && externalRoot == "") {
			continue
		}
		language := row["language"].(string)
		for _, calculated := range real.Entries {
			if calculated.Language == language && calculated.Role == "sample" && row["session_sha256"] != calculated.SessionSHA256 {
				return fmt.Errorf("%s editing session digest changed", language)
			}
		}
	}
	return nil
}

func checkJSON(path string, value any, write bool) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if write {
		return os.WriteFile(path, data, 0o644)
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(old) != string(data) {
		return fmt.Errorf("%s changed; regenerate with go run ./cmd/benchfixturehash --write and review the diff", path)
	}
	return nil
}
