package grammargen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPythonPatternMatchingParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "match event.get():\n" +
		"    case Click(position=(x, y)):\n" +
		"        handle_click_at(x, y)\n" +
		"    case KeyPress(key_name=\"Q\") | Quit():\n" +
		"        game.quit()\n" +
		"    case KeyPress(key_name=\"up arrow\"):\n" +
		"        game.go_north()\n" +
		"        ...\n" +
		"    case KeyPress():\n" +
		"        pass # Ignore other keystrokes\n" +
		"    case other_event:\n" +
		"        raise ValueError(f\"Unrecognized event: {other_event}\")\n"

	assertPythonParity(t, genLang, refLang, sample)
}

func TestPythonFStringLiteralParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "# nested!\n" +
		"f\"a {b(f'c {e} d')} e\"\n" +
		"f\"\"\"a\"{b}c\"\"\"\n" +
		"f\"\"\"a\"\"{b}c\"\"\"\n" +
		"f\"a {{}} e\"\n" +
		"f\"a {b}}}\"\n" +
		"f\"a {{{b}\"\n" +
		"f\"a {{b}}\"\n" +
		"f\"a {{{b}}}\"\n" +
		"f\"{c,}\"\n" +
		"f\"{yield d}\"\n" +
		"f\"{*a,}\"\n" +
		"\n" +
		"def function():\n" +
		"    return f\"\"\"\n" +
		"{\"string1\" if True else\n" +
		" \"string2\"}\"\"\"\n" +
		"\n" +
		"def test(self):\n" +
		"    self.assertEqual(f'''A complex trick: {\n" +
		"2  # two\n" +
		"}''', 'A complex trick: 2')\n"

	assertPythonParity(t, genLang, refLang, sample)
}

func TestPython2PrintChevronParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "def driver(file, gulp):\n" +
		"    print >> sys.stdout, 1, 2, 3\n" +
		"    print >> sys.stdout\n" +
		"    print >> gulp, 1, 2, 3,\n" +
		"    print >> file, 'hello world'\n"

	assertPythonParity(t, genLang, refLang, sample)
}

func TestPythonTypeAliasStatementParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	samples := []string{
		"type Point = tuple[float, float]\n",
		"type Point[T] = tuple[T, T]\n",
		"type IntFunc[**P] = Callable[P, int]\n",
		"type LabeledTuple[*Ts] = tuple[str, *Ts]\n",
		"type HashableSequence[T: Hashable] = Sequence[T]\n",
		"type IntOrStrSequence[T: (int, str)] = Sequence[T]\n",
		"type Point = tuple[float, float]\n" +
			"type Point[T] = tuple[T, T]\n" +
			"type IntFunc[**P] = Callable[P, int]  # ParamSpec\n" +
			"type LabeledTuple[*Ts] = tuple[str, *Ts]  # TypeVarTuple\n" +
			"type HashableSequence[T: Hashable] = Sequence[T]  # TypeVar with bound\n" +
			"type IntOrStrSequence[T: (int, str)] = Sequence[T]  # TypeVar with constraints\n",
	}

	for _, sample := range samples {
		assertPythonParity(t, genLang, refLang, sample)
	}
}

func TestPythonDoubleStarParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	samples := []string{
		"def g(**kwarg):\n    pass\n",
		"def g(h, i, /, j, *, k=100, **kwarg):\n    pass\n",
		"async def i(a, b=c, *c, **d):\n    a\n",
		"x = a ** b\n",
		"type IntFunc[**P] = Callable[P, int]\n",
	}

	for _, sample := range samples {
		assertPythonParity(t, genLang, refLang, sample)
	}
}

func TestPythonGeneratedRepeatContinuationParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	samples := []string{
		"a.b.c",
		"2**2**3\n-2**2",
		"if a:\n  b\n  c",
		"{a, b, c,}\n{*{}}",
		`"\x12 \123 \u1234"`,
	}

	for _, sample := range samples {
		assertPythonParity(t, genLang, refLang, sample)
	}
}

func TestPythonGeneratedRepeatContinuationParityWithExternalOrderAdapter(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	if scanner, ok := gotreesitter.AdaptExternalScannerByExternalOrder(refLang, genLang); ok {
		genLang.ExternalScanner = scanner
	}

	samples := []string{
		"a.b.c",
		"2**2**3\n-2**2",
		"if a:\n  b\n  c",
		"{a, b, c,}\n{*{}}",
		`"\x12 \123 \u1234"`,
	}

	for _, sample := range samples {
		assertPythonParity(t, genLang, refLang, sample)
	}
}

func TestPythonGeneratedExternalOrderCorpusFloorCanary(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	if scanner, ok := gotreesitter.AdaptExternalScannerByExternalOrder(refLang, genLang); ok {
		genLang.ExternalScanner = scanner
	}

	samples := loadPythonCorpusBlocksForTest(t, 20)
	parser := gotreesitter.NewParser(genLang)
	for i, sample := range samples {
		tree, err := parser.Parse([]byte(sample))
		if err != nil {
			t.Fatalf("sample %d parse returned error: %v\nsource:\n%s", i, err, sample)
		}
		t.Cleanup(tree.Release)
		if root := tree.RootNode(); root.HasError() {
			t.Fatalf("sample %d has ERROR\nGEN: %s\nsource:\n%s", i, root.SExpr(genLang), sample)
		}
	}
}

