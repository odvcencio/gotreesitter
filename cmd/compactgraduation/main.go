// Command compactgraduation checks the measured matrix and default routes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/odvcencio/gotreesitter/internal/graduation"
)

func main() {
	path := flag.String("matrix", "", "external measured graduation matrix (required unless checking empty defaults)")
	root := flag.String("root", ".", "repository checkout authenticated by the receipt")
	write := flag.Bool("write", false, "derive decisions in the external receipt; never change runtime defaults")
	flag.Parse()
	if err := run(*root, *path, *write); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, path string, write bool) error {
	if path == "" {
		if write {
			return fmt.Errorf("--matrix is required to derive decisions")
		}
		if err := graduation.VerifyDefaults(nil); err != nil {
			return err
		}
		fmt.Println("compact graduation defaults: zero enabled; external receipt required for any promotion")
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	matrix, err := graduation.Read(file)
	file.Close()
	if err != nil {
		return err
	}
	if write {
		for i := range matrix.Languages {
			language := &matrix.Languages[i]
			language.Blockers = matrix.Blockers(*language)
			language.Graduated = len(language.Blockers) == 0
		}
	}
	if err := matrix.Validate(); err != nil {
		return err
	}
	required := map[string]bool{}
	for _, grammar := range []string{"go", "java", "javascript", "typescript", "python", "rust", "c", "cpp", "c_sharp", "ruby", "php", "bash"} {
		required[grammar] = true
	}
	for _, language := range matrix.Languages {
		if !required[language.Grammar] {
			return fmt.Errorf("unexpected graduation grammar %s", language.Grammar)
		}
		delete(required, language.Grammar)
	}
	if len(required) != 0 {
		return fmt.Errorf("graduation matrix must cover all twelve requested languages")
	}
	if err := matrix.VerifySources(root); err != nil {
		return err
	}
	names := matrix.Graduated()
	if err := graduation.VerifyDefaults(matrix); err != nil {
		return err
	}
	if write {
		data, err := json.MarshalIndent(matrix, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			return err
		}
	}
	fmt.Printf("compact graduation matrix: %d languages, %d graduated\n", len(matrix.Languages), len(names))
	return nil
}
