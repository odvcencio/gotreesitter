package main

// The blob freshness check compares each shipped ts2go blob with a fresh
// conversion of its pinned, patched parser.c by the current converter. It
// compares decoded tables, not bytes: a blob without a version header differs
// in bytes from a fresh blob even when every table matches.
//
// TestTS2GoBlobFreshness runs the full check. It reads the pinned sources from
// the cache that -fetch-sources writes (sources.go) and needs no network:
//
//	go run ./cmd/ts2go -manifest grammars/languages.lock -fetch-sources <dir>
//	GTS_TS2GO_SOURCES=<dir> go test ./cmd/ts2go -run '^TestTS2GoBlobFreshness$' -count=1 -v
//
// It reports each checked grammar as equal or stale with its difference
// classes. The stale grammars are listed in testdata/blob_freshness_allowlist.txt.
// The check fails when a stale blob is not listed, when a listed blob is no
// longer stale, or when a listed blob's classes change, so the list can only
// shrink. GTS_TS2GO_FRESHNESS_WRITE=1 rewrites the list from the measurement.

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

const (
	freshnessSourcesEnv = "GTS_TS2GO_SOURCES"
	freshnessWriteEnv   = "GTS_TS2GO_FRESHNESS_WRITE"
	freshnessReportEnv  = "GTS_TS2GO_FRESHNESS_REPORT"
	freshnessJobsEnv    = "GTS_TS2GO_FRESHNESS_JOBS"

	freshnessAllowlistPath = "testdata/blob_freshness_allowlist.txt"
	freshnessLockPath      = "../../grammars/languages.lock"
	freshnessBlobDir       = "../../grammars/grammar_blobs"
)

// Difference classes. A table field maps to one class; a lexer state field
// maps by the kind of difference. A difference that the map does not name is
// STRUCT.
//
//	ABI      LanguageVersion
//	CHARSET  a lexer state's transitions send some character to another
//	         target (for example, a character range that the converter lost)
//	CPOL     ConflictPolicies
//	CREC     the C-recovery capability flags
//	ELS      ExternalLexStates
//	EOFACC   a main lexer state's end-of-input acceptance
//	KWEOF    a keyword lexer state's end-of-input acceptance
//	NTAM     NonTerminalAliasMap
//	RW       ReservedWords and MaxReservedWordSetSize
//	SUPT     the supertype tables
//	SYMNAME  symbol names (SymbolNames and the names in SymbolMetadata)
//	STRUCT   any other table: parse tables, counts, lex modes, symbol flags,
//	         and the other lexer state fields
var freshnessFieldClass = map[string]string{
	"LanguageVersion":                          "ABI",
	"ConflictPolicies":                         "CPOL",
	"CRecoveryCostCompetitionCapable":          "CREC",
	"CRecoveryCostCompetitionEnabledByDefault": "CREC",
	"ExternalLexStates":                        "ELS",
	"NonTerminalAliasMap":                      "NTAM",
	"ReservedWords":                            "RW",
	"MaxReservedWordSetSize":                   "RW",
	"SupertypeSymbols":                         "SUPT",
	"SupertypeMapSlices":                       "SUPT",
	"SupertypeMapEntries":                      "SUPT",
	"SymbolNames":                              "SYMNAME",
}

var freshnessClasses = map[string]bool{
	"ABI": true, "CHARSET": true, "CPOL": true, "CREC": true, "ELS": true, "EOFACC": true,
	"KWEOF": true, "NTAM": true, "RW": true, "SUPT": true, "SYMNAME": true, "STRUCT": true,
}

// freshnessDiff is one kind of difference in one table field.
type freshnessDiff struct {
	Field   string   `json:"field"`
	Kind    string   `json:"kind,omitempty"`
	Class   string   `json:"class"`
	Count   int      `json:"count"`
	Shipped int      `json:"len_shipped,omitempty"`
	Fresh   int      `json:"len_fresh,omitempty"`
	Samples []string `json:"samples,omitempty"`
}

// freshnessResult is the check's verdict on one grammar.
type freshnessResult struct {
	Name    string          `json:"name"`
	Error   string          `json:"error,omitempty"`
	Classes []string        `json:"classes"`
	Diffs   []freshnessDiff `json:"diffs,omitempty"`
}

