package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseEntryLine(t *testing.T) {
	entry, err := parseEntryLine("go https://github.com/tree-sitter/tree-sitter-go 2346a3ab1bb3857b48b29d779a1ef9799a248cd7 src .go")
	if err != nil {
		t.Fatalf("parse entry: %v", err)
	}
	if entry.Name != "go" {
		t.Fatalf("unexpected name: %q", entry.Name)
	}
	if entry.RepoURL != "https://github.com/tree-sitter/tree-sitter-go" {
		t.Fatalf("unexpected repo: %q", entry.RepoURL)
	}
	if entry.Commit != "2346a3ab1bb3857b48b29d779a1ef9799a248cd7" {
		t.Fatalf("unexpected commit: %q", entry.Commit)
	}
	if entry.Subdir != "src" {
		t.Fatalf("unexpected subdir: %q", entry.Subdir)
	}
	if len(entry.Extensions) != 1 || entry.Extensions[0] != ".go" {
		t.Fatalf("unexpected extensions: %#v", entry.Extensions)
	}
}

func TestParseEntryLineManifestStyle(t *testing.T) {
	entry, err := parseEntryLine("xml https://github.com/tree-sitter-grammars/tree-sitter-xml xml/src .xml")
	if err != nil {
		t.Fatalf("parse entry: %v", err)
	}
	if entry.Commit != "" {
		t.Fatalf("expected empty commit, got %q", entry.Commit)
	}
	if entry.Subdir != "xml/src" {
		t.Fatalf("unexpected subdir: %q", entry.Subdir)
	}
	if got, want := strings.Join(entry.Extensions, ","), ".xml"; got != want {
		t.Fatalf("extensions mismatch: got %q want %q", got, want)
	}
}

func TestSyncMissingEntriesFromManifest(t *testing.T) {
	lock := &lockFile{
		lines: []lockLine{
			{raw: "# header"},
			{isEntry: true, entry: lockEntry{Name: "go", RepoURL: "https://example.com/go", Commit: "aaaaaaaa", Subdir: "src", Extensions: []string{".go"}}},
		},
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "languages.manifest")
	manifest := strings.Join([]string{
		"# name repo [subdir] [exts]",
		"go https://example.com/go src .go",
		"rust https://example.com/rust src .rs",
	}, "\n")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	addedNames, err := syncMissingEntriesFromManifest(lock, manifestPath)
	if err != nil {
		t.Fatalf("sync manifest: %v", err)
	}
	if len(addedNames) != 1 {
		t.Fatalf("added mismatch: got %d want 1", len(addedNames))
	}
	if _, ok := addedNames["rust"]; !ok {
		t.Fatalf("rust was not reported as added: %#v", addedNames)
	}

	entries := lock.entryPointers()
	if len(entries) != 2 {
		t.Fatalf("entry count mismatch: got %d want 2", len(entries))
	}
	if entries[1].Name != "rust" {
		t.Fatalf("expected rust entry, got %q", entries[1].Name)
	}
}

func TestWriteLockFilePreservesComments(t *testing.T) {
	lock := &lockFile{
		lines: []lockLine{
			{raw: "# first"},
			{isEntry: true, entry: lockEntry{Name: "go", RepoURL: "https://example.com/go", Commit: "aaaaaaaa", Subdir: "src", Extensions: []string{".go"}}},
			{raw: ""},
			{raw: "# second"},
			{isEntry: true, entry: lockEntry{Name: "rust", RepoURL: "https://example.com/rust", Commit: "bbbbbbbb", Subdir: "src", Extensions: []string{".rs"}}},
		},
	}
	outPath := filepath.Join(t.TempDir(), "languages.lock")
	if err := writeLockFile(outPath, lock); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "# first\n") || !strings.Contains(got, "\n# second\n") {
		t.Fatalf("comments not preserved:\n%s", got)
	}
	if !strings.Contains(got, "go https://example.com/go aaaaaaaa src .go") {
		t.Fatalf("missing go entry: %s", got)
	}
}

func TestVerifyRemoteCommit(t *testing.T) {
	repo, commit := makeTestRemoteRepo(t)

	if err := verifyRemoteCommit(repo, commit); err != nil {
		t.Fatalf("expected pinned commit to be fetchable: %v", err)
	}

	missing := strings.Repeat("f", 40)
	if err := verifyRemoteCommit(repo, missing); err == nil {
		t.Fatal("expected missing pinned commit to fail verification")
	}
}

