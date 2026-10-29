package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testRefA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testRefB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// fixtureSources returns two query sources: an Apache-2.0 repository whose one
// file is modified, and an MIT repository whose one file is not.
func fixtureSources() []querySource {
	return []querySource{
		{
			grammarEntry: grammarEntry{
				Name:            "nvim",
				Repo:            "https://github.com/example/nvim",
				Ref:             testRefA,
				SPDX:            "Apache-2.0",
				LicenseFileURL:  "https://github.com/example/nvim/blob/" + testRefA + "/LICENSE",
				RepositoryOwner: "example",
			},
			Files: []queryFile{{File: "a.scm", Upstream: "runtime/queries/a/highlights.scm", Modified: true}},
		},
		{
			grammarEntry: grammarEntry{
				Name:             "mit",
				Repo:             "https://github.com/example/mit",
				Ref:              testRefB,
				SPDX:             "MIT",
				CopyrightHolders: []string{"Copyright (c) 2020 Example"},
				LicenseFileURL:   "https://github.com/example/mit/blob/" + testRefB + "/LICENSE",
			},
			Files: []queryFile{{File: "b.scm", Upstream: "queries/highlights.scm"}},
		},
	}
}

const (
	headerA = "; Source: example/nvim runtime/queries/a/highlights.scm at " + testRefA + " (Apache-2.0)\n"
	headerB = "; Source: example/mit queries/highlights.scm at " + testRefB + " (MIT).\n"
	body    = "\n(identifier) @variable\n"
)

// fixtureFiles returns query files whose headers agree with fixtureSources.
func fixtureFiles() map[string]string {
	return map[string]string{
		"a.scm": headerA + "; Change: the parent was renamed.\n" + body,
		"b.scm": headerB + "; Change: none.\n" + body,
	}
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// wantError fails unless err is non-nil and mentions every string in want.
func wantError(t *testing.T, err error, want []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want one mentioning %q", want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not mention %q", err, w)
		}
	}
}

// inRepoRoot runs the test from the repository root, where run reads its
// repo-relative paths. It changes the process working directory, so a test
// that calls it must not run in parallel.
func inRepoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if _, err := os.Stat(auditPath); err != nil {
		t.Fatalf("not at the repository root: %v", err)
	}
}

// TestRepositoryNoticesAreCurrent runs the same check CI runs: the audits
// cover languages.lock and grammars/highlight_queries/, every query file
// header agrees with the audit, and THIRD_PARTY_NOTICES is up to date.
func TestRepositoryNoticesAreCurrent(t *testing.T) {
	inRepoRoot(t)
	if err := run(true); err != nil {
		t.Fatal(err)
	}
}

func TestReadQueryAuditPromotesLicenseFields(t *testing.T) {
	dir := writeFiles(t, map[string]string{"q.json": `{
  "source_count": 1,
  "sources": [{
    "name": "nvim",
    "repo": "https://github.com/example/nvim",
    "ref": "` + testRefA + `",
    "spdx": "Apache-2.0",
    "copyright_holders": ["Copyright (c) 2020 Example"],
    "license_file_url": "https://example.test/LICENSE",
    "repository_owner": "example",
    "note": "a note",
    "files": [{"file": "a.scm", "upstream": "u/a.scm", "modified": true}]
  }]
}`})
	a, err := readQueryAudit(filepath.Join(dir, "q.json"))
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceCount != 1 || len(a.Sources) != 1 {
		t.Fatalf("got %d sources (count field %d), want 1", len(a.Sources), a.SourceCount)
	}
	s := a.Sources[0]
	if s.Name != "nvim" || s.Ref != testRefA || s.SPDX != "Apache-2.0" || s.RepositoryOwner != "example" ||
		s.Note != "a note" || s.LicenseFileURL != "https://example.test/LICENSE" ||
		len(s.CopyrightHolders) != 1 || s.CopyrightHolders[0] != "Copyright (c) 2020 Example" {
		t.Errorf("license fields were not decoded: %+v", s.grammarEntry)
	}
	if len(s.Files) != 1 || s.Files[0] != (queryFile{File: "a.scm", Upstream: "u/a.scm", Modified: true}) {
		t.Errorf("files were not decoded: %+v", s.Files)
	}

	_, err = readQueryAudit(filepath.Join(dir, "missing.json"))
	wantError(t, err, []string{"read ", "missing.json"})
	dir = writeFiles(t, map[string]string{"bad.json": "{"})
	_, err = readQueryAudit(filepath.Join(dir, "bad.json"))
	wantError(t, err, []string{"parse ", "bad.json"})
}

