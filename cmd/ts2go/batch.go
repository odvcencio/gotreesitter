package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ManifestEntry represents one language in the batch manifest.
type ManifestEntry struct {
	Name    string // language name (e.g. "python")
	RepoURL string // git URL for tree-sitter grammar
	Commit  string // optional pinned commit hash from lock files
	Subdir  string // subdirectory containing parser.c (e.g. "src")
	// Optional comma-separated file extensions from manifest column 4.
	Extensions []string
}

// ParseManifest reads a manifest file with lines of format:
//
//	name repo_url [commit] [subdir] [ext1,ext2,...]
//
// Lines starting with # are comments. Empty lines are skipped.
func ParseManifest(path string) ([]ManifestEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []ManifestEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("invalid manifest line: %q", line)
		}
		entry := ManifestEntry{
			Name:    fields[0],
			RepoURL: fields[1],
			Subdir:  "src",
		}

		nextField := 2
		if len(fields) > nextField && looksLikeCommitHash(fields[nextField]) {
			entry.Commit = fields[nextField]
			nextField++
		}

		if len(fields) > nextField {
			entry.Subdir = fields[nextField]
			nextField++
		}
		if len(fields) > nextField && strings.TrimSpace(fields[nextField]) != "" {
			for _, ext := range strings.Split(fields[nextField], ",") {
				ext = strings.TrimSpace(ext)
				if ext != "" {
					entry.Extensions = append(entry.Extensions, ext)
				}
			}
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
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
