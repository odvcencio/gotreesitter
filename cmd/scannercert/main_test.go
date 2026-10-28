package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/cmd/scannercert/internal/scannercert"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Tests run in this directory, two levels below the repository root.
const root = "../.."

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = run(append([]string{"-root", root}, args...), &out, &errOut)
	return out.String(), errOut.String(), err
}

func readReport(t *testing.T, path string) scannercert.Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report scannercert.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("%s is not a report: %v", path, err)
	}
	return report
}

// writeTwice runs the command twice into separate directories and returns the
// two reports for name, after checking that the files are byte-identical.
func writeTwice(t *testing.T, name string, args ...string) scannercert.Report {
	t.Helper()
	var files [2][]byte
	for i := range files {
		dir := t.TempDir()
		if _, _, err := runCLI(t, append([]string{"-language", name, "-out", dir}, args...)...); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		files[i] = data
	}
	if !bytes.Equal(files[0], files[1]) {
		t.Fatalf("%s: two runs produced different reports", name)
	}
	var report scannercert.Report
	if err := json.Unmarshal(files[0], &report); err != nil {
		t.Fatalf("%s: not a report: %v", name, err)
	}
	return report
}

func TestRunWritesOneDeterministicReportPerGrammar(t *testing.T) {
	for _, name := range []string{"lua", "go"} {
		report := writeTwice(t, name, "-session=false")
		corpus, err := scannercert.LoadCorpus(root, name, scannercert.CorpusOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if report.Schema != scannercert.Schema || report.Language != name || report.Corpus.Sources != 1 ||
			report.Corpus.SampleSHA256 != corpus.SampleSHA256 || report.Corpus.ManifestSHA256 != corpus.ManifestSHA256 {
			t.Fatalf("%s: report identity = %+v", name, report.Corpus)
		}
		if report.Calls.Recorded == 0 || report.Calls.Distinct == 0 || report.Checks.Replayed != report.Calls.Distinct {
			t.Fatalf("%s: calls %+v checks %+v, want every distinct call replayed", name, report.Calls, report.Checks)
		}
	}
}

// The editing session adds 72 sources, most of them with errors. Their reports
// must be as stable as the sample's.
func TestRunWithTheEditingSessionIsDeterministic(t *testing.T) {
	report := writeTwice(t, "svelte")
	if report.Corpus.Sources != 73 || report.Parses.Sources != 73 || report.Parses.ErrorTrees == 0 || !report.Parses.Transparent {
		t.Fatalf("parses %+v corpus %+v, want 73 transparent sources, most with errors", report.Parses, report.Corpus)
	}
}

func TestRunPrintsASingleReportToStandardOutput(t *testing.T) {
	stdout, _, err := runCLI(t, "-language", "lua", "-session=false")
	if err != nil {
		t.Fatal(err)
	}
	var report scannercert.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.Language != "lua" || report.Corpus.Sources != 1 {
		t.Fatalf("stdout = %.200q, err %v; want the lua report for the sample alone", stdout, err)
	}
}

// The command fails any report whose parse the recorder changed, so these
// grammars must come out transparent. Each has a different mix of the optional
// scanner interfaces the recorder forwards: stateless and failure-preserving
// (go), failure-retaining with checkpoints (swift), checkpoints that preserve
// failures (html), checkpoint identity (scala), an error-mode retry opinion
// (matlab), a packed-GSS opinion (dart), and none (bash, lua).
func TestRecordingLeavesTheParseUnchanged(t *testing.T) {
	for _, name := range []string{"go", "swift", "html", "scala", "matlab", "dart", "bash", "lua"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if _, _, err := runCLI(t, "-language", name, "-session=false", "-out", dir); err != nil {
				t.Fatal(err)
			}
			report := readReport(t, filepath.Join(dir, name+".json"))
			if !report.Parses.Transparent || report.Calls.Recorded == 0 {
				t.Fatalf("parses %+v calls %+v, want a transparent parse that called the scanner", report.Parses, report.Calls)
			}
		})
	}
}

// A report whose recorded parse differs from the production parse is not
// evidence about the scanner. The command still writes it, so a reader can see
// which sources changed, but it must name the verdict and exit nonzero.
func TestRunFailsAReportWhoseRecordedParseDiffers(t *testing.T) {
	notTransparent := func(_, name string, _ scannercert.CorpusOptions) (*scannercert.Report, error) {
		return &scannercert.Report{
			Schema: scannercert.Schema, Language: name, Verdict: scannercert.VerdictNotTransparent,
			Parses: scannercert.ParseInfo{Sources: 73, Divergent: 73},
		}, nil
	}
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	err := runWith([]string{"-root", root, "-language", "matlab,lua", "-out", dir}, &out, &errOut, notTransparent)
	if err == nil || !strings.Contains(err.Error(), "matlab") || !strings.Contains(err.Error(), "73 of 73") {
		t.Fatalf("err = %v, want the failure to name matlab and the 73 of 73 sources that changed", err)
	}
	if !strings.Contains(errOut.String(), "matlab: not-transparent") {
		t.Fatalf("stderr = %q, want the verdict printed", errOut.String())
	}
	for _, name := range []string{"matlab", "lua"} {
		if report := readReport(t, filepath.Join(dir, name+".json")); report.Verdict != scannercert.VerdictNotTransparent {
			t.Fatalf("%s report verdict = %q, want it written for inspection", name, report.Verdict)
		}
	}
}

// The report names the grammar blob the language was loaded from, so two
// reports for one grammar can be told apart when a blob differs.
func TestReportNamesTheGrammarBlob(t *testing.T) {
	blob := grammars.BlobByName("lua")
	if len(blob) == 0 {
		t.Skip("this build embeds no lua blob")
	}
	want := fmt.Sprintf("%x", sha256.Sum256(blob))
	report := writeTwice(t, "lua", "-session=false")
	if report.GrammarBlobSHA256 != want {
		t.Fatalf("grammar blob digest = %q, want the lua blob's %q", report.GrammarBlobSHA256, want)
	}
}

