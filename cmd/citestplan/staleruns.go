// Stale -run/--run name checking.
//
// CI workflows name specific Test/Benchmark/Fuzz/Example functions in
// `go test -run '<pattern>'` (or `--run`) arguments. When a test is renamed
// or deleted but the workflow reference is not updated, that CI step keeps
// passing while silently testing nothing. This file scans every workflow
// under a glob for `-run`/`--run` patterns, decomposes each pattern into its
// concrete name or prefix alternatives, and checks each alternative against
// every `func TestX`/`BenchmarkX`/`FuzzX`/`ExampleX` declaration found under
// a repository root. The declaration scan is a plain text scan of every
// `*_test.go` file, so it sees a name regardless of the file's build tags.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// altKind classifies one concrete alternative produced by expandPattern.
type altKind int

const (
	// altExact requires an identical defined name.
	altExact altKind = iota
	// altPrefix requires some defined name to start with the alternative's
	// text (the pattern ends in an unbounded repetition such as ".*").
	altPrefix
	// altUnknown marks a regexp construct this decomposer does not model.
	// Alternatives of this kind are skipped rather than reported, so an
	// unmodeled construct never produces a false stale-name failure.
	altUnknown
)

type alt struct {
	text string
	kind altKind
}

// expandPattern parses a Go regexp source string and enumerates the set of
// concrete alternatives it can match. An alternative that is not required to
// reach the end of the input (the pattern has no trailing "$"/"\z") is
// downgraded from an exact match requirement to a prefix requirement,
// matching how go test's -run actually behaves: regexp.MatchString accepts
// any substring match, so an unanchored tail lets arbitrary text follow.
func expandPattern(pattern string) ([]alt, error) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}
	alts := expandRegexp(re)
	if !endsAnchored(re) {
		for i := range alts {
			if alts[i].kind == altExact {
				alts[i].kind = altPrefix
			}
		}
	}
	return alts, nil
}