func TestCheckQueryAudit(t *testing.T) {
	tests := []struct {
		name   string
		change func(a *queryAudit)
		want   []string // nil means the audit is valid
	}{
		{"valid", func(a *queryAudit) {}, nil},
		{"count mismatch", func(a *queryAudit) { a.SourceCount = 3 }, []string{"source_count 3", "2 sources"}},
		{"empty name", func(a *queryAudit) { a.Sources[0].Name = "" }, []string{"empty name"}},
		{"duplicate name", func(a *queryAudit) { a.Sources[1].Name = "nvim" }, []string{"duplicate source", "nvim"}},
		{"no repo", func(a *queryAudit) { a.Sources[0].Repo = "" }, []string{"nvim", "repo and license_file_url"}},
		{"no license file", func(a *queryAudit) { a.Sources[0].LicenseFileURL = "" }, []string{"nvim", "repo and license_file_url"}},
		{"short commit", func(a *queryAudit) { a.Sources[0].Ref = "abc1234" }, []string{"nvim", "abc1234", "40-character"}},
		{"upper-case commit", func(a *queryAudit) { a.Sources[0].Ref = strings.ToUpper(testRefA) }, []string{"40-character"}},
		{"blank license", func(a *queryAudit) { a.Sources[0].SPDX = "" }, []string{"nvim", "spdx"}},
		{"unknown license", func(a *queryAudit) { a.Sources[0].SPDX = "UNKNOWN" }, []string{"nvim", "spdx", "unknown"}},
		{"copyleft", func(a *queryAudit) { a.Sources[0].Copyleft = true }, []string{"nvim", "copyleft", "gotreesitter_no_copyleft"}},
		{"notice url", func(a *queryAudit) { a.Sources[0].NoticeFileURL = "https://example.test/NOTICE" }, []string{"nvim", "NOTICE"}},
		{"notice file", func(a *queryAudit) { a.Sources[0].NoticeFile = "licenses/notices/x.txt" }, []string{"nvim", "NOTICE"}},
		{"nobody to credit", func(a *queryAudit) { a.Sources[0].RepositoryOwner = "" }, []string{"nvim", "copyright_holders"}},
		{"no files", func(a *queryAudit) { a.Sources[0].Files = nil }, []string{"nvim", "no files"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := queryAudit{SourceCount: 2, Sources: fixtureSources()}
			tc.change(&a)
			err := checkQueryAudit(a)
			if tc.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			wantError(t, err, tc.want)
		})
	}
}

