// Command gen_license_notices regenerates THIRD_PARTY_NOTICES from
// licenses/grammars.json, the confirmed per-grammar license audit for every
// upstream tree-sitter grammar gotreesitter vendors, and from
// licenses/highlight_queries.json, the audit of the upstream repositories that
// the embedded highlight queries in grammars/highlight_queries/ were copied
// from. See docs/licensing.md for the audit method and the GPL-3.0/MPL-2.0
// handling this file documents.
//
// Run from the repository root:
//
//	go run ./cmd/gen_license_notices          # write THIRD_PARTY_NOTICES
//	go run ./cmd/gen_license_notices -check   # fail if it would change
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	auditPath      = "licenses/grammars.json"
	queryAuditPath = "licenses/highlight_queries.json"
	textsDir       = "licenses/texts"
	lockPath       = "grammars/languages.lock"
	queryDir       = "grammars/highlight_queries"
	outputPath     = "THIRD_PARTY_NOTICES"
)

// fullCommit matches the 40-character commit hash a query source must pin.
var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// grammarEntry mirrors one element of licenses/grammars.json's "entries"
// array. Field names match that file's JSON keys exactly so the audit stays
// the single source of truth; this generator never invents license facts.
type grammarEntry struct {
	Name             string   `json:"name"`
	Repo             string   `json:"repo"`
	Ref              string   `json:"ref"`
	SPDX             string   `json:"spdx"`
	LicenseName      string   `json:"license_name"`
	CopyrightHolders []string `json:"copyright_holders"`
	LicenseFileURL   string   `json:"license_file_url"`
	NoticeFileURL    string   `json:"notice_file_url"`
	NoticeFile       string   `json:"notice_file"`
	Method           string   `json:"method"`
	Copyleft         bool     `json:"copyleft"`
	Note             string   `json:"note"`
	RepositoryOwner  string   `json:"repository_owner"`
}

type audit struct {
	EntryCount int            `json:"entry_count"`
	Entries    []grammarEntry `json:"entries"`
}

// queryFile is one embedded query file in grammars/highlight_queries/ and the
// upstream file it was copied from. Modified is true when the embedded query
// text differs from the upstream file; the header comment each copy carries
// does not count.
type queryFile struct {
	File     string `json:"file"`
	Upstream string `json:"upstream"`
	Modified bool   `json:"modified"`
}

// querySource is one upstream repository, at one commit, that highlight query
// files were copied from. It records the same license facts as a grammarEntry,
// under the same JSON keys, plus the files taken from it. Query sources are not
// grammars, so languages.lock does not list them.
type querySource struct {
	grammarEntry
	Files []queryFile `json:"files"`
}

// queryAudit mirrors licenses/highlight_queries.json.
type queryAudit struct {
	SourceCount int           `json:"source_count"`
	Sources     []querySource `json:"sources"`
}

// spdxTextFiles maps an SPDX identifier (or a component of a compound
// expression like "Apache-2.0 OR MIT") to the licenses/texts/*.txt file that
// carries its canonical, unfilled license body.
var spdxTextFiles = map[string]string{
	"MIT":        "MIT.txt",
	"Apache-2.0": "Apache-2.0.txt",
	"GPL-3.0":    "GPL-3.0.txt",
	"MPL-2.0":    "MPL-2.0.txt",
	"ISC":        "ISC.txt",
	"CC0-1.0":    "CC0-1.0.txt",
	"WTFPL":      "WTFPL.txt",
	"Unlicense":  "Unlicense.txt",
}

func main() {
	check := flag.Bool("check", false, "check THIRD_PARTY_NOTICES without writing changes")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "gen_license_notices:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	data, err := os.ReadFile(auditPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", auditPath, err)
	}
	var a audit
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("parse %s: %w", auditPath, err)
	}
	if len(a.Entries) != a.EntryCount {
		return fmt.Errorf("%s: entry_count %d does not match %d entries", auditPath, a.EntryCount, len(a.Entries))
	}

	lockNames, err := readLockNames(lockPath)
	if err != nil {
		return err
	}
	if err := checkCoverage(lockNames, a.Entries); err != nil {
		return err
	}

	queries, err := readQueryAudit(queryAuditPath)
	if err != nil {
		return err
	}
	if err := checkQueryAudit(queries); err != nil {
		return err
	}
	if err := checkQueryFiles(queryDir, queries.Sources); err != nil {
		return err
	}

	texts, err := loadTexts(textsDir, a.Entries, queries.Sources)
	if err != nil {
		return err
	}

	out, err := render(a.Entries, queries.Sources, texts)
	if err != nil {
		return err
	}

	old, readErr := os.ReadFile(outputPath)
	if readErr == nil && bytes.Equal(old, out) {
		return nil
	}
	if check {
		return fmt.Errorf("%s is stale; run `go run ./cmd/gen_license_notices`", outputPath)
	}
	return os.WriteFile(outputPath, out, 0644)
}

