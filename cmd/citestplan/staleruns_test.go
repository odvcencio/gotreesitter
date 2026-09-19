package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestExpandPatternExactLiteral(t *testing.T) {
	alts, err := expandPattern(`^TestFoo$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(alts) != 1 || alts[0].kind != altExact || alts[0].text != "TestFoo" {
		t.Fatalf("alts = %+v", alts)
	}
}

func TestExpandPatternAlternation(t *testing.T) {
	alts, err := expandPattern(`^(TestA|TestB)$`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]altKind{}
	for _, a := range alts {
		got[a.text] = a.kind
	}
	want := map[string]altKind{"TestA": altExact, "TestB": altExact}
	if len(got) != len(want) {
		t.Fatalf("alts = %+v", alts)
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Fatalf("alt %q kind = %v, want %v", name, got[name], kind)
		}
	}
}

func TestExpandPatternMultiLineListWithPrefixFragment(t *testing.T) {
	// Mirrors ci.yml's phase0_tagged_suite shape: a mix of exact names and a
	// trailing ".*" prefix fragment joined into one anchored alternation.
	alts, err := expandPattern(`^(TestAdmissionCandidateBoundedRecursiveInsertionCorpus|TestCompactIncrementalExecution.*)$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(alts) != 2 {
		t.Fatalf("alts = %+v", alts)
	}
	byText := map[string]altKind{}
	for _, a := range alts {
		byText[a.text] = a.kind
	}
	if byText["TestAdmissionCandidateBoundedRecursiveInsertionCorpus"] != altExact {
		t.Fatalf("exact alt missing/misclassified: %+v", alts)
	}
	if byText["TestCompactIncrementalExecution"] != altPrefix {
		t.Fatalf("prefix alt missing/misclassified: %+v", alts)
	}
}

func TestExpandPatternNestedGroupAndOptional(t *testing.T) {
	alts, err := expandPattern(`^TestPackage2Scala(Falsifier(PhysicalMergeMinimal|OwnedLexerComposition)|Incremental((EOFComposition)?CleanMissingTransitions))$`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"TestPackage2ScalaFalsifierPhysicalMergeMinimal":                    true,
		"TestPackage2ScalaFalsifierOwnedLexerComposition":                   true,
		"TestPackage2ScalaIncrementalCleanMissingTransitions":               true,
		"TestPackage2ScalaIncrementalEOFCompositionCleanMissingTransitions": true,
	}
	if len(alts) != len(want) {
		t.Fatalf("alts = %+v", alts)
	}
	for _, a := range alts {
		if a.kind != altExact {
			t.Fatalf("alt %+v should be exact", a)
		}
		if !want[a.text] {
			t.Fatalf("unexpected alternative %q", a.text)
		}
	}
}

func TestExpandPatternUnanchoredTailIsPrefix(t *testing.T) {
	// No trailing "$": go test's -run does a substring match, so this
	// matches anything starting with the literal, not just an exact name.
	alts, err := expandPattern(`^TestMergeEventCensus`)
	if err != nil {
		t.Fatal(err)
	}
	if len(alts) != 1 || alts[0].kind != altPrefix || alts[0].text != "TestMergeEventCensus" {
		t.Fatalf("alts = %+v", alts)
	}
}

func TestExpandPatternUnanchoredHeadStaysExactWhenTailAnchored(t *testing.T) {
	// The subtest-path first segment form used for
	// "^Name$/${var}$": no leading "^" but the retained first segment still
	// ends in "$", so it should still require an exact identifier.
	alts, err := expandPattern(`TestAdmissionCandidateExactExternalPayloadCorpus$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(alts) != 1 || alts[0].kind != altExact || alts[0].text != "TestAdmissionCandidateExactExternalPayloadCorpus" {
		t.Fatalf("alts = %+v", alts)
	}
}

func TestExpandPatternInvalidRegexpErrors(t *testing.T) {
	if _, err := expandPattern(`^Test(Unclosed`); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestFirstSegmentSplitsOnSubtestSlash(t *testing.T) {
	if got := firstSegment(`^TestFoo$/${grammar}$`); got != `^TestFoo$` {
		t.Fatalf("firstSegment = %q", got)
	}
	if got := firstSegment(`^TestFoo$`); got != `^TestFoo$` {
		t.Fatalf("firstSegment = %q", got)
	}
}

func TestExtractScriptVarsSingleLine(t *testing.T) {
	script := `
set -euo pipefail
admission='^TestFoo(A|B)$'
GOMAXPROCS=1 go test . -run "$admission" -count=1
`
	sv := extractScriptVars(script)
	if sv.byName["admission"] != `^TestFoo(A|B)$` {
		t.Fatalf("byName[admission] = %q", sv.byName["admission"])
	}
	if len(sv.blocks) != 0 {
		t.Fatalf("blocks = %v", sv.blocks)
	}
}

func TestExtractScriptVarsMultiLineBlock(t *testing.T) {
	script := "set -euo pipefail\n" +
		"tests='\n" +
		"TestOne\n" +
		"TestTwo\n" +
		"TestThree.*\n" +
		"'\n" +
		"pattern=\"^($(printf '%s' \"$tests\" | tr -s '[:space:]' '\\n' | sed '/^$/d' | paste -sd'|'))\\$\"\n" +
		"go test . -run \"$pattern\" -count=1\n"
	sv := extractScriptVars(script)
	if len(sv.blocks) != 1 {
		t.Fatalf("blocks = %v", sv.blocks)
	}
	want := "^(TestOne|TestTwo|TestThree.*)$"
	if sv.blocks[0] != want {
		t.Fatalf("blocks[0] = %q, want %q", sv.blocks[0], want)
	}
	// "pattern" itself resolves via naive same-line quote matching to a
	// truncated, command-substitution-looking value; it must not be usable
	// directly, only through the sole-block fallback.
	if v, ok := sv.byName["pattern"]; ok && !looksDynamic(v) {
		t.Fatalf("byName[pattern] = %q should look dynamic", v)
	}
}

func TestResolveContentBareVariableFallsBackToSoleBlock(t *testing.T) {
	sv := scriptVars{
		byName: map[string]string{"pattern": "$(printf '%s' "},
		blocks: []string{"^(TestOne|TestTwo)$"},
	}
	pattern, skip, reason := resolveContent("$pattern", sv)
	if skip {
		t.Fatalf("unexpected skip: %s", reason)
	}
	if pattern != "^(TestOne|TestTwo)$" {
		t.Fatalf("pattern = %q", pattern)
	}
}

func TestResolveContentSkipsDynamicAndNoOpAndUnresolved(t *testing.T) {
	cases := []struct {
		raw string
		sv  scriptVars
	}{
		{"^$", scriptVars{byName: map[string]string{}}},
		{"^${{ matrix.target }}$", scriptVars{byName: map[string]string{}}},
		{"$unknownVar", scriptVars{byName: map[string]string{}}},
	}
	for _, tc := range cases {
		_, skip, reason := resolveContent(tc.raw, tc.sv)
		if !skip {
			t.Fatalf("raw %q: expected skip", tc.raw)
		}
		if reason == "" {
			t.Fatalf("raw %q: expected a reason", tc.raw)
		}
	}
}

func TestFindRunPatternsIgnoresBacktickDocLines(t *testing.T) {
	script := "set -euo pipefail\n" +
		"go test . -run '^TestReal$' -count=1\n" +
		"echo \"`go test . -run '^TestGhostOnlyInDocs$'` exited\"\n"
	cleaned := stripDocLines(script)
	got := findRunPatterns(cleaned)
	want := []string{"^TestReal$"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("findRunPatterns = %v, want %v (the backtick doc line must not surface TestGhostOnlyInDocs)", got, want)
	}
}

func TestCollectDefinedFuncNamesIgnoresBuildTags(t *testing.T) {
	dir := t.TempDir()
	src := "//go:build some_exotic_tag\n\npackage pkg\n\nimport \"testing\"\n\nfunc TestTagged(t *testing.T) {}\nfunc BenchmarkTagged(b *testing.B) {}\nfunc FuzzTagged(f *testing.F) {}\nfunc ExampleTagged() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "tagged_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := collectDefinedFuncNames(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TestTagged", "BenchmarkTagged", "FuzzTagged", "ExampleTagged"} {
		if !names[want] {
			t.Fatalf("names missing %q: %v", want, names)
		}
	}
}

func TestAuditWorkflowFileFindsStaleName(t *testing.T) {
	data := []byte(`
jobs:
  example_job:
    steps:
      - name: run tests
        run: |
          go test . -run '^(TestReal|TestGhost)$' -count=1
`)
	defined := map[string]bool{"TestReal": true}
	sortedNames := []string{"TestReal"}
	findings, checked, err := auditWorkflowFile("fake.yml", data, defined, sortedNames)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("checked = %d", checked)
	}
	if len(findings) != 1 || findings[0].Name != "TestGhost" {
		t.Fatalf("findings = %+v", findings)
	}
	if !strings.Contains(findings[0].String(), "TestGhost") {
		t.Fatalf("finding string = %q", findings[0].String())
	}
}

func TestAuditWorkflowFilePassesWhenAllNamesResolve(t *testing.T) {
	data := []byte(`
jobs:
  example_job:
    steps:
      - name: run tests
        run: |
          go test . -run '^(TestReal|TestAlsoReal)$' -count=1
`)
	defined := map[string]bool{"TestReal": true, "TestAlsoReal": true}
	sortedNames := []string{"TestAlsoReal", "TestReal"}
	findings, checked, err := auditWorkflowFile("fake.yml", data, defined, sortedNames)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 1 || len(findings) != 0 {
		t.Fatalf("checked=%d findings=%+v", checked, findings)
	}
}

func TestAuditWorkflowFileHandlesPhase0StyleList(t *testing.T) {
	data := []byte("jobs:\n" +
		"  phase0_tagged_suite:\n" +
		"    steps:\n" +
		"      - name: Tagged tests\n" +
		"        run: |\n" +
		"          set -euo pipefail\n" +
		"          tests='\n" +
		"          TestReal\n" +
		"          TestGhost\n" +
		"          '\n" +
		"          pattern=\"^($(printf '%s' \"$tests\" | tr -s '[:space:]' '\\n' | sed '/^$/d' | paste -sd'|'))\\$\"\n" +
		"          go test . -run \"$pattern\" -count=1\n")
	defined := map[string]bool{"TestReal": true}
	sortedNames := []string{"TestReal"}
	findings, checked, err := auditWorkflowFile("fake.yml", data, defined, sortedNames)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("checked = %d", checked)
	}
	if len(findings) != 1 || findings[0].Name != "TestGhost" {
		t.Fatalf("findings = %+v", findings)
	}
}

func TestAuditWorkflowFileIgnoresBacktickDocExample(t *testing.T) {
	// Mirrors ci.yml's admission_route_equality_fuzz job: a real -run
	// invocation plus a step-summary echo line that quotes an illustrative
	// (and here, deliberately stale) command inside backticks. Only the
	// real invocation may be checked.
	data := []byte(`
jobs:
  example_job:
    steps:
      - name: fuzz
        run: |
          set -euo pipefail
          go test . -run '^TestReal$' -count=1
          echo "` + "`" + `go test . -run '^TestGhostOnlyInDocs$' -count=1` + "`" + ` exited with status ${status}"
`)
	defined := map[string]bool{"TestReal": true}
	sortedNames := []string{"TestReal"}
	findings, checked, err := auditWorkflowFile("fake.yml", data, defined, sortedNames)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("checked = %d, want 1 (only the real -run invocation, not the doc example)", checked)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none (TestGhostOnlyInDocs must not surface from the backtick doc line)", findings)
	}
}

func TestAuditWorkflowFileSkipsMatrixDrivenPatterns(t *testing.T) {
	data := []byte(`
jobs:
  example_job:
    strategy:
      matrix:
        include:
          - name: shard0
    steps:
      - name: matrix run
        run: |
          go test . -race -run '^${{ matrix.target }}$'
`)
	defined := map[string]bool{}
	findings, checked, err := auditWorkflowFile("fake.yml", data, defined, nil)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 0 || len(findings) != 0 {
		t.Fatalf("checked=%d findings=%+v, want fully skipped", checked, findings)
	}
}

func TestRunStaleRunNamesCheckEndToEnd(t *testing.T) {
	dir := t.TempDir()
	workflowsDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package pkg\n\nimport \"testing\"\n\nfunc TestReal(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(dir, "real_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	staleWorkflow := []byte(`
jobs:
  example_job:
    steps:
      - name: run tests
        run: |
          go test . -run '^(TestReal|TestRenamedAwayLongAgo)$' -count=1
`)
	if err := os.WriteFile(filepath.Join(workflowsDir, "ci.yml"), staleWorkflow, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := runStaleRunNamesCheck(filepath.Join(workflowsDir, "*.yml"), dir, &out)
	if err == nil {
		t.Fatal("expected an error for the deliberately stale name")
	}
	if !strings.Contains(err.Error(), "1 -run/--run reference") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(out.String(), "TestRenamedAwayLongAgo") {
		t.Fatalf("output = %q", out.String())
	}

	// Fixing the reference makes the same check pass cleanly.
	fixedWorkflow := []byte(`
jobs:
  example_job:
    steps:
      - name: run tests
        run: |
          go test . -run '^TestReal$' -count=1
`)
	if err := os.WriteFile(filepath.Join(workflowsDir, "ci.yml"), fixedWorkflow, 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runStaleRunNamesCheck(filepath.Join(workflowsDir, "*.yml"), dir, &out); err != nil {
		t.Fatalf("unexpected error after fix: %v (output: %s)", err, out.String())
	}
}

func TestHasPrefixMatch(t *testing.T) {
	names := []string{"TestAlpha", "TestBeta", "TestBetaExtra", "TestGamma"}
	sort.Strings(names)
	if !hasPrefixMatch(names, "TestBeta") {
		t.Fatal("expected a prefix match for TestBeta")
	}
	if hasPrefixMatch(names, "TestDelta") {
		t.Fatal("did not expect a prefix match for TestDelta")
	}
}
