package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goSrc = "package main\n\nfunc add(a, b int) int { return a + b }\n\nfunc main() { _ = add(1, 2) }\n"

// runCLI runs the CLI in process and returns its exit code and output.
func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	c := &cli{strings.NewReader(stdin), &out, &errb}
	return c.run(args), out.String(), errb.String()
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"bogus"},
		{"parse"},
		{"parse", "-l", "nosuchlang", "-"},
		{"parse", "no/such/file.go"},
		{"parse", "-badflag"},
		{"query", "x.go"},
		{"test"},
	} {
		if code, _, _ := runCLI(t, "", args...); code != 2 {
			t.Errorf("run(%v) = %d, want 2", args, code)
		}
	}
	if code, out, _ := runCLI(t, "", "help"); code != 0 || !strings.Contains(out, "usage:") {
		t.Errorf("help = %d %q", code, out)
	}
}

func TestParse(t *testing.T) {
	code, out, _ := runCLI(t, "", "parse", writeFile(t, "a.go", goSrc))
	if code != 0 || !strings.Contains(out, "(source_file") || !strings.Contains(out, "function_declaration") {
		t.Fatalf("parse go = %d %q", code, out)
	}
	code, out, _ = runCLI(t, `{"a": [1, 2]}`, "parse", "-l", "json", "-")
	if code != 0 || !strings.Contains(out, "(document") {
		t.Fatalf("parse stdin json = %d %q", code, out)
	}
	code, out, _ = runCLI(t, "", "parse", "-json", writeFile(t, "a.go", goSrc))
	var got struct {
		Language string
		HasError bool `json:"has_error"`
		Sexp     string
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || got.Language != "go" || got.HasError || got.Sexp == "" {
		t.Fatalf("parse -json = %d %q (%v)", code, out, err)
	}
}

func TestParseSyntaxErrorExitsOne(t *testing.T) {
	code, _, errOut := runCLI(t, "", "parse", writeFile(t, "bad.go", "package main\n\nfunc ( {{{\n"))
	if code != 1 || !strings.Contains(errOut, "syntax error") {
		t.Fatalf("parse invalid = %d, stderr %q; want 1 and a syntax error", code, errOut)
	}
}

func TestQuery(t *testing.T) {
	f := writeFile(t, "a.go", goSrc)
	code, out, _ := runCLI(t, "", "query", "-q", "(function_declaration name: (identifier) @name)", f)
	if code != 0 || !strings.Contains(out, "@name") || !strings.Contains(out, "add") || !strings.Contains(out, "main") {
		t.Fatalf("query = %d %q", code, out)
	}
	if code, _, _ = runCLI(t, "", "query", "-q", "(nosuchnode) @x", f); code != 2 {
		t.Fatalf("bad query = %d, want 2", code)
	}
	code, out, _ = runCLI(t, "", "query", "-json", "-q", "(function_declaration) @fn", f)
	var matches []struct{ Captures []struct{ Name string } }
	if err := json.Unmarshal([]byte(out), &matches); err != nil || code != 0 || len(matches) != 2 {
		t.Fatalf("query -json = %d %q (%v)", code, out, err)
	}
}

func TestHighlight(t *testing.T) {
	code, out, _ := runCLI(t, "", "highlight", writeFile(t, "a.go", goSrc))
	if code != 0 || !strings.Contains(out, "@keyword") {
		t.Fatalf("highlight = %d %q", code, out)
	}
}

func TestTagsAndOutline(t *testing.T) {
	f := writeFile(t, "a.go", goSrc)
	code, out, _ := runCLI(t, "", "tags", f)
	if code != 0 || !strings.Contains(out, "definition.function\tadd") {
		t.Fatalf("tags = %d %q", code, out)
	}
	code, out, _ = runCLI(t, "", "outline", f)
	if code != 0 || !strings.Contains(out, "function add") {
		t.Fatalf("outline = %d %q", code, out)
	}
	code, out, _ = runCLI(t, "", "outline", "-json", f)
	if code != 0 || !json.Valid([]byte(out)) {
		t.Fatalf("outline -json = %d %q", code, out)
	}
}

func TestGrep(t *testing.T) {
	f := writeFile(t, "a.go", goSrc)
	code, out, _ := runCLI(t, "", "grep", "add($A, $B)", f)
	if code != 0 || !strings.Contains(out, "a.go:5:") {
		t.Fatalf("grep = %d %q", code, out)
	}
	code, out, _ = runCLI(t, "", "grep", "-r", "sum($A, $B)", "add($A, $B)", f)
	if code != 0 || !strings.Contains(out, "_ = sum(1, 2)") {
		t.Fatalf("grep -r = %d %q", code, out)
	}
	if code, _, _ = runCLI(t, "", "grep", "-r", "x", "add($A, $B)", f, f); code != 2 {
		t.Fatalf("grep -r on two files without -w = %d, want 2", code)
	}
	if code, _, _ = runCLI(t, "", "grep", "-w", "-r", "sum($A, $B)", "add($A, $B)", f); code != 0 {
		t.Fatalf("grep -w = %d", code)
	}
	if b, _ := os.ReadFile(f); !strings.Contains(string(b), "sum(1, 2)") {
		t.Fatalf("grep -w did not rewrite the file: %q", b)
	}
}

func TestLanguages(t *testing.T) {
	code, out, _ := runCLI(t, "", "languages")
	if code != 0 || !strings.Contains(out, "go\t.go") {
		t.Fatalf("languages = %d, output has no go row", code)
	}
}

func TestCorpusTest(t *testing.T) {
	// Lay the vendored CSS fixtures out as <root>/css/corpus/.
	src, err := os.ReadFile(filepath.Join("..", "..", "corpuscheck", "testdata", "upstream_corpus", "css", "declarations.txt"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "css", "corpus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "css", "corpus", "declarations.txt"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "report.json")
	code, out, _ := runCLI(t, "", "test", "--root", root, "--langs", "css", "--json", report)
	if code != 0 || !strings.Contains(out, "pass rate (strict): 100.00%") {
		t.Fatalf("test css = %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(report); !json.Valid(b) {
		t.Fatalf("report is not JSON: %q", b)
	}
	// With 'all', languages without a checkout are not attempted.
	if code, out, _ = runCLI(t, "", "test", "--root", root, "--langs", "all"); code != 0 || !strings.Contains(out, "css") {
		t.Fatalf("test all = %d\n%s", code, out)
	}
	// A named language with no checkout, or an empty list, is an input error.
	for _, langs := range []string{"css,json", "json", ","} {
		if code, _, _ = runCLI(t, "", "test", "--root", root, "--langs", langs); code != 2 {
			t.Fatalf("test --langs %q = %d, want 2", langs, code)
		}
	}
	// A wrong expectation is a failure, so the exit code is 1.
	bad := "==================\nx\n==================\n\n.a { color: red; }\n\n------------------\n\n(stylesheet (nothing))\n"
	if err := os.WriteFile(filepath.Join(root, "css", "corpus", "bad.txt"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ = runCLI(t, "", "test", "--root", root, "--langs", "css"); code != 1 {
		t.Fatalf("test with a mismatch = %d, want 1", code)
	}
}