// readLockNames returns every grammar name recorded in languages.lock
// (column 1 of each non-comment, non-blank line).
func readLockNames(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	names := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		names[fields[0]] = true
	}
	return names, nil
}

// checkCoverage fails loudly when languages.lock and the audit disagree:
// every locked grammar needs a confirmed-or-UNKNOWN audit entry, and the
// audit should not carry stale entries for grammars no longer locked.
func checkCoverage(lockNames map[string]bool, entries []grammarEntry) error {
	audited := map[string]grammarEntry{}
	for _, e := range entries {
		if e.Name == "" {
			return fmt.Errorf("%s: entry with empty name", auditPath)
		}
		if _, dup := audited[e.Name]; dup {
			return fmt.Errorf("%s: duplicate entry %q", auditPath, e.Name)
		}
		audited[e.Name] = e
		if e.SPDX == "" {
			return fmt.Errorf("%s: %q has no spdx value (use \"UNKNOWN\" if genuinely unconfirmed, never blank)", auditPath, e.Name)
		}
	}

	var missing, extra []string
	for name := range lockNames {
		if _, ok := audited[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range audited {
		if !lockNames[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		return fmt.Errorf("%s is missing an audit entry for %d grammar(s) locked in %s: %s", auditPath, len(missing), lockPath, strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		return fmt.Errorf("%s has %d stale entr(y/ies) no longer in %s: %s", auditPath, len(extra), lockPath, strings.Join(extra, ", "))
	}
	return nil
}

// readQueryAudit reads licenses/highlight_queries.json.
func readQueryAudit(path string) (queryAudit, error) {
	var a queryAudit
	data, err := os.ReadFile(path)
	if err != nil {
		return a, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return a, fmt.Errorf("parse %s: %w", path, err)
	}
	return a, nil
}

// checkQueryAudit validates the query sources one by one. Like checkCoverage
// for grammars, it fails on anything that would leave a notice without a
// confirmed license, an exact commit, or someone to credit.
func checkQueryAudit(a queryAudit) error {
	if len(a.Sources) != a.SourceCount {
		return fmt.Errorf("%s: source_count %d does not match %d sources", queryAuditPath, a.SourceCount, len(a.Sources))
	}
	seen := map[string]bool{}
	for _, s := range a.Sources {
		where := fmt.Sprintf("%s: source %q", queryAuditPath, s.Name)
		switch {
		case s.Name == "":
			return fmt.Errorf("%s: source with empty name", queryAuditPath)
		case seen[s.Name]:
			return fmt.Errorf("%s: duplicate source %q", queryAuditPath, s.Name)
		case s.Repo == "" || s.LicenseFileURL == "":
			return fmt.Errorf("%s needs repo and license_file_url", where)
		case !fullCommit.MatchString(s.Ref):
			return fmt.Errorf("%s: ref %q is not a full 40-character commit hash", where, s.Ref)
		case s.SPDX == "" || s.SPDX == "UNKNOWN":
			return fmt.Errorf("%s needs a confirmed spdx value; do not ship query files whose license is unknown", where)
		case s.Copyleft:
			return fmt.Errorf("%s is copyleft; the gotreesitter_no_copyleft build tag does not exclude embedded query files, so the generator cannot record one yet", where)
		case s.NoticeFileURL != "" || s.NoticeFile != "":
			return fmt.Errorf("%s has a NOTICE file; extend the generator to reproduce it before recording one", where)
		case len(s.CopyrightHolders) == 0 && s.RepositoryOwner == "":
			return fmt.Errorf("%s needs copyright_holders, or repository_owner when upstream states no copyright line", where)
		case len(s.Files) == 0:
			return fmt.Errorf("%s lists no files", where)
		}
		seen[s.Name] = true
	}
	return nil
}

// queryUse ties one embedded query file to one source that lists it.
type queryUse struct {
	source querySource
	file   queryFile
}

// checkQueryFiles fails loudly when the embedded query files and the audit
// disagree: every .scm file in dir needs a recorded source, every recorded
// file must exist, and each file's header comment must say what the audit
// records. THIRD_PARTY_NOTICES takes its attribution from the audit; the
// headers keep that attribution true to the files.
func checkQueryFiles(dir string, sources []querySource) error {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	onDisk := map[string]bool{}
	for _, de := range dirEntries {
		if !de.IsDir() && strings.HasSuffix(de.Name(), ".scm") {
			onDisk[de.Name()] = true
		}
	}

	uses := map[string][]queryUse{}
	var stale []string
	for _, s := range sources {
		listed := map[string]bool{}
		for _, f := range s.Files {
			if f.File == "" || f.File != filepath.Base(f.File) || !strings.HasSuffix(f.File, ".scm") {
				return fmt.Errorf("%s: source %q lists %q; list .scm file names inside %s", queryAuditPath, s.Name, f.File, dir)
			}
			if f.Upstream == "" {
				return fmt.Errorf("%s: source %q lists %s without its upstream path", queryAuditPath, s.Name, f.File)
			}
			if listed[f.File] {
				return fmt.Errorf("%s: source %q lists %s twice", queryAuditPath, s.Name, f.File)
			}
			listed[f.File] = true
			if !onDisk[f.File] {
				stale = append(stale, f.File)
				continue
			}
			uses[f.File] = append(uses[f.File], queryUse{source: s, file: f})
		}
	}

	var missing []string
	for name := range onDisk {
		if len(uses[name]) == 0 {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		return fmt.Errorf("%s has no source for %d file(s) in %s: %s (record where each was copied from)", queryAuditPath, len(missing), dir, strings.Join(missing, ", "))
	}
	if len(stale) > 0 {
		return fmt.Errorf("%s lists %d file(s) that are not in %s: %s", queryAuditPath, len(stale), dir, strings.Join(stale, ", "))
	}

	names := make([]string, 0, len(uses))
	for name := range uses {
		names = append(names, name)
	}
	sort.Strings(names)
	var problems []string
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read query file %s: %w", name, err)
		}
		problems = append(problems, checkQueryHeader(name, queryHeader(string(data)), uses[name])...)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problem(s) between the file headers in %s and %s:\n  %s", len(problems), dir, queryAuditPath, strings.Join(problems, "\n  "))
	}
	return nil
}

// queryHeader returns the comment lines that open a query file: every line
// before the first one that is neither blank nor a ";" comment.
func queryHeader(text string) []string {
	var header []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, ";") {
			break
		}
		header = append(header, line)
	}
	return header
}

// repoName shortens a GitHub repository URL to the owner/name form that the
// query file headers use.
func repoName(repo string) string {
	return strings.TrimPrefix(repo, "https://github.com/")
}

// namedBy reports whether a "; Source:" header line names the upstream
// repository, file, full commit and license that the audit records for u.
func (u queryUse) namedBy(line string) bool {
	for _, want := range []string{
		repoName(u.source.Repo),
		u.file.Upstream,
		u.source.Ref,
		"(" + u.source.SPDX + ")",
	} {
		if !strings.Contains(line, want) {
			return false
		}
	}
	return true
}

// checkQueryHeader compares one query file's header with the sources that
// list it. The header needs a "; Source:" line for each of them and a
// "; Change:" line that starts with "none" exactly when no source marks the
// file modified. It returns one message per disagreement.
func checkQueryHeader(name string, header []string, uses []queryUse) []string {
	var sourceLines []string
	change, hasChange := "", false
	for _, line := range header {
		switch {
		case strings.HasPrefix(line, "; Source:"):
			sourceLines = append(sourceLines, line)
		case strings.HasPrefix(line, "; Change:") && !hasChange:
			change, hasChange = strings.TrimSpace(strings.TrimPrefix(line, "; Change:")), true
		}
	}

	var problems []string
	named := make([]bool, len(sourceLines))
	modified := false
	for _, u := range uses {
		modified = modified || u.file.Modified
		found := false
		for i, line := range sourceLines {
			if u.namedBy(line) {
				named[i], found = true, true
			}
		}
		if !found {
			problems = append(problems, fmt.Sprintf("%s: no \"; Source:\" line for %s %s at %s (%s)",
				name, repoName(u.source.Repo), u.file.Upstream, u.source.Ref, u.source.SPDX))
		}
	}
	for i, line := range sourceLines {
		if !named[i] {
			problems = append(problems, fmt.Sprintf("%s: header line %q matches no source recorded for this file", name, line))
		}
	}

	none := strings.HasPrefix(strings.ToLower(change), "none")
	switch {
	case !hasChange:
		problems = append(problems, fmt.Sprintf("%s: no \"; Change:\" line (write \"; Change: none.\" for an unchanged copy)", name))
	case modified && none:
		problems = append(problems, fmt.Sprintf("%s: the header says \"Change: none\" but %s marks the file modified", name, queryAuditPath))
	case !modified && !none:
		problems = append(problems, fmt.Sprintf("%s: the header describes a change but %s marks the file unmodified", name, queryAuditPath))
	}
	return problems
}

// loadTexts reads every canonical license text the audits' SPDX values
// reference (splitting compound expressions like "Apache-2.0 OR MIT" and
// "Apache-2.0 WITH LLVM-exception" on their connective keywords), so a
// grammar or query source carrying an SPDX id with no matching
// licenses/texts/*.txt file fails the generator instead of silently shipping
// a notice with a missing license body.
func loadTexts(dir string, entries []grammarEntry, sources []querySource) (map[string]string, error) {
	needed := map[string]bool{}
	need := func(spdx string) {
		if spdx == "UNKNOWN" {
			return
		}
		for _, id := range splitSPDX(spdx) {
			needed[id] = true
		}
	}
	for _, e := range entries {
		need(e.SPDX)
	}
	for _, s := range sources {
		need(s.SPDX)
	}
	texts := map[string]string{}
	var missing []string
	for id := range needed {
		file, ok := spdxTextFiles[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return nil, fmt.Errorf("read license text for %s: %w", id, err)
		}
		texts[id] = strings.TrimRight(string(data), "\n")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("no canonical license text registered for SPDX id(s): %s (add licenses/texts/<id>.txt and a spdxTextFiles entry)", strings.Join(missing, ", "))
	}
	return texts, nil
}

// splitSPDX breaks a (possibly compound) SPDX expression into the license
// identifiers it references, dropping WITH-exception qualifiers (recorded in
// the rendered output as a note, not as a separate license body).
func splitSPDX(expr string) []string {
	expr = strings.SplitN(expr, " WITH ", 2)[0]
	var ids []string
	for _, part := range strings.Split(expr, " OR ") {
		part = strings.TrimSpace(part)
		if part != "" {
			ids = append(ids, part)
		}
	}
	return ids
}

func render(entries []grammarEntry, sources []querySource, texts map[string]string) ([]byte, error) {
	var b bytes.Buffer

	b.WriteString("THIRD_PARTY_NOTICES\n")
	b.WriteString("====================\n\n")
	b.WriteString("Generated by cmd/gen_license_notices from licenses/grammars.json and\n")
	b.WriteString("licenses/highlight_queries.json; DO NOT EDIT BY HAND.\n")
	b.WriteString("To update, edit one of those audit files (the sources of truth; see\n")
	b.WriteString("docs/licensing.md) and run `go run ./cmd/gen_license_notices`.\n\n")
	b.WriteString("gotreesitter's own source is MIT-licensed; see LICENSE. This file lists the\n")
	b.WriteString("upstream tree-sitter grammar repositories gotreesitter vendors grammar tables\n")
	b.WriteString("and, for a small number of grammars, hand-ported external scanner logic from,\n")
	b.WriteString("together with each repository's confirmed license, at the exact commit pinned\n")
	b.WriteString("in grammars/languages.lock. It also lists the upstream repositories that the\n")
	b.WriteString("highlight queries in grammars/highlight_queries/ were copied from.\n\n")

	writeSummary(&b, entries, sources)
	writeCopyleftSection(&b, entries)
	if err := writeNoticeFileAppendix(&b, entries); err != nil {
		return nil, err
	}
	writeQuerySources(&b, sources, texts)
	writeComponentGroups(&b, entries, texts)

	return b.Bytes(), nil
}

func writeSummary(b *bytes.Buffer, entries []grammarEntry, sources []querySource) {
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.SPDX]++
	}
	var ids []string
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if counts[ids[i]] != counts[ids[j]] {
			return counts[ids[i]] > counts[ids[j]]
		}
		return ids[i] < ids[j]
	})

	b.WriteString("Summary\n-------\n\n")
	fmt.Fprintf(b, "%d vendored grammar repositories, by confirmed SPDX identifier:\n\n", len(entries))
	for _, id := range ids {
		fmt.Fprintf(b, "  %4d  %s\n", counts[id], id)
	}
	b.WriteString("\n")

	if len(sources) > 0 {
		files := map[string]bool{}
		for _, s := range sources {
			for _, f := range s.Files {
				files[f.File] = true
			}
		}
		fmt.Fprintf(b, "%d embedded highlight query files, copied from %d upstream repositories\n", len(files), len(sources))
		b.WriteString("(see \"Imported highlight queries\").\n\n")
	}
}