func (r freshnessResult) stale() bool { return len(r.Classes) > 0 }

func (r freshnessResult) verdict() string {
	switch {
	case r.Error != "":
		return "error: " + r.Error
	case r.stale():
		return "stale " + strings.Join(r.Classes, ",")
	default:
		return "equal"
	}
}

const freshnessMaxSamples = 4

func (d *freshnessDiff) sample(format string, args ...any) {
	if len(d.Samples) < freshnessMaxSamples {
		s := fmt.Sprintf(format, args...)
		if len(s) > 160 {
			s = s[:160] + "..."
		}
		d.Samples = append(d.Samples, s)
	}
}

// compareDecodedTables lists the differences between every exported table of
// two decoded languages, sorted by field and kind.
func compareDecodedTables(shipped, fresh *gotreesitter.Language) []freshnessDiff {
	var diffs []freshnessDiff
	va, vb := reflect.ValueOf(shipped).Elem(), reflect.ValueOf(fresh).Elem()
	t := va.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || f.Name == "ExternalScanner" {
			continue
		}
		switch f.Name {
		case "LexStates":
			diffs = append(diffs, diffLexStates(f.Name, shipped.LexStates, fresh.LexStates, shipped, fresh, true)...)
			continue
		case "KeywordLexStates":
			diffs = append(diffs, diffLexStates(f.Name, shipped.KeywordLexStates, fresh.KeywordLexStates, shipped, fresh, false)...)
			continue
		case "SymbolMetadata":
			diffs = append(diffs, diffSymbolMetadata(shipped.SymbolMetadata, fresh.SymbolMetadata)...)
			continue
		}
		fa, fb := va.Field(i), vb.Field(i)
		if reflect.DeepEqual(fa.Interface(), fb.Interface()) {
			continue
		}
		// A nil and an empty slice or map decode the same table.
		if (fa.Kind() == reflect.Slice || fa.Kind() == reflect.Map) && fa.Len() == 0 && fb.Len() == 0 {
			continue
		}
		class := freshnessFieldClass[f.Name]
		if class == "" {
			class = "STRUCT"
		}
		d := freshnessDiff{Field: f.Name, Class: class}
		switch fa.Kind() {
		case reflect.Slice:
			d.Shipped, d.Fresh = fa.Len(), fb.Len()
			n := min(fa.Len(), fb.Len())
			for j := 0; j < n; j++ {
				if !reflect.DeepEqual(fa.Index(j).Interface(), fb.Index(j).Interface()) {
					d.Count++
					d.sample("%d: %+v -> %+v", j, fa.Index(j).Interface(), fb.Index(j).Interface())
				}
			}
			d.Count += max(fa.Len(), fb.Len()) - n
		case reflect.Map:
			d.Shipped, d.Fresh = fa.Len(), fb.Len()
			d.Count = mapDifferenceCount(fa, fb)
		default:
			d.Count = 1
			d.sample("%+v -> %+v", fa.Interface(), fb.Interface())
		}
		diffs = append(diffs, d)
	}
	sort.SliceStable(diffs, func(i, j int) bool {
		if diffs[i].Field != diffs[j].Field {
			return diffs[i].Field < diffs[j].Field
		}
		return diffs[i].Kind < diffs[j].Kind
	})
	return diffs
}

func mapDifferenceCount(a, b reflect.Value) int {
	count := 0
	iter := a.MapRange()
	for iter.Next() {
		other := b.MapIndex(iter.Key())
		if !other.IsValid() || !reflect.DeepEqual(iter.Value().Interface(), other.Interface()) {
			count++
		}
	}
	iter = b.MapRange()
	for iter.Next() {
		if !a.MapIndex(iter.Key()).IsValid() {
			count++
		}
	}
	return count
}

