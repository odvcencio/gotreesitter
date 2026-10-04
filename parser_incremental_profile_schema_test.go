package gotreesitter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestIncrementalFallbackProfileIncludesBothAttempts(t *testing.T) {
	timing := &incrementalParseTiming{
		totalNanos:                     40,
		reuseNanos:                     5,
		reusedSubtrees:                 2,
		reusedBytes:                    100,
		tokenInvariantDependencyChecks: 3,
		newNodes:                       23,
		acceptedErrorRetryAttempts:     1,
		acceptedErrorRetryAdopted:      true,
		acceptedErrorRetryMergePerKey:  3,
		acceptedErrorRetryCause:        IncrementalRetryCauseAcceptedErrorBaseMerge,
		oldTreeReuseRoute:              true,
		tokensConsumed:                 31,
		arenaBytesAllocated:            150,
		arenaBaselineBytes:             120,
		scratchBytesAllocated:          230,
		scratchBaselineBytes:           180,
		parserLoopNanos:                17,
		maxStacksSeen:                  5,
		entryScratchPeak:               8,
	}
	fallback := &Tree{parseRuntime: &ParseRuntime{
		StopReason:            ParseStopAccepted,
		ExpectedEOFByte:       128,
		LastTokenEndByte:      128,
		TokensConsumed:        11,
		NodesAllocated:        7,
		ArenaBytesAllocated:   70,
		ArenaBaselineBytes:    50,
		ScratchBytesAllocated: 110,
		ScratchBaselineBytes:  90,
		ParserLoopNanos:       19,
		MaxStacksSeen:         9,
		EntryScratchPeak:      6,
	}}

	timing.recordFreshFallback(fallback, 13, forestRecoveryFallbackReuseReason)
	got := timing.toProfile()
	if got.ReuseCursorNanos != 5 || got.ReparseNanos != 48 {
		t.Fatalf("operation time = reuse %d, reparse %d; want 5 and 48", got.ReuseCursorNanos, got.ReparseNanos)
	}
	if got.TokensConsumed != 42 || got.NewNodesAllocated != 30 || got.TokenInvariantDependencyChecks != 3 {
		t.Fatalf("operation work did not include both attempts: %+v", got)
	}
	if got.ArenaBytesAllocated != 220 || got.ArenaBaselineBytes != 170 ||
		got.ScratchBytesAllocated != 340 || got.ScratchBaselineBytes != 270 || got.ParserLoopNanos != 36 {
		t.Fatalf("operation storage or phase totals did not include both attempts: %+v", got)
	}
	if got.MaxStacksSeen != 9 || got.EntryScratchPeak != 8 {
		t.Fatalf("operation peaks = stacks %d, scratch %d; want 9 and 8", got.MaxStacksSeen, got.EntryScratchPeak)
	}
	if got.ReusedSubtrees != 0 || got.ReusedBytes != 0 || !got.ReuseUnsupported ||
		got.ReuseUnsupportedReason != forestRecoveryFallbackReuseReason || got.OldTreeReuseRoute {
		t.Fatalf("selected fallback fields retained the abandoned attempt: %+v", got)
	}
	if got.StopReason != ParseStopAccepted || got.LastTokenEndByte != 128 || got.ExpectedEOFByte != 128 {
		t.Fatalf("selected parse boundary did not come from the fallback: %+v", got)
	}
	if got.AcceptedErrorRetryAttempts != 1 || !got.AcceptedErrorRetryAdopted ||
		got.AcceptedErrorRetryMergePerKey != 3 || got.AcceptedErrorRetryCause != IncrementalRetryCauseAcceptedErrorBaseMerge {
		t.Fatalf("operation retry fields changed after the fallback: %+v", got)
	}
}

