package gotreesitter

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/sched"
)

func TestAdmissionRoutePrecedence(t *testing.T) {
	previous := admissionCandidateRouteDefault.Load()
	defer admissionCandidateRouteDefault.Store(previous)
	previousList := admissionCandidateLanguageAllowlist
	defer func() { admissionCandidateLanguageAllowlist = previousList }()
	admissionCandidateLanguageAllowlist = map[string]bool{"test": true}
	lang := &Language{Name: "test"}
	for _, global := range []uint32{0, 1, 2} {
		for _, parser := range []admissionRouteMode{admissionRouteFollowDefault, admissionRouteCandidateForced, admissionRouteProductionForced} {
			for _, listed := range []bool{false, true} {
				name := fmt.Sprintf("global=%d/parser=%d/listed=%v", global, parser, listed)
				t.Run(name, func(t *testing.T) {
					admissionCandidateRouteDefault.Store(global)
					admissionCandidateLanguageAllowlist["test"] = listed
					p := &Parser{language: lang, admissionCandidateRoute: parser}
					want := listed && global == 0 || global == 2
					switch parser {
					case admissionRouteCandidateForced:
						want = true
					case admissionRouteProductionForced:
						want = false
					}
					if got := p.admissionCandidateRouteEnabled(); got != want {
						t.Fatalf("enabled=%v, want %v", got, want)
					}
				})
			}
		}
	}
	SetAdmissionCandidateRouteDefault(false)
	if admissionCandidateRouteDefault.Load() != 1 {
		t.Fatal("setter off must be explicit")
	}
	SetAdmissionCandidateRouteDefault(true)
	if admissionCandidateRouteDefault.Load() != 2 {
		t.Fatal("setter on must be explicit")
	}
}

func TestAdmissionEnvironmentPrecedence(t *testing.T) {
	previous, present := os.LookupEnv("GTS_ADMISSION_CANDIDATE")
	defer func() {
		if present {
			os.Setenv("GTS_ADMISSION_CANDIDATE", previous)
		} else {
			os.Unsetenv("GTS_ADMISSION_CANDIDATE")
		}
	}()
	for _, tc := range []struct {
		value string
		want  uint32
	}{
		{"", 0}, {"unknown", 0}, {"0", 1}, {" FALSE ", 1}, {"off", 1}, {"no", 1}, {"1", 2}, {"TRUE", 2}, {"on", 2}, {"yes", 2},
	} {
		os.Setenv("GTS_ADMISSION_CANDIDATE", tc.value)
		if got := admissionCandidateEnvMode(); got != tc.want {
			t.Errorf("%q: mode=%d, want %d", tc.value, got, tc.want)
		}
	}
}

// TestSchedParserModesMatchAdmissionEligibility checks the open capability
// flags against admissionCandidateFullParseEligible. For every parser
// configuration, a fresh request starts the compact engine exactly when
// policy allows it and the request needs no open flag.
func TestSchedParserModesMatchAdmissionEligibility(t *testing.T) {
	previousDefault := admissionCandidateRouteDefault.Load()
	defer admissionCandidateRouteDefault.Store(previousDefault)
	previousForest := glrForestEnabled
	defer SetGLRForestEnabled(previousForest)
	admissionCandidateRouteDefault.Store(2)

	plain := &Language{Name: "sched_modes_test"}
	forest := &Language{Name: "sched_modes_test", WantsForest: true}
	logger := ParserLogger(func(ParserLogType, string) {})
	cases := 0
	for mask := 0; mask < 1<<8; mask++ {
		for _, route := range []admissionRouteMode{admissionRouteFollowDefault, admissionRouteCandidateForced, admissionRouteProductionForced} {
			has := func(bit int) bool { return mask&(1<<bit) != 0 }
			p := &Parser{language: plain, admissionCandidateRoute: route}
			if has(0) {
				p.included = []Range{{StartByte: 0, EndByte: 1}}
			}
			if has(1) {
				p.logger = logger
			}
			if has(2) {
				p.glrTrace = true
			}
			if has(3) {
				p.ambiguityProfile = NewAmbiguityProfile()
			}
			if has(4) {
				p.parseWorkLimits = ParseWorkLimits{IterationLimit: 1}
			}
			if has(5) {
				p.language = forest
			}
			if has(6) {
				p.admissionRouteSuppressed = 1
			}
			SetGLRForestEnabled(has(7))
			for _, dfa := range []bool{true, false} {
				entry := sched.Mode(0)
				if !dfa {
					entry = sched.TokenSource
				}
				req := p.schedCall(entry, nil)
				policy := p.admissionRouteSuppressed == 0 && p.admissionCandidateRouteEnabled()
				eligible := p.admissionCandidateFullParseEligible(nil, dfa)
				if eligible != (policy && sched.Supports(req)) {
					t.Fatalf("mask=%08b route=%d dfa=%v: eligible=%v, policy=%v, modes=%#x, open=%#x",
						mask, route, dfa, eligible, policy, req.All(), req.OpenModes())
				}
				cases++
			}
		}
	}
	t.Logf("checked %d parser configurations", cases)
}

