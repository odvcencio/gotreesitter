package grammarpatch

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Regenerate rebuilds parser.c after an overlay using its pinned generator.
// Grammars with an npm lockfile retain their own installer; Make uses the CLI
// version already pinned by the parity Docker image because it has no lockfile.
func Regenerate(repoDir, subdir string, spec Spec) error {
	generator := filepath.Join(repoDir, "node_modules", ".bin", "tree-sitter")
	if spec.GeneratorVersion == "" {
		// npm lifecycle scripts install the CLI executable; keep them enabled.
		install := exec.Command("npm", "ci")
		install.Dir = repoDir
		if out, err := install.CombinedOutput(); err != nil {
			return fmt.Errorf("npm ci: %v: %s", err, strings.TrimSpace(string(out)))
		}
	} else {
		generator = "tree-sitter"
		out, err := exec.Command(generator, "--version").CombinedOutput()
		if err != nil {
			return fmt.Errorf("tree-sitter %s required to regenerate patched grammar: %w: %s", spec.GeneratorVersion, err, strings.TrimSpace(string(out)))
		}
		fields := strings.Fields(string(out))
		if len(fields) < 2 || fields[0] != "tree-sitter" || fields[1] != spec.GeneratorVersion {
			return fmt.Errorf("tree-sitter %s required to regenerate patched grammar; found %q", spec.GeneratorVersion, strings.TrimSpace(string(out)))
		}
	}
	generate := exec.Command(generator, "generate")
	generate.Dir = filepath.Join(repoDir, filepath.Dir(subdir))
	if out, err := generate.CombinedOutput(); err != nil {
		return fmt.Errorf("tree-sitter generate: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
