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
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Engine files are root parser, GLR, lexer, scanner dispatch, incremental,
// conflict, token-invariant, and result-compat files, plus internal engine
// packages. New internal packages are engine code unless listed here as support
// packages. Tests are excluded from the R6 language rule. A registry records
// the prefixes it was written under, so a PR that widens this list is compared
// with the base only inside the base's scope.
var rootEnginePrefixes = []string{"parser", "glr", "lexer", "lex_", "scanner", "external_scanner", "incremental", "conflict", "token_invariant", "result_compat"}

// legacyEnginePrefixes is the scope of a registry that has no prefixes header.
var legacyEnginePrefixes = []string{"parser", "glr", "lexer", "lex_", "scanner", "external_scanner"}

var internalSupportPackages = map[string]bool{
	"benchfixtures": true, "grammarpatch": true, "grammarsubsettest": true, "luapattern": true,
}

func engineFile(path string) bool { return engineFileIn(path, rootEnginePrefixes) }

func engineFileIn(path string, prefixes []string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	if !strings.Contains(path, "/") {
		for _, prefix := range prefixes {
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

// isTrackedName reports whether e is a language name: a .Name selector, a
// tracked local, or a call to a helper listed in aliases as "helper()".
func isTrackedName(e ast.Expr, aliases map[string]bool) bool {
	e = unparen(e)
	if isName(e) {
		return true
	}
	switch x := e.(type) {
	case *ast.Ident:
		return aliases[x.Name]
	case *ast.CallExpr:
		id, ok := x.Fun.(*ast.Ident)
		return ok && aliases[id.Name+"()"]
	}
	return false
}

func containsTrackedName(e ast.Expr, aliases map[string]bool) bool {
	if e == nil {
		return false
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if expr, ok := n.(ast.Expr); ok && isTrackedName(expr, aliases) {
			found = true
		}
		return !found
	})
	return found
}

func comparisonCall(e ast.Expr, aliases map[string]bool) bool {
	var name string
	switch x := unparen(e).(type) {
	case *ast.SelectorExpr:
		name = x.Sel.Name
	case *ast.Ident:
		if aliases[x.Name] {
			return true
		}
		name = x.Name
	}
	switch name {
	case "EqualFold", "Compare", "Equal", "HasPrefix", "HasSuffix", "Contains":
		return true
	}
	return false
}

func isEnvFunction(e ast.Expr, aliases, osImports map[string]bool, dotOS bool) bool {
	switch x := unparen(e).(type) {
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && osImports[pkg.Name] && (x.Sel.Name == "Getenv" || x.Sel.Name == "LookupEnv")
	case *ast.Ident:
		return aliases[x.Name] || (dotOS && (x.Name == "Getenv" || x.Name == "LookupEnv"))
	default:
		return false
	}
}

// Track local copies of a guarded value within one function. A fixed point
// also catches a copy of a copy. Conservative matches cannot hide a new use.
func collectAliases(n ast.Node, aliases map[string]bool, source func(ast.Expr, map[string]bool) bool) {
	for changed := true; changed; {
		changed = false
		mark := func(lhs, rhs ast.Expr) {
			id, ok := lhs.(*ast.Ident)
			if ok && !aliases[id.Name] && source(rhs, aliases) {
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

type aliasScope struct {
	start, end   token.Pos
	names        map[string]bool
	envFuncs     map[string]bool
	compareFuncs map[string]bool
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

type source struct {
	path string
	fset *token.FileSet
	file *ast.File
}

type project struct {
	sources  []source
	grammars map[string]bool
	funcs    map[string]bool // "directory|name" of every package-level function
}

func loadProject(root string) (*project, error) {
	paths, err := sourceFiles(root)
	if err != nil {
		return nil, err
	}
	grammars, err := loadGrammarNames(root)
	if err != nil {
		return nil, err
	}
	p := &project{grammars: grammars, funcs: map[string]bool{}}
	for _, path := range paths {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(root, path), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		p.sources = append(p.sources, source{path, fset, file})
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				p.funcs[pkgDir(path)+"|"+fn.Name.Name] = true
			}
		}
	}
	return p, nil
}

// paramTaints maps "directory|function" to the parameter positions that an
// engine-file caller feeds with a language name.
type paramTaints map[string]map[int]bool

func (t paramTaints) add(key string, position int) bool {
	if t[key][position] {
		return false
	}
	if t[key] == nil {
		t[key] = map[int]bool{}
	}
	t[key][position] = true
	return true
}

func pkgDir(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

// seedParams marks the string parameters of fn that receive a language name.
func seedParams(names map[string]bool, fn *ast.FuncDecl, positions map[int]bool) {
	i := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			i++
			continue
		}
		typ, ok := field.Type.(*ast.Ident)
		isString := ok && typ.Name == "string"
		for _, id := range field.Names {
			if isString && positions[i] && id.Name != "_" {
				names[id.Name] = true
			}
			i++
		}
	}
}

// scan finds language-name branches and environment reads. With flow == nil it
// tracks .Name selectors and their local copies. With flow set it also follows
// a name into same-package functions that receive it, unchanged, as an
// argument from an engine file (and treats languageName(...) as a name), and
// it returns the calls that pass a name, for the caller to merge into flow.
// Method calls, wrapped names, and chains through non-engine callers are not
// followed, so the result is a lower bound.
func (p *project) scan(flow paramTaints) (language, env []finding, calls paramTaints) {
	grammars := p.grammars
	calls = paramTaints{}
	for _, src := range p.sources {
		path, fset, file := src.path, src.fset, src.file
		dir := pkgDir(path)
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
		envSource := func(e ast.Expr, aliases map[string]bool) bool {
			return isEnvFunction(e, aliases, osImports, dotOS)
		}
		globalNames := map[string]bool{}
		if flow != nil {
			globalNames["languageName()"] = true
		}
		globalEnvFuncs := map[string]bool{}
		globalCompareFuncs := map[string]bool{}
		var aliasScopes []aliasScope
		for _, decl := range file.Decls {
			if _, ok := decl.(*ast.FuncDecl); !ok {
				collectAliases(decl, globalNames, containsTrackedName)
				collectAliases(decl, globalEnvFuncs, envSource)
				collectAliases(decl, globalCompareFuncs, comparisonCall)
			}
		}
		for _, decl := range file.Decls {
			// A go:linkname declaration has no body, so it has no local aliases.
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				names := make(map[string]bool, len(globalNames))
				for name := range globalNames {
					names[name] = true
				}
				envFuncs := make(map[string]bool, len(globalEnvFuncs))
				for name := range globalEnvFuncs {
					envFuncs[name] = true
				}
				compareFuncs := make(map[string]bool, len(globalCompareFuncs))
				for name := range globalCompareFuncs {
					compareFuncs[name] = true
				}
				if flow != nil && fn.Recv == nil {
					seedParams(names, fn, flow[dir+"|"+fn.Name.Name])
				}
				collectAliases(fn.Body, names, containsTrackedName)
				collectAliases(fn.Body, envFuncs, envSource)
				collectAliases(fn.Body, compareFuncs, comparisonCall)
				aliasScopes = append(aliasScopes, aliasScope{fn.Pos(), fn.End(), names, envFuncs, compareFuncs})
			}
		}
		aliasesAt := func(pos token.Pos) aliasScope {
			i := sort.Search(len(aliasScopes), func(i int) bool { return aliasScopes[i].start > pos }) - 1
			if i >= 0 && pos < aliasScopes[i].end {
				return aliasScopes[i]
			}
			return aliasScope{names: globalNames, envFuncs: globalEnvFuncs, compareFuncs: globalCompareFuncs}
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
				case *ast.CallExpr:
					if len(x.Args) == 2 && comparisonCall(x.Fun, aliases.compareFuncs) {
						for i, arg := range x.Args {
							if !containsTrackedName(arg, aliases.names) {
								continue
							}
							other := x.Args[1-i]
							if v, ok := stringValue(other, constants); ok && v != "" {
								add(&language, x, "compare-call", v)
							} else if !containsTrackedName(other, aliases.names) {
								add(&language, x, "compare-call-dynamic", "language.Name")
							}
						}
					}
					if id, ok := x.Fun.(*ast.Ident); ok && flow != nil && p.funcs[dir+"|"+id.Name] {
						for i, arg := range x.Args {
							if isTrackedName(arg, aliases.names) {
								calls.add(dir+"|"+id.Name, i)
							}
						}
					}
				case *ast.BinaryExpr:
					if x.Op == token.EQL || x.Op == token.NEQ {
						if containsTrackedName(x.X, aliases.names) {
							if v, ok := stringValue(x.Y, constants); ok && v != "" {
								add(&language, x, "compare", v)
							} else {
								add(&language, x, "compare-dynamic", "language.Name")
							}
						}
						if containsTrackedName(x.Y, aliases.names) {
							if v, ok := stringValue(x.X, constants); ok && v != "" {
								add(&language, x, "compare", v)
							} else if !containsTrackedName(x.X, aliases.names) {
								add(&language, x, "compare-dynamic", "language.Name")
							}
						}
					}
				case *ast.SwitchStmt:
					if containsTrackedName(x.Tag, aliases.names) {
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
					} else if containsTrackedName(x.Index, aliases.names) {
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
			if !isEnvFunction(call.Fun, aliases.envFuncs, osImports, dotOS) {
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
	return language, env, calls
}

// flowFindings returns the language-name uses that exist only because a name
// reaches a function through a parameter. Each is a plain scan finding plus
// the ones the plain scan cannot see, so the plain registry stays stable.
func (p *project) flowFindings(plain []finding) []finding {
	taints := paramTaints{}
	for changed := true; changed; {
		changed = false
		_, _, calls := p.scan(taints)
		for key, positions := range calls {
			for position := range positions {
				if taints.add(key, position) {
					changed = true
				}
			}
		}
	}
	all, _, _ := p.scan(taints)
	skip := countFindings(plain)
	var out []finding
	for _, item := range all {
		if skip[item.key] > 0 {
			skip[item.key]--
			continue
		}
		out = append(out, item)
	}
	return out
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

const prefixesHeader = "# prefixes: "

// registryPrefixes returns the engine prefixes a registry was written under.
func registryPrefixes(data []byte) []string {
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, prefixesHeader); ok {
			return strings.Split(strings.TrimSpace(rest), ",")
		}
	}
	return legacyEnginePrefixes
}

// readRegistry reads a registry. A scoped registry (one keyed by engine file)
// must carry the current engine prefixes, so a prefix change forces a rewrite.
func readRegistry(path string, scoped bool) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if scoped && !slices.Equal(registryPrefixes(data), rootEnginePrefixes) {
		return nil, fmt.Errorf("%s: engine prefixes differ from the guard's; run go run ./cmd/v1guard -init", path)
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

// checkScopedGrowth fails when current exceeds the base registry. A scoped
// registry written under narrower engine prefixes is compared only for files
// inside that narrower scope, so widening the scope adds rows without
// loosening any existing ceiling.
func checkScopedGrowth(registry string, baseData []byte, scoped bool, current map[string]int) error {
	old, err := parseCounts(baseData, registry)
	if err != nil {
		return err
	}
	if basePrefixes := registryPrefixes(baseData); scoped && !slices.Equal(basePrefixes, rootEnginePrefixes) {
		inScope := map[string]int{}
		for key, n := range current {
			file, _, _ := strings.Cut(key, "|")
			if engineFileIn(file, basePrefixes) {
				inScope[key] = n
			}
		}
		current = inScope
	}
	return checkNoGrowth(registry, old, current)
}

func checkBaseRegistry(root, base, registry string, scoped bool, current map[string]int) error {
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
	return checkScopedGrowth(registry, data, scoped, current)
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

func writeCounts(path string, counts map[string]int, scoped bool) error {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteString("# R6 baseline. Counts may only shrink; remove a row when its last use leaves.\n")
	if scoped {
		buf.WriteString(prefixesHeader + strings.Join(rootEnginePrefixes, ",") + "\n")
	}
	for _, key := range keys {
		fmt.Fprintf(&buf, "%d\t%s\n", counts[key], key)
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func run(root string, init bool, base string) error {
	p, err := loadProject(root)
	if err != nil {
		return err
	}
	language, env, _ := p.scan(nil)
	flow := p.flowFindings(language)
	langPath := filepath.Join(root, "cmd/v1guard/language_allowlist.txt")
	flowPath := filepath.Join(root, "cmd/v1guard/language_flow_allowlist.txt")
	envPath := filepath.Join(root, "cmd/v1guard/env_registry.txt")
	if init {
		if err := writeCounts(langPath, countFindings(language), true); err != nil {
			return err
		}
		if err := writeCounts(flowPath, countFindings(flow), true); err != nil {
			return err
		}
		return writeCounts(envPath, countFindings(env), false)
	}
	langAllowed, err := readRegistry(langPath, true)
	if err != nil {
		return err
	}
	flowAllowed, err := readRegistry(flowPath, true)
	if err != nil {
		return err
	}
	envAllowed, err := readRegistry(envPath, false)
	if err != nil {
		return err
	}
	if base != "" {
		if err := checkBaseRegistry(root, base, "language_allowlist.txt", true, langAllowed); err != nil {
			return err
		}
		if err := checkBaseRegistry(root, base, "language_flow_allowlist.txt", true, flowAllowed); err != nil {
			return err
		}
		if err := checkBaseRegistry(root, base, "env_registry.txt", false, envAllowed); err != nil {
			return err
		}
	}
	if err := checkAllowlist("language names", language, langAllowed); err != nil {
		return err
	}
	if err := checkAllowlist("language names through parameters", flow, flowAllowed); err != nil {
		return err
	}
	if err := checkAllowlist("environment", env, envAllowed); err != nil {
		return err
	}
	fmt.Printf("R6 guardrails: %d language-name uses (%d more through parameters) and %d environment reads match their checked-in baselines\n", len(language), len(flow), len(env))
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
