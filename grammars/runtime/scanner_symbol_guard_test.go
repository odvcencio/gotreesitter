//go:build !grammar_subset

package grammarruntime

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This guard would have caught the ocaml regression at build time: an ocaml
// grammar bump renumbered the upstream ExternalSymbols so that
// ocamlSymComment (hardcoded 147) landed on a different grammar rule, and
// nothing in the repository compared a hand-written scanner's hardcoded
// gotreesitter.Symbol constants against the shipped blob's ExternalSymbols.
// Most hand-written scanners never bind at load time (see
// grammars/runtime/external_scanner_binding.go and
// embedded_loader.go:524-539 for the 12 languages that do: dart, rust,
// kotlin, hcl, scala, c_sharp, tsx, typescript, javascript, python, swift,
// sql); those 12 resolve real symbol IDs positionally against the loaded
// Language and are covered separately in load_bound_scanner_order_test.go.
// Every other scanner file hardcodes absolute gotreesitter.Symbol values
// that must stay members of the loaded blob's ExternalSymbols, or the
// scanner silently emits the wrong token.
//
// Receipt (recorded against the shipped blobs on 2026-09-20, before any
// pending grammar bump lands): 121 grammars/runtime/*_scanner.go files, of
// which 2 are shared scanner-helper libraries with no directly registered
// language (blade_scanner.go's HTML tag helpers and rawstring_scanner.go's
// C++-style raw-string helpers, both called from other languages' Scan
// implementations with the caller's own local Symbol constants), 12 bind
// their scanner to the Language at load time and are exempted from the
// hardcoded-constant check below, and the remaining 107 hardcode absolute
// Symbol constants. Running this guard against today's checked-in blobs
// found 4 hardcoded constants already outside their language's
// ExternalSymbols, in two files unrelated to ocaml: editorconfig_scanner.go
// (editorconfigSymEndOfFile, editorconfigSymIntegerRangeStart) and
// liquid_scanner.go (liquidSymInlineCommentContent,
// liquidSymPairedCommentContent). Both are pre-existing, live bugs this
// guard's author is not authorized to fix (scanner files are owned by other
// agents; see knownPreExistingSymbolDrift below), reported separately.
// Every other checked file's constants are members of their language's
// ExternalSymbols and below their language's SymbolNames length.
func TestHardcodedScannerSymbolsAreExternalOnLoadedBlob(t *testing.T) {
	start := time.Now()

	files, err := filepath.Glob("*_scanner.go")
	if err != nil {
		t.Fatalf("glob *_scanner.go: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no *_scanner.go files found; guard did not run")
	}
	sort.Strings(files)

	fset := token.NewFileSet()
	asts := make(map[string]*ast.File, len(files))
	for _, f := range files {
		a, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		asts[f] = a
	}

	wrappers := collectResultSymbolWrappers(asts)
	tables := collectSymbolTables(asts)
	basedFuncs := collectBasedSymbolFuncs(asts)

	var (
		helperCount    int
		boundCount     int
		hardcodedCount int
		outsideCount   int
	)

	for _, file := range files {
		file := file
		if scannerHelperFiles[file] {
			helperCount++
			t.Logf("%s: shared scanner-helper library, not itself a registered language scanner", file)
			continue
		}

		name := scannerFileLanguageName(file)
		scanner := LookupExternalScanner(name)
		if scanner == nil {
			t.Errorf("%s: maps to language %q but no external scanner is registered under that name; map the new scanner file in scannerFileLanguageOverrides or scannerHelperFiles", file, name)
			continue
		}

		if _, ok := scanner.(languageBoundExternalScanner); ok {
			boundCount++
			if loadBoundLanguagesWithDedicatedPinTests[name] {
				t.Logf("%s (%s): binds at load time; positional order already pinned by a dedicated binding test", file, name)
			} else {
				t.Logf("%s (%s): binds at load time; positional order checked in load_bound_scanner_order_test.go", file, name)
			}
			continue
		}

		hardcodedCount++
		lang := Language(name)
		if lang == nil {
			t.Errorf("%s: language %q did not load a blob", file, name)
			continue
		}

		externalSet := make(map[gotreesitter.Symbol]bool, len(lang.ExternalSymbols))
		for _, s := range lang.ExternalSymbols {
			externalSet[s] = true
		}

		consts := collectSymbolConstants(asts[file])
		idents := collectResultSymbolIdents(asts[file], wrappers, tables, basedFuncs)

		checked := make([]string, 0, len(idents))
		for identName := range idents {
			checked = append(checked, identName)
		}
		sort.Strings(checked)

		t.Run(file, func(t *testing.T) {
			for _, identName := range checked {
				cd, ok := consts[identName]
				if !ok {
					// Not a locally declared Symbol constant (e.g. a
					// dynamically resolved var/field such as bladeSyms.comment,
					// or a scanner-local variable). Nothing hardcoded to check.
					continue
				}
				sym := gotreesitter.Symbol(cd.value)
				if externalSet[sym] {
					continue
				}
				msg := formatOutsideExternalsMessage(file, identName, cd.value, name, lang.ExternalSymbols)
				if reason, known := knownPreExistingSymbolDrift[file][identName]; known {
					outsideCount++
					t.Logf("KNOWN PRE-EXISTING BUG (not introduced by this guard, not fixed here: %s): %s", reason, msg)
					continue
				}
				outsideCount++
				t.Error(msg)
			}

			constNames := make([]string, 0, len(consts))
			for constName := range consts {
				constNames = append(constNames, constName)
			}
			sort.Strings(constNames)
			for _, constName := range constNames {
				cd := consts[constName]
				if cd.value < 0 || cd.value >= len(lang.SymbolNames) {
					t.Errorf("%s: %s = %d is out of range for %s.SymbolNames (len=%d)", file, constName, cd.value, name, len(lang.SymbolNames))
				}
			}

			if len(idents) == 0 && len(consts) > 0 {
				t.Logf("%s (%s): declares %d Symbol constants but no SetResultSymbol call (direct or one-level-wrapped) references them; checked range-only", file, name, len(consts))
			}
		})
	}

	t.Logf("scanner symbol guard receipt: %d files, %d shared helpers skipped, %d bind at load time, %d hardcode absolute symbols, %d hardcoded constants found outside ExternalSymbols (including known pre-existing ones logged above), took %s",
		len(files), helperCount, boundCount, hardcodedCount, outsideCount, time.Since(start))
}

