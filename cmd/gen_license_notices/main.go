// Command gen_license_notices regenerates THIRD_PARTY_NOTICES from
// licenses/grammars.json, the confirmed per-grammar license audit for every
// upstream tree-sitter grammar gotreesitter vendors. See docs/licensing.md
// for the audit method and the GPL-3.0/MPL-2.0 handling this file documents.
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
	"sort"
	"strings"
)

const (
	auditPath  = "licenses/grammars.json"
	textsDir   = "licenses/texts"
	lockPath   = "grammars/languages.lock"
	outputPath = "THIRD_PARTY_NOTICES"
)

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

	texts, err := loadTexts(textsDir, a.Entries)
	if err != nil {
		return err
	}

	out, err := render(a.Entries, texts)
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

// loadTexts reads every canonical license text the audit's SPDX values
// reference (splitting compound expressions like "Apache-2.0 OR MIT" and
// "Apache-2.0 WITH LLVM-exception" on their connective keywords), so a
// grammar carrying an SPDX id with no matching licenses/texts/*.txt file
// fails the generator instead of silently shipping a notice with a missing
// license body.
func loadTexts(dir string, entries []grammarEntry) (map[string]string, error) {
	needed := map[string]bool{}
	for _, e := range entries {
		if e.SPDX == "UNKNOWN" {
			continue
		}
		for _, id := range splitSPDX(e.SPDX) {
			needed[id] = true
		}
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

func render(entries []grammarEntry, texts map[string]string) ([]byte, error) {
	var b bytes.Buffer

	b.WriteString("THIRD_PARTY_NOTICES\n")
	b.WriteString("====================\n\n")
	b.WriteString("Generated by cmd/gen_license_notices from licenses/grammars.json; DO NOT EDIT BY HAND.\n")
	b.WriteString("To update, edit licenses/grammars.json (the audit source of truth; see\n")
	b.WriteString("docs/licensing.md) and run `go run ./cmd/gen_license_notices`.\n\n")
	b.WriteString("gotreesitter's own source is MIT-licensed; see LICENSE. This file lists the\n")
	b.WriteString("upstream tree-sitter grammar repositories gotreesitter vendors grammar tables\n")
	b.WriteString("and, for a small number of grammars, hand-ported external scanner logic from,\n")
	b.WriteString("together with each repository's confirmed license, at the exact commit pinned\n")
	b.WriteString("in grammars/languages.lock.\n\n")

	writeSummary(&b, entries)
	writeCopyleftSection(&b, entries)
	if err := writeNoticeFileAppendix(&b, entries); err != nil {
		return nil, err
	}
	writeComponentGroups(&b, entries, texts)

	return b.Bytes(), nil
}

func writeSummary(b *bytes.Buffer, entries []grammarEntry) {
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
			b.WriteString("\n")
		}

		for _, id := range splitSPDX(group) {
			fmt.Fprintf(b, "-- %s license text --\n\n", id)
			b.WriteString(texts[id])
			b.WriteString("\n\n")
		}
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
