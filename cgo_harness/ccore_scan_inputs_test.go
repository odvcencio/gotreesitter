//go:build cgo && treesitter_c_parity

package cgoharness

// Inputs for any grammar with
// an external scanner: the committed sample, other committed real files of
// the language, and the 72-step edit session of the sample (every step
// parsed fresh). CCORE_LANG=<language> or <language>-session.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// ccoreBaseLang strips the "-session" and "-eof" suffixes.
func ccoreBaseLang(name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(name, "-session"), "-eof")
}

// ccoreEOFInputs are sources that end without a trailing newline or with an
// invalid byte, where the lexer runs at the end of input (CCORE_LANG=<language>-eof).
func ccoreEOFInputs(lang string) []ccoreInput {
	switch lang {
	case "json":
		return []ccoreInput{
			{"json/eof/open_string", []byte("{\"a")},
			{"json/eof/open_object", []byte("{\"a\": [1, 2")},
			{"json/eof/invalid_byte", []byte("[\xff]\n")},
			{"json/eof/invalid_byte_end", []byte("[1, \xff")},
			{"json/eof/at_sign", []byte("[1, @]")},
			{"json/eof/empty", []byte("")},
			{"json/eof/newline", []byte("\n")},
			{"json/eof/bom", []byte("\xef\xbb\xbf[]")},
			{"json/eof/bom_only", []byte("\xef\xbb\xbf")},
			{"json/eof/bom_invalid_byte", []byte("\xef\xbb\xbf[\xff]\n")},
		}
	case "python":
		return []ccoreInput{
			{"python/eof/dedent_in_error", []byte("def f():\n    g(1,")},
			{"python/eof/open_call_newline", []byte("def f():\n    g(1,\n")},
			{"python/eof/nested", []byte("class A:\n  def f(self):\n    x = (1,")},
			{"python/eof/invalid_byte", []byte("x = 1\xff")},
			{"python/eof/indent_only", []byte("if x:\n    ")},
			{"python/eof/backslash_open_call", []byte("(1, \\")},
			{"python/eof/backslash_only", []byte("\\")},
			{"python/eof/backslash_newline", []byte("x = 1 \\\n")},
			{"python/eof/bom", []byte("\xef\xbb\xbfx = 1\n")},
		}
	case "c", "cpp", "glsl":
		// An accept before a SKIP: the preprocessor argument ends at a backslash-newline
		// and the lexer skips the newline (states 33, 66, 146 of the C grammar).
		return []ccoreInput{
			{lang + "/eof/define_continued", []byte("#define A \\\n")},
			{lang + "/eof/define_continued_twice", []byte("#define A \\\n  \\\n")},
			{lang + "/eof/define_continued_body", []byte("#define A \\\n  1\n")},
			{lang + "/eof/define_continued_no_newline", []byte("#define A \\")},
		}
	case "earthfile":
		// Start states whose first transition is ADVANCE_MAP(0, ...): at the end of input C returns
		// the "\0" token.
		return []ccoreInput{
			{"earthfile/eof/empty", []byte("")},
			{"earthfile/eof/newline", []byte("\n")},
			{"earthfile/eof/version", []byte("VERSION 0.8")},
			{"earthfile/eof/version_newline", []byte("VERSION 0.8\n")},
			{"earthfile/eof/from", []byte("FROM alpine")},
			{"earthfile/eof/from_newline", []byte("FROM alpine\n")},
			{"earthfile/eof/target", []byte("build:\n    RUN echo")},
			{"earthfile/eof/run_quote", []byte("RUN \"\"")},
			{"earthfile/eof/run_quote_newline", []byte("RUN \"\"\n")},
			{"earthfile/eof/run_space", []byte("RUN ")},
			{"earthfile/eof/backslash", []byte("RUN a \\")},
			{"earthfile/eof/comment", []byte("# a")},
		}
	}
	return nil
}

// ccoreFrozen is the folder of frozen copies of this repository's own files,
// for languages that have too few files under a testdata folder.
const ccoreFrozen = "cgo_harness/testdata/ccore/"