func formatOutsideExternalsMessage(file, identName string, value int, name string, externals []gotreesitter.Symbol) string {
	return fmt.Sprintf("%s: %s = %d is not a member of %s.ExternalSymbols %v; the scanner will emit the wrong token if the shipped blob renumbers this symbol", file, identName, value, name, externals)
}

// knownPreExistingSymbolDrift lists (file, constant) pairs this guard found
// already broken on main, reported here rather than fixed because
// *_scanner.go files belong to other tasks. The first run on 2026-09-20 found
// editorconfig (31/32 against ExternalSymbols [23 24]) and liquid (96/97
// against [98 99 100 101 102 103]); both scanners now bind their symbols at
// load time (PR #1185), so the map is empty. Add an entry only for a live
// defect with a filed fix, and remove it when the fix lands; a corrected
// constant that is still wrong then fails loudly again.
var knownPreExistingSymbolDrift = map[string]map[string]string{}

// scannerHelperFiles lists grammars/runtime/*_scanner.go files that define
// shared, parameterized scanning helpers reused by several languages' Scan
// implementations instead of a directly registered ExternalScanner. Neither
// file declares a hardcoded gotreesitter.Symbol constant of its own (their
// SetResultSymbol calls forward a caller-supplied parameter), so they carry
// nothing for this guard to check directly; the languages that call them
// (angular, astro, blade, html, svelte, vue for blade_scanner.go; arduino,
// cpp, cuda, hlsl for rawstring_scanner.go) are checked normally through
// their own scanner files via the one-level indirection resolved below.
var scannerHelperFiles = map[string]bool{
	"blade_scanner.go":     true,
	"rawstring_scanner.go": true,
}

// scannerFileLanguageOverrides maps a *_scanner.go file name to its
// registered language name when the two disagree. Every other scanner file
// name, minus the "_scanner.go" suffix, is used as the registered language
// name directly.
var scannerFileLanguageOverrides = map[string]string{
	"csharp_scanner.go":         "c_sharp",
	"blade_external_scanner.go": "blade",
}

func scannerFileLanguageName(file string) string {
	if name, ok := scannerFileLanguageOverrides[file]; ok {
		return name
	}
	return strings.TrimSuffix(file, "_scanner.go")
}