// diffLexStates compares lexer states field by field. Transitions that match
// the same characters to the same targets in another order or split count as
// equal.
func diffLexStates(field string, a, b []gotreesitter.LexState, la, lb *gotreesitter.Language, main bool) []freshnessDiff {
	byKind := map[string]*freshnessDiff{}
	add := func(kind, class string, state int, format string, args ...any) {
		d := byKind[kind]
		if d == nil {
			d = &freshnessDiff{Field: field, Kind: kind, Class: class, Shipped: len(a), Fresh: len(b)}
			byKind[kind] = d
		}
		d.Count++
		d.sample("%d: "+format, append([]any{state}, args...)...)
	}
	eofClass := "EOFACC"
	if !main {
		eofClass = "KWEOF"
	}
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		x, y := a[i], b[i]
		if x.AcceptEOF != y.AcceptEOF {
			add("accept_eof", eofClass, i, "%v -> %v", x.AcceptEOF, y.AcceptEOF)
		}
		if main {
			if ea, eb := la.LexerStateAcceptsEndBeforeEOF(i), lb.LexerStateAcceptsEndBeforeEOF(i); ea != eb {
				add("end_before_eof", eofClass, i, "%v -> %v", ea, eb)
			}
		}
		if !reflect.DeepEqual(x.Transitions, y.Transitions) && !transitionsEquivalent(x.Transitions, y.Transitions) {
			add("transitions", "CHARSET", i, "%d -> %d ranges", len(x.Transitions), len(y.Transitions))
		}
		if x.AcceptToken != y.AcceptToken {
			add("accept_token", "STRUCT", i, "%d -> %d", x.AcceptToken, y.AcceptToken)
		}
		if x.AcceptPriority != y.AcceptPriority {
			add("accept_priority", "STRUCT", i, "%d -> %d", x.AcceptPriority, y.AcceptPriority)
		}
		if x.Skip != y.Skip {
			add("skip", "STRUCT", i, "%v -> %v", x.Skip, y.Skip)
		}
		if x.Default != y.Default {
			add("default", "STRUCT", i, "%d -> %d", x.Default, y.Default)
		}
		if x.EOF != y.EOF {
			add("eof_edge", "STRUCT", i, "%d -> %d", x.EOF, y.EOF)
		}
	}
	if len(a) != len(b) {
		add("length", "STRUCT", n, "%d -> %d states", len(a), len(b))
	}
	out := make([]freshnessDiff, 0, len(byKind))
	for _, d := range byKind {
		out = append(out, *d)
	}
	return out
}

// transitionsEquivalent reports whether two transition lists send every
// character to the same target with the same skip flag (first match wins).
func transitionsEquivalent(a, b []gotreesitter.LexTransition) bool {
	points := map[rune]bool{0: true}
	for _, list := range [][]gotreesitter.LexTransition{a, b} {
		for _, t := range list {
			points[t.Lo] = true
			points[t.Hi+1] = true
		}
	}
	keys := make([]rune, 0, len(points))
	for k := range points {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	look := func(list []gotreesitter.LexTransition, r rune) (int, bool, bool) {
		for _, t := range list {
			if r >= t.Lo && r <= t.Hi {
				return t.NextState, t.Skip, true
			}
		}
		return 0, false, false
	}
	for _, k := range keys {
		na, sa, oka := look(a, k)
		nb, sb, okb := look(b, k)
		if oka != okb || na != nb || sa != sb {
			return false
		}
	}
	return true
}

// diffSymbolMetadata puts name differences in SYMNAME and flag differences in
// STRUCT.
func diffSymbolMetadata(a, b []gotreesitter.SymbolMetadata) []freshnessDiff {
	names := freshnessDiff{Field: "SymbolMetadata", Kind: "name", Class: "SYMNAME", Shipped: len(a), Fresh: len(b)}
	flags := freshnessDiff{Field: "SymbolMetadata", Kind: "flags", Class: "STRUCT", Shipped: len(a), Fresh: len(b)}
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		x, y := a[i], b[i]
		if x.Name != y.Name {
			names.Count++
			names.sample("%d: %q -> %q", i, x.Name, y.Name)
		}
		x.Name, y.Name = "", ""
		if x != y {
			flags.Count++
			flags.sample("%d: %+v -> %+v", i, x, y)
		}
	}
	if len(a) != len(b) {
		flags.Count += max(len(a), len(b)) - n
	}
	var out []freshnessDiff
	for _, d := range []freshnessDiff{flags, names} {
		if d.Count > 0 {
			out = append(out, d)
		}
	}
	return out
}

