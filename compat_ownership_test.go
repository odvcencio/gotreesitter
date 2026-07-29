package gotreesitter

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const resultCompatOwnershipRegistryPath = "testdata/result_compat_ownership_v1.json"

var resultCompatRetiredCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type resultCompatOwnershipRegistry struct {
	Schema          string                       `json:"schema"`
	SourceOfTruth   bool                         `json:"source_of_truth"`
	Denominator     resultCompatDenominator      `json:"denominator"`
	OwnerCategories []string                     `json:"owner_categories"`
	Entries         []resultCompatOwnershipEntry `json:"entries"`
}

type resultCompatDenominator struct {
	DispatcherArms            int `json:"dispatcher_arms"`
	DispatcherLanguages       int `json:"dispatcher_languages"`
	DispatcherPredicates      int `json:"dispatcher_predicates"`
	GenericPasses             int `json:"generic_passes"`
	PostFinalizationArms      int `json:"post_finalization_arms"`
	PostFinalizationLanguages int `json:"post_finalization_languages"`
}

type resultCompatOwnershipEntry struct {
	ID                  string                    `json:"id"`
	Kind                string                    `json:"kind"`
	Functions           []string                  `json:"functions"`
	Files               []string                  `json:"files"`
	Languages           []string                  `json:"languages"`
	MatchPredicate      string                    `json:"match_predicate,omitempty"`
	EntrypointCalls     []string                  `json:"entrypoint_calls,omitempty"`
	SwitchArms          []resultCompatSwitchArm   `json:"switch_arms,omitempty"`
	Purpose             string                    `json:"purpose"`
	AuthoritativeOwner  string                    `json:"authoritative_owner"`
	Witnesses           []string                  `json:"witnesses"`
	RetirementCondition string                    `json:"retirement_condition"`
	RouteCoverage       resultCompatRouteCoverage `json:"route_coverage"`
	Status              string                    `json:"status"`
	ReceiptRefs         []string                  `json:"receipt_refs,omitempty"`
	RetiredCommit       string                    `json:"retired_commit,omitempty"`
}

type resultCompatSwitchArm struct {
	Languages []string `json:"languages"`
	Functions []string `json:"functions"`
}

type resultCompatRouteCoverage struct {
	Production  string `json:"production"`
	Compact     string `json:"compact"`
	Forest      string `json:"forest"`
	Incremental string `json:"incremental"`
	COracle     string `json:"c_oracle"`
}

