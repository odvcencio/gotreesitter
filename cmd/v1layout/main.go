// Command v1layout enforces the L0 root, API, and test-move guardrails.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

const rootBudget = 756

// The root has eleven build-tag atoms. Snapshot the default build and each
// atom separately so a tag-only exported declaration cannot disappear.
var apiTags = []string{
	"default",
	"gts_derivation_set_census",
	"gts_eof_history_census",
	"gts_eof_recovery_admission_contract",
	"gts_eof_recovery_shadow",
	"gts_merge_census",
	"gts_no_parsercorephase0",
	"gts_parsercorephase0",
	"gts_recovery_telemetry",
	"gts_workcount",
	"perf",
	"race",
}

func main() {
	write := flag.Bool("write", false, "refresh checked-in root and API snapshots")
	base := flag.String("base", "", "git base for move inventory check")
	flag.Parse()
	if err := run(*write, *base); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(write bool, base string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		return err
	}
	for i := range files {
		files[i] = filepath.Base(files[i])
	}
	sort.Strings(files)
	if err := checkTagAtoms(root, files); err != nil {
		return err
	}
	if len(files) > rootBudget {
		return fmt.Errorf("root has %d Go files; L0 budget is %d", len(files), rootBudget)
	}
	if err := snapshot(filepath.Join(root, "cmd/v1layout/root_files.txt"), files, write); err != nil {
		return fmt.Errorf("root file allowlist: %w", err)
	}
	if base != "" && !write {
		if err := checkRootAllowlistGrowth(root, base, files); err != nil {
			return err
		}
	}
	defaultAPI, err := apiSurface(root, "default")
	if err != nil {
		return fmt.Errorf("API tag set default: %w", err)
	}
	if err := snapshot(filepath.Join(root, "cmd/v1layout/api/default.txt"), defaultAPI, write); err != nil {
		return fmt.Errorf("API tag set default: %w", err)
	}
	for _, tag := range apiTags[1:] {
		api, err := apiSurface(root, tag)
		if err != nil {
			return fmt.Errorf("API tag set %s: %w", tag, err)
		}
		added, removed := difference(defaultAPI, api)
		var delta []string
		for _, line := range added {
			delta = append(delta, "+ "+line)
		}
		for _, line := range removed {
			delta = append(delta, "- "+line)
		}
		sort.Strings(delta)
		if err := snapshot(filepath.Join(root, "cmd/v1layout/api", tag+".txt"), delta, write); err != nil {
			return fmt.Errorf("API tag set %s: %w", tag, err)
		}
	}
	moves := 0
	if base != "" && !write {
		moves, err = checkMoveInventory(root, base)
		if err != nil {
			return err
		}
	}
	fmt.Printf("L0 guardrails: %d/%d root Go files, %d API tag sets, %d test-file moves checked\n", len(files), rootBudget, len(apiTags), moves)
	return nil
}

func checkTagAtoms(root string, files []string) error {
	found := map[string]bool{}
	var collect func(constraint.Expr)
	collect = func(expr constraint.Expr) {
		switch x := expr.(type) {
		case *constraint.TagExpr:
			found[x.Tag] = true
		case *constraint.NotExpr:
			collect(x.X)
		case *constraint.AndExpr:
			collect(x.X)
			collect(x.Y)
		case *constraint.OrExpr:
			collect(x.X)
			collect(x.Y)
		}
	}
	for _, name := range files {
		file, err := os.Open(filepath.Join(root, name))
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "package ") {
				break
			}
			if strings.HasPrefix(line, "//go:build ") {
				expr, err := constraint.Parse(line)
				if err != nil {
					file.Close()
					return fmt.Errorf("%s: %w", name, err)
				}
				collect(expr)
			}
		}
		err = scanner.Err()
		file.Close()
		if err != nil {
			return err
		}
	}
	known := map[string]bool{}
	for _, tag := range apiTags[1:] {
		known[tag] = true
	}
	for tag := range found {
		if !known[tag] {
			return fmt.Errorf("root build-tag atom %q has no API snapshot", tag)
		}
	}
	for tag := range known {
		if !found[tag] {
			return fmt.Errorf("API snapshot tag %q has no root build constraint", tag)
		}
	}
	return nil
}