func TestVerifyRemotePinsDeduplicatesRepoCommits(t *testing.T) {
	repo, commit := makeTestRemoteRepo(t)
	missing := strings.Repeat("e", 40)
	entries := []*lockEntry{
		{Name: "one", RepoURL: repo, Commit: commit},
		{Name: "two", RepoURL: repo, Commit: commit},
		{Name: "missing", RepoURL: repo, Commit: missing},
		{Name: "unpinned", RepoURL: repo},
	}

	if got := countRemotePins(entries); got != 2 {
		t.Fatalf("countRemotePins() = %d, want 2", got)
	}
	errs := verifyRemotePins(entries, 2)
	if len(errs) != 1 {
		t.Fatalf("verifyRemotePins() error count = %d, want 1: %#v", len(errs), errs)
	}
	if _, ok := errs[pinKey(repo, missing)]; !ok {
		t.Fatalf("missing pin error not reported: %#v", errs)
	}
}

func makeTestRemoteRepo(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	work := filepath.Join(root, "work")
	runGit(t, "", "init", "-q", work)
	runGit(t, work, "config", "user.email", "test@example.invalid")
	runGit(t, work, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(work, "grammar.js"), []byte("module.exports = {}\n"), 0o644); err != nil {
		t.Fatalf("write grammar.js: %v", err)
	}
	runGit(t, work, "add", "grammar.js")
	runGit(t, work, "commit", "-q", "-m", "initial")
	commit := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	bare := filepath.Join(root, "remote.git")
	runGit(t, "", "clone", "-q", "--bare", work, bare)
	return bare, commit
}

// TestApplyPlanHoldsBackExcludedNames is the core regression test for the
// grammar-lock-update repair: a guard hit on one grammar must not block the
// rest. It reproduces the kotlin/scala/swift incident with synthetic data —
// three names are excluded (guard-blocked) among five available updates, and
// the two non-excluded names must still apply while the excluded three keep
// their old commit.
func TestApplyPlanHoldsBackExcludedNames(t *testing.T) {
	entries := []*lockEntry{
		{Name: "go", RepoURL: "https://example.test/go", Commit: "old-go"},
		{Name: "rust", RepoURL: "https://example.test/rust", Commit: "old-rust"},
		{Name: "kotlin", RepoURL: "https://example.test/kotlin", Commit: "old-kotlin"},
		{Name: "scala", RepoURL: "https://example.test/scala", Commit: "old-scala"},
		{Name: "swift", RepoURL: "https://example.test/swift", Commit: "old-swift"},
	}
	planIndex := map[string]updateResult{
		"go":     {Name: "go", RepoURL: "https://example.test/go", OldRef: "old-go", NewRef: "new-go", Status: updateStatusAvailable},
		"rust":   {Name: "rust", RepoURL: "https://example.test/rust", OldRef: "old-rust", NewRef: "new-rust", Status: updateStatusAvailable},
		"kotlin": {Name: "kotlin", RepoURL: "https://example.test/kotlin", OldRef: "old-kotlin", NewRef: "new-kotlin", Status: updateStatusAvailable},
		"scala":  {Name: "scala", RepoURL: "https://example.test/scala", OldRef: "old-scala", NewRef: "new-scala", Status: updateStatusAvailable},
		"swift":  {Name: "swift", RepoURL: "https://example.test/swift", OldRef: "old-swift", NewRef: "new-swift", Status: updateStatusAvailable},
	}
	excludeSet := map[string]struct{}{"kotlin": {}, "scala": {}, "swift": {}}

	results, counts := applyPlan(entries, planIndex, excludeSet, 0, true)

	if counts.applied != 2 {
		t.Fatalf("applied count = %d, want 2", counts.applied)
	}
	if counts.heldBack != 3 {
		t.Fatalf("heldBack count = %d, want 3", counts.heldBack)
	}
	if counts.checked != 5 {
		t.Fatalf("checked count = %d, want 5", counts.checked)
	}

	byName := make(map[string]updateResult, len(results))
	for _, r := range results {
		byName[r.Name] = r
	}

	for _, name := range []string{"go", "rust"} {
		if byName[name].Status != updateStatusApplied || !byName[name].Applied {
			t.Fatalf("%s: expected applied, got %+v", name, byName[name])
		}
	}
	for _, name := range []string{"kotlin", "scala", "swift"} {
		if byName[name].Status != updateStatusHeldBack || byName[name].Applied {
			t.Fatalf("%s: expected held_back, got %+v", name, byName[name])
		}
		// The held-back result still records the newer ref for the PR body,
		// but must not have touched the lock entry itself.
		if byName[name].NewRef == "" {
			t.Fatalf("%s: expected new_ref preserved for reporting, got empty", name)
		}
	}

	// The lock entries themselves must be untouched for held-back names, and
	// updated for the applied names — this is the "keep old lock entries"
	// contract from the incident report.
	entryByName := map[string]*lockEntry{}
	for _, e := range entries {
		entryByName[e.Name] = e
	}
	if entryByName["go"].Commit != "new-go" {
		t.Fatalf("go commit = %q, want new-go", entryByName["go"].Commit)
	}
	if entryByName["rust"].Commit != "new-rust" {
		t.Fatalf("rust commit = %q, want new-rust", entryByName["rust"].Commit)
	}
	for _, name := range []string{"kotlin", "scala", "swift"} {
		if got, want := entryByName[name].Commit, "old-"+name; got != want {
			t.Fatalf("%s commit = %q, want %q (held back)", name, got, want)
		}
	}
}