// TestSchedRequestAddsParserAndOldTreeModes checks each mode that a
// schedCall request derives from parser state or from the old tree.
func TestSchedRequestAddsParserAndOldTreeModes(t *testing.T) {
	previousForest := glrForestEnabled
	defer SetGLRForestEnabled(previousForest)
	SetGLRForestEnabled(true)
	lang := &Language{Name: "sched_modes_test"}
	cancel := uint32(0)
	clean := &Tree{language: lang, compactMaterialized: true, edits: []InputEdit{{}}, root: &Node{}}
	for _, tc := range []struct {
		name  string
		p     *Parser
		entry sched.Mode
		old   *Tree
		want  sched.Mode
	}{
		{"nil parser", nil, sched.Strict, nil, sched.Strict},
		{"plain", &Parser{language: lang}, 0, nil, 0},
		{"timeout", &Parser{language: lang, timeoutMicros: 1}, 0, nil, sched.Deadline},
		{"cancellation", &Parser{language: lang, cancellationFlag: &cancel}, 0, nil, sched.Deadline},
		{"included ranges", &Parser{language: lang, included: []Range{{EndByte: 1}}}, 0, nil, sched.IncludedRanges},
		{"work limits", &Parser{language: lang, parseWorkLimits: ParseWorkLimits{NodeLimit: 1}}, 0, nil, sched.WorkLimits},
		{"forest default", &Parser{language: &Language{Name: "sched_modes_test", WantsForest: true}}, 0, nil, sched.ForestRoute},
		{"forest forced candidate", &Parser{language: &Language{Name: "sched_modes_test", WantsForest: true}, admissionCandidateRoute: admissionRouteCandidateForced}, 0, nil, 0},
		{"incremental compact tree", &Parser{language: lang}, sched.Incremental, clean, sched.Incremental},
		{"incremental nil tree", &Parser{language: lang}, sched.Incremental, nil, sched.Incremental | sched.OldTreeReuse},
		{"incremental legacy tree", &Parser{language: lang}, sched.Incremental, &Tree{language: lang, edits: []InputEdit{{}}, root: &Node{}}, sched.Incremental | sched.OldTreeReuse},
		{"incremental no edit", &Parser{language: lang}, sched.Incremental, &Tree{language: lang, compactMaterialized: true, root: &Node{}}, sched.Incremental | sched.OldTreeReuse},
		{"incremental other language", &Parser{language: &Language{Name: "other"}}, sched.Incremental, clean, sched.Incremental | sched.OldTreeReuse},
		{"incremental reuse disabled", &Parser{language: lang}, sched.Incremental, &Tree{language: lang, compactMaterialized: true, incrementalReuseDisabled: true, edits: []InputEdit{{}}, root: &Node{}}, sched.Incremental | sched.OldTreeReuse},
		{"incremental error root", &Parser{language: lang}, sched.Incremental, &Tree{language: lang, compactMaterialized: true, edits: []InputEdit{{}}, root: &Node{flags: nodeFlagHasError}}, sched.Incremental | sched.OldTreeReuse},
		{"incremental stateful scanner", &Parser{language: &Language{Name: "sched_modes_test", ExternalScanner: schedStatefulScanner{}}}, sched.Incremental, nil, sched.Incremental | sched.OldTreeReuse | sched.ScannerStateReuse},
		{"incremental stateless scanner", &Parser{language: &Language{Name: "sched_modes_test", ExternalScanner: schedStatelessScanner{}}}, sched.Incremental, nil, sched.Incremental | sched.OldTreeReuse},
	} {
		if got := tc.p.schedCall(tc.entry, tc.old).All(); got != tc.want {
			t.Errorf("%s: modes=%#x, want %#x", tc.name, got, tc.want)
		}
	}
}

type schedStatefulScanner struct{}

func (schedStatefulScanner) Create() any                           { return nil }
func (schedStatefulScanner) Destroy(any)                           {}
func (schedStatefulScanner) Serialize(any, []byte) int             { return 0 }
func (schedStatefulScanner) Deserialize(any, []byte)               {}
func (schedStatefulScanner) Scan(any, *ExternalLexer, []bool) bool { return false }

