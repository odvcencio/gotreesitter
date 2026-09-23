package grammargen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestLanguageVersionStampedForSupertypeMapWithoutReservedWords pins the
// Stage-1 fix to assemble()'s LanguageVersion stamping: any ABI-15-only
// surface (not just the reserved-word table) must bump LanguageVersion to
// 15. Before the fix, assemble() hardcoded LanguageVersion=14 and only
// bumped it to 15 inside buildReservedWordTables, so a grammar with
// supertypes but no reserved words shipped a non-empty SupertypeMapEntries
// (an ABI-15 surface) under a LanguageVersion=14 stamp — an internally
// inconsistent blob.
func TestLanguageVersionStampedForSupertypeMapWithoutReservedWords(t *testing.T) {
	g := NewGrammar("supertype_only_abi15")
	g.Define("source_file", Sym("expression"))
	g.Define("expression", Choice(
		Sym("number_literal"),
		Sym("string_literal"),
	))
	g.Define("number_literal", Str("1"))
	g.Define("string_literal", Str("x"))
	g.SetSupertypes("expression")

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	if len(lang.SupertypeMapEntries) == 0 {
		t.Fatal("test grammar did not exercise SupertypeMapEntries; test is not testing what it claims")
	}
	if len(lang.ReservedWords) != 0 {
		t.Fatal("test grammar unexpectedly populated ReservedWords; test is not isolating the supertype-map surface")
	}
	if lang.LanguageVersion != 15 {
		t.Fatalf("LanguageVersion = %d, want 15 (SupertypeMapEntries is an ABI-15 surface)", lang.LanguageVersion)
	}
}

// TestLanguageVersionStaysAtBaselineWithoutAnyABI15Surface guards the other
// direction: a grammar with no reserved words and no supertypes must not be
// over-stamped to 15 — that would make the language
// falsely claim ABI-15 features it doesn't have.
func TestLanguageVersionStaysAtBaselineWithoutAnyABI15Surface(t *testing.T) {
	g := NewGrammar("no_abi15_surface")
	g.Define("source_file", Str("x"))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	if len(lang.SupertypeMapEntries) != 0 || len(lang.ReservedWords) != 0 {
		t.Fatal("test grammar unexpectedly populated an ABI-15 surface; test is not isolating the baseline case")
	}
	if lang.LanguageVersion != 14 {
		t.Fatalf("LanguageVersion = %d, want 14 (no ABI-15 surface present)", lang.LanguageVersion)
	}
}

func TestGeneratedCABI14LexModesRuntime(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler is unavailable")
	}

	g := NewGrammar("abi14_lex_modes_runtime")
	g.Define("source_file", Str("x"))
	lang, err := GenerateLanguageForC(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	if lang.LanguageVersion != 14 {
		t.Fatalf("LanguageVersion = %d, want 14", lang.LanguageVersion)
	}
	if len(lang.LexModes) < 2 {
		t.Fatalf("LexModes = %d, want a multi-state table", len(lang.LexModes))
	}
	hasLaterMode := false
	for _, mode := range lang.LexModes[1:] {
		if mode.LexState != 0 || mode.ExternalLexState != 0 {
			hasLaterMode = true
			break
		}
	}
	if !hasLaterMode {
		t.Fatal("generated grammar has no nonzero lex mode after state zero")
	}
	// EmitC synthesizes the EOF token at each parser-start lexer state.
	// Keep the generated DFA unchanged so this probe exercises the ABI-14
	// lex-mode layout rather than an artificial EOF state.
	code, err := EmitC(g.Name, lang)
	if err != nil {
		t.Fatalf("EmitC: %v", err)
	}
	if !strings.Contains(code, "static const TSLexMode ts_lex_modes") {
		t.Fatal("ABI-14 C output does not use the locked runtime's TSLexMode layout")
	}
	if strings.Contains(code, "static const TSLexerMode ts_lex_modes") || strings.Contains(code, ".reserved_word_set_id") {
		t.Fatal("ABI-14 C output leaked the ABI-15 lexer-mode surface")
	}

	runtimeDir := lockedTreeSitterRuntimeDir(t)
	tmp, parserPath := writeLockedRuntimeProbeFiles(t, runtimeDir, code)
	mainSource := `#include <stdint.h>
#include <tree_sitter/api.h>

const TSLanguage *tree_sitter_abi14_lex_modes_runtime(void);

int main(void) {
  const TSLanguage *language = tree_sitter_abi14_lex_modes_runtime();
  if (ts_language_abi_version(language) != 14) return 10;
  TSParser *parser = ts_parser_new();
  if (!parser || !ts_parser_set_language(parser, language)) return 11;
  TSTree *tree = ts_parser_parse_string(parser, NULL, "x", 1);
  if (!tree) return 12;
  TSNode root = ts_tree_root_node(tree);
  if (ts_node_has_error(root)) return 13;
  if (ts_node_start_byte(root) != 0 || ts_node_end_byte(root) != 1) return 14;
  ts_tree_delete(tree);
  ts_parser_delete(parser);
  return 0;
}
`
	mainPath := filepath.Join(tmp, "main.c")
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		t.Fatalf("write C runtime probe: %v", err)
	}
	artifact := filepath.Join(tmp, "abi14-lex-modes-probe")
	args := []string{
		"-std=c11", "-O0", "-D_DEFAULT_SOURCE",
		"-I" + filepath.Join(tmp, "include"),
		"-I" + filepath.Join(runtimeDir, "include"),
		"-I" + filepath.Join(runtimeDir, "src"),
		parserPath, filepath.Join(runtimeDir, "src", "lib.c"), mainPath,
		"-pthread", "-o", artifact,
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile generated ABI-14 parser: %v\n%s", err, out)
	}
	if out, err := exec.Command(artifact).CombinedOutput(); err != nil {
		t.Fatalf("run generated ABI-14 multi-state parse probe: %v\n%s", err, out)
	}
}

