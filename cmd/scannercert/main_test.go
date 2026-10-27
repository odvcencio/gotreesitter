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
		corpus, err := scannercert.LoadCorpus(root, name, false)
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
	notTransparent := func(_, name string, _ bool) (*scannercert.Report, error) {
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
