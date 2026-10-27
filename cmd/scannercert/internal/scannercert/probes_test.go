package scannercert

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The recorder stands in for a grammar's scanner during a parse, so anything
// the root package asks of the scanner's dynamic type reaches the recorder, not
// the scanner. The root asks in three ways: a type assertion to an optional
// interface, a type switch, and reflection. This file reads the root package's
// source, lists every such question about a scanner value, and fails when the
// root asks one that the recorder neither answers like the scanner nor is
// known to be asked only by incremental reuse.
//
// Certify compares the parse with and without the recorder, but a tree digest
// cannot see a changed sequence of Scan calls. This test closes that gap at the
// source: a new probe in a full-parse path is a failure here before it is a
// silent difference in a report.

// rootProbe is one place where the root package asks about a scanner's type.
// The key is the method asked about when the asked interface is declared in
// the root, and the written type ("type incr.X", "type *adapter") when it is
// not an interface the checker could resolve; "reflect.TypeOf" and
// "reflect.ValueOf" name reflection.
type rootProbe struct {
	key  string
	file string
	line int
}

func (p rootProbe) String() string { return fmt.Sprintf("%s:%d (%s)", p.file, p.line, p.key) }

// emptyImporter gives every import an empty package. The scan needs only the
// root package's own types, so a reference into another package stays invalid
// and the probe is keyed by how it is written.
type emptyImporter struct{}

func (emptyImporter) Import(path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	pkg := types.NewPackage(path, path[strings.LastIndex(path, "/")+1:])
	pkg.MarkComplete()
	return pkg, nil
}

func validType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.(*types.Basic)
	return !ok || b.Kind() != types.Invalid
}

// scannerProbes lists the questions the non-test Go files in dir ask about a
// value whose static type implements ExternalScanner. Every file counts,
// whatever its build constraints.
func scannerProbes(dir string) ([]rootProbe, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}}
	// Type errors from the empty imports are expected. The checker goes on
	// past them, and the types the scan needs do not depend on an import.
	conf := types.Config{Importer: emptyImporter{}, Error: func(error) {}, FakeImportC: true}
	pkg, _ := conf.Check("root", fset, files, info)
	scanner, _ := pkg.Scope().Lookup("ExternalScanner").(*types.TypeName)
	if scanner == nil {
		return nil, fmt.Errorf("%s declares no ExternalScanner type", dir)
	}
	iface, ok := scanner.Type().Underlying().(*types.Interface)
	if !ok {
		return nil, fmt.Errorf("%s: ExternalScanner is not an interface", dir)
	}
	base := map[string]bool{}
	for i := 0; i < iface.NumMethods(); i++ {
		base[iface.Method(i).Name()] = true
	}
	isScanner := func(e ast.Expr) bool {
		t := info.TypeOf(e)
		return validType(t) && types.Implements(t, iface)
	}
	qualify := func(p *types.Package) string {
		if p == pkg {
			return ""
		}
		return p.Name()
	}

	var probes []rootProbe
	add := func(pos token.Pos, key string) {
		at := fset.Position(pos)
		probes = append(probes, rootProbe{key: key, file: filepath.Base(at.Filename), line: at.Line})
	}
	// asked adds one probe for each question the asked type puts to the scanner.
	asked := func(typeExpr ast.Expr) {
		t := info.TypeOf(typeExpr)
		switch {
		case !validType(t):
			add(typeExpr.Pos(), "type "+types.ExprString(typeExpr))
		case types.IsInterface(t):
			asks := t.Underlying().(*types.Interface)
			for i := 0; i < asks.NumMethods(); i++ {
				if name := asks.Method(i).Name(); !base[name] {
					add(typeExpr.Pos(), name)
				}
			}
		default:
			add(typeExpr.Pos(), "type "+types.TypeString(t, qualify))
		}
	}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.TypeAssertExpr:
				// A type switch guard has no type; its cases come below.
				if n.Type != nil && isScanner(n.X) {
					asked(n.Type)
				}
			case *ast.TypeSwitchStmt:
				var guard ast.Expr
				switch assign := n.Assign.(type) {
				case *ast.ExprStmt:
					guard = assign.X
				case *ast.AssignStmt:
					if len(assign.Rhs) == 1 {
						guard = assign.Rhs[0]
					}
				}
				if assert, ok := guard.(*ast.TypeAssertExpr); ok && isScanner(assert.X) {
					for _, clause := range n.Body.List {
						for _, typeExpr := range clause.(*ast.CaseClause).List {
							asked(typeExpr)
						}
					}
				}
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok || len(n.Args) != 1 || !isScanner(n.Args[0]) {
					return true
				}
				pkgName, ok := info.Uses[identOf(sel.X)].(*types.PkgName)
				if ok && pkgName.Imported().Path() == "reflect" && (sel.Sel.Name == "TypeOf" || sel.Sel.Name == "ValueOf") {
					add(n.Pos(), "reflect."+sel.Sel.Name)
				}
			}
			return true
		})
	}
	sort.SliceStable(probes, func(i, j int) bool {
		a, b := probes[i], probes[j]
		if a.file != b.file {
			return a.file < b.file
		}
		return a.line < b.line
	})
	return probes, nil
}