// loadBoundLanguagesWithDedicatedPinTests names the load-bound languages
// whose positional-binding table is already pinned against the shipped blob
// by a dedicated test: kotlin/swift/dart/rust/javascript/typescript/tsx in
// external_scanner_positional_binding_test.go, hcl in hcl_scanner_test.go,
// and c_sharp/scala/erlang in load_bound_scanner_order_test.go (erlang added
// alongside its load-time-binding conversion). python and sql are checked by
// name-order assertions in the same file instead of a pinned symbol table.
var loadBoundLanguagesWithDedicatedPinTests = map[string]bool{
	"kotlin":      true,
	"swift":       true,
	"dart":        true,
	"rust":        true,
	"javascript":  true,
	"typescript":  true,
	"tsx":         true,
	"hcl":         true,
	"c_sharp":     true,
	"scala":       true,
	"erlang":      true,
	"python":      true,
	"sql":         true,
	"powershell":  true,
	"beancount":   true,
	"caddy":       true,
	"doxygen":     true,
	"blade":       true,
	"angular":     true,
	"css":         true,
	"toml":        true,
	"html":        true,
	"bash":        true,
	"ruby":        true,
	"php":         true,
	"cmake":       true,
	"cpp":         true,
	"elixir":      true,
	"elm":         true,
	"go":          true,
	"haskell":     true,
	"julia":       true,
	"lua":         true,
	"markdown":    true,
	"nix":         true,
	"scss":        true,
	"svelte":      true,
	"awk":         true,
	"xml":         true,
	"yaml":        true,
	"agda":        true,
	"arduino":     true,
	"astro":       true,
	"bicep":       true,
	"bitbake":     true,
	"cairo":       true,
	"cuda":        true,
	"comment":     true,
	"cooklang":    true,
	"cue":         true,
	"disassembly": true,
	"djot":        true,
}

type symbolConstant struct {
	value int
}

// collectSymbolConstants returns every top-level constant of type
// gotreesitter.Symbol (or a bare Symbol identifier, for a hypothetical file
// inside package gotreesitter itself) declared in f, keyed by name, for
// constants whose value is a plain integer literal. Every hardcoded scanner
// in the repository today declares its Symbol constants this way (verified
// by grep before writing this guard); a constant defined by any other
// expression form is intentionally left unchecked rather than mis-evaluated.
func collectSymbolConstants(f *ast.File) map[string]symbolConstant {
	out := map[string]symbolConstant{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || vs.Type == nil || !isSymbolTypeExpr(vs.Type) {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				val, err := strconv.Atoi(lit.Value)
				if err != nil {
					continue
				}
				out[name.Name] = symbolConstant{value: val}
			}
		}
	}
	return out
}

func isSymbolTypeExpr(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name == "Symbol"
	case *ast.Ident:
		return t.Name == "Symbol"
	}
	return false
}

// resultSymbolWrapper records, for a package-level function, which of its
// own flattened parameter positions it forwards into a SetResultSymbol call
// on an ExternalLexer. This is the "one level of local indirection" the
// guard resolves: rawStringScan (rawstring_scanner.go) and the
// htmlScan*(blade_scanner.go) helpers all set the result symbol from a
// caller-supplied parameter rather than a constant they declare themselves.
type resultSymbolWrapper struct {
	paramPositions []int
}

// collectResultSymbolWrappers scans every parsed scanner file, package-wide,
// for non-method functions whose body calls <expr>.SetResultSymbol(p) where
// p is one of the function's own parameters, and records which flattened
// parameter position(s) are used that way. Scanning package-wide (not just
// the file under test) is required because the shared helpers this guard
// needs to see through live in blade_scanner.go and rawstring_scanner.go,
// not in the calling language's own scanner file.
func collectResultSymbolWrappers(asts map[string]*ast.File) map[string]resultSymbolWrapper {
	wrappers := map[string]resultSymbolWrapper{}
	for _, f := range asts {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil || fn.Type.Params == nil {
				continue
			}
			paramPos := map[string]int{}
			pos := 0
			for _, field := range fn.Type.Params.List {
				if len(field.Names) == 0 {
					pos++
					continue
				}
				for _, n := range field.Names {
					paramPos[n.Name] = pos
					pos++
				}
			}
			if len(paramPos) == 0 {
				continue
			}

			seen := map[int]bool{}
			var positions []int
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "SetResultSymbol" || len(call.Args) == 0 {
					return true
				}
				id, ok := call.Args[0].(*ast.Ident)
				if !ok {
					return true
				}
				if p, ok := paramPos[id.Name]; ok && !seen[p] {
					seen[p] = true
					positions = append(positions, p)
				}
				return true
			})
			if len(positions) > 0 {
				sort.Ints(positions)
				wrappers[fn.Name.Name] = resultSymbolWrapper{paramPositions: positions}
			}
		}
	}
	return wrappers
}

