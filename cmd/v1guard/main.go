// v1guard enforces the R6 engine and environment guardrails.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Engine files are root parser, GLR, lexer, and scanner dispatch files, plus
// internal engine packages. New internal packages are engine code unless listed
// here as support packages. Tests are excluded from the R6 language rule.
var rootEnginePrefixes = []string{"parser", "glr", "lexer", "lex_", "scanner", "external_scanner"}
var internalSupportPackages = map[string]bool{
	"benchfixtures": true, "grammarpatch": true, "grammarsubsettest": true, "luapattern": true,
}

func engineFile(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	if !strings.Contains(path, "/") {
		for _, prefix := range rootEnginePrefixes {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
		return false
	}
	parts := strings.Split(path, "/")
	return len(parts) > 2 && parts[0] == "internal" && !internalSupportPackages[parts[1]]
}

type finding struct {
	key  string
	file string
	line int
}

func sourceFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "testdata" || d.Name() == "cgo_harness" || d.Name() == "examples") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	return paths, err
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func literal(e ast.Expr) (string, bool) {
	e = unparen(e)
	if s, ok := e.(*ast.BasicLit); ok && s.Kind == token.STRING {
		v, err := strconv.Unquote(s.Value)
		return v, err == nil
	}
	return "", false
}

func stringValue(e ast.Expr, constants map[string]string) (string, bool) {
	e = unparen(e)
	if v, ok := literal(e); ok {
		return v, true
	}
	if id, ok := e.(*ast.Ident); ok {
		v, ok := constants[id.Name]
		return v, ok
	}
	return "", false
}

func isName(e ast.Expr) bool {
	e = unparen(e)
	s, ok := e.(*ast.SelectorExpr)
	return ok && s.Sel.Name == "Name"
}

func isTrackedName(e ast.Expr, aliases map[string]bool) bool {
	e = unparen(e)
	if isName(e) {
		return true
	}
	id, ok := e.(*ast.Ident)
	return ok && aliases[id.Name]
}

// Track local copies of a Name field within one function. A fixed point also
// catches a copy of a copy. Conservative matches are safe for this lint: they
// may require an existing use to be listed, but cannot hide a new branch.
func collectNameAliases(n ast.Node, aliases map[string]bool) {
	for changed := true; changed; {
		changed = false
		mark := func(lhs, rhs ast.Expr) {
			id, ok := lhs.(*ast.Ident)
			if ok && !aliases[id.Name] && isTrackedName(rhs, aliases) {
				aliases[id.Name] = true
				changed = true
			}
		}
		ast.Inspect(n, func(node ast.Node) bool {
			switch x := node.(type) {
			case *ast.AssignStmt:
				if len(x.Lhs) == len(x.Rhs) {
					for i := range x.Lhs {
						mark(x.Lhs[i], x.Rhs[i])
					}
				}
			case *ast.ValueSpec:
				if len(x.Names) == len(x.Values) {
					for i := range x.Names {
						mark(x.Names[i], x.Values[i])
					}
				}
			}
			return true
		})
	}
}

type nameAliasScope struct {
	start, end token.Pos
	names      map[string]bool
}

func loadGrammarNames(root string) (map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(root, "grammars/grammar_blobs/*.bin"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no grammar blobs found")
	}
	names := make(map[string]bool, len(files))
	for _, path := range files {
		names[strings.TrimSuffix(filepath.Base(path), ".bin")] = true
	}
	return names, nil
}