// classesOf returns the sorted difference classes of diffs.
func classesOf(diffs []freshnessDiff) []string {
	seen := map[string]bool{}
	classes := []string{}
	for _, d := range diffs {
		if d.Count > 0 && !seen[d.Class] {
			seen[d.Class] = true
			classes = append(classes, d.Class)
		}
	}
	sort.Strings(classes)
	return classes
}

// compareBlobs decodes both blobs and compares their tables.
func compareBlobs(name string, shippedBlob, freshBlob []byte) freshnessResult {
	r := freshnessResult{Name: name, Classes: []string{}}
	shipped, err := gotreesitter.LoadLanguage(shippedBlob)
	if err != nil {
		r.Error = "decode shipped blob: " + err.Error()
		return r
	}
	fresh, err := gotreesitter.LoadLanguage(freshBlob)
	if err != nil {
		r.Error = "decode fresh blob: " + err.Error()
		return r
	}
	r.Diffs = compareDecodedTables(shipped, fresh)
	r.Classes = classesOf(r.Diffs)
	return r
}

// checkBlobFreshness converts the cached pinned parser.c of entry and compares
// it with the shipped blob.
func checkBlobFreshness(entry ManifestEntry, sourceDir, manifestDir, blobDir string) freshnessResult {
	identity, err := freshnessSourceIdentity(entry, manifestDir)
	if err != nil {
		return freshnessResult{Name: entry.Name, Classes: []string{}, Error: err.Error()}
	}
	source, err := readFreshnessSource(filepath.Join(sourceDir, safeFileBase(entry.Name)), identity)
	if err != nil {
		return freshnessResult{Name: entry.Name, Classes: []string{}, Error: fmt.Sprintf(
			"source cache: %v (refresh it: go run ./cmd/ts2go -manifest grammars/languages.lock -fetch-sources %s)", err, sourceDir)}
	}
	shippedBlob, err := os.ReadFile(filepath.Join(blobDir, safeFileBase(entry.Name)+".bin"))
	if err != nil {
		return freshnessResult{Name: entry.Name, Classes: []string{}, Error: err.Error()}
	}
	freshBlob, err := convertParserSource(entry.Name, source)
	if err != nil {
		return freshnessResult{Name: entry.Name, Classes: []string{}, Error: "convert: " + err.Error()}
	}
	return compareBlobs(entry.Name, shippedBlob, freshBlob)
}

// readFreshnessAllowlist reads "name CLASS,CLASS" lines.
func readFreshnessAllowlist(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	allowed := map[string][]string{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: want \"name CLASS,CLASS\", have %q", path, i+1, line)
		}
		if _, dup := allowed[fields[0]]; dup {
			return nil, fmt.Errorf("%s:%d: %s is listed twice", path, i+1, fields[0])
		}
		classes := strings.Split(fields[1], ",")
		if !sort.StringsAreSorted(classes) {
			return nil, fmt.Errorf("%s:%d: classes of %s are not sorted", path, i+1, fields[0])
		}
		for _, class := range classes {
			if !freshnessClasses[class] {
				return nil, fmt.Errorf("%s:%d: unknown class %q", path, i+1, class)
			}
		}
		allowed[fields[0]] = classes
	}
	return allowed, nil
}

const freshnessAllowlistHeader = `# Shipped ts2go blobs whose decoded tables differ from a fresh conversion of
# their pinned, patched parser.c, with the difference classes. The check is
# TestTS2GoBlobFreshness in cmd/ts2go/blob_freshness_test.go, which also
# defines the classes. It fails when a stale blob is not listed, when a listed
# blob is no longer stale, or when a listed blob's classes change. Remove a
# line when its blob is regenerated. Rewrite the whole list with:
#   go run ./cmd/ts2go -manifest grammars/languages.lock -fetch-sources <dir>
#   GTS_TS2GO_SOURCES=<dir> GTS_TS2GO_FRESHNESS_WRITE=1 go test ./cmd/ts2go -run '^TestTS2GoBlobFreshness$' -count=1
`

