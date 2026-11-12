package lexbuild

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoLanguageNames: no file of the package (tests excepted) names a
// grammar. "json" is allowed only as the name of the grammar.json file format
// (encoding/json, the grammar.json file name, struct tags, and the upper-case
// word JSON).
func TestNoLanguageNames(t *testing.T) {
	names := []string{"ini", "regex", "yaml", "swift", "python", "javascript", "typescript", "toml", "markdown", "bash", "ruby", "scss", "nginx"}
	allowedJSON := regexp.MustCompile(`grammar\.json|encoding/json|json\.|json:"|json\b[ :]|json\)`)
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
		text := string(b)
		for _, name := range names {
			if regexp.MustCompile(`(?i)\b` + name + `\b`).MatchString(text) {
				t.Errorf("%s names %s", f, name)
			}
		}
		rest := allowedJSON.ReplaceAllString(text, "")
		if strings.Contains(rest, "json") {
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