type schedStatelessScanner struct{ schedStatefulScanner }

func (schedStatelessScanner) ExternalScannerIsStateless() bool { return true }

// TestPublicParseMethodsCallSchedParse checks the engine seam in source.
// Every exported Parse method of *Parser must call sched.Parse, must carry
// //go:noinline, and must not call another exported Parse method of the same
// parser, so each public call builds one request. Exported Parse methods of
// other types must delegate to an exported Parse method.
func TestPublicParseMethodsCallSchedParse(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var seamed, delegated []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || !fn.Name.IsExported() ||
				!strings.HasPrefix(fn.Name.Name, "Parse") || !schedTakesSource(fn) {
				continue
			}
			recvType, recvName := schedReceiver(fn)
			name := recvType + "." + fn.Name.Name
			if recvType != "Parser" {
				if !schedCallsExportedParse(fn.Body, "") {
					t.Errorf("%s: %s does not delegate to an exported Parse method", fset.Position(fn.Pos()), name)
				}
				delegated = append(delegated, name)
				continue
			}
			if !schedCallsSchedParse(fn.Body, recvName) {
				t.Errorf("%s: %s does not call sched.Parse with %s.schedCall(...)", fset.Position(fn.Pos()), name, recvName)
			}
			if !schedHasNoinline(fn) {
				t.Errorf("%s: %s lacks //go:noinline, so it would inline into callers", fset.Position(fn.Pos()), name)
			}
			if schedCallsExportedParse(fn.Body, recvName) {
				t.Errorf("%s: %s calls another exported Parse method, so one call would build two requests", fset.Position(fn.Pos()), name)
			}
			seamed = append(seamed, name)
		}
	}
	sort.Strings(seamed)
	sort.Strings(delegated)
	if len(seamed) == 0 {
		t.Fatal("found no exported Parse methods on *Parser")
	}
	t.Logf("%d *Parser methods call sched.Parse: %s", len(seamed), strings.Join(seamed, ", "))
	t.Logf("%d methods of other types delegate: %s", len(delegated), strings.Join(delegated, ", "))
}

// schedTakesSource reports whether fn's first parameter is a source slice,
// []byte or []uint16. It separates parse methods from accessors such as
// Tree.ParseRuntime and Parser.ParseWorkLimits.
func schedTakesSource(fn *ast.FuncDecl) bool {
	params := fn.Type.Params.List
	if len(params) == 0 {
		return false
	}
	array, ok := params[0].Type.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	elem, ok := array.Elt.(*ast.Ident)
	return ok && (elem.Name == "byte" || elem.Name == "uint16")
}

func schedHasNoinline(fn *ast.FuncDecl) bool {
	if fn.Doc == nil {
		return false
	}
	for _, comment := range fn.Doc.List {
		if comment.Text == "//go:noinline" {
			return true
		}
	}
	return false
}

func schedReceiver(fn *ast.FuncDecl) (typeName, recvName string) {
	field := fn.Recv.List[0]
	if len(field.Names) > 0 {
		recvName = field.Names[0].Name
	}
	expr := field.Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		typeName = ident.Name
	}
	return typeName, recvName
}

// schedCallsSchedParse reports whether body calls sched.Parse with a request
// built by recvName.schedCall, so the request carries the parser's modes.
func schedCallsSchedParse(body *ast.BlockStmt, recvName string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		fun := call.Fun
		if index, ok := fun.(*ast.IndexExpr); ok {
			fun = index.X
		}
		sel, ok := fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Parse" || len(call.Args) == 0 {
			return !found
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "sched" {
			return !found
		}
		request, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return !found
		}
		builder, ok := request.Fun.(*ast.SelectorExpr)
		if !ok || builder.Sel.Name != "schedCall" {
			return !found
		}
		if recv, ok := builder.X.(*ast.Ident); ok && recv.Name == recvName {
			found = true
		}
		return !found
	})
	return found
}

// schedCallsExportedParse reports whether body calls an exported Parse
// method. With a non-empty recvName it counts only calls on that receiver.
func schedCallsExportedParse(body *ast.BlockStmt, recvName string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !strings.HasPrefix(sel.Sel.Name, "Parse") || !sel.Sel.IsExported() {
			return !found
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "sched" {
			return !found
		}
		if recvName == "" {
			found = true
		} else if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == recvName {
			found = true
		}
		return !found
	})
	return found
}