func formatFreshnessAllowlist(results []freshnessResult) string {
	var b strings.Builder
	b.WriteString(freshnessAllowlistHeader)
	for _, r := range results {
		if r.stale() {
			fmt.Fprintf(&b, "%s %s\n", r.Name, strings.Join(r.Classes, ","))
		}
	}
	return b.String()
}

// freshnessAllowlistProblems returns the ratchet failures, sorted.
func freshnessAllowlistProblems(results []freshnessResult, allowed map[string][]string) []string {
	var problems []string
	checked := map[string]bool{}
	for _, r := range results {
		checked[r.Name] = true
		want, listed := allowed[r.Name]
		switch {
		case r.Error != "":
			problems = append(problems, fmt.Sprintf("%s: %s", r.Name, r.Error))
		case r.stale() && !listed:
			problems = append(problems, fmt.Sprintf("%s: the shipped blob is stale (%s) and not on the allowlist; regenerate the blob", r.Name, strings.Join(r.Classes, ",")))
		case !r.stale() && listed:
			problems = append(problems, fmt.Sprintf("%s: the shipped blob now matches a fresh conversion; remove it from %s", r.Name, freshnessAllowlistPath))
		case r.stale() && strings.Join(want, ",") != strings.Join(r.Classes, ","):
			problems = append(problems, fmt.Sprintf("%s: the difference classes changed from %s to %s; a converter or decoder change altered this grammar's tables", r.Name, strings.Join(want, ","), strings.Join(r.Classes, ",")))
		}
	}
	for name := range allowed {
		if !checked[name] {
			problems = append(problems, fmt.Sprintf("%s: listed in %s but not a checked grammar", name, freshnessAllowlistPath))
		}
	}
	sort.Strings(problems)
	return problems
}

func freshnessJobs(t *testing.T) int {
	jobs := min(runtime.GOMAXPROCS(0), 4)
	if raw := strings.TrimSpace(os.Getenv(freshnessJobsEnv)); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			t.Fatalf("%s=%q: want a positive integer", freshnessJobsEnv, raw)
		}
		jobs = n
	}
	return max(jobs, 1)
}