func TestCheckQueryFiles(t *testing.T) {
	both := fixtureSources()
	both[1].Files = append(both[1].Files, queryFile{File: "a.scm", Upstream: "queries/a.scm", Modified: true})

	tests := []struct {
		name    string
		sources func() []querySource
		files   func(f map[string]string)
		want    []string // nil means the files agree with the audit
	}{
		{"agree", nil, nil, nil},
		{
			"other files in the directory are ignored", nil,
			func(f map[string]string) { f["README.txt"] = "not a query" }, nil,
		},
		{
			"a Source line in the body is not part of the header", nil,
			func(f map[string]string) { f["b.scm"] += "; Source: bogus/repo x at " + testRefA + " (MIT)\n" }, nil,
		},
		{
			"file without a recorded source", nil,
			func(f map[string]string) { f["c.scm"] = headerB + "; Change: none.\n" + body },
			[]string{"no source for 1 file", "c.scm"},
		},
		{
			"recorded file is missing", nil,
			func(f map[string]string) { delete(f, "b.scm") },
			[]string{"not in", "b.scm"},
		},
		{
			"no Source line", nil,
			func(f map[string]string) { f["a.scm"] = "; Change: the parent was renamed.\n" + body },
			[]string{"a.scm", `no "; Source:" line`, "example/nvim"},
		},
		{
			"Source line names another commit", nil,
			func(f map[string]string) {
				f["a.scm"] = strings.Replace(f["a.scm"], testRefA, testRefB, 1)
			},
			[]string{"a.scm", `no "; Source:" line`, testRefA, "matches no source"},
		},
		{
			"Source line names another license", nil,
			func(f map[string]string) {
				f["a.scm"] = strings.Replace(f["a.scm"], "(Apache-2.0)", "(MIT)", 1)
			},
			[]string{"a.scm", `no "; Source:" line`, "Apache-2.0"},
		},
		{
			"Source line names another upstream file", nil,
			func(f map[string]string) {
				f["a.scm"] = strings.Replace(f["a.scm"], "runtime/queries/a/", "runtime/queries/z/", 1)
			},
			[]string{"a.scm", `no "; Source:" line`, "runtime/queries/a/highlights.scm"},
		},
		{
			"Source line no source records", nil,
			func(f map[string]string) {
				f["b.scm"] = headerB + "; Source: other/repo x.scm at " + testRefA + " (MIT)\n; Change: none.\n" + body
			},
			[]string{"b.scm", "other/repo", "matches no source"},
		},
		{
			"no Change line", nil,
			func(f map[string]string) { f["b.scm"] = headerB + body },
			[]string{"b.scm", `no "; Change:" line`},
		},
		{
			"header says none but the audit says modified", nil,
			func(f map[string]string) { f["a.scm"] = headerA + "; Change: none.\n" + body },
			[]string{"a.scm", "Change: none", "modified"},
		},
		{
			"header describes a change but the audit says unmodified", nil,
			func(f map[string]string) { f["b.scm"] = headerB + "; Change: dropped a pattern.\n" + body },
			[]string{"b.scm", "describes a change", "unmodified"},
		},
		{
			"two sources need two Source lines",
			func() []querySource { return both },
			func(f map[string]string) {
				f["a.scm"] = headerA + "; Source: example/mit queries/a.scm at " + testRefB + " (MIT)\n; Change: joined.\n" + body
			},
			nil,
		},
		{
			"one of two sources is missing from the header",
			func() []querySource { return both },
			nil,
			[]string{"a.scm", `no "; Source:" line`, "example/mit queries/a.scm"},
		},
		{
			"file name with a directory", func() []querySource {
				s := fixtureSources()
				s[0].Files[0].File = "sub/a.scm"
				return s
			}, nil,
			[]string{"sub/a.scm", "list .scm file names"},
		},
		{
			"file name without .scm", func() []querySource {
				s := fixtureSources()
				s[0].Files[0].File = "a.txt"
				return s
			}, nil,
			[]string{"a.txt", "list .scm file names"},
		},
		{
			"file listed twice", func() []querySource {
				s := fixtureSources()
				s[0].Files = append(s[0].Files, s[0].Files[0])
				return s
			}, nil,
			[]string{"nvim", "a.scm twice"},
		},
		{
			"no upstream path", func() []querySource {
				s := fixtureSources()
				s[0].Files[0].Upstream = ""
				return s
			}, nil,
			[]string{"nvim", "a.scm", "upstream path"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sources := fixtureSources()
			if tc.sources != nil {
				sources = tc.sources()
			}
			files := fixtureFiles()
			if tc.files != nil {
				tc.files(files)
			}
			err := checkQueryFiles(writeFiles(t, files), sources)
			if tc.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			wantError(t, err, tc.want)
		})
	}

	t.Run("directory is missing", func(t *testing.T) {
		wantError(t, checkQueryFiles(filepath.Join(t.TempDir(), "absent"), fixtureSources()), []string{"read ", "absent"})
	})
	t.Run("every problem is reported", func(t *testing.T) {
		files := fixtureFiles()
		files["a.scm"] = headerA + "; Change: none.\n" + body // audit says modified
		files["b.scm"] = headerB + body                       // no Change line
		wantError(t, checkQueryFiles(writeFiles(t, files), fixtureSources()), []string{"2 problem(s)", "a.scm", "b.scm"})
	})
}

func TestQueryHeader(t *testing.T) {
	text := "; one\n\n  ; two\n(node) @a\n; Source: not header\n"
	got := queryHeader(text)
	want := []string{"; one", "", "; two"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("queryHeader = %q, want %q", got, want)
	}
	if got := queryHeader("(node) @a\n; Source: x\n"); len(got) != 0 {
		t.Errorf("a file with no header gave %q", got)
	}
	if got := queryHeader("; only\r\n; comments\r\n"); len(got) != 3 || got[0] != "; only" {
		t.Errorf("CRLF header gave %q", got)
	}
}