// TestApplyPlanRespectsMaxUpdatesBudget verifies held-back names never
// consume the max-updates budget, and that once the budget runs out, the
// remaining cleared updates stay "available" (not applied, not held back)
// for the next run to pick up.
func TestApplyPlanRespectsMaxUpdatesBudget(t *testing.T) {
	entries := []*lockEntry{
		{Name: "a", RepoURL: "https://example.test/a", Commit: "old-a"},
		{Name: "b", RepoURL: "https://example.test/b", Commit: "old-b"},
		{Name: "blocked", RepoURL: "https://example.test/blocked", Commit: "old-blocked"},
	}
	planIndex := map[string]updateResult{
		"a":       {Name: "a", RepoURL: "https://example.test/a", OldRef: "old-a", NewRef: "new-a", Status: updateStatusAvailable},
		"b":       {Name: "b", RepoURL: "https://example.test/b", OldRef: "old-b", NewRef: "new-b", Status: updateStatusAvailable},
		"blocked": {Name: "blocked", RepoURL: "https://example.test/blocked", OldRef: "old-blocked", NewRef: "new-blocked", Status: updateStatusAvailable},
	}
	excludeSet := map[string]struct{}{"blocked": {}}

	results, counts := applyPlan(entries, planIndex, excludeSet, 1, true)

	if counts.applied != 1 {
		t.Fatalf("applied count = %d, want 1 (budget capped)", counts.applied)
	}
	if counts.available != 1 {
		t.Fatalf("available count = %d, want 1 (budget exhausted, name not excluded)", counts.available)
	}
	if counts.heldBack != 1 {
		t.Fatalf("heldBack count = %d, want 1 (excluded, doesn't count against budget)", counts.heldBack)
	}

	byName := make(map[string]updateResult, len(results))
	for _, r := range results {
		byName[r.Name] = r
	}
	if byName["a"].Status != updateStatusApplied {
		t.Fatalf("a status = %q, want applied", byName["a"].Status)
	}
	if byName["b"].Status != updateStatusAvailable {
		t.Fatalf("b status = %q, want available", byName["b"].Status)
	}
	if byName["blocked"].Status != updateStatusHeldBack {
		t.Fatalf("blocked status = %q, want held_back", byName["blocked"].Status)
	}
}