func TestEmitCRejectsMalformedSupertypeSurfaces(t *testing.T) {
	g := NewGrammar("supertype_contract")
	g.Define("source_file", Sym("expression"))
	g.Define("expression", Choice(Sym("number_literal"), Sym("string_literal")))
	g.Define("number_literal", Str("1"))
	g.Define("string_literal", Str("x"))
	g.SetSupertypes("expression")
	base, err := GenerateLanguageForC(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	clone := func() *gotreesitter.Language {
		fresh, err := GenerateLanguageForC(g)
		if err != nil {
			t.Fatalf("GenerateLanguage clone: %v", err)
		}
		return fresh
	}
	supertype := base.SupertypeSymbols[0]
	tests := []struct {
		name string
		edit func(*gotreesitter.Language)
		want string
	}{
		{"ABI 14", func(l *gotreesitter.Language) { l.LanguageVersion = 14 }, "requires ABI 15"},
		{"missing symbols", func(l *gotreesitter.Language) { l.SupertypeSymbols = nil }, "incomplete"},
		{"missing slices", func(l *gotreesitter.Language) { l.SupertypeMapSlices = nil }, "incomplete"},
		{"missing entries", func(l *gotreesitter.Language) { l.SupertypeMapEntries = nil }, "incomplete"},
		{"short slice table", func(l *gotreesitter.Language) { l.SupertypeMapSlices = l.SupertypeMapSlices[:1] }, "slices"},
		{"supertype outside symbols", func(l *gotreesitter.Language) { l.SupertypeSymbols[0] = gotreesitter.Symbol(len(l.SymbolNames)) }, "outside symbol tables"},
		{"missing supertype metadata", func(l *gotreesitter.Language) { l.SymbolMetadata[supertype].Supertype = false }, "without supertype metadata"},
		{"zero-length slice", func(l *gotreesitter.Language) { l.SupertypeMapSlices[supertype][1] = 0 }, "invalid supertype slice"},
		{"out-of-bounds slice", func(l *gotreesitter.Language) {
			l.SupertypeMapSlices[supertype] = [2]uint16{uint16(len(l.SupertypeMapEntries)), 1}
		}, "invalid supertype slice"},
		{"subtype outside symbols", func(l *gotreesitter.Language) { l.SupertypeMapEntries[0] = gotreesitter.Symbol(len(l.SymbolNames)) }, "outside symbol names"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := clone()
			tt.edit(lang)
			_, err := EmitC(g.Name, lang)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("EmitC error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestGeneratedCABI15SupertypeRuntime(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler is unavailable")
	}

	g := NewGrammar("supertype_only_abi15")
	g.Define("source_file", Sym("expression"))
	g.Define("expression", Choice(Sym("number_literal"), Sym("string_literal")))
	g.Define("number_literal", Str("1"))
	g.Define("string_literal", Str("x"))
	g.SetSupertypes("expression")

	lang, err := GenerateLanguageForC(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	lang.Metadata = gotreesitter.LanguageMetadata{MajorVersion: 1, MinorVersion: 2, PatchVersion: 3}
	code, err := EmitC(g.Name, lang)
	if err != nil {
		t.Fatalf("EmitC: %v", err)
	}
	supertype, ok := lang.SymbolByName("expression")
	if !ok {
		t.Fatal("generated language missing expression supertype")
	}
	subtypes := lang.SupertypeChildren(supertype)
	if len(subtypes) != 2 {
		t.Fatalf("Go supertype children = %v, want two", subtypes)
	}

	runtimeDir := lockedTreeSitterRuntimeDir(t)
	tmp, parserPath := writeLockedRuntimeProbeFiles(t, runtimeDir, code)

	mainSource := fmt.Sprintf(`#include <stdint.h>
#include <tree_sitter/api.h>
#include <string.h>

const TSLanguage *tree_sitter_supertype_only_abi15(void);

int main(void) {
  const TSLanguage *language = tree_sitter_supertype_only_abi15();
  if (ts_language_abi_version(language) != 15) return 10;
  if (!ts_language_name(language) || strcmp(ts_language_name(language), "supertype_only_abi15") != 0) return 14;
  const TSLanguageMetadata *metadata = ts_language_metadata(language);
  if (!metadata || metadata->major_version != 1 || metadata->minor_version != 2 || metadata->patch_version != 3) return 15;
  uint32_t count = 0;
  const TSSymbol *supertypes = ts_language_supertypes(language, &count);
  if (!supertypes || count != 1 || supertypes[0] != %d) return 11;
  const TSSymbol *children = ts_language_subtypes(language, %d, &count);
  if (!children || count != 2) return 12;
  if (children[0] != %d || children[1] != %d) return 13;
  return 0;
}
`, supertype, supertype, subtypes[0], subtypes[1])
	mainPath := filepath.Join(tmp, "main.c")
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		t.Fatalf("write C runtime probe: %v", err)
	}
	artifact := filepath.Join(tmp, "supertype-probe")
	args := []string{
		"-std=c11", "-O0", "-ffunction-sections", "-fdata-sections", "-Wl,--gc-sections",
		"-I" + filepath.Join(tmp, "include"),
		"-I" + filepath.Join(runtimeDir, "include"),
		"-I" + filepath.Join(runtimeDir, "src"),
		parserPath, filepath.Join(runtimeDir, "src", "language.c"), mainPath,
		"-o", artifact,
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile generated ABI-15 parser: %v\n%s", err, out)
	}
	if out, err := exec.Command(artifact).CombinedOutput(); err != nil {
		t.Fatalf("run generated ABI-15 subtype probe: %v\n%s", err, out)
	}
}

func lockedTreeSitterRuntimeDir(t *testing.T) string {
	t.Helper()
	// cgo_harness/go.mod declares a newer `go` directive than this root
	// module, so `go -C ../cgo_harness list -m` running under an older
	// toolchain triggers GOTOOLCHAIN=auto: on a cache that does not
	// already have that newer version, `go` downloads it before
	// continuing. The CI job that runs this test installs a toolchain
	// that already satisfies cgo_harness's floor (see grammargen_stable in
	// ci.yml), so that download should not happen there, but a caller
	// with an older toolchain -- a local run, or some other job -- can
	// still hit it. Output(), not CombinedOutput(), guards that case: a
	// CombinedOutput() capture merges the "go: downloading go1.X.Y
	// (linux/amd64)" progress line (stderr) with the real `{{.Dir}}` path
	// (stdout), so the line lands in front of the path and every
	// filepath.Join built from it resolves to a nonexistent directory
	// ("open go: downloading ... /src/parser.h: no such file or
	// directory") instead of a clear error. stderr still reaches the
	// failure message via cmd.Stderr below.
	//
	// `go mod download` first, unconditionally: on a runner where nothing
	// earlier in the job has already built or tested anything inside
	// cgo_harness, `go list -m -f {{.Dir}}` alone can print nothing at all
	// (no stdout, no stderr, exit 0) instead of fetching the module on
	// demand -- observed on a CI runner with the right toolchain already
	// installed and a cold cgo_harness cache. Downloading first removes
	// that dependency on module-cache state entirely.
	dlCmd := exec.Command("go", "-C", filepath.Join("..", "cgo_harness"), "mod", "download", "github.com/tree-sitter/go-tree-sitter")
	var dlStderr bytes.Buffer
	dlCmd.Stderr = &dlStderr
	if err := dlCmd.Run(); err != nil {
		t.Fatalf("download locked tree-sitter runtime: %v\n%s", err, dlStderr.String())
	}
	cmd := exec.Command("go", "-C", filepath.Join("..", "cgo_harness"), "list", "-m", "-f", "{{.Dir}}", "github.com/tree-sitter/go-tree-sitter")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	moduleOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("locate locked tree-sitter runtime: %v\n%s", err, stderr.String())
	}
	dir := strings.TrimSpace(string(moduleOut))
	if dir == "" {
		t.Fatalf("locate locked tree-sitter runtime: `go list -m -f {{.Dir}}` printed nothing\nstderr:\n%s", stderr.String())
	}
	return dir
}

func writeLockedRuntimeProbeFiles(t *testing.T, runtimeDir, code string) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	includeDir := filepath.Join(tmp, "include", "tree_sitter")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatalf("create include dir: %v", err)
	}
	parserHeader, err := os.ReadFile(filepath.Join(runtimeDir, "src", "parser.h"))
	if err != nil {
		t.Fatalf("read locked parser.h: %v", err)
	}
	if err := os.WriteFile(filepath.Join(includeDir, "parser.h"), parserHeader, 0o644); err != nil {
		t.Fatalf("write parser.h: %v", err)
	}
	parserPath := filepath.Join(tmp, "parser.c")
	if err := os.WriteFile(parserPath, []byte(code), 0o644); err != nil {
		t.Fatalf("write generated parser.c: %v", err)
	}
	return tmp, parserPath
}