func TestResultCompatibilityOwnershipRegistry(t *testing.T) {
	registry := loadResultCompatOwnershipRegistry(t)

	allowedOwners := map[string]bool{
		"scheduler_action_semantics":    true,
		"derivation_election_selection": true,
		"materialization":               true,
		"scanner_checkpoint_state":      true,
		"incremental_edit_reuse":        true,
		"public_compatibility":          true,
	}
	if got, want := sortedStrings(registry.OwnerCategories), sortedMapKeys(allowedOwners); !equalStrings(got, want) {
		t.Fatalf("owner_categories = %v, want %v", got, want)
	}

	entriesByKind := make(map[string][]resultCompatOwnershipEntry)
	seenIDs := make(map[string]bool)
	allowedKinds := map[string]bool{
		"dispatcher_arm":       true,
		"dispatcher_predicate": true,
		"generic_pass":         true,
		"second_pass_fixpoint": true,
	}
	for _, entry := range registry.Entries {
		if entry.ID == "" || seenIDs[entry.ID] {
			t.Fatalf("registry entry has empty or duplicate id %q", entry.ID)
		}
		seenIDs[entry.ID] = true
		if !allowedOwners[entry.AuthoritativeOwner] {
			t.Errorf("%s authoritative_owner = %q, want registered owner category", entry.ID, entry.AuthoritativeOwner)
		}
		if !allowedKinds[entry.Kind] {
			t.Errorf("%s has unsupported kind %q", entry.ID, entry.Kind)
		}
		if entry.Purpose == "" || entry.RetirementCondition == "" {
			t.Errorf("%s must declare purpose and retirement_condition", entry.ID)
		}
		if len(entry.Functions) == 0 || len(entry.Files) == 0 || len(entry.Languages) == 0 || len(entry.Witnesses) == 0 {
			t.Errorf("%s must declare functions, files, languages, and witnesses", entry.ID)
		}
		if ownershipHasDuplicateStrings(entry.Functions) {
			t.Errorf("%s functions must be a duplicate-free set: %v", entry.ID, entry.Functions)
		}
		assertOwnershipStatusAndRoutes(t, entry)
		switch entry.Status {
		case "live":
			if len(entry.Functions) == 0 {
				t.Errorf("%s is live but has no functions", entry.ID)
			}
		case "retired":
			if !resultCompatRetiredCommitPattern.MatchString(entry.RetiredCommit) {
				t.Errorf("%s retired_commit = %q, want a lowercase 40-character commit hash", entry.ID, entry.RetiredCommit)
			}
			if len(entry.ReceiptRefs) == 0 {
				t.Errorf("%s is retired but lacks receipt_refs", entry.ID)
			}
		default:
			t.Errorf("%s has unsupported status %q", entry.ID, entry.Status)
		}
		if entry.Status != "retired" {
			for _, path := range append(append([]string{}, entry.Files...), entry.Witnesses...) {
				if _, err := os.Stat(filepath.Clean(path)); err != nil {
					t.Errorf("%s references missing path %q: %v", entry.ID, path, err)
				}
			}
		}
		if entry.Status == "live" {
			assertRegistryFunctionsExist(t, entry)
		}
		if entry.Status != "retired" {
			entriesByKind[entry.Kind] = append(entriesByKind[entry.Kind], entry)
		}
	}

	assertDispatcherRegistry(t, registry.Denominator, entriesByKind)
	assertGenericPassRegistry(t, registry.Denominator, entriesByKind)
	assertPostFinalizationRegistry(t, registry.Denominator, entriesByKind)
}