// TestApplyPlanDryRunAppliesNothing verifies writeChanges=false never
// mutates lock entries, matching the -write=false plan pass used to decide
// what the guard should evaluate before anything touches the lock file.
func TestApplyPlanDryRunAppliesNothing(t *testing.T) {
	entries := []*lockEntry{
		{Name: "go", RepoURL: "https://example.test/go", Commit: "old-go"},
	}
	planIndex := map[string]updateResult{
		"go": {Name: "go", RepoURL: "https://example.test/go", OldRef: "old-go", NewRef: "new-go", Status: updateStatusAvailable},
	}

	results, counts := applyPlan(entries, planIndex, nil, 0, false)

	if counts.applied != 0 {
		t.Fatalf("applied count = %d, want 0 in dry run", counts.applied)
	}
	if counts.available != 1 {
		t.Fatalf("available count = %d, want 1 in dry run", counts.available)
	}
	if entries[0].Commit != "old-go" {
		t.Fatalf("dry run mutated lock entry: got %q, want old-go", entries[0].Commit)
	}
	if results[0].Status != updateStatusAvailable {
		t.Fatalf("status = %q, want available", results[0].Status)
	}
}

// TestApplyPlanCarriesOverSkippedAndErrorStatuses ensures grammars the plan
// pass marked skipped, unchanged, or errored pass through untouched instead
// of being silently dropped or reclassified.
func TestApplyPlanCarriesOverSkippedAndErrorStatuses(t *testing.T) {
	entries := []*lockEntry{
		{Name: "skipped-lang", RepoURL: "https://example.test/skipped", Commit: "commit-a"},
		{Name: "error-lang", RepoURL: "https://example.test/error", Commit: "commit-b"},
		{Name: "unchanged-lang", RepoURL: "https://example.test/unchanged", Commit: "commit-c"},
		{Name: "new-lang", RepoURL: "https://example.test/new", Commit: "commit-d"},
	}
	planIndex := map[string]updateResult{
		"skipped-lang":   {Name: "skipped-lang", Status: updateStatusSkipped},
		"error-lang":     {Name: "error-lang", Status: updateStatusError, Error: "resolved empty remote ref"},
		"unchanged-lang": {Name: "unchanged-lang", OldRef: "commit-c", NewRef: "commit-c", Status: updateStatusUnchanged},
		// "new-lang" intentionally absent from the plan.
	}

	results, counts := applyPlan(entries, planIndex, nil, 0, true)

	if counts.skipped != 1 || counts.errored != 1 || counts.unchanged != 2 {
		t.Fatalf("counts = %+v, want skipped=1 errored=1 unchanged=2", counts)
	}
	byName := make(map[string]updateResult, len(results))
	for _, r := range results {
		byName[r.Name] = r
	}
	if byName["skipped-lang"].Status != updateStatusSkipped {
		t.Fatalf("skipped-lang status = %q", byName["skipped-lang"].Status)
	}
	if byName["error-lang"].Status != updateStatusError || byName["error-lang"].Error == "" {
		t.Fatalf("error-lang result = %+v", byName["error-lang"])
	}
	if byName["new-lang"].Status != updateStatusUnchanged {
		t.Fatalf("new-lang (absent from plan) status = %q, want unchanged", byName["new-lang"].Status)
	}
	// Lock commits for anything not applied must stay exactly as they were.
	for _, e := range entries {
		want := map[string]string{
			"skipped-lang":   "commit-a",
			"error-lang":     "commit-b",
			"unchanged-lang": "commit-c",
			"new-lang":       "commit-d",
		}[e.Name]
		if e.Commit != want {
			t.Fatalf("%s commit = %q, want %q", e.Name, e.Commit, want)
		}
	}
}

// TestReadPlanIndex verifies the plan report round-trips through JSON with
// no network access, so the split logic is fully testable offline.
func TestReadPlanIndex(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")

	plan := updateReport{
		Results: []updateResult{
			{Name: "go", RepoURL: "https://example.test/go", OldRef: "old", NewRef: "new", Status: updateStatusAvailable},
			{Name: "rust", Status: updateStatusSkipped},
		},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	idx, err := readPlanIndex(planPath)
	if err != nil {
		t.Fatalf("readPlanIndex: %v", err)
	}
	want := map[string]updateResult{
		"go":   {Name: "go", RepoURL: "https://example.test/go", OldRef: "old", NewRef: "new", Status: updateStatusAvailable},
		"rust": {Name: "rust", Status: updateStatusSkipped},
	}
	if !reflect.DeepEqual(idx, want) {
		t.Fatalf("readPlanIndex = %#v, want %#v", idx, want)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}
