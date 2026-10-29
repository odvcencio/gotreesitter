package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	ForestCorpusManifestSchema = "forest-corpus-manifest-v2"
	ForestAuditResultSchema    = "forest-audit-result-v2"
	ForestAuditReportSchema    = "forest-audit-report-v4"
)

type ForestCorpusManifest struct {
	Schema               string                     `json:"schema"`
	GotreesitterRevision string                     `json:"gotreesitter_revision"`
	CorpusLock           ForestCorpusManifestLock   `json:"corpus_lock"`
	Selection            ForestCorpusSelection      `json:"selection"`
	Files                []ForestCorpusManifestFile `json:"files"`
}

type ForestCorpusManifestLock struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ForestCorpusSelection struct {
	Order        string   `json:"order"`
	MaxFiles     int      `json:"max_files"`
	MinFileBytes int64    `json:"min_file_bytes"`
	MaxFileBytes int64    `json:"max_file_bytes"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
}

type ForestCorpusManifestFile struct {
	Language   string `json:"language"`
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
}

type ForestCorpusMaterializeOptions struct {
	GotreesitterRevision string
	CorpusLockPath       string
	CorpusRoot           string
	Languages            []string
	RegistryExtensions   map[string][]string
	Selection            ForestCorpusSelection
}

type forestCorpusLockEntry struct {
	Language   string
	Repository string
	Revision   string
	Subdir     string
	Matchers   []string
}

func forestRequireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("forest JSON contains trailing value")
		}
		return fmt.Errorf("decode trailing forest JSON data: %w", err)
	}
	return nil
}

func forestManifestLanguage(language string) error {
	if strings.TrimSpace(language) != language || language == "" || language == "." || language == ".." || strings.ContainsAny(language, `/\`) {
		return fmt.Errorf("invalid language %q", language)
	}
	return nil
}

func forestManifestPath(root, kind, rel string) (string, error) {
	if strings.TrimSpace(rel) != rel || rel == "" || strings.ContainsRune(rel, '\\') || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid %s path %q", kind, rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != rel && rel != "." {
		return "", fmt.Errorf("invalid %s path %q", kind, rel)
	}
	filePath := filepath.Join(root, clean)
	relToRoot, err := filepath.Rel(root, filePath)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s path %q escapes root", kind, rel)
	}
	return filePath, nil
}

func forestManifestDigest(kind, raw string) (string, error) {
	digest := strings.ToLower(strings.TrimSpace(raw))
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("invalid %s sha256 %q", kind, raw)
	}
	return digest, nil
}

func forestManifestRevision(raw string) error {
	revision := strings.ToLower(strings.TrimSpace(raw))
	if (len(revision) != 40 && len(revision) != 64) || revision != raw {
		return fmt.Errorf("invalid revision %q", raw)
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return fmt.Errorf("invalid revision %q", raw)
	}
	return nil
}

func forestVerifyManifestFile(filePath string, wantBytes int64, wantDigest, label string) error {
	info, err := os.Lstat(filePath)
	if err != nil {
		return fmt.Errorf("read authenticated %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("authenticated %s is not a regular file", label)
	}
	if info.Size() != wantBytes {
		return fmt.Errorf("authenticated %s byte count %d, want %d", label, info.Size(), wantBytes)
	}
	digest, err := forestFileSHA256(filePath)
	if err != nil {
		return err
	}
	if digest != wantDigest {
		return fmt.Errorf("authenticated %s sha256 %s, want %s", label, digest, wantDigest)
	}
	return nil
}

func forestFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func validateForestCorpusManifestMetadata(manifest ForestCorpusManifest) error {
	if err := validateForestCorpusManifestHeader(manifest); err != nil {
		return err
	}
	return validateForestCorpusManifestFileMetadata(manifest.Files)
}

func validateForestCorpusManifestHeader(manifest ForestCorpusManifest) error {
	if manifest.Schema != ForestCorpusManifestSchema {
		return fmt.Errorf("forest corpus manifest schema %q, want %q", manifest.Schema, ForestCorpusManifestSchema)
	}
	if err := forestManifestRevision(manifest.GotreesitterRevision); err != nil {
		return err
	}
	if _, err := forestManifestDigest("corpus lock", manifest.CorpusLock.SHA256); err != nil {
		return err
	}
	normalizedSelection, err := normalizeForestCorpusSelection(manifest.Selection)
	if err != nil {
		return err
	}
	if normalizedSelection.Order != manifest.Selection.Order || strings.Join(normalizedSelection.ExcludePaths, "\x00") != strings.Join(manifest.Selection.ExcludePaths, "\x00") {
		return fmt.Errorf("forest corpus manifest selection is not canonical")
	}
	if len(manifest.Files) == 0 {
		return fmt.Errorf("forest corpus manifest has no files")
	}
	return nil
}

func validateForestCorpusManifestFileMetadata(files []ForestCorpusManifestFile) error {
	seen := make(map[string]bool, len(files))
	previous := ""
	for i, file := range files {
		if err := validateForestCorpusManifestFile(file, i); err != nil {
			return err
		}
		key := file.Language + "\x00" + file.Path
		if seen[key] {
			return fmt.Errorf("forest corpus manifest file %d duplicates %s/%s", i, file.Language, file.Path)
		}
		if previous != "" && key < previous {
			return fmt.Errorf("forest corpus manifest files are not sorted")
		}
		seen[key] = true
		previous = key
	}
	return nil
}

func validateForestCorpusManifestFile(file ForestCorpusManifestFile, index int) error {
	if err := forestManifestLanguage(file.Language); err != nil {
		return fmt.Errorf("forest corpus manifest file %d: %w", index, err)
	}
	if strings.TrimSpace(file.Repository) == "" {
		return fmt.Errorf("forest corpus manifest file %d has empty repository", index)
	}
	if err := forestManifestRevision(file.Revision); err != nil {
		return fmt.Errorf("forest corpus manifest file %d: %w", index, err)
	}
	if _, err := forestManifestPath("/", "corpus file", file.Path); err != nil {
		return fmt.Errorf("forest corpus manifest file %d: %w", index, err)
	}
	if file.Bytes < 0 {
		return fmt.Errorf("forest corpus manifest file %d has negative byte count", index)
	}
	if _, err := forestManifestDigest("corpus file", file.SHA256); err != nil {
		return fmt.Errorf("forest corpus manifest file %d: %w", index, err)
	}
	return nil
}

func readForestAuditJSON(filePath string, value any) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return decodeForestAuditJSON(data, value)
}

func decodeForestAuditJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return forestRequireJSONEOF(decoder)
}

func writeForestAuditJSON(filePath string, value any) error {
	return writeForestAuditJSONMode(filePath, value, false)
}

func writeForestAuditJSONExclusive(filePath string, value any) error {
	return writeForestAuditJSONMode(filePath, value, true)
}

func writeForestAuditJSONMode(filePath string, value any, exclusive bool) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if exclusive {
		destination, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		if _, err := destination.Write(data); err != nil {
			destination.Close()
			_ = os.Remove(filePath)
			return err
		}
		if err := destination.Sync(); err != nil {
			destination.Close()
			_ = os.Remove(filePath)
			return err
		}
		if err := destination.Close(); err != nil {
			_ = os.Remove(filePath)
			return err
		}
		return nil
	}
	temp, err := os.CreateTemp(dir, ".forest-audit-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, filePath)
}