func writeCopyleftSection(b *bytes.Buffer, entries []grammarEntry) {
	var copyleft []grammarEntry
	for _, e := range entries {
		if e.Copyleft {
			copyleft = append(copyleft, e)
		}
	}
	if len(copyleft) == 0 {
		return
	}
	sort.Slice(copyleft, func(i, j int) bool { return copyleft[i].Name < copyleft[j].Name })

	b.WriteString("Copyleft components (GPL-3.0 / MPL-2.0)\n")
	b.WriteString("----------------------------------------\n\n")
	b.WriteString("The following grammars come from copyleft-licensed upstream repositories.\n")
	b.WriteString("gotreesitter's own code stays MIT; these entries mark where a downstream\n")
	b.WriteString("consumer that embeds gotreesitter takes on GPL-3.0 or MPL-2.0 obligations for\n")
	b.WriteString("the affected grammar. See docs/licensing.md for what that means and for the\n")
	b.WriteString("`gotreesitter_no_copyleft` build tag, which excludes the hand-ported\n")
	b.WriteString("caddy/disassembly/nim external scanner sources and removes all four grammars\n")
	b.WriteString("(caddy, disassembly, jq, nim) from the public grammar registry.\n\n")
	for _, e := range copyleft {
		fmt.Fprintf(b, "  - %-12s %s  (%s)\n", e.Name, e.Repo, e.SPDX)
	}
	b.WriteString("\n")
}