// endsAnchored reports whether re's match is required to reach the end of
// the input (i.e. it ends in "$" / "\z").
func endsAnchored(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpEndText:
		return true
	case syntax.OpCapture:
		return endsAnchored(re.Sub[0])
	case syntax.OpConcat:
		if len(re.Sub) == 0 {
			return false
		}
		return endsAnchored(re.Sub[len(re.Sub)-1])
	case syntax.OpAlternate:
		if len(re.Sub) == 0 {
			return false
		}
		for _, s := range re.Sub {
			if !endsAnchored(s) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func expandRegexp(re *syntax.Regexp) []alt {
	switch re.Op {
	case syntax.OpBeginText, syntax.OpEndText, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpEmptyMatch, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return []alt{{kind: altExact}}
	case syntax.OpLiteral:
		return []alt{{text: string(re.Rune), kind: altExact}}
	case syntax.OpCapture:
		return expandRegexp(re.Sub[0])
	case syntax.OpAlternate:
		var out []alt
		for _, s := range re.Sub {
			out = append(out, expandRegexp(s)...)
		}
		return out
	case syntax.OpConcat:
		combined := []alt{{kind: altExact}}
		for _, s := range re.Sub {
			subAlts := expandRegexp(s)
			next := make([]alt, 0, len(combined)*len(subAlts))
			for _, c := range combined {
				if c.kind != altExact {
					// Already open-ended; further concatenation cannot
					// narrow it back down.
					next = append(next, c)
					continue
				}
				for _, a := range subAlts {
					next = append(next, alt{text: c.text + a.text, kind: a.kind})
				}
			}
			combined = next
		}
		return combined
	case syntax.OpQuest:
		subAlts := expandRegexp(re.Sub[0])
		return append([]alt{{kind: altExact}}, subAlts...)
	case syntax.OpStar, syntax.OpPlus, syntax.OpRepeat:
		// An unbounded (or wide) repetition makes everything from here to
		// the end of the pattern an open suffix; treat the alternative as a
		// prefix requirement from this point.
		return []alt{{kind: altPrefix}}
	case syntax.OpCharClass:
		// Go's regexp parser factors a single-character alternation such as
		// "TestA|TestB" into a shared literal prefix "Test" concatenated
		// with a character class "[AB]" rather than keeping it as a plain
		// OpAlternate. Enumerate small classes so that shape still expands
		// to exact per-character alternatives; a wide or unbounded-looking
		// class degrades to altUnknown to avoid a combinatorial blow-up.
		const maxClassRunes = 16
		var out []alt
		total := 0
		for i := 0; i+1 < len(re.Rune); i += 2 {
			lo, hi := re.Rune[i], re.Rune[i+1]
			total += int(hi-lo) + 1
			if total > maxClassRunes {
				return []alt{{kind: altUnknown}}
			}
			for r := lo; r <= hi; r++ {
				out = append(out, alt{text: string(r), kind: altExact})
			}
		}
		if len(out) == 0 {
			return []alt{{kind: altUnknown}}
		}
		return out
	default:
		return []alt{{kind: altUnknown}}
	}
}

// firstSegment returns the portion of a -run pattern before the first "/".
// go test's -run flag treats "/" as a subtest separator and matches each
// segment against the corresponding path element independently; only the
// first segment names a top-level Test/Benchmark/Fuzz/Example function.
func firstSegment(pattern string) string {
	if idx := strings.IndexByte(pattern, '/'); idx >= 0 {
		return pattern[:idx]
	}
	return pattern
}

// looksDynamic reports whether s contains GitHub Actions template syntax
// ("${{ ... }}"), shell parameter expansion ("${...}"), or shell command
// substitution ("$(...)"). None of those constructs can be resolved
// statically, so content containing them is skipped rather than checked.
func looksDynamic(s string) bool {
	return strings.Contains(s, "${") || strings.Contains(s, "$(")
}

var bareVarRe = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)$`)

// scriptVars holds the shell variable assignments a single run: script
// makes, resolved just enough to recover the literal -run patterns it
// builds. It supports two shapes seen in this repository's workflows:
//
//   - a single-quoted single-line assignment holding a literal pattern
//     directly, e.g. admission='^TestFoo(A|B)$';
//   - a single-quoted multi-line assignment holding a whitespace-separated
//     list of names/fragments, e.g. tests='\nTestA\nTestB\n', which the
//     script later joins into "^(TestA|TestB)$" with tr/paste. Since that
//     join happens inside a command substitution this scanner cannot
//     evaluate literally, any later "-run \"$var\"" reference to an
//     unresolved variable falls back to the script's sole list variable.
type scriptVars struct {
	byName map[string]string
	blocks []string
}

// Go's regexp package (RE2) has no backreferences, so the single- and
// double-quoted single-line assignment forms need separate patterns instead
// of one pattern with a \1-style same-quote-close backreference.
var (
	assignSingleQuoteRe = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z_][A-Za-z0-9_]*)='([^'\n]*)'[ \t]*$`)
	assignDoubleQuoteRe = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z_][A-Za-z0-9_]*)="([^"\n]*)"[ \t]*$`)
	assignBlockRe       = regexp.MustCompile(`(?s)([A-Za-z_][A-Za-z0-9_]*)='\n(.*?)\n[ \t]*'`)
)

func extractScriptVars(script string) scriptVars {
	sv := scriptVars{byName: map[string]string{}}
	for _, m := range assignSingleQuoteRe.FindAllStringSubmatch(script, -1) {
		sv.byName[m[1]] = m[2]
	}
	for _, m := range assignDoubleQuoteRe.FindAllStringSubmatch(script, -1) {
		sv.byName[m[1]] = m[2]
	}
	for _, m := range assignBlockRe.FindAllStringSubmatch(script, -1) {
		name, body := m[1], m[2]
		var tokens []string
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			tokens = append(tokens, line)
		}
		if len(tokens) == 0 {
			continue
		}
		resolved := "^(" + strings.Join(tokens, "|") + ")$"
		sv.byName[name] = resolved
		sv.blocks = append(sv.blocks, resolved)
	}
	return sv
}