func scan(root string) ([]finding, []finding, error) {
	paths, err := sourceFiles(root)
	if err != nil {
		return nil, nil, err
	}
	grammars, err := loadGrammarNames(root)
	if err != nil {
		return nil, nil, err
	}
	var language, env []finding
	for _, path := range paths {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(root, path), nil, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		osImports := map[string]bool{}
		dotOS := false
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil || importPath != "os" {
				continue
			}
			name := "os"
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if name == "." {
				dotOS = true
			} else {
				osImports[name] = true
			}
		}
		constants := map[string]string{}
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.ValueSpec)
			if !ok || len(decl.Values) != len(decl.Names) {
				return true
			}
			for i, name := range decl.Names {
				if v, ok := literal(decl.Values[i]); ok {
					constants[name.Name] = v
				}
			}
			return true
		})
		globalAliases := map[string]bool{}
		var aliasScopes []nameAliasScope
		for _, decl := range file.Decls {
			if _, ok := decl.(*ast.FuncDecl); !ok {
				collectNameAliases(decl, globalAliases)
			}
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				names := make(map[string]bool, len(globalAliases))
				for name := range globalAliases {
					names[name] = true
				}
				collectNameAliases(fn.Body, names)
				aliasScopes = append(aliasScopes, nameAliasScope{fn.Pos(), fn.End(), names})
			}
		}
		aliasesAt := func(pos token.Pos) map[string]bool {
			i := sort.Search(len(aliasScopes), func(i int) bool { return aliasScopes[i].start > pos }) - 1
			if i >= 0 && pos < aliasScopes[i].end {
				return aliasScopes[i].names
			}
			return globalAliases
		}
		add := func(dst *[]finding, n ast.Node, kind, value string) {
			*dst = append(*dst, finding{path + "|" + kind + "|" + value, path, fset.Position(n.Pos()).Line})
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return true
			}
			aliases := aliasesAt(n.Pos())
			if engineFile(path) {
				switch x := n.(type) {
				case *ast.BinaryExpr:
					if x.Op == token.EQL || x.Op == token.NEQ {
						if isTrackedName(x.X, aliases) {
							if v, ok := stringValue(x.Y, constants); ok && v != "" {
								add(&language, x, "compare", v)
							} else {
								add(&language, x, "compare-dynamic", "language.Name")
							}
						}
						if isTrackedName(x.Y, aliases) {
							if v, ok := stringValue(x.X, constants); ok && v != "" {
								add(&language, x, "compare", v)
							} else if !isTrackedName(x.X, aliases) {
								add(&language, x, "compare-dynamic", "language.Name")
							}
						}
					}
				case *ast.SwitchStmt:
					if isTrackedName(x.Tag, aliases) {
						for _, stmt := range x.Body.List {
							for _, expr := range stmt.(*ast.CaseClause).List {
								if v, ok := stringValue(expr, constants); ok {
									add(&language, expr, "switch", v)
								} else {
									add(&language, expr, "switch-dynamic", "language.Name")
								}
							}
						}
					}
				case *ast.KeyValueExpr:
					// A Go composite with a string key is a map, including named
					// map types and nested literals whose type is omitted.
					if v, ok := stringValue(x.Key, constants); ok && grammars[v] {
						add(&language, x, "map", v)
					}
				case *ast.IndexExpr:
					if v, ok := stringValue(x.Index, constants); ok && grammars[v] {
						add(&language, x, "map-index", v)
					} else if isTrackedName(x.Index, aliases) {
						add(&language, x, "map-index", "language.Name")
					}
				}
			}
			// These existing readers accept a variable, but their callers keep the
			// set of possible names in literal arguments or a literal range list.
			// Count each name so adding one cannot hide behind a dynamic call site.
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "readEnvBoolKnob" && len(call.Args) == 1 {
					if v, ok := stringValue(call.Args[0], constants); ok {
						add(&env, call, "env", v)
					} else {
						add(&env, call, "dynamic", "readEnvBoolKnob")
					}
				}
			}
			if loop, ok := n.(*ast.RangeStmt); ok && path == "parser_config.go" {
				if id, ok := loop.Value.(*ast.Ident); ok && id.Name == "name" {
					if list, ok := loop.X.(*ast.CompositeLit); ok {
						for _, expr := range list.Elts {
							if v, ok := literal(expr); ok {
								add(&env, expr, "env", v)
							}
						}
					}
				}
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			isEnvRead := false
			switch fun := unparen(call.Fun).(type) {
			case *ast.SelectorExpr:
				if pkg, ok := fun.X.(*ast.Ident); ok && osImports[pkg.Name] && (fun.Sel.Name == "Getenv" || fun.Sel.Name == "LookupEnv") {
					isEnvRead = true
				}
			case *ast.Ident:
				if dotOS && (fun.Name == "Getenv" || fun.Name == "LookupEnv") {
					isEnvRead = true
				}
			}
			if !isEnvRead {
				return true
			}
			if v, ok := literal(call.Args[0]); ok {
				add(&env, call, "env", v)
				return true
			}
			if id, ok := call.Args[0].(*ast.Ident); ok {
				if v, ok := constants[id.Name]; ok {
					add(&env, call, "env", v)
					return true
				}
				add(&env, call, "dynamic", id.Name)
				return true
			}
			add(&env, call, "dynamic", "expression")
			return true
		})
	}
	return language, env, nil
}