func writeNoticeFileAppendix(b *bytes.Buffer, entries []grammarEntry) error {
	var withNotice []grammarEntry
	for _, e := range entries {
		if e.NoticeFileURL != "" {
			withNotice = append(withNotice, e)
		}
	}
	if len(withNotice) == 0 {
		return nil
	}
	sort.Slice(withNotice, func(i, j int) bool { return withNotice[i].Name < withNotice[j].Name })

	b.WriteString("Upstream NOTICE files\n---------------------\n\n")
	b.WriteString("Apache-2.0 requires a redistributed NOTICE file's attribution content to\n")
	b.WriteString("travel with the work. The following upstream repositories ship one, reproduced\n")
	b.WriteString("here in full as fetched at the pinned ref (see the URL for the live copy):\n\n")
	for _, e := range withNotice {
		fmt.Fprintf(b, "### %s NOTICE -- %s\n\n", e.Name, e.NoticeFileURL)
		if e.Note != "" {
			fmt.Fprintf(b, "%s\n\n", wrapNote(e.Note))
		}
		if e.NoticeFile == "" {
			continue
		}
		data, err := os.ReadFile(e.NoticeFile)
		if err != nil {
			return fmt.Errorf("read vendored notice file for %s: %w", e.Name, err)
		}
		b.Write(bytes.TrimRight(data, "\n"))
		b.WriteString("\n\n")
	}
	return nil
}