func loadResultCompatOwnershipRegistry(t *testing.T) resultCompatOwnershipRegistry {
	t.Helper()
	f, err := os.Open(resultCompatOwnershipRegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var registry resultCompatOwnershipRegistry
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		t.Fatalf("decode %s: %v", resultCompatOwnershipRegistryPath, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("%s has trailing JSON data: %v", resultCompatOwnershipRegistryPath, err)
	}
	if registry.Schema != "gotreesitter/result-compat-ownership/v1" || !registry.SourceOfTruth {
		t.Fatalf("registry schema/source_of_truth = %q/%t", registry.Schema, registry.SourceOfTruth)
	}
	return registry
}

func assertOwnershipStatusAndRoutes(t *testing.T, entry resultCompatOwnershipEntry) {
	t.Helper()
	allowedStatuses := map[string]map[string]bool{
		"dispatcher_arm":       {"live": true, "retired": true},
		"dispatcher_predicate": {"live": true, "retired": true},
		"generic_pass":         {"live": true, "retired": true},
		"second_pass_fixpoint": {"live": true, "retired": true},
	}
	if !allowedStatuses[entry.Kind][entry.Status] {
		t.Errorf("%s kind %q cannot have status %q", entry.ID, entry.Kind, entry.Status)
		return
	}
	if entry.Status == "live" && entry.RetiredCommit != "" {
		t.Errorf("%s is live but declares retired_commit", entry.ID)
	}

	var want resultCompatRouteCoverage
	if entry.Status == "retired" {
		switch entry.RouteCoverage.Compact {
		case "retired_exact_receipt", "retired_exact_or_fail_closed_receipt":
		default:
			t.Errorf("%s has unsupported retired compact receipt %q", entry.ID, entry.RouteCoverage.Compact)
		}
		switch entry.RouteCoverage.Forest {
		case "retired_exact_receipt", "retired_exact_or_fail_closed_receipt":
		default:
			t.Errorf("%s has unsupported retired forest receipt %q", entry.ID, entry.RouteCoverage.Forest)
		}
		want = resultCompatRouteCoverage{
			Production:  "retired_exact_receipt",
			Compact:     entry.RouteCoverage.Compact,
			Forest:      entry.RouteCoverage.Forest,
			Incremental: "retired_exact_receipt",
			COracle:     entry.RouteCoverage.COracle,
		}
		switch entry.RouteCoverage.COracle {
		case "retired_exact_receipt", "retired_known_divergence_receipt":
		default:
			t.Errorf("%s has unsupported retired C-oracle receipt %q", entry.ID, entry.RouteCoverage.COracle)
		}
	} else {
		switch entry.Kind {
		case "dispatcher_arm", "dispatcher_predicate", "generic_pass":
			want = resultCompatRouteCoverage{
				Production:  "shared_result_compatibility_tail",
				Compact:     "shared_result_compatibility_tail",
				Forest:      "shared_result_compatibility_tail",
				Incremental: "shared_result_compatibility_tail",
				COracle:     "curated_single_grammar_parity",
			}
		case "second_pass_fixpoint":
			want = resultCompatRouteCoverage{
				Production:  "post_finalization_fixpoint",
				Compact:     "post_finalization_fixpoint",
				Forest:      "post_finalization_fixpoint",
				Incremental: "post_finalization_fixpoint",
				COracle:     "language_witnesses_required",
			}
		default:
			return
		}
	}
	if entry.RouteCoverage != want {
		t.Errorf("%s route_coverage = %+v, want closed-vocabulary semantics %+v", entry.ID, entry.RouteCoverage, want)
	}

	switch entry.Kind {
	case "dispatcher_predicate":
		if entry.MatchPredicate == "" || len(entry.EntrypointCalls) != 0 || len(entry.SwitchArms) != 0 {
			t.Errorf("%s dispatcher predicate must declare only match_predicate kind metadata", entry.ID)
		}
	case "second_pass_fixpoint":
		if entry.MatchPredicate != "" || len(entry.EntrypointCalls) == 0 || len(entry.SwitchArms) == 0 {
			t.Errorf("%s second-pass fixpoint must declare entrypoint_calls and switch_arms", entry.ID)
		}
	case "dispatcher_arm", "generic_pass":
		if entry.MatchPredicate != "" || len(entry.EntrypointCalls) != 0 || len(entry.SwitchArms) != 0 {
			t.Errorf("%s kind %q has incompatible predicate/fixpoint metadata", entry.ID, entry.Kind)
		}
	}
}

func assertRegistryFunctionsExist(t *testing.T, entry resultCompatOwnershipEntry) {
	t.Helper()
	declared := make(map[string]bool)
	fset := token.NewFileSet()
	for _, path := range entry.Files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Errorf("%s parse %s: %v", entry.ID, path, err)
			continue
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				declared[fn.Name.Name] = true
			}
		}
	}
	for _, function := range entry.Functions {
		if !declared[function] {
			t.Errorf("%s live function %s is absent from registered files %v", entry.ID, function, entry.Files)
		}
	}
}

