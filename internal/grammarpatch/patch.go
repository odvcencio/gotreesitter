// Package grammarpatch declares pinned upstream grammar overlays.
package grammarpatch

import "strings"

// Spec describes one checked-in grammar source overlay.
type Spec struct {
	File             string
	RegenerateParser bool
	// GeneratorVersion uses the harness-provided CLI instead of a grammar-owned npm lockfile.
	GeneratorVersion string
}

// RegenerateForHarness keeps mixed-revision oracle harnesses compatible.
func (s Spec) RegenerateForHarness(repoDir, subdir string) error {
	return Regenerate(repoDir, subdir, s)
}

var byLanguage = map[string]Spec{
	"make": {
		File:             "tree-sitter-make-function-variables.patch",
		RegenerateParser: true,
		GeneratorVersion: "0.24.7",
	},
	"typescript": {
		File:             "tree-sitter-typescript-import-type.patch",
		RegenerateParser: true,
	},
	"tsx": {
		File:             "tree-sitter-typescript-import-type.patch",
		RegenerateParser: true,
	},
	"yaml": {
		File: "tree-sitter-yaml-multiline-quoted-scalars.patch",
	},
}

// Lookup returns the overlay specification for a grammar.
func Lookup(name string) (Spec, bool) {
	spec, ok := byLanguage[strings.ToLower(strings.TrimSpace(name))]
	return spec, ok
}