func resolvePatternVar(name string, sv scriptVars) (string, bool) {
	if v, ok := sv.byName[name]; ok && !looksDynamic(v) {
		return v, true
	}
	if len(sv.blocks) == 1 {
		return sv.blocks[0], true
	}
	return "", false
}

// resolveContent turns one raw -run/--run argument into a checkable regexp
// pattern, or reports why it cannot be checked statically.
func resolveContent(raw string, sv scriptVars) (pattern string, skip bool, reason string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "^$" {
		return "", true, "intentional no-op pattern"
	}
	if looksDynamic(raw) {
		return "", true, "dynamic value (GitHub Actions expression or shell substitution): " + raw
	}
	if m := bareVarRe.FindStringSubmatch(raw); m != nil {
		resolved, ok := resolvePatternVar(m[1], sv)
		if !ok {
			return "", true, "unresolved shell variable: " + raw
		}
		return resolveContent(resolved, sv)
	}
	return raw, false, ""
}

var (
	runSingleQuotedRe = regexp.MustCompile(`-{1,2}run\s+'([^']*)'`)
	runDoubleQuotedRe = regexp.MustCompile(`-{1,2}run\s+"([^"]*)"`)
)

// stripDocLines blanks out lines that quote a shell command inside
// backticks for a step-summary message (for example an `echo` line building
// Markdown). Those lines can contain an illustrative "-run '...'" example
// that was never passed to go test, so they must not be scanned as a real
// invocation.
func stripDocLines(script string) string {
	lines := strings.Split(script, "\n")
	for i, line := range lines {
		if strings.Contains(line, "`") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// findRunPatterns returns the raw (unresolved) quoted arguments that follow
// -run or --run in a shell script.
func findRunPatterns(script string) []string {
	var out []string
	for _, m := range runSingleQuotedRe.FindAllStringSubmatch(script, -1) {
		out = append(out, m[1])
	}
	for _, m := range runDoubleQuotedRe.FindAllStringSubmatch(script, -1) {
		out = append(out, m[1])
	}
	return out
}

// finding describes one -run/--run reference this checker could not match
// to a real, defined test/benchmark/fuzz/example.
type finding struct {
	File    string
	Job     string
	Step    int
	Pattern string
	Name    string
	Reason  string
}

func (f finding) String() string {
	if f.Reason != "" {
		return fmt.Sprintf("%s: job %q step %d: -run %q: %s", f.File, f.Job, f.Step, f.Pattern, f.Reason)
	}
	return fmt.Sprintf("%s: job %q step %d: -run %q references undefined test/benchmark/fuzz/example %q", f.File, f.Job, f.Step, f.Pattern, f.Name)
}

func hasPrefixMatch(sorted []string, prefix string) bool {
	if prefix == "" {
		return len(sorted) > 0
	}
	i := sort.SearchStrings(sorted, prefix)
	return i < len(sorted) && strings.HasPrefix(sorted[i], prefix)
}

// auditWorkflowFile checks every -run/--run reference in path's job steps
// against defined (and, for prefix alternatives, sortedNames). It returns
// the stale references found and the number of patterns it was able to
// check.
func auditWorkflowFile(path string, data []byte, defined map[string]bool, sortedNames []string) ([]finding, int, error) {
	wf, err := parseWorkflow(data)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}

	jobNames := make([]string, 0, len(wf.Jobs))
	for name := range wf.Jobs {
		jobNames = append(jobNames, name)
	}
	sort.Strings(jobNames)

	var findings []finding
	checked := 0
	for _, jobName := range jobNames {
		j := wf.Jobs[jobName]
		for stepIdx, step := range j.Steps {
			if strings.TrimSpace(step.Run) == "" {
				continue
			}
			script := stripDocLines(step.Run)
			sv := extractScriptVars(script)
			for _, raw := range findRunPatterns(script) {
				pattern, skip, _ := resolveContent(raw, sv)
				if skip {
					continue
				}
				top := firstSegment(pattern)
				if top == "" || top == "^$" || looksDynamic(top) {
					continue
				}
				alts, err := expandPattern(top)
				if err != nil {
					findings = append(findings, finding{
						File: path, Job: jobName, Step: stepIdx, Pattern: raw,
						Reason: "invalid regexp: " + err.Error(),
					})
					continue
				}
				checked++
				for _, a := range alts {
					switch a.kind {
					case altUnknown:
						continue
					case altExact:
						if a.text != "" && !defined[a.text] {
							findings = append(findings, finding{
								File: path, Job: jobName, Step: stepIdx, Pattern: raw, Name: a.text,
							})
						}
					case altPrefix:
						if !hasPrefixMatch(sortedNames, a.text) {
							findings = append(findings, finding{
								File: path, Job: jobName, Step: stepIdx, Pattern: raw, Name: a.text,
								Reason: fmt.Sprintf("prefix %q matches no defined test/benchmark/fuzz/example", a.text),
							})
						}
					}
				}
			}
		}
	}
	return findings, checked, nil
}

var funcDeclRe = regexp.MustCompile(`^func\s+((?:Test|Benchmark|Fuzz|Example)[A-Za-z0-9_]*)\s*\(`)

// collectDefinedFuncNames scans every *_test.go file under root for
// top-level Test/Benchmark/Fuzz/Example function declarations. It is a
// plain text scan, not a build, so a name is found regardless of the
// file's build tags or containing Go module.
func collectDefinedFuncNames(root string) (map[string]bool, error) {
	names := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimLeft(line, " \t")
			if !strings.HasPrefix(trimmed, "func ") {
				continue
			}
			if m := funcDeclRe.FindStringSubmatch(trimmed); m != nil {
				names[m[1]] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// runStaleRunNamesCheck is the entry point wired into cmd/citestplan's
// -stale-run-names mode.
func runStaleRunNamesCheck(workflowGlob, repoRoot string, out io.Writer) error {
	paths, err := filepath.Glob(workflowGlob)
	if err != nil {
		return fmt.Errorf("glob workflows %q: %w", workflowGlob, err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("no workflow files matched %q", workflowGlob)
	}
	sort.Strings(paths)

	defined, err := collectDefinedFuncNames(repoRoot)
	if err != nil {
		return fmt.Errorf("scan test definitions under %q: %w", repoRoot, err)
	}
	if len(defined) == 0 {
		return fmt.Errorf("found no Test/Benchmark/Fuzz/Example definitions under %q", repoRoot)
	}
	sortedNames := make([]string, 0, len(defined))
	for n := range defined {
		sortedNames = append(sortedNames, n)
	}
	sort.Strings(sortedNames)

	var allFindings []finding
	checked := 0
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		findings, n, err := auditWorkflowFile(p, data, defined, sortedNames)
		if err != nil {
			return err
		}
		allFindings = append(allFindings, findings...)
		checked += n
	}

	if len(allFindings) > 0 {
		for _, f := range allFindings {
			fmt.Fprintln(out, f.String())
		}
		return fmt.Errorf("%d -run/--run reference(s) name a test, benchmark, fuzz target, or example that does not exist", len(allFindings))
	}

	fmt.Fprintf(out, "Checked %d -run/--run pattern(s) across %d workflow file(s) against %d defined Test/Benchmark/Fuzz/Example function(s); no stale references found.\n",
		checked, len(paths), len(defined))
	return nil
}

var errNoJobs = errors.New("workflow has no jobs")

func parseWorkflow(data []byte) (workflow, error) {
	var wf workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return workflow{}, err
	}
	if wf.Jobs == nil {
		return workflow{}, errNoJobs
	}
	return wf, nil
}