func identOf(e ast.Expr) *ast.Ident {
	id, _ := e.(*ast.Ident)
	return id
}

// writeRoot lays out a small stand-in for the root package.
func writeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestScannerProbesFindEveryWayTheRootAsksAboutAScanner(t *testing.T) {
	dir := writeRoot(t, map[string]string{
		"language.go": `package root

type ExternalScanner interface {
	Create() any
	Destroy(payload any)
	Serialize(payload any, buf []byte) int
	Deserialize(payload any, buf []byte)
	Scan(payload any, lexer *Lexer, valid []bool) bool
}

type Lexer struct{}

type Language struct{ ExternalScanner ExternalScanner }

type Optional interface {
	ExternalScanner
	Optional() bool
}

type wrapper struct{ ExternalScanner }
`,
		"probes.go": `package root

import (
	"reflect"

	"example.com/remote"
)

func probes(lang *Language, s ExternalScanner, anything any) {
	_, _ = s.(Optional)
	_, _ = lang.ExternalScanner.(interface{ Anonymous(int) bool })
	_, _ = s.(remote.Declared)
	_, _ = s.(*wrapper)
	switch s.(type) {
	case Optional:
	case *wrapper:
	}
	switch got := lang.ExternalScanner.(type) {
	case interface{ Switched() }:
		_ = got
	}
	_ = reflect.TypeOf(s)
	_ = reflect.ValueOf(lang.ExternalScanner)

	// None of these asks a scanner anything beyond the base interface.
	_, _ = anything.(Optional)
	_, _ = s.(ExternalScanner)
	_ = reflect.TypeOf(anything)
}
`,
		"skipped_test.go": `package root

func skipped(s ExternalScanner) { _, _ = s.(interface{ NotCounted() }) }
`,
	})
	got, err := scannerProbes(dir)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, p := range got {
		if p.file != "probes.go" {
			t.Fatalf("probe %v comes from a file other than probes.go", p)
		}
		keys = append(keys, p.key)
	}
	want := []string{
		"Optional", "Anonymous", "type remote.Declared", "type *wrapper",
		"Optional", "type *wrapper", "Switched", "reflect.TypeOf", "reflect.ValueOf",
	}
	if !slices.Equal(keys, want) {
		t.Fatalf("probes = %q, want %q", keys, want)
	}
	if got[0].line != 10 {
		t.Fatalf("first probe at line %d, want 10", got[0].line)
	}
}

func TestScannerProbesRequireAScannerType(t *testing.T) {
	dir := writeRoot(t, map[string]string{"x.go": "package root\n"})
	if _, err := scannerProbes(dir); err == nil || !strings.Contains(err.Error(), "ExternalScanner") {
		t.Fatalf("err = %v, want a root without ExternalScanner to be refused", err)
	}
}

// mirroredProbes lists the capabilities the recorder implements for every
// scanner, and the root files that ask for each. Where the scanner has no
// opinion the recorder answers false, or true for the two retry and ordering
// opinions the parser also assumes true. That matches production exactly
// wherever the root treats an absent interface and a false answer alike.
// classifyExternalScannerQuiescence is the one place that does not: it reads a
// false SupportsIncrementalReuse as a refusal, not as no opinion, and only
// incremental reuse and the compact route reach it.
//
// A probe in a file that is not listed may read that difference during a full
// parse. Check how the new code treats an absent interface, then add the file.
var mirroredProbes = map[string][]string{
	"CheckpointIdentity":             {"external_scanner_checkpoint_capability.go"},
	"ExternalScannerIsStateless":     {"admission_switch.go", "external_scanner_quiescence.go", "glr.go", "incremental.go", "parser.go", "parser_recover_c.go", "parsercore_phase0_per_version_lexer_state.go", "token_invariant_dependency.go", "token_invariant_lexical_proof.go"},
	"PreservesStateOnScanFailure":    {"parser_dfa_token_source.go"},
	"RetainsStateOnScanFailure":      {"parser_dfa_token_source.go"},
	"SupportsErrorModeExternalRetry": {"parser_dfa_token_source.go"},
	"SupportsIncrementalReuse":       {"external_scanner_quiescence.go", "parser.go"},
	"SupportsPackedGSSVersionOrder":  {"parser.go"},
	"UsesExternalScannerCheckpoints": {"external_scanner_checkpoint_capability.go", "external_scanner_checkpoints.go"},
}