func writeComponentGroups(b *bytes.Buffer, entries []grammarEntry, texts map[string]string) {
	byGroup := map[string][]grammarEntry{}
	for _, e := range entries {
		byGroup[e.SPDX] = append(byGroup[e.SPDX], e)
	}
	var groups []string
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	b.WriteString("Components by license\n----------------------\n\n")

	for _, group := range groups {
		list := byGroup[group]
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

		fmt.Fprintf(b, "### %s (%d component%s)\n\n", group, len(list), plural(len(list)))

		if group == "UNKNOWN" {
			b.WriteString("License could not be confirmed for the following. See docs/licensing.md\n")
			b.WriteString("for the recommended action on each.\n\n")
			for _, e := range list {
				fmt.Fprintf(b, "  - %-12s %s @ %s\n", e.Name, e.Repo, shortRef(e.Ref))
			}
			b.WriteString("\n")
			continue
		}

		for _, e := range list {
			writeComponent(b, e)
			b.WriteString("\n")
		}

		writeLicenseTexts(b, group, texts)
	}
}

// writeQuerySources renders the upstream repositories that highlight query
// files were copied from, grouped by license like the grammar components. Each
// group is followed by its license text, so the section stands on its own: the
// recipient of an Apache-2.0 file gets the license from the same place that
// names the source.
func writeQuerySources(b *bytes.Buffer, sources []querySource, texts map[string]string) {
	if len(sources) == 0 {
		return
	}
	byGroup := map[string][]querySource{}
	for _, s := range sources {
		byGroup[s.SPDX] = append(byGroup[s.SPDX], s)
	}
	var groups []string
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	b.WriteString("Imported highlight queries\n--------------------------\n\n")
	b.WriteString("gotreesitter embeds the highlight queries in grammars/highlight_queries/*.scm.\n")
	b.WriteString("They were copied from the repositories below, each at the exact commit named,\n")
	b.WriteString("under the license that repository states at that commit. Each file opens with a\n")
	b.WriteString("header comment that names its source file, commit and license and says what\n")
	b.WriteString("changed. A file marked (modified) differs from its upstream file beyond that\n")
	b.WriteString("header. The full text of each license follows its group.\n\n")

	for _, group := range groups {
		list := byGroup[group]
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

		fmt.Fprintf(b, "### %s (%d source%s)\n\n", group, len(list), plural(len(list)))
		for _, s := range list {
			writeComponent(b, s.grammarEntry)
			b.WriteString("  Files:\n")
			files := append([]queryFile(nil), s.Files...)
			sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
			for _, f := range files {
				modified := ""
				if f.Modified {
					modified = " (modified)"
				}
				fmt.Fprintf(b, "    %-20s <- %s%s\n", f.File, f.Upstream, modified)
			}
			b.WriteString("\n")
		}

		writeLicenseTexts(b, group, texts)
	}
}