func assertDispatcherRegistry(t *testing.T, denominator resultCompatDenominator, byKind map[string][]resultCompatOwnershipEntry) {
	t.Helper()
	file := parseOwnershipGoFile(t, "parser_result_compat.go")
	dispatcher := findOwnershipFunction(t, file, "runLanguageResultCompatibility")
	var languageSwitch *ast.SwitchStmt
	ast.Inspect(dispatcher.Body, func(node ast.Node) bool {
		if languageSwitch != nil {
			return false
		}
		if sw, ok := node.(*ast.SwitchStmt); ok {
			languageSwitch = sw
			return false
		}
		return true
	})
	if languageSwitch == nil {
		t.Fatal("runLanguageResultCompatibility has no language switch")
	}

	sourceArms := make(map[string][]string)
	sourceLanguages := make(map[string]bool)
	for _, statement := range languageSwitch.Body.List {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		var languages []string
		for _, expression := range clause.List {
			literal, ok := expression.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatalf("dispatcher case has non-string expression %T", expression)
			}
			language, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			if sourceLanguages[language] {
				t.Fatalf("dispatcher language %q appears in multiple arms", language)
			}
			sourceLanguages[language] = true
			languages = append(languages, language)
		}
		if len(languages) == 0 {
			continue
		}
		key := ownershipLanguageKey(languages)
		sourceArms[key] = ownershipNormalizeCalls(clause)
	}

	registryArms := make(map[string][]string)
	for _, entry := range byKind["dispatcher_arm"] {
		if !strings.HasPrefix(entry.ID, "dispatch.") {
			t.Errorf("%s dispatcher arm id must have dispatch. prefix", entry.ID)
		}
		key := ownershipLanguageKey(entry.Languages)
		if _, exists := registryArms[key]; exists {
			t.Fatalf("duplicate dispatcher registry languages %q", key)
		}
		registryArms[key] = sortedStrings(entry.Functions)
	}
	if got, want := len(sourceArms), denominator.DispatcherArms; got != want {
		t.Errorf("source dispatcher arms = %d, frozen denominator = %d", got, want)
	}
	if got, want := len(sourceLanguages), denominator.DispatcherLanguages; got != want {
		t.Errorf("source dispatcher languages = %d, frozen denominator = %d", got, want)
	}
	if got, want := len(registryArms), denominator.DispatcherArms; got != want {
		t.Errorf("registered dispatcher arms = %d, frozen denominator = %d", got, want)
	}
	for key, sourceFunctions := range sourceArms {
		registryFunctions, ok := registryArms[key]
		if !ok {
			t.Errorf("dispatcher arm %q is not registered", key)
			continue
		}
		if sourceFunctions = sortedStrings(sourceFunctions); !equalStrings(sourceFunctions, registryFunctions) {
			t.Errorf("dispatcher arm %q functions = %v, registry = %v", key, sourceFunctions, registryFunctions)
		}
	}
	for key := range registryArms {
		if _, ok := sourceArms[key]; !ok {
			t.Errorf("registered dispatcher arm %q no longer exists", key)
		}
	}

	sourcePredicates := ownershipTopLevelNormalizePredicates(t, dispatcher)
	predicates := byKind["dispatcher_predicate"]
	if got, want := len(sourcePredicates), denominator.DispatcherPredicates; got != want {
		t.Errorf("source dispatcher predicates = %d, frozen denominator = %d", got, want)
	}
	if got, want := len(predicates), denominator.DispatcherPredicates; got != want {
		t.Fatalf("registered dispatcher predicates = %d, frozen denominator = %d", got, want)
	}
	registryPredicates := make(map[string]resultCompatOwnershipEntry)
	for _, entry := range predicates {
		if entry.MatchPredicate == "" {
			t.Errorf("%s must declare match_predicate", entry.ID)
			continue
		}
		if _, exists := registryPredicates[entry.MatchPredicate]; exists {
			t.Fatalf("duplicate dispatcher predicate %q", entry.MatchPredicate)
		}
		predicateFunction := findOwnershipFunctionInFiles(t, entry.Files, entry.MatchPredicate)
		sourceLanguages := ownershipCanonicalPredicateLanguages(t, entry.MatchPredicate, predicateFunction)
		if got, want := sourceLanguages, sortedStrings(entry.Languages); !equalStrings(got, want) {
			t.Errorf("%s predicate languages = %v from AST, registry = %v", entry.ID, got, want)
		}
		registryPredicates[entry.MatchPredicate] = entry
	}
	for predicate, sourceFunctions := range sourcePredicates {
		entry, ok := registryPredicates[predicate]
		if !ok {
			t.Errorf("dispatcher predicate %q is not registered", predicate)
			continue
		}
		registryFunctions := sortedStrings(entry.Functions)
		if sourceFunctions = sortedStrings(sourceFunctions); !equalStrings(sourceFunctions, registryFunctions) {
			t.Errorf("dispatcher predicate %q functions = %v, registry = %v", predicate, sourceFunctions, registryFunctions)
		}
	}
	for predicate := range registryPredicates {
		if _, ok := sourcePredicates[predicate]; !ok {
			t.Errorf("registered dispatcher predicate %q no longer exists", predicate)
		}
	}
}