func TestTS2GoBlobFreshness(t *testing.T) {
	sourceDir := strings.TrimSpace(os.Getenv(freshnessSourcesEnv))
	if sourceDir == "" {
		t.Skipf("%s is not set: the check needs the pinned sources (go run ./cmd/ts2go -manifest grammars/languages.lock -fetch-sources <dir>)", freshnessSourcesEnv)
	}
	entries, err := ParseManifest(freshnessLockPath)
	if err != nil {
		t.Fatal(err)
	}
	checked := freshnessEntries(entries)
	results := make([]freshnessResult, len(checked))
	var wg sync.WaitGroup
	slots := make(chan struct{}, freshnessJobs(t))
	for i, entry := range checked {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			results[i] = checkBlobFreshness(entry, sourceDir, filepath.Dir(freshnessLockPath), freshnessBlobDir)
		}()
	}
	wg.Wait()

	equal, stale, failed := 0, 0, 0
	classCount := map[string]int{}
	for _, r := range results {
		t.Logf("%-20s %s", r.Name, r.verdict())
		switch {
		case r.Error != "":
			failed++
		case r.stale():
			stale++
			for _, class := range r.Classes {
				classCount[class]++
			}
		default:
			equal++
		}
	}
	exempt := make([]string, 0, len(freshnessExempt))
	for name := range freshnessExempt {
		exempt = append(exempt, name)
	}
	sort.Strings(exempt)
	t.Logf("checked %d grammars: %d equal, %d stale, %d errors (exempt: %s)", len(results), equal, stale, failed, strings.Join(exempt, ", "))
	classes := make([]string, 0, len(classCount))
	for class := range classCount {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	for _, class := range classes {
		t.Logf("stale by class: %-8s %d", class, classCount[class])
	}

	if path := strings.TrimSpace(os.Getenv(freshnessReportEnv)); path != "" {
		var b strings.Builder
		for _, r := range results {
			line, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			b.Write(line)
			b.WriteByte('\n')
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if os.Getenv(freshnessWriteEnv) == "1" {
		if failed > 0 {
			t.Fatalf("%d grammars failed; not writing %s", failed, freshnessAllowlistPath)
		}
		if err := os.WriteFile(freshnessAllowlistPath, []byte(formatFreshnessAllowlist(results)), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d stale grammars)", freshnessAllowlistPath, stale)
		return
	}
	allowed, err := readFreshnessAllowlist(freshnessAllowlistPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range freshnessAllowlistProblems(results, allowed) {
		t.Error(problem)
	}
}

// The allowlist must stay readable and name only checked grammars. This runs
// without the pinned sources.
func TestBlobFreshnessAllowlistNamesCheckedGrammars(t *testing.T) {
	allowed, err := readFreshnessAllowlist(freshnessAllowlistPath)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseManifest(freshnessLockPath)
	if err != nil {
		t.Fatal(err)
	}
	checked := map[string]bool{}
	for _, entry := range freshnessEntries(entries) {
		checked[entry.Name] = true
	}
	for name := range allowed {
		if !checked[name] {
			t.Errorf("%s names %s, which is not a lock entry that the check covers", freshnessAllowlistPath, name)
		}
	}
	data, err := os.ReadFile(freshnessAllowlistPath)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, strings.Fields(line)[0])
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("%s is not sorted by name", freshnessAllowlistPath)
	}
	if !strings.HasPrefix(string(data), freshnessAllowlistHeader) {
		t.Errorf("%s lost its header; rewrite it with %s=1", freshnessAllowlistPath, freshnessWriteEnv)
	}
}

func TestFreshnessEntriesLeaveOutOwnedAndExemptBlobs(t *testing.T) {
	entries, err := ParseManifest(freshnessLockPath)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, entry := range freshnessEntries(entries) {
		got[entry.Name] = true
	}
	for name := range grammargenOwnedBlobs {
		if got[name] {
			t.Errorf("grammargen-owned %s is checked", name)
		}
	}
	for name := range freshnessExempt {
		if got[name] {
			t.Errorf("exempt %s is checked", name)
		}
	}
	for _, name := range []string{"json", "c", "make", "typescript", "tsx"} {
		if !got[name] {
			t.Errorf("%s is not checked", name)
		}
	}
	if want := len(entries) - len(grammargenOwnedBlobs) - len(freshnessExempt); len(got) != want {
		t.Errorf("checked %d entries, want %d", len(got), want)
	}
}

func TestFreshnessSourceCacheRejectsAMismatch(t *testing.T) {
	dir := t.TempDir()
	entry := ManifestEntry{Name: "json", RepoURL: "https://example.invalid/json", Commit: "254c42a6476413b776221e03982ac8ae159eeb72", Subdir: "src"}
	identity, err := freshnessSourceIdentity(entry, filepath.Dir(freshnessLockPath))
	if err != nil {
		t.Fatal(err)
	}
	if want := "json https://example.invalid/json 254c42a6476413b776221e03982ac8ae159eeb72 src patch=none"; identity != want {
		t.Fatalf("identity %q, want %q", identity, want)
	}
	patched, err := freshnessSourceIdentity(ManifestEntry{Name: "make", RepoURL: "u", Commit: "c", Subdir: "src"}, filepath.Dir(freshnessLockPath))
	if err != nil || strings.HasSuffix(patched, "patch=none") {
		t.Fatalf("a patched entry needs a patch digest: %q, %v", patched, err)
	}
	folder := filepath.Join(dir, "json")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(source, record string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(folder, "parser.c"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, freshnessSourceFile), []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const source = "int x;\n"
	write(source, identity+"\nparser_sha256="+strings.Repeat("0", 64)+"\n")
	if _, err := readFreshnessSource(folder, identity); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("a wrong digest must be rejected: %v", err)
	}
	write(source, fmt.Sprintf("%s\nparser_sha256=%x\n", identity, sha256.Sum256([]byte(source))))
	if got, err := readFreshnessSource(folder, identity); err != nil || string(got) != source {
		t.Fatalf("a matching cache entry must be read: %q, %v", got, err)
	}
	other := strings.Replace(identity, "254c42a6", "00000000", 1)
	if _, err := readFreshnessSource(folder, other); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("a cache entry for another commit must be rejected: %v", err)
	}
}