// onlyIncrementalProbes lists the questions the recorder does not answer,
// each with the reason it cannot change a full parse. The recorder reads as if
// the scanner lacked these capabilities. Certify parses each source from
// scratch with no old tree, and these are asked only when a new parse reuses
// an old one.
var onlyIncrementalProbes = map[string]string{
	"AllowsIncrementalReuseWithoutCheckpoint": "languageAllowsCheckpointlessExternalReuse: looks up a checkpoint for a reused subtree",
	"ExternalScannerASCIIEquivalenceClass":    "tokenInvariantScannerASCIIEditEquivalent: judges whether an edit leaves a token unchanged",
	"RequiresIncrementalPrefixFrontierProof":  "languageRequiresExternalScannerPrefixFrontierProof: gates an incremental reparse (parser_api.go)",
	"SupportsCheckpointedNonLeafReuse":        "languageSupportsCheckpointedNonLeafReuse: gates composite subtree reuse (incremental.go)",
	"SupportsIncrementalReuseFromErrorTree":   "languageSupportsIncrementalReuseFromErrorTree: gates reuse of an error-bearing old tree",

	"type *externalScannerOrderAdapter":    "checkpoint identity unwrapping for incremental reuse; only languages built from an override blob carry the adapter",
	"type incr.StatelessReadScanner":       "legacyReuseReadsEligible: incremental and compact reuse eligibility",
	"type incr.LargeStatelessReadScanner":  "legacyReuseReadsEligible: incremental and compact reuse eligibility",
	"type incr.CheckpointReadScanner":      "legacyReuseReadsEligible: incremental and compact reuse eligibility",
	"type incr.LargeCheckpointReadScanner": "legacyReuseReadsEligible: incremental and compact reuse eligibility",
	"reflect.TypeOf":                       "parser-core phase zero diagnostics (parsercore_phase0_*.go), not the parse",
	"reflect.ValueOf":                      "parser-core phase zero diagnostics (parsercore_phase0_*.go), not the parse",
}

// adapterFile holds the root's own scanner wrapper. Its probes ask about the
// scanner inside the adapter. The recorder wraps the adapter, not that scanner.
const adapterFile = "external_scanner_adapter.go"

func TestRecorderImplementsEveryMirroredProbe(t *testing.T) {
	typ := reflect.TypeOf(&identityRecorder{})
	for key := range mirroredProbes {
		if _, ok := typ.MethodByName(key); !ok {
			t.Errorf("%s is listed as mirrored but the recorder has no such method", key)
		}
	}
	for key := range onlyIncrementalProbes {
		if _, ok := typ.MethodByName(key); ok {
			t.Errorf("%s is listed as unmirrored but the recorder implements it; move it to mirroredProbes", key)
		}
	}
}

// probeProblems returns one message for each probe that the recorder neither
// answers like the scanner nor is known to be asked only by incremental reuse.
func probeProblems(probes []rootProbe) []string {
	var problems []string
	for _, p := range probes {
		if p.file == adapterFile {
			continue
		}
		if files, mirrored := mirroredProbes[p.key]; mirrored {
			if !slices.Contains(files, p.file) {
				problems = append(problems, fmt.Sprintf("%v: the recorder answers %s for every scanner, and this file is not reviewed. "+
					"Check that the code treats an absent interface and a false answer alike, then add the file to mirroredProbes", p, p.key))
			}
			continue
		}
		if _, incremental := onlyIncrementalProbes[p.key]; !incremental {
			problems = append(problems, fmt.Sprintf("%v: the root asks a scanner this question and the recorder neither answers it nor is it known to be incremental-only. "+
				"If a full parse asks it, mirror it in the recorder; if only incremental reuse does, add it to onlyIncrementalProbes with the reason", p))
		}
	}
	return problems
}

func TestProbeProblemsNameWhatTheRecorderDoesNotCover(t *testing.T) {
	covered := []rootProbe{
		{key: "SupportsIncrementalReuse", file: "parser.go", line: 1},
		{key: "SupportsCheckpointedNonLeafReuse", file: "incremental.go", line: 2},
		{key: "type incr.CheckpointReadScanner", file: "incremental.go", line: 3},
		{key: "NewQuestion", file: adapterFile, line: 4},
	}
	if got := probeProblems(covered); len(got) != 0 {
		t.Fatalf("problems = %q, want none for probes the tables cover", got)
	}
	got := probeProblems([]rootProbe{
		{key: "NewQuestion", file: "parser.go", line: 10},
		{key: "SupportsIncrementalReuse", file: "new_path.go", line: 20},
	})
	if len(got) != 2 || !strings.Contains(got[0], "parser.go:10 (NewQuestion)") || !strings.Contains(got[0], "neither answers it") ||
		!strings.Contains(got[1], "new_path.go:20") || !strings.Contains(got[1], "not reviewed") {
		t.Fatalf("problems = %q, want an unknown question and an unreviewed file", got)
	}
}

func TestRootScannerProbesAreMirroredOrOnlyIncremental(t *testing.T) {
	probes, err := scannerProbes(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	// An empty or short result would mean the scan lost the root, not that the
	// root asks nothing.
	if len(probes) < 20 {
		t.Fatalf("found %d probes in the root, want the scan to see the parser's checks", len(probes))
	}
	for _, problem := range probeProblems(probes) {
		t.Error(problem)
	}
}
