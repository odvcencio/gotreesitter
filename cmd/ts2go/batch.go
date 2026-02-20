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
	Subdir  string // subdirectory containing parser.c (e.g. "src")
	// Optional comma-separated file extensions from manifest column 4.
	Extensions []string
}

// ParseManifest reads a manifest file with lines of format:
//
//	name repo_url [subdir] [ext1,ext2,...]
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
		if len(fields) >= 3 {
			entry.Subdir = fields[2]
		}
		if len(fields) >= 4 && strings.TrimSpace(fields[3]) != "" {
			for _, ext := range strings.Split(fields[3], ",") {
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