func TestWriteQuerySources(t *testing.T) {
	texts := map[string]string{"Apache-2.0": "APACHE TEXT", "MIT": "MIT TEXT"}
	var b bytes.Buffer
	writeQuerySources(&b, fixtureSources(), texts)
	got := b.String()

	for _, want := range []string{
		"Imported highlight queries\n--------------------------\n",
		"### Apache-2.0 (1 source)\n",
		"### MIT (1 source)\n",
		"nvim -- https://github.com/example/nvim @ aaaaaaaaaaaa\n",
		"  (no copyright line stated upstream; repository owner: example)\n",
		"  License file: https://github.com/example/nvim/blob/" + testRefA + "/LICENSE\n",
		"mit -- https://github.com/example/mit @ bbbbbbbbbbbb\n  Copyright (c) 2020 Example\n",
		"    a.scm                <- runtime/queries/a/highlights.scm (modified)\n",
		"    b.scm                <- queries/highlights.scm\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "queries/highlights.scm (modified)") {
		t.Errorf("an unmodified file is marked modified:\n%s", got)
	}

	// Each group is followed by its own license text, in group order, so the
	// section stands alone.
	last := -1
	for _, marker := range []string{
		"### Apache-2.0", "-- Apache-2.0 license text --\n\nAPACHE TEXT\n\n",
		"### MIT", "-- MIT license text --\n\nMIT TEXT\n\n",
	} {
		i := strings.Index(got, marker)
		if i < 0 || i < last {
			t.Fatalf("%q is missing or out of order (at %d, after %d):\n%s", marker, i, last, got)
		}
		last = i
	}

	var empty bytes.Buffer
	writeQuerySources(&empty, nil, texts)
	if empty.Len() != 0 {
		t.Errorf("no sources should write nothing, got %q", empty.String())
	}
}

func TestWriteQuerySourcesSortsAndKeepsInputOrder(t *testing.T) {
	sources := fixtureSources()
	sources[0].Files = []queryFile{{File: "z.scm", Upstream: "z"}, {File: "a.scm", Upstream: "a"}}
	before := sources[0].Files[0].File
	var b bytes.Buffer
	writeQuerySources(&b, sources, map[string]string{"Apache-2.0": "A", "MIT": "M"})
	if i, j := strings.Index(b.String(), "    a.scm "), strings.Index(b.String(), "    z.scm "); i < 0 || j < 0 || i > j {
		t.Errorf("files are not sorted by name:\n%s", b.String())
	}
	if sources[0].Files[0].File != before {
		t.Error("writeQuerySources reordered the caller's files")
	}
}

func TestWriteSummaryCountsQueryFiles(t *testing.T) {
	entries := []grammarEntry{{SPDX: "MIT"}}

	var none bytes.Buffer
	writeSummary(&none, entries, nil)
	if strings.Contains(none.String(), "highlight query") {
		t.Errorf("no sources should add no line:\n%s", none.String())
	}

	// One file taken from two repositories counts once.
	sources := fixtureSources()
	sources[1].Files = append(sources[1].Files, queryFile{File: "a.scm", Upstream: "queries/a.scm"})
	var b bytes.Buffer
	writeSummary(&b, entries, sources)
	if want := "2 embedded highlight query files, copied from 2 upstream repositories\n"; !strings.Contains(b.String(), want) {
		t.Errorf("summary lacks %q:\n%s", want, b.String())
	}
}

func TestLoadTextsCoversQuerySources(t *testing.T) {
	dir := writeFiles(t, map[string]string{"Apache-2.0.txt": "APACHE\n\n", "MIT.txt": "MIT\n"})

	// A license that only a query source uses still gets its text loaded.
	texts, err := loadTexts(dir, []grammarEntry{{SPDX: "MIT"}}, fixtureSources())
	if err != nil {
		t.Fatal(err)
	}
	if texts["Apache-2.0"] != "APACHE" || texts["MIT"] != "MIT" {
		t.Errorf("texts = %q", texts)
	}

	// A query source with no registered license text fails the run.
	sources := fixtureSources()
	sources[0].SPDX = "BSD-3-Clause"
	_, err = loadTexts(dir, nil, sources)
	wantError(t, err, []string{"no canonical license text", "BSD-3-Clause"})
}

func TestRenderPlacesQuerySourcesBeforeComponents(t *testing.T) {
	entries := []grammarEntry{{
		Name: "g", Repo: "https://github.com/example/g", Ref: testRefA, SPDX: "MIT",
		CopyrightHolders: []string{"Copyright (c) 2020 G"}, LicenseFileURL: "https://example.test/g",
	}}
	texts := map[string]string{"Apache-2.0": "APACHE TEXT", "MIT": "MIT TEXT"}

	out, err := render(entries, fixtureSources(), texts)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	last := -1
	for _, marker := range []string{"Summary\n", "Imported highlight queries\n", "Components by license\n"} {
		i := strings.Index(got, marker)
		if i < 0 || i < last {
			t.Fatalf("%q is missing or out of order:\n%s", marker, got)
		}
		last = i
	}
	if !strings.Contains(got, "licenses/highlight_queries.json") {
		t.Error("the header does not name the query audit")
	}

	// Without query sources the output has no query section at all.
	out, err = render(entries, nil, texts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "Imported highlight queries\n---") || strings.Contains(string(out), "embedded highlight query files") {
		t.Errorf("a query section appeared with no sources:\n%s", out)
	}
}