func TestTokenPrecStringDoesNotGloballyOverrideLongerBareString(t *testing.T) {
	g := NewGrammar("token_prec_prefix")
	g.Define("source_file", Choice(Sym("short"), Sym("long"), Seq(Str("q"), Sym("contextual"))))
	g.Define("short", Seq(Str("*"), Str("x")))
	g.Define("long", Seq(Str("**"), Str("y")))
	g.Define("contextual", Seq(Token(Prec(1, Str("*"))), Str("z")))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}

	tree, err := gotreesitter.NewParser(lang).Parse([]byte("**y"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(3); got != want {
		t.Fatalf("root end byte = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.SExpr(lang), "(source_file (long))"; got != want {
		t.Fatalf("SExpr = %s, want %s", got, want)
	}
}

func loadGeneratedPythonLanguageForParity(t *testing.T) *gotreesitter.Language {
	t.Helper()

	gram := loadPythonGrammarJSONForTest(t)
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Python language: %v", err)
	}
	return genLang
}

func loadPythonCorpusBlocksForTest(t *testing.T, limit int) []string {
	t.Helper()
	root := ""
	for _, candidate := range []string{
		"/tmp/grammar_parity/python",
		".parity_seed/python",
		"../.parity_seed/python",
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			root = candidate
			break
		}
	}
	if root == "" {
		t.Skip("Python corpus unavailable")
	}

	dir := filepath.Join(root, "test", "corpus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("Python corpus unavailable: %v", err)
	}
	var samples []string
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		for _, block := range extractPythonCorpusBlocksForTest(string(data)) {
			normalized := strings.ReplaceAll(block, "\r\n", "\n")
			trimmed := strings.TrimSpace(normalized)
			if trimmed == "" || seen[trimmed] {
				continue
			}
			seen[trimmed] = true
			samples = append(samples, normalized)
		}
	}
	sort.Slice(samples, func(i, j int) bool { return len(samples[i]) < len(samples[j]) })
	if len(samples) == 0 {
		t.Skip("Python corpus has no blocks")
	}
	if limit > 0 && len(samples) > limit {
		samples = samples[:limit]
	}
	return samples
}

func extractPythonCorpusBlocksForTest(data string) []string {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var out []string
	for i := 0; i < len(lines); {
		if !pythonCorpusEqualsFenceForTest(lines[i]) {
			i++
			continue
		}
		i++
		for i < len(lines) && !pythonCorpusEqualsFenceForTest(lines[i]) {
			i++
		}
		if i >= len(lines) {
			break
		}
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		start := i
		for i < len(lines) && !pythonCorpusDashFenceForTest(lines[i]) {
			i++
		}
		if i > start {
			src := strings.Trim(strings.Join(lines[start:i], "\n"), "\n")
			if strings.TrimSpace(src) != "" {
				out = append(out, src)
			}
		}
		if i < len(lines) {
			i++
		}
	}
	return out
}

func pythonCorpusEqualsFenceForTest(line string) bool {
	return pythonCorpusFenceForTest(line, '=')
}

func pythonCorpusDashFenceForTest(line string) bool {
	return pythonCorpusFenceForTest(line, '-')
}

func pythonCorpusFenceForTest(line string, want rune) bool {
	s := strings.TrimSpace(line)
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if r != want {
			return false
		}
	}
	return true
}

func assertPythonParity(t *testing.T, genLang, refLang *gotreesitter.Language, sample string) {
	t.Helper()

	genTree, err := gotreesitter.NewParser(genLang).Parse([]byte(sample))
	if err != nil {
		t.Fatalf("generated parse returned error: %v", err)
	}
	refTree, err := gotreesitter.NewParser(refLang).Parse([]byte(sample))
	if err != nil {
		t.Fatalf("reference parse returned error: %v", err)
	}
	t.Cleanup(genTree.Release)
	t.Cleanup(refTree.Release)

	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()
	genSexp := genRoot.SExpr(genLang)
	refSexp := refRoot.SExpr(refLang)

	if genRoot.HasError() || refRoot.HasError() {
		t.Fatalf("error mismatch\nGEN hasError=%v\nGEN: %s\nREF hasError=%v\nREF: %s",
			genRoot.HasError(), genSexp, refRoot.HasError(), refSexp)
	}
	if genSexp != refSexp {
		t.Fatalf("SExpr mismatch\nGEN: %s\nREF: %s", genSexp, refSexp)
	}
	if divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10); len(divs) > 0 {
		t.Fatalf("deep mismatch: %s\nGEN: %s\nREF: %s", divs[0].String(), genSexp, refSexp)
	}
}