func assertGenericPassRegistry(t *testing.T, denominator resultCompatDenominator, byKind map[string][]resultCompatOwnershipEntry) {
	t.Helper()
	file := parseOwnershipGoFile(t, "parser_result_compat.go")
	fn := findOwnershipFunction(t, file, "normalizeResultCompatibility")
	sourceFunctions := sortedStrings(ownershipNormalizeCalls(fn.Body))

	var registryFunctions []string
	for _, entry := range byKind["generic_pass"] {
		registryFunctions = append(registryFunctions, entry.Functions...)
	}
	registryFunctions = sortedStrings(registryFunctions)
	if got, want := len(byKind["generic_pass"]), denominator.GenericPasses; got != want {
		t.Errorf("registered generic passes = %d, frozen denominator = %d", got, want)
	}
	if got, want := len(sourceFunctions), denominator.GenericPasses; got != want {
		t.Errorf("source generic normalization calls = %d, frozen denominator = %d (%v)", got, want, sourceFunctions)
	}
	if !equalStrings(sourceFunctions, registryFunctions) {
		t.Errorf("generic normalization calls = %v, registry = %v", sourceFunctions, registryFunctions)
	}
}

func assertPostFinalizationRegistry(t *testing.T, denominator resultCompatDenominator, byKind map[string][]resultCompatOwnershipEntry) {
	t.Helper()
	entries := byKind["second_pass_fixpoint"]
	if denominator.PostFinalizationArms == 0 && denominator.PostFinalizationLanguages == 0 {
		if len(entries) != 0 {
			t.Fatalf("live second_pass_fixpoint entries = %d, want 0", len(entries))
		}
		file := parseOwnershipGoFile(t, "parser_api.go")
		for _, removed := range []string{"normalizeReturnedTree", "normalizePostFinalizationReturnedTree"} {
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if ok && function.Name.Name == removed &&
					(removed != "normalizeReturnedTree" || function.Recv == nil) {
					t.Errorf("retired post-finalization function %s still exists", removed)
				}
			}
		}
		return
	}
	if denominator.PostFinalizationArms == 0 || denominator.PostFinalizationLanguages == 0 {
		t.Fatalf(
			"post-finalization denominator is inconsistent: arms=%d languages=%d",
			denominator.PostFinalizationArms,
			denominator.PostFinalizationLanguages,
		)
	}
	if len(entries) != 1 {
		t.Fatalf("live second_pass_fixpoint entries = %d, want 1", len(entries))
	}
	entry := entries[0]

	file := parseOwnershipGoFile(t, "parser_api.go")
	entrypoint := findOwnershipFunction(t, file, "normalizePostFinalizationReturnedTree")
	if got, want := sortedStrings(ownershipNormalizeCallOccurrences(entrypoint.Body)), sortedStrings(entry.EntrypointCalls); !equalStrings(got, want) {
		t.Errorf("post-finalization entrypoint normalize calls = %v, registry = %v", got, want)
	}
	if len(entry.EntrypointCalls) != 1 {
		t.Fatalf("%s entrypoint_calls = %v, want exactly one switch helper", entry.ID, entry.EntrypointCalls)
	}
	switchHelper := findOwnershipFunction(t, file, entry.EntrypointCalls[0])
	languageSwitch := ownershipFirstSwitch(t, switchHelper, entry.EntrypointCalls[0])
	if !ownershipIsLanguageNameSelector(languageSwitch.Tag) {
		t.Errorf("%s switch tag is %T, want exact lang.Name selector", entry.EntrypointCalls[0], languageSwitch.Tag)
	}
	sourceArms, sourceLanguages := ownershipSwitchNormalizationArms(t, languageSwitch, "post-finalization")

	registryArms := make(map[string][]string)
	var registrySwitchFunctions []string
	for _, arm := range entry.SwitchArms {
		if len(arm.Languages) == 0 || len(arm.Functions) == 0 ||
			ownershipHasDuplicateStrings(arm.Languages) || ownershipHasDuplicateStrings(arm.Functions) {
			t.Errorf("%s switch arm must have duplicate-free languages and functions: %+v", entry.ID, arm)
		}
		key := ownershipLanguageKey(arm.Languages)
		if _, exists := registryArms[key]; exists {
			t.Fatalf("%s has duplicate switch arm %q", entry.ID, key)
		}
		registryArms[key] = sortedStrings(arm.Functions)
		registrySwitchFunctions = append(registrySwitchFunctions, arm.Functions...)
	}
	if got, want := len(sourceArms), denominator.PostFinalizationArms; got != want {
		t.Errorf("post-finalization source arms = %d, frozen denominator = %d", got, want)
	}
	if got, want := len(sourceLanguages), denominator.PostFinalizationLanguages; got != want {
		t.Errorf("post-finalization source languages = %d, frozen denominator = %d", got, want)
	}
	if got, want := len(registryArms), denominator.PostFinalizationArms; got != want {
		t.Errorf("post-finalization registered arms = %d, frozen denominator = %d", got, want)
	}
	for key, sourceFunctions := range sourceArms {
		registryFunctions, ok := registryArms[key]
		if !ok {
			t.Errorf("post-finalization arm %q is not registered", key)
			continue
		}
		if sourceFunctions = sortedStrings(sourceFunctions); !equalStrings(sourceFunctions, registryFunctions) {
			t.Errorf("post-finalization arm %q functions = %v, registry = %v", key, sourceFunctions, registryFunctions)
		}
	}
	for key := range registryArms {
		if _, ok := sourceArms[key]; !ok {
			t.Errorf("registered post-finalization arm %q no longer exists", key)
		}
	}
	if got, want := sortedMapKeys(sourceLanguages), sortedStrings(entry.Languages); !equalStrings(got, want) {
		t.Errorf("post-finalization languages = %v, registry entry languages = %v", got, want)
	}
	registrySwitchFunctions = sortedStrings(registrySwitchFunctions)
	if got := sortedStrings(ownershipNormalizeCallOccurrences(switchHelper.Body)); !equalStrings(got, registrySwitchFunctions) {
		t.Errorf("post-finalization helper normalize call occurrences = %v, registered non-default clause calls = %v", got, registrySwitchFunctions)
	}
	allRegisteredFunctions := append([]string{"normalizePostFinalizationReturnedTree"}, entry.EntrypointCalls...)
	allRegisteredFunctions = append(allRegisteredFunctions, registrySwitchFunctions...)
	if got, want := sortedStrings(entry.Functions), sortedStrings(allRegisteredFunctions); !equalStrings(got, want) {
		t.Errorf("%s functions = %v, exact entrypoint/switch function set = %v", entry.ID, got, want)
	}
}