func TestIncrementalMemoryBudgetRetryProfileIncludesBothAttempts(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	first := &Tree{
		language: lang,
		root:     &Node{endByte: 1, flags: nodeFlagHasError},
		parseRuntime: &ParseRuntime{
			StopReason:                   ParseStopMemoryBudget,
			ExpectedEOFByte:              1,
			LastTokenEndByte:             1,
			IncrementalOldTreeReuseRoute: true,
		},
	}
	timing := &incrementalParseTiming{
		totalNanos:          40,
		reuseNanos:          5,
		reusedSubtrees:      2,
		reusedBytes:         100,
		newNodes:            23,
		oldTreeReuseRoute:   true,
		tokensConsumed:      31,
		arenaBytesAllocated: 150,
		parserLoopNanos:     17,
	}

	result := parser.retryIncrementalMemoryBudgetAsPlainFullWithDFA([]byte("1"), first, timing)
	if result == nil {
		t.Fatal("memory-budget retry returned no tree")
	}
	defer result.Release()
	runtime := result.ParseRuntime()
	profile := timing.toProfile()
	if profile.TokensConsumed != 31+runtime.TokensConsumed ||
		profile.NewNodesAllocated != 23+uint64(runtime.NodesAllocated) ||
		profile.ArenaBytesAllocated != 150+runtime.ArenaBytesAllocated {
		t.Fatalf("retry profile omitted one attempt: profile=%+v runtime=%+v", profile, runtime)
	}
	if profile.ParserLoopNanos < 17 || profile.ReparseNanos < 35 {
		t.Fatalf("retry profile lost first-attempt timing: %+v", profile)
	}
	if !profile.ReuseUnsupported ||
		profile.ReuseUnsupportedReason != "incremental_parse_memory_budget_full_retry" ||
		profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 || profile.OldTreeReuseRoute {
		t.Fatalf("retry profile selected the abandoned attempt: %+v", profile)
	}
	if profile.StopReason != runtime.StopReason || profile.LastTokenEndByte != runtime.LastTokenEndByte ||
		profile.ExpectedEOFByte != runtime.ExpectedEOFByte {
		t.Fatalf("retry profile did not select the fallback boundary: %+v", profile)
	}
}

func TestProfileForestRecoveryFallbackNilTreePreservesAttempt(t *testing.T) {
	timing := &incrementalParseTiming{totalNanos: 40, tokensConsumed: 31, newNodes: 23}
	want := timing.toProfile()
	got := profileForestRecoveryFallback(timing, nil, 13*time.Nanosecond)
	if got != want {
		t.Fatalf("nil fallback changed the existing profile: got %+v, want %+v", got, want)
	}
}

// TestIncrementalParseProfileSchemaComplete keeps every private field in one scope.
// It also checks every public field in the profile conversion.
func TestIncrementalParseProfileSchemaComplete(t *testing.T) {
	parserFile := parseGoFileForProfileSchema(t, "parser.go")
	supportFile := parseGoFileForProfileSchema(t, "parser_incremental_support.go")

	publicFields := profileStructFields(t, parserFile, "IncrementalParseProfile")
	timingFields := profileStructFields(t, parserFile, "incrementalParseTiming")
	profileKeys := profileCompositeKeys(t, supportFile, "toProfile", "IncrementalParseProfile")
	assertProfileFieldSet(t, "toProfile fields", profileKeys, publicFields)

	selectedFields := map[string]bool{
		"reusedSubtrees": true, "reusedBytes": true,
		"reuseUnsupported": true, "reuseUnsupportedReason": true,
		"oldTreeReuseRoute": true, "stopReason": true,
		"lastTokenEndByte": true, "expectedEOFByte": true,
	}
	retryFields := map[string]bool{
		"acceptedErrorRetryAttempts": true, "acceptedErrorRetryAdopted": true,
		"acceptedErrorRetryMergePerKey": true, "acceptedErrorRetryCause": true,
	}
	peakFields := map[string]bool{"maxStacksSeen": true, "entryScratchPeak": true}
	summedFields := profileSameFieldAdditions(t, supportFile, "addAttempt")
	copiedFields := profileSameFieldCopies(t, supportFile, "selectAttempt")
	referencedFields := profileReceiverFields(t, supportFile, "addAttempt")

	for field := range timingFields {
		switch {
		case selectedFields[field]:
			if !copiedFields[field] {
				t.Errorf("selected field %q is not copied by selectAttempt", field)
			}
			if referencedFields["t."+field] || referencedFields["other."+field] {
				t.Errorf("selected field %q participates in operation aggregation", field)
			}
		case retryFields[field]:
			// The retry controller assigns these operation fields after aggregation.
			if referencedFields["t."+field] || referencedFields["other."+field] {
				t.Errorf("retry field %q participates in attempt aggregation", field)
			}
		case peakFields[field]:
			if !referencedFields["t."+field] || !referencedFields["other."+field] {
				t.Errorf("peak field %q is not aggregated by addAttempt", field)
			}
		default:
			if !summedFields[field] {
				t.Errorf("operation field %q is not summed by addAttempt", field)
			}
		}
	}
	for field := range copiedFields {
		if !selectedFields[field] {
			t.Errorf("selectAttempt copies operation field %q", field)
		}
	}
}