// collectSymbolTables finds package-level "var name = [N]gotreesitter.Symbol{
// ident, ident, ... }" (or a slice literal) declarations, package-wide, and
// records each table's element identifier names in literal order. Several
// scanners (djot, fennel, pkl, purescript, rst) dispatch SetResultSymbol
// through such a table indexed by scan state (djotTokenToSym[tok]) rather
// than passing a Symbol constant directly; resolveSymbolArgIdents treats an
// index into one of these tables as referencing every element it holds,
// since the guard cannot know which index a given run takes.
func collectSymbolTables(asts map[string]*ast.File) map[string][]string {
	tables := map[string][]string{}
	for _, f := range asts {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
					continue
				}
				cl, ok := vs.Values[0].(*ast.CompositeLit)
				if !ok {
					continue
				}
				at, ok := cl.Type.(*ast.ArrayType)
				if !ok || !isSymbolTypeExpr(at.Elt) {
					continue
				}
				var names []string
				for _, elt := range cl.Elts {
					if id, ok := elt.(*ast.Ident); ok {
						names = append(names, id.Name)
					}
				}
				if len(names) > 0 {
					tables[vs.Names[0].Name] = names
				}
			}
		}
	}
	return tables
}

// collectBasedSymbolFuncs finds package-level functions of the exact shape
// `func f(tok T) gotreesitter.Symbol { return someBase +
// gotreesitter.Symbol(tok) }` (or the commutative order) and records
// funcName -> the additive base constant's identifier name. norg's norgSym
// is the only scanner using this pattern today: it derives a contiguous run
// of external symbol IDs from one hardcoded base plus an untyped int token
// enum offset (norgTok = iota, not a gotreesitter.Symbol constant, so
// collectSymbolConstants does not see it). This guard can therefore confirm
// the base itself stays a member of ExternalSymbols, which catches a whole-
// block shift, but it cannot verify every derived offset without also
// modeling the separate int enum, so a reorder confined to that enum (with
// the base left untouched) would not be caught here.
func collectBasedSymbolFuncs(asts map[string]*ast.File) map[string]string {
	out := map[string]string{}
	for _, f := range asts {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil || len(fn.Body.List) != 1 {
				continue
			}
			ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			bin, ok := ret.Results[0].(*ast.BinaryExpr)
			if !ok || bin.Op != token.ADD {
				continue
			}
			base := basedSymbolAdditiveBase(bin.X, bin.Y)
			if base == "" {
				base = basedSymbolAdditiveBase(bin.Y, bin.X)
			}
			if base != "" {
				out[fn.Name.Name] = base
			}
		}
	}
	return out
}

// basedSymbolAdditiveBase returns constExpr's identifier name when constExpr
// is a plain identifier and offsetExpr is a gotreesitter.Symbol(...)
// conversion, identifying constExpr as the additive base operand of a
// based-symbol function (see collectBasedSymbolFuncs).
func basedSymbolAdditiveBase(constExpr, offsetExpr ast.Expr) string {
	id, ok := constExpr.(*ast.Ident)
	if !ok {
		return ""
	}
	call, ok := offsetExpr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !isSymbolTypeExpr(call.Fun) {
		return ""
	}
	return id.Name
}

// resolveSymbolArgIdents returns the identifier name(s) an argument
// expression to a result-symbol call could reference: itself, if it is a
// plain identifier; every element of a known symbol table, if it is an
// index into one; or a based-symbol function's additive base constant, if
// it is a call to one (see collectBasedSymbolFuncs). Any other expression
// form (a struct field such as bladeSyms.comment, a dynamically resolved
// local variable, an unrecognized function call) is not a hardcoded
// constant and is left unresolved.
func resolveSymbolArgIdents(expr ast.Expr, tables map[string][]string, basedFuncs map[string]string) []string {
	switch e := expr.(type) {
	case *ast.Ident:
		return []string{e.Name}
	case *ast.IndexExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			if names, ok := tables[id.Name]; ok {
				return names
			}
		}
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok {
			if base, ok := basedFuncs[id.Name]; ok {
				return []string{base}
			}
		}
	}
	return nil
}

// collectResultSymbolIdents returns the set of identifier names passed to a
// result-symbol call in f: directly to <expr>.SetResultSymbol(arg), through
// one level of indirection to a package-level wrapper function recorded in
// wrappers, through a package-level symbol table indexed at the call site
// (see collectSymbolTables), or through a based-symbol function's additive
// base constant (see collectBasedSymbolFuncs).
func collectResultSymbolIdents(f *ast.File, wrappers map[string]resultSymbolWrapper, tables map[string][]string, basedFuncs map[string]string) map[string]bool {
	idents := map[string]bool{}
	add := func(names []string) {
		for _, n := range names {
			idents[n] = true
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel.Name == "SetResultSymbol" && len(call.Args) > 0 {
				add(resolveSymbolArgIdents(call.Args[0], tables, basedFuncs))
			}
		case *ast.Ident:
			if w, ok := wrappers[fun.Name]; ok {
				for _, p := range w.paramPositions {
					if p < len(call.Args) {
						add(resolveSymbolArgIdents(call.Args[p], tables, basedFuncs))
					}
				}
			}
		}
		return true
	})
	return idents
}