// ccoreExtraFiles lists, by name, the files of a language that the equality
// tests read besides the committed sample (paths relative to the repository
// root; the test runs in cgo_harness). Every path is a file under a testdata
// folder. No pattern: an edit to a tool, script or source file must not change
// the input of a required check. A listed file that cannot be read fails the
// test.
func ccoreExtraFiles(lang string) []string {
	switch lang {
	case "typescript":
		return []string{
			ccoreFrozen + "typescript/typescript_sample.ts",
			"grammars/testdata/typescript_issue_544.ts",
			"grammars/testdata/typescript_issue1429.ts",
			"testdata/outline/typescript/model.ts",
		}
	case "tsx":
		return []string{"testdata/outline/tsx/view.tsx"}
	case "c":
		return []string{
			"testdata/incremental_gate/c_repeated_functions.c",
			"testdata/outline/c/registry.c",
		}
	case "python":
		return []string{
			"testdata/incremental_gate/python_large_indent.py",
			"testdata/incremental_gate/python_setup.py",
			"testdata/dispatcher_census_a0/doxygen/medium__metrics.py",
			"testdata/outline/python/app.py",
			"testdata/parser_result/python/module_assignment.py",
			"testdata/parser_result/python/module_import_from.py",
			"testdata/parser_result/python/terminal_class_and_if_suffix.py",
			"testdata/parser_result/python/terminal_if_suffix.py",
			"cgo_harness/testdata/canonical_incremental_python_scanner.py",
			ccoreFrozen + "python/cgo_harness__corpus_structural__python_sample.py",
			ccoreFrozen + "python/cgo_harness__oracle__diff_join.py",
			ccoreFrozen + "python/cgo_harness__oracle__refresh_sources.py",
			ccoreFrozen + "python/cgo_harness__oracle__test_diff_join.py",
			ccoreFrozen + "python/cgo_harness__tier_scan__gen_external_lex_elections.py",
			ccoreFrozen + "python/github_scripts__fuzz_issues.py",
			ccoreFrozen + "python/scripts__benchfixture_corpus.py",
			ccoreFrozen + "python/scripts__check_incremental_invariant_ledger.py",
			ccoreFrozen + "python/scripts__grammar_receipts_test.py",
			ccoreFrozen + "python/scripts__plan_grammar_receipts.py",
			ccoreFrozen + "python/scripts__render_grammar_update_issue.py",
			ccoreFrozen + "python/scripts__summarize_grammar_receipts.py",
			ccoreFrozen + "python/scripts__test_grammar_update_issue.py",
			ccoreFrozen + "python/scripts__test_w5_private_oracle.py",
			ccoreFrozen + "python/scripts__test_w5_private_oracle_parent.py",
			ccoreFrozen + "python/scripts__w5_private_oracle.py",
			ccoreFrozen + "python/scanner_state_indents.py",
		}
	case "bash":
		return []string{"testdata/incremental_gate/bash_clean_old.sh", "testdata/compact_converged_split/bash.sh"}
	}
	return nil
}

// ccoreScanInputs returns the inputs of a scanner-grammar run; ok is false
// for the names the trunk harness handles itself.
func ccoreScanInputs(t testing.TB, name string, small bool) ([]ccoreInput, bool) {
	switch name {
	case "json", "go", "go-corpus", "json-rec-d", "go-rec-a", "go-rec-b", "go-rec-c", "go-rec-e":
		return nil, false
	}
	lang := ccoreBaseLang(name)
	if strings.HasSuffix(name, "-eof") {
		return ccoreEOFInputs(lang), true
	}
	sample := ccoreManifestSample(t, lang)
	if strings.HasSuffix(name, "-session") {
		var out []ccoreInput
		for i, step := range benchfixtures.EditingSession(sample) {
			out = append(out, ccoreInput{lang + "/session/" + itoa(i+1), step.Source})
		}
		return out, true
	}
	out := []ccoreInput{{lang + "/sample", sample}}
	if small {
		return out, true
	}
	largest := ccoreInput{lang + "/largest", sample}
	for _, f := range ccoreExtraFiles(lang) {
		src, err := os.ReadFile(filepath.Join("..", f))
		if err != nil {
			t.Fatalf("listed input %s: %v", f, err)
		}
		out = append(out, ccoreInput{lang + "/file/" + filepath.ToSlash(f), src})
		if len(src) > len(largest.Src) {
			largest.Src = src
		}
	}
	out = append(out, largest)
	return out, true
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

func ccoreScanBenchID(id string) bool {
	return strings.HasSuffix(id, "/sample") || strings.HasSuffix(id, "/largest")
}

// ccoreNormLine removes backslashes from a log line when CCORE_NORM_NAMES is
// set. The NUL token is named "\0" in the Go table and logged as sym:\\0 (the log
// escapes the backslash); C prints its name as an empty string, because the
// name holds a NUL byte.
func ccoreNormLine(s string) string {
	if os.Getenv("CCORE_NORM_NAMES") != "" {
		s = strings.ReplaceAll(s, "sym:\\\\0,", "sym:,")
		return strings.ReplaceAll(s, "\\", "")
	}
	return s
}