func parseGoFileForProfileSchema(t *testing.T, name string) *ast.File {
	t.Helper()
	source, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func profileStructFields(t *testing.T, file *ast.File, typeName string) map[string]bool {
	t.Helper()
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range generic.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s is not a struct", typeName)
			}
			fields := make(map[string]bool)
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					fields[name.Name] = true
				}
			}
			return fields
		}
	}
	t.Fatalf("type %s was not found", typeName)
	return nil
}

func profileFunction(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == name {
			return function
		}
	}
	t.Fatalf("function %s was not found", name)
	return nil
}

func profileCompositeKeys(t *testing.T, file *ast.File, functionName, typeName string) map[string]bool {
	t.Helper()
	keys := make(map[string]bool)
	ast.Inspect(profileFunction(t, file, functionName).Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		identifier, ok := literal.Type.(*ast.Ident)
		if !ok || identifier.Name != typeName {
			return true
		}
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := pair.Key.(*ast.Ident)
			if ok {
				keys[key.Name] = true
			}
		}
		return true
	})
	return keys
}

func profileSameFieldAdditions(t *testing.T, file *ast.File, functionName string) map[string]bool {
	t.Helper()
	fields := make(map[string]bool)
	ast.Inspect(profileFunction(t, file, functionName).Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ADD_ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		leftReceiver, leftField, leftOK := profileSelector(assignment.Lhs[0])
		rightReceiver, rightField, rightOK := profileSelector(assignment.Rhs[0])
		if leftOK && rightOK && leftReceiver == "t" && rightReceiver == "other" && leftField == rightField {
			fields[leftField] = true
		}
		return true
	})
	return fields
}

func profileSameFieldCopies(t *testing.T, file *ast.File, functionName string) map[string]bool {
	t.Helper()
	fields := make(map[string]bool)
	ast.Inspect(profileFunction(t, file, functionName).Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		leftReceiver, leftField, leftOK := profileSelector(assignment.Lhs[0])
		rightReceiver, rightField, rightOK := profileSelector(assignment.Rhs[0])
		if leftOK && rightOK && leftReceiver == "t" && rightReceiver == "other" && leftField == rightField {
			fields[leftField] = true
		}
		return true
	})
	return fields
}

func profileReceiverFields(t *testing.T, file *ast.File, functionName string) map[string]bool {
	t.Helper()
	fields := make(map[string]bool)
	ast.Inspect(profileFunction(t, file, functionName).Body, func(node ast.Node) bool {
		receiver, field, ok := profileSelector(node)
		if ok {
			fields[receiver+"."+field] = true
		}
		return true
	})
	return fields
}

func profileSelector(expression ast.Node) (string, string, bool) {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return receiver.Name, selector.Sel.Name, true
}

func assertProfileFieldSet(t *testing.T, label string, got, want map[string]bool) {
	t.Helper()
	missing := make([]string, 0)
	extra := make([]string, 0)
	for field := range want {
		if !got[field] {
			missing = append(missing, field)
		}
	}
	for field := range got {
		if !want[field] {
			extra = append(extra, field)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("%s mismatch: missing=%v extra=%v", label, missing, extra)
	}
	if gotCount, wantCount := len(got), reflect.TypeOf(IncrementalParseProfile{}).NumField(); gotCount != wantCount {
		t.Fatalf("%s count = %d, want %d", label, gotCount, wantCount)
	}
}