func TestBlobFreshnessAllowlistOnlyShrinks(t *testing.T) {
	results := []freshnessResult{
		{Name: "a", Classes: []string{}},
		{Name: "b", Classes: []string{"ABI", "EOFACC"}},
		{Name: "c", Classes: []string{"EOFACC"}},
		{Name: "d", Classes: []string{"CPOL", "EOFACC"}},
		{Name: "e", Classes: []string{}, Error: "convert: boom"},
	}
	allowed := map[string][]string{
		"a": {"EOFACC"},
		"b": {"ABI", "EOFACC"},
		"d": {"EOFACC"},
		"z": {"ABI"},
	}
	got := freshnessAllowlistProblems(results, allowed)
	want := []string{
		"a: the shipped blob now matches a fresh conversion; remove it from " + freshnessAllowlistPath,
		"c: the shipped blob is stale (EOFACC) and not on the allowlist; regenerate the blob",
		"d: the difference classes changed from EOFACC to CPOL,EOFACC; a converter or decoder change altered this grammar's tables",
		"e: convert: boom",
		"z: listed in " + freshnessAllowlistPath + " but not a checked grammar",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := freshnessAllowlistProblems(results[1:2], map[string][]string{"b": {"ABI", "EOFACC"}}); len(got) != 0 {
		t.Fatalf("a listed stale blob with the same classes passes: %v", got)
	}
	text := formatFreshnessAllowlist(results)
	if !strings.HasSuffix(text, "b ABI,EOFACC\nc EOFACC\nd CPOL,EOFACC\n") {
		t.Fatalf("allowlist text:\n%s", text)
	}
	path := filepath.Join(t.TempDir(), "allow.txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := readFreshnessAllowlist(path)
	if err != nil || len(read) != 3 || strings.Join(read["d"], ",") != "CPOL,EOFACC" {
		t.Fatalf("round trip: %v, %v", read, err)
	}
	for _, bad := range []string{"x EOFACC,ABI\n", "x NOPE\n", "x\n", "x ABI\nx ABI\n"} {
		if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readFreshnessAllowlist(path); err == nil {
			t.Errorf("allowlist %q must be rejected", bad)
		}
	}
}

// Each kind of table difference lands in its class, and a blob compared with
// itself is equal. The base is the shipped json blob.
func TestBlobFreshnessClassifiesTableDifferences(t *testing.T) {
	blob, err := os.ReadFile(filepath.Join(freshnessBlobDir, "json.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if r := compareBlobs("json", blob, blob); r.Error != "" || r.stale() || len(r.Diffs) != 0 {
		t.Fatalf("a blob compared with itself: %+v", r)
	}
	plainState := func(l *gotreesitter.Language) int {
		for i, s := range l.LexStates {
			if s.AcceptToken == 0 && !s.Skip && !s.AcceptEOF && len(s.Transitions) > 0 {
				return i
			}
		}
		t.Fatal("json has no plain lexer state")
		return -1
	}
	copyStates := func(states []gotreesitter.LexState) []gotreesitter.LexState {
		out := append([]gotreesitter.LexState(nil), states...)
		for i := range out {
			out[i].Transitions = append([]gotreesitter.LexTransition(nil), out[i].Transitions...)
		}
		return out
	}
	cases := []struct {
		name   string
		mutate func(l *gotreesitter.Language)
		want   string
	}{
		{"language version", func(l *gotreesitter.Language) { l.LanguageVersion++ }, "ABI"},
		{"C recovery flag", func(l *gotreesitter.Language) {
			l.CRecoveryCostCompetitionCapable = !l.CRecoveryCostCompetitionCapable
		}, "CREC"},
		{"end-of-input acceptance", func(l *gotreesitter.Language) {
			l.LexStates = copyStates(l.LexStates)
			l.LexStates[plainState(l)].AcceptEOF = true
		}, "EOFACC"},
		{"unconditional end acceptance", func(l *gotreesitter.Language) {
			if err := l.SetLexerEndStates([]uint32{uint32(plainState(l))}); err != nil {
				t.Fatal(err)
			}
		}, "EOFACC"},
		{"keyword lexer added", func(l *gotreesitter.Language) {
			l.KeywordLexStates = []gotreesitter.LexState{{Default: -1, EOF: -1, AcceptEOF: true}}
		}, "STRUCT"},
		{"transition target", func(l *gotreesitter.Language) {
			l.LexStates = copyStates(l.LexStates)
			l.LexStates[plainState(l)].Transitions[0].NextState++
		}, "CHARSET"},
		{"conflict policies", func(l *gotreesitter.Language) {
			l.ConflictPolicies = append(l.ConflictPolicies, gotreesitter.ConflictPolicy{State: 1, Lookahead: 1})
		}, "CPOL"},
		{"external lex states", func(l *gotreesitter.Language) { l.ExternalLexStates = [][]bool{{false}, {true}} }, "ELS"},
		{"alias map", func(l *gotreesitter.Language) { l.NonTerminalAliasMap = [][]gotreesitter.Symbol{{1}} }, "NTAM"},
		{"reserved words", func(l *gotreesitter.Language) { l.MaxReservedWordSetSize++ }, "RW"},
		{"supertype entries", func(l *gotreesitter.Language) {
			l.SupertypeMapEntries = append(append([]gotreesitter.Symbol(nil), l.SupertypeMapEntries...), 1)
		}, "SUPT"},
		{"symbol name", func(l *gotreesitter.Language) {
			l.SymbolNames = append([]string(nil), l.SymbolNames...)
			l.SymbolMetadata = append([]gotreesitter.SymbolMetadata(nil), l.SymbolMetadata...)
			l.SymbolNames[1] += "x"
			l.SymbolMetadata[1].Name += "x"
		}, "SYMNAME"},
		{"symbol flag", func(l *gotreesitter.Language) {
			l.SymbolMetadata = append([]gotreesitter.SymbolMetadata(nil), l.SymbolMetadata...)
			l.SymbolMetadata[1].Visible = !l.SymbolMetadata[1].Visible
		}, "STRUCT"},
		{"state count", func(l *gotreesitter.Language) { l.StateCount++ }, "STRUCT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shipped, err := gotreesitter.LoadLanguage(blob)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := gotreesitter.LoadLanguage(blob)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(fresh)
			diffs := compareDecodedTables(shipped, fresh)
			if got := strings.Join(classesOf(diffs), ","); got != tc.want {
				t.Fatalf("classes %q, want %q: %+v", got, tc.want, diffs)
			}
		})
	}
	// A keyword lexer present on both sides that differs only in end-of-input
	// acceptance is KWEOF.
	shipped, _ := gotreesitter.LoadLanguage(blob)
	fresh, _ := gotreesitter.LoadLanguage(blob)
	shipped.KeywordLexStates = []gotreesitter.LexState{{Default: -1, EOF: -1}}
	fresh.KeywordLexStates = []gotreesitter.LexState{{Default: -1, EOF: -1, AcceptEOF: true}}
	if got := strings.Join(classesOf(compareDecodedTables(shipped, fresh)), ","); got != "KWEOF" {
		t.Fatalf("keyword end acceptance: %q", got)
	}
	// Transitions that split one range into two with the same target are the
	// same lexer state.
	a := []gotreesitter.LexTransition{{Lo: 'a', Hi: 'z', NextState: 3}}
	b := []gotreesitter.LexTransition{{Lo: 'a', Hi: 'm', NextState: 3}, {Lo: 'n', Hi: 'z', NextState: 3}}
	if !transitionsEquivalent(a, b) {
		t.Fatal("a split range must be equivalent")
	}
	b[1].NextState = 4
	if transitionsEquivalent(a, b) {
		t.Fatal("a changed target must differ")
	}
	// A nil and an empty table are the same table.
	shipped, _ = gotreesitter.LoadLanguage(blob)
	fresh, _ = gotreesitter.LoadLanguage(blob)
	shipped.ConflictPolicies, fresh.ConflictPolicies = nil, []gotreesitter.ConflictPolicy{}
	if diffs := compareDecodedTables(shipped, fresh); len(diffs) != 0 {
		t.Fatalf("nil and empty slices differ: %+v", diffs)
	}
}