// writeComponent writes one component's name and source, its copyright lines,
// its license file and its note. The caller ends the entry with a blank line.
func writeComponent(b *bytes.Buffer, e grammarEntry) {
	fmt.Fprintf(b, "%s -- %s @ %s\n", e.Name, e.Repo, shortRef(e.Ref))
	if len(e.CopyrightHolders) > 0 {
		for _, h := range e.CopyrightHolders {
			fmt.Fprintf(b, "  %s\n", h)
		}
	} else if e.RepositoryOwner != "" {
		fmt.Fprintf(b, "  (no copyright line stated upstream; repository owner: %s)\n", e.RepositoryOwner)
	}
	fmt.Fprintf(b, "  License file: %s\n", e.LicenseFileURL)
	if e.Note != "" {
		fmt.Fprintf(b, "  Note: %s\n", wrapNote(e.Note))
	}
}

// writeLicenseTexts writes the canonical text of each license in a (possibly
// compound) SPDX expression.
func writeLicenseTexts(b *bytes.Buffer, group string, texts map[string]string) {
	for _, id := range splitSPDX(group) {
		fmt.Fprintf(b, "-- %s license text --\n\n", id)
		b.WriteString(texts[id])
		b.WriteString("\n\n")
	}
}

func shortRef(ref string) string {
	if len(ref) > 12 {
		return ref[:12]
	}
	return ref
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// wrapNote keeps long note strings from producing absurdly long lines in the
// generated file; it does not need to be pretty, just readable.
func wrapNote(note string) string {
	const width = 96
	words := strings.Fields(note)
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() > 0 && cur.Len()+1+len(w) > width {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return strings.Join(lines, "\n    ")
}
