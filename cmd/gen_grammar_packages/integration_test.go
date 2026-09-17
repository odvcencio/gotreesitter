package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Set GTS_STANDALONE_LANGUAGES to one grammar, a comma-separated list, or all.
// Run this test in a bounded container. Each process loads one grammar.
func TestStandaloneGrammarPackages(t *testing.T) {
	names := strings.Split(os.Getenv("GTS_STANDALONE_LANGUAGES"), ",")
	if names[0] == "" {
		names = []string{"go", "json", "javascript", "python", "yaml"}
	}
	changeTestDirectory(t, filepath.Join("..", ".."))
	if names[0] == "all" {
		paths, err := filepath.Glob("grammars/grammar_blobs/*.bin")
		if err != nil {
			t.Fatal(err)
		}
		names = nil
		for _, path := range paths {
			names = append(names, strings.TrimSuffix(filepath.Base(path), ".bin"))
		}
	}
	dir := t.TempDir()
	run := func(t *testing.T, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	build := func(t *testing.T, name, imports, loader string) string {
		t.Helper()
		source := filepath.Join(dir, name+".go")
		if err := os.WriteFile(source, []byte(fmt.Sprintf(packageProbe, imports, loader)), 0644); err != nil {
			t.Fatal(err)
		}
		binary := filepath.Join(dir, name)
		run(t, "go", "build", "-o", binary, source)
		return binary
	}
	aggregate := build(t, "aggregate", `"github.com/odvcencio/gotreesitter/grammars"`, `grammars.DetectLanguageByName(os.Args[1]).Language()`)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path := "github.com/odvcencio/gotreesitter/grammars/" + name
			deps := run(t, "go", "list", "-deps", path)
			for _, dep := range strings.Fields(string(deps)) {
				if dep == "github.com/odvcencio/gotreesitter/grammars" {
					t.Fatal("standalone package imports the aggregate catalog")
				}
			}
			binary := build(t, name, fmt.Sprintf("selected %q", path), "selected.Language()")
			symbols := run(t, "go", "tool", "nm", binary)
			count := bytes.Count(symbols, []byte("/grammars/grammar_blobs.blob"))
			if count != 1 {
				t.Fatalf("binary retains %d grammar blob symbols, want 1", count)
			}
			// Compare every serializable language field before parsing mutates caches.
			got := run(t, binary, name)
			want := run(t, aggregate, name)
			if !bytes.Equal(got, want) {
				t.Fatalf("standalone output differs\ngot: %s\nwant: %s", got, want)
			}
			info, err := os.Stat(binary)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("standalone binary: %d bytes; one retained grammar blob", info.Size())
			// Keep disk use bounded during the complete catalog check.
			if err := os.Remove(binary); err != nil {
				t.Fatal(err)
			}
		})
	}
}

const packageProbe = `package main
import (
"crypto/sha256"
"encoding/json"
"fmt"
"os"
"reflect"
"github.com/odvcencio/gotreesitter"
grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
%s
)
func main() {
lang := %s
fields := make(map[string]any)
value := reflect.ValueOf(lang).Elem()
for i:=0; i<value.NumField(); i++ {
field := value.Type().Field(i)
if !field.IsExported() { continue }
if field.Name == "ExternalScanner" { fields[field.Name] = fmt.Sprintf("%%T", lang.ExternalScanner); continue }
fields[field.Name] = value.Field(i).Interface()
}
data, err := json.Marshal(fields)
if err != nil { panic(err) }
fmt.Printf("language=%%x\n", sha256.Sum256(data))
src := []byte("x\n")
parser := gotreesitter.NewParser(lang)
var tree *gotreesitter.Tree
if factory := grammarruntime.TokenSourceFactory(os.Args[1]); factory != nil {
tree, err = parser.ParseWithTokenSource(src, factory(src, lang))
} else { tree, err = parser.Parse(src) }
fmt.Printf("error=%%v\n", err)
if tree != nil {
defer tree.Release()
fmt.Printf("tree=%%x early=%%v\n", sha256.Sum256([]byte(tree.RootNode().SExpr(lang))), tree.ParseStoppedEarly())
}
}
`