// writeLuaCorpus lays out a manifest for lua under a temporary root: the
// committed sample and one fetched median whose bytes sit in the fetch
// directory, laid out as scripts/benchfixture_corpus.py fetch lays them out.
func writeLuaCorpus(t *testing.T, median string) (root, fetched string) {
	t.Helper()
	const sample = "local x = 1\nreturn x\n"
	root, fetched = t.TempDir(), t.TempDir()
	dir := filepath.Join(root, "internal", "benchfixtures")
	if err := os.MkdirAll(filepath.Join(dir, "testdata", "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", "real", "lua"), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fetched, "lua"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fetched, "lua", "median_1"), []byte(median), 0o644); err != nil {
		t.Fatal(err)
	}
	digest := func(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text))) }
	manifest := map[string]any{"schema": "benchfixture-real-corpus-v1", "entries": []map[string]any{
		{"language": "lua", "role": "sample", "committed_path": "testdata/real/lua", "bytes": len(sample),
			"sha256": digest(sample), "session_sha256": benchfixtures.EditingSessionSHA256([]byte(sample))},
		{"language": "lua", "role": "median_1", "committed_path": "", "bytes": len(median), "sha256": digest(median)},
	}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real_corpus.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, fetched
}

func TestRunParsesTheFetchedInputsAndRecordsThem(t *testing.T) {
	const median = "local function f(a)\n  return a + 1\nend\nprint(f(2))\n"
	tmpRoot, fetched := writeLuaCorpus(t, median)
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-root", tmpRoot, "-language", "lua", "-session=false", "-fetched", fetched, "-out", out}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	report := readReport(t, filepath.Join(out, "lua.json"))
	wantDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(median)))
	if report.Corpus.Sources != 2 || report.Parses.Sources != 2 || len(report.Corpus.Fetched) != 1 {
		t.Fatalf("corpus %+v parses %+v, want the sample and the median", report.Corpus, report.Parses)
	}
	if got := report.Corpus.Fetched[0]; got.Role != "median_1" || got.Source != 1 || got.Bytes != len(median) || got.SHA256 != wantDigest || got.HasError {
		t.Fatalf("fetched = %+v, want median_1 at source 1 with the pinned digest and a clean parse", got)
	}
	// A second run produces the same bytes.
	again := t.TempDir()
	if err := run([]string{"-root", tmpRoot, "-language", "lua", "-session=false", "-fetched", fetched, "-out", again}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(out, "lua.json"))
	second, _ := os.ReadFile(filepath.Join(again, "lua.json"))
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("two runs over the same fetched inputs produced different reports")
	}
	// Without -fetched the same root yields the sample alone.
	if err := run([]string{"-root", tmpRoot, "-language", "lua", "-session=false", "-out", out}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if again := readReport(t, filepath.Join(out, "lua.json")); again.Corpus.Sources != 1 || len(again.Corpus.Fetched) != 0 {
		t.Fatalf("corpus %+v, want committed inputs only by default", again.Corpus)
	}
}

func TestRunRefusesFetchedInputsThatAreMissingOrChanged(t *testing.T) {
	tmpRoot, fetched := writeLuaCorpus(t, "return 1\n")
	if err := os.WriteFile(filepath.Join(fetched, "lua", "median_1"), []byte("return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"-root", tmpRoot, "-language", "lua", "-fetched", fetched, "-out", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "lua: ") || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("err = %v, want the changed median named", err)
	}
	err = run([]string{"-root", tmpRoot, "-language", "lua", "-fetched", t.TempDir(), "-out", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "benchfixture_corpus.py fetch") {
		t.Fatalf("err = %v, want the fetch command named", err)
	}
}

func TestListNamesGrammarsThatHaveAScannerAndASample(t *testing.T) {
	stdout, _, err := runCLI(t, "-list")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, name := range strings.Fields(stdout) {
		listed[name] = true
	}
	for _, name := range []string{"lua", "yaml", "bash", "go"} {
		if !listed[name] {
			t.Fatalf("-list omits %s: %q", name, stdout)
		}
	}
	if listed["json"] {
		t.Fatal("-list names json, which has no external scanner")
	}
}

func TestRunRefusesWhatItCannotReport(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no grammar":       {nil, "-language"},
		"unknown grammar":  {[]string{"-language", "nosuchgrammar"}, "unknown grammar"},
		"no scanner":       {[]string{"-language", "json"}, "no external scanner"},
		"several, no -out": {[]string{"-language", "lua,go"}, "-out"},
		"stray argument":   {[]string{"lua"}, "unexpected argument"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := runCLI(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestRunReportsEveryFailureAndStillWritesTheRest(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runCLI(t, "-language", "nosuchgrammar,lua,json", "-session=false", "-out", dir)
	if err == nil || !strings.Contains(err.Error(), "nosuchgrammar: unknown grammar") || !strings.Contains(err.Error(), "json: ") {
		t.Fatalf("err = %v, want both failures named", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "lua.json")); statErr != nil {
		t.Fatal("the valid grammar's report was not written")
	}
}

func TestSplitNamesTrimsAndDeduplicates(t *testing.T) {
	got := splitNames(" lua, go ,lua,,swift ")
	if strings.Join(got, "|") != "lua|go|swift" {
		t.Fatalf("names = %q, want lua, go and swift once each", got)
	}
	if len(splitNames("")) != 0 {
		t.Fatal("an empty list names a grammar")
	}
}
