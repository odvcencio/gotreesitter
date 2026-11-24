package tablebuild

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoLanguageNames: no file of the package (tests excepted) names a
// grammar. "json" is allowed only as the name of the grammar.json file format.
func TestNoLanguageNames(t *testing.T) {
	names := []string{"ini", "regex", "yaml", "swift", "python", "javascript", "typescript", "toml", "markdown", "bash", "ruby", "scss", "nginx"}
	// "Go" in a comment names the language this package is written in; in a
	// string literal it would name the grammar.
	literalOnly := []string{"go", "golang"}
	allowedJSON := regexp.MustCompile(`grammar\.json|encoding/json|json\.|json:"`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		n++
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// Go source names its own keywords and packages; only comments and
		// string literals are searched for grammar names.
		text := strings.Join(regexp.MustCompile(`//[^\n]*|"(?:[^"\\\n]|\\.)*"`).FindAllString(string(b), -1), "\n")
		literals := strings.Join(regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`).FindAllString(string(b), -1), "\n")
		for _, name := range names {
			if regexp.MustCompile(`(?i)\b` + name + `\b`).MatchString(text) {
				t.Errorf("%s names %s", f, name)
			}
		}
		for _, name := range literalOnly {
			if regexp.MustCompile(`(?i)\b` + name + `\b`).MatchString(literals) {
				t.Errorf("%s names %s in a string", f, name)
			}
		}
		if strings.Contains(allowedJSON.ReplaceAllString(text, ""), "json") {
			t.Errorf("%s names json outside the file format", f)
		}
	}
	if n == 0 {
		t.Fatal("no source files found")
	}
}

// TestNoEnvironmentRead: the package reads no environment variable (tests excepted).
func TestNoEnvironmentRead(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, bad := range []string{"os.Getenv", "os.LookupEnv", "os.Environ", `"os"`} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s uses %s", f, bad)
			}
		}
	}
}