func checkRootAllowlistGrowth(root, base string, files []string) error {
	cmd := exec.Command("git", "show", base+":cmd/v1layout/root_files.txt")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		// L0 introduces the first baseline.
		return nil
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields(string(output)) {
		allowed[name] = true
	}
	for _, name := range files {
		if !allowed[name] {
			return fmt.Errorf("root file %q was not present in the base allowlist; root files may only shrink", name)
		}
	}
	return nil
}

func snapshot(path string, actual []string, write bool) error {
	data := []byte(strings.Join(actual, "\n") + "\n")
	if write {
		return os.WriteFile(path, data, 0644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(want, data) {
		old := strings.Split(strings.TrimSpace(string(want)), "\n")
		added, removed := difference(old, actual)
		return fmt.Errorf("%s differs from checked-in snapshot; added %v, removed %v (refresh with go run ./cmd/v1layout -write after an intentional change)", path, first(added, 5), first(removed, 5))
	}
	return nil
}

func difference(old, now []string) ([]string, []string) {
	a, b := map[string]bool{}, map[string]bool{}
	for _, s := range old {
		a[s] = true
	}
	for _, s := range now {
		b[s] = true
	}
	var added, removed []string
	for s := range b {
		if !a[s] {
			added = append(added, s)
		}
	}
	for s := range a {
		if !b[s] {
			removed = append(removed, s)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func first(items []string, n int) []string {
	if len(items) > n {
		return items[:n]
	}
	return items
}

func printNode(fset *token.FileSet, node any) string {
	var out bytes.Buffer
	if err := format.Node(&out, fset, node); err != nil {
		panic(err)
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func apiSurface(root, tag string) ([]string, error) {
	args := []string{"list", "-json"}
	if tag != "default" {
		args = append(args, "-tags="+tag)
	}
	args = append(args, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, output)
	}
	var listed struct{ GoFiles, CgoFiles []string }
	if err := json.Unmarshal(output, &listed); err != nil {
		return nil, err
	}
	var surface []string
	fset := token.NewFileSet()
	for _, name := range append(listed.GoFiles, listed.CgoFiles...) {
		file, err := parser.ParseFile(fset, filepath.Join(root, name), nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				receiver := ""
				if d.Recv != nil && len(d.Recv.List) > 0 {
					receiver = printNode(fset, d.Recv.List[0].Type) + "."
				}
				surface = append(surface, "func "+receiver+d.Name.Name+strings.TrimPrefix(printNode(fset, d.Type), "func"))
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							assignment := " "
							if s.Assign.IsValid() {
								assignment = " = "
							}
							surface = append(surface, "type "+s.Name.Name+assignment+printNode(fset, s.Type))
						}
					case *ast.ValueSpec:
						for i, name := range s.Names {
							if !name.IsExported() {
								continue
							}
							line := d.Tok.String() + " " + name.Name
							if s.Type != nil {
								line += " " + printNode(fset, s.Type)
							}
							if i < len(s.Values) {
								line += " = " + printNode(fset, s.Values[i])
							}
							surface = append(surface, line)
						}
					}
				}
			}
		}
	}
	sort.Strings(surface)
	return surface, nil
}

var testDecl = regexp.MustCompile(`^(Test|Benchmark|Fuzz|Example)[A-Za-z0-9_]*$`)

func testNames(data []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", data, 0)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && testDecl.MatchString(fn.Name.Name) {
			names = append(names, fn.Name.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func checkMoveInventory(root, base string) (int, error) {
	cmd := exec.Command("git", "diff", "--name-status", "-M", base, "HEAD", "--", "*.go")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git move inventory: %w", err)
	}
	moves := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || !strings.HasPrefix(fields[0], "R") || !strings.HasSuffix(fields[1], "_test.go") || !strings.HasSuffix(fields[2], "_test.go") {
			continue
		}
		oldCmd := exec.Command("git", "show", base+":"+fields[1])
		oldCmd.Dir = root
		old, err := oldCmd.Output()
		if err != nil {
			return moves, err
		}
		newer, err := os.ReadFile(filepath.Join(root, fields[2]))
		if err != nil {
			return moves, err
		}
		before, err := testNames(old)
		if err != nil {
			return moves, err
		}
		after, err := testNames(newer)
		if err != nil {
			return moves, err
		}
		if !reflect.DeepEqual(before, after) {
			return moves, fmt.Errorf("test inventory changed in move %s -> %s: before %v, after %v", fields[1], fields[2], before, after)
		}
		moves++
	}
	return moves, nil
}