func ownershipFirstSwitch(t *testing.T, fn *ast.FuncDecl, label string) *ast.SwitchStmt {
	t.Helper()
	var found *ast.SwitchStmt
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		if statement, ok := node.(*ast.SwitchStmt); ok {
			found = statement
			return false
		}
		return true
	})
	if found == nil {
		t.Fatalf("%s has no switch", label)
	}
	return found
}

func ownershipSwitchNormalizationArms(t *testing.T, languageSwitch *ast.SwitchStmt, label string) (map[string][]string, map[string]bool) {
	t.Helper()
	arms := make(map[string][]string)
	languages := make(map[string]bool)
	for _, statement := range languageSwitch.Body.List {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		var armLanguages []string
		for _, expression := range clause.List {
			literal, ok := expression.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatalf("%s switch case has non-string expression %T", label, expression)
			}
			language, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			if languages[language] {
				t.Fatalf("%s language %q appears in multiple arms", label, language)
			}
			languages[language] = true
			armLanguages = append(armLanguages, language)
		}
		if len(armLanguages) == 0 {
			if calls := ownershipNormalizeCallOccurrences(clause); len(calls) != 0 {
				t.Errorf("%s default clause has unregistered normalize call occurrences %v", label, calls)
			}
			continue
		}
		arms[ownershipLanguageKey(armLanguages)] = sortedStrings(ownershipNormalizeCallOccurrences(clause))
	}
	return arms, languages
}

func parseOwnershipGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func findOwnershipFunction(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("function %s not found", name)
	return nil
}

func findOwnershipFunctionInFiles(t *testing.T, paths []string, name string) *ast.FuncDecl {
	t.Helper()
	for _, path := range paths {
		file := parseOwnershipGoFile(t, path)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
				return fn
			}
		}
	}
	t.Fatalf("function %s not found in registered files %v", name, paths)
	return nil
}