func parseCounts(data []byte, path string) (map[string]int, error) {
	counts := map[string]int{}
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s: malformed line %q", path, line)
		}
		n, err := strconv.Atoi(parts[0])
		if err != nil || n < 1 || counts[parts[1]] != 0 {
			return nil, fmt.Errorf("%s: invalid count or duplicate key %q", path, line)
		}
		counts[parts[1]] = n
	}
	return counts, s.Err()
}

func readCounts(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseCounts(data, path)
}

func checkNoGrowth(label string, old, current map[string]int) error {
	for key, count := range current {
		if count > old[key] {
			return fmt.Errorf("%s grew at %s: %d to %d", label, key, old[key], count)
		}
	}
	return nil
}

func checkBaseRegistry(root, base, registry string, current map[string]int) error {
	verify := exec.Command("git", "-C", root, "rev-parse", "--verify", base+"^{commit}")
	if out, err := verify.CombinedOutput(); err != nil {
		return fmt.Errorf("verify base %q: %w: %s", base, err, out)
	}
	object := base + ":cmd/v1guard/" + registry
	if err := exec.Command("git", "-C", root, "cat-file", "-e", object).Run(); err != nil {
		// R6 introduces the registries; all later PRs compare with main.
		return nil
	}
	data, err := exec.Command("git", "-C", root, "show", object).Output()
	if err != nil {
		return fmt.Errorf("read %s: %w", object, err)
	}
	old, err := parseCounts(data, object)
	if err != nil {
		return err
	}
	return checkNoGrowth(registry, old, current)
}

func countFindings(items []finding) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		counts[item.key]++
	}
	return counts
}

func checkAllowlist(label string, items []finding, allowed map[string]int) error {
	actual := countFindings(items)
	var failures []string
	for key, n := range actual {
		if n > allowed[key] {
			failures = append(failures, fmt.Sprintf("%s: %s has %d occurrence(s), allowlist permits %d", label, key, n, allowed[key]))
		}
	}
	for key, n := range allowed {
		if actual[key] < n {
			failures = append(failures, fmt.Sprintf("%s: shrink allowlist for %s from %d to %d", label, key, n, actual[key]))
		}
	}
	sort.Strings(failures)
	if len(failures) != 0 {
		return fmt.Errorf("%s", strings.Join(failures, "\n"))
	}
	return nil
}

func writeCounts(path string, counts map[string]int) error {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteString("# R6 baseline. Counts may only shrink; remove a row when its last use leaves.\n")
	for _, key := range keys {
		fmt.Fprintf(&buf, "%d\t%s\n", counts[key], key)
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func run(root string, init bool, base string) error {
	language, env, err := scan(root)
	if err != nil {
		return err
	}
	langPath := filepath.Join(root, "cmd/v1guard/language_allowlist.txt")
	envPath := filepath.Join(root, "cmd/v1guard/env_registry.txt")
	if init {
		if err := writeCounts(langPath, countFindings(language)); err != nil {
			return err
		}
		return writeCounts(envPath, countFindings(env))
	}
	langAllowed, err := readCounts(langPath)
	if err != nil {
		return err
	}
	envAllowed, err := readCounts(envPath)
	if err != nil {
		return err
	}
	if base != "" {
		if err := checkBaseRegistry(root, base, "language_allowlist.txt", langAllowed); err != nil {
			return err
		}
		if err := checkBaseRegistry(root, base, "env_registry.txt", envAllowed); err != nil {
			return err
		}
	}
	if err := checkAllowlist("language names", language, langAllowed); err != nil {
		return err
	}
	if err := checkAllowlist("environment", env, envAllowed); err != nil {
		return err
	}
	fmt.Printf("R6 guardrails: %d language-name uses and %d environment reads match their checked-in baselines\n", len(language), len(env))
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	init := flag.Bool("init", false, "write initial baselines")
	base := flag.String("base", "", "git revision whose R6 registries are the shrink-only ceilings")
	flag.Parse()
	if err := run(*root, *init, *base); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