func ownershipCanonicalPredicateLanguages(t *testing.T, predicate string, fn *ast.FuncDecl) []string {
	t.Helper()
	if predicate != "isCobolLanguage" {
		t.Errorf("dispatcher predicate %q lacks a canonical AST validator", predicate)
		return nil
	}
	if len(fn.Body.List) != 1 {
		t.Errorf("%s body has %d statements, want one canonical return", predicate, len(fn.Body.List))
		return nil
	}
	returnStatement, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returnStatement.Results) != 1 {
		t.Errorf("%s body must be exactly one single-result return", predicate)
		return nil
	}
	guarded, ok := returnStatement.Results[0].(*ast.BinaryExpr)
	if !ok || guarded.Op != token.LAND || !ownershipIsLangNonNilGuard(guarded.X) {
		t.Errorf("%s return must start with exact lang != nil && guard", predicate)
		return nil
	}
	parenthesized, ok := guarded.Y.(*ast.ParenExpr)
	if !ok {
		t.Errorf("%s guarded language alternatives must be parenthesized", predicate)
		return nil
	}
	alternatives, ok := parenthesized.X.(*ast.BinaryExpr)
	if !ok || alternatives.Op != token.LOR {
		t.Errorf("%s parenthesized predicate must be exactly one OR", predicate)
		return nil
	}
	left, leftOK := ownershipExactLanguageNameEquality(alternatives.X)
	right, rightOK := ownershipExactLanguageNameEquality(alternatives.Y)
	if !leftOK || !rightOK {
		t.Errorf("%s OR operands must be exact lang.Name == string equalities", predicate)
		return nil
	}
	languages := sortedStrings([]string{left, right})
	if want := []string{"COBOL", "cobol"}; !equalStrings(languages, want) {
		t.Errorf("%s exact equality languages = %v, want %v", predicate, languages, want)
	}
	return languages
}

func ownershipIsLangNonNilGuard(expression ast.Expr) bool {
	guard, ok := expression.(*ast.BinaryExpr)
	if !ok || guard.Op != token.NEQ {
		return false
	}
	left, leftOK := guard.X.(*ast.Ident)
	right, rightOK := guard.Y.(*ast.Ident)
	return leftOK && rightOK && left.Name == "lang" && right.Name == "nil"
}

func ownershipExactLanguageNameEquality(expression ast.Expr) (string, bool) {
	equality, ok := expression.(*ast.BinaryExpr)
	if !ok || equality.Op != token.EQL || !ownershipIsLanguageNameSelector(equality.X) {
		return "", false
	}
	literal, ok := equality.Y.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	language, err := strconv.Unquote(literal.Value)
	return language, err == nil
}

func ownershipIsLanguageNameSelector(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Name" {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Name == "lang"
}

func ownershipTopLevelNormalizePredicates(t *testing.T, fn *ast.FuncDecl) map[string][]string {
	t.Helper()
	result := make(map[string][]string)
	for _, statement := range fn.Body.List {
		conditional, ok := statement.(*ast.IfStmt)
		if !ok {
			continue
		}
		functions := ownershipNormalizeCalls(conditional.Body)
		if len(functions) == 0 {
			continue
		}
		var predicates []string
		ast.Inspect(conditional.Cond, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if identifier, ok := call.Fun.(*ast.Ident); ok {
				predicates = append(predicates, identifier.Name)
			}
			return true
		})
		if len(predicates) != 1 {
			t.Errorf("normalization-bearing top-level predicate has callable predicates %v, want exactly one registered predicate", predicates)
			continue
		}
		if _, exists := result[predicates[0]]; exists {
			t.Errorf("normalization-bearing top-level predicate %q appears more than once", predicates[0])
			continue
		}
		result[predicates[0]] = functions
	}
	return result
}

func ownershipNormalizeCalls(root ast.Node) []string {
	seen := make(map[string]bool)
	for _, name := range ownershipNormalizeCallOccurrences(root) {
		seen[name] = true
	}
	return sortedMapKeys(seen)
}

func ownershipNormalizeCallOccurrences(root ast.Node) []string {
	var calls []string
	ast.Inspect(root, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch function := call.Fun.(type) {
		case *ast.Ident:
			name = function.Name
		case *ast.SelectorExpr:
			name = function.Sel.Name
		}
		if strings.HasPrefix(name, "normalize") {
			calls = append(calls, name)
		}
		return true
	})
	return calls
}

func ownershipLanguageKey(languages []string) string {
	return strings.Join(sortedStrings(languages), ",")
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func sortedMapKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func ownershipHasDuplicateStrings(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}
