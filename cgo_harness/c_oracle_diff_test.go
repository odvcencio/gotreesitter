//go:build cgo && treesitter_c_parity && gts_oracle_diff

package cgoharness

// Oracle diff tool. For every (grammar, input) pair it parses the input on the
// locked C oracle and on gotreesitter, and writes one JSONL row with both
// deep digests. Run the tool once against each oracle checkout, then join the
// two files on (grammar, class, id) to see what an oracle upgrade changes.
//
// It does nothing unless GTS_ORACLE_DIFF_OUT is set. Run it in the parity
// container, one process at a time:
//
//	GTS_ORACLE_DIFF_OUT=/workspace/out.jsonl \
//	  go test . -tags 'treesitter_c_parity gts_oracle_diff' \
//	  -run '^TestCOracleDiff$' -count=1 -timeout 3h -v
//
// Environment:
//
//	GTS_ORACLE_DIFF_OUT       JSONL output path (required). A .meta.json file
//	                          with the oracle identity is written beside it.
//	GTS_ORACLE_DIFF_LANGS     comma list of grammars (default: all registered)
//	GTS_ORACLE_DIFF_CLASSES   comma list of input classes (default: all)
//	GTS_ORACLE_DIFF_TIMEOUT   per-parse timeout, Go duration (default 20s)
//	GTS_ORACLE_DIFF_HANG      per-input hard limit before the watchdog stops
//	                          the process, default 4x the parse timeout
//	GTS_ORACLE_DIFF_RESUME=1  skip inputs that already have rows in the
//	                          output file and append the rest
//
// A parser can spin inside a scanner without reaching a timeout check. When
// one input exceeds the hard limit, the watchdog records the input and the
// side (C or Go) in <out>.hangs and exits with status 75. Rerun with
// GTS_ORACLE_DIFF_RESUME=1: the input is retried with the hung side skipped,
// and its row says so in c_error or go_error.
//
// Input classes: smoke, smoke_fallback, real, edit1 (the source after the first
// edit of the pinned editing session, as in the counter ledger),
// recovery_board, tiny, mut, witness, known1545 (the inputs of the twelve
// known Go/C differences in issue 1545). Rows are ordered by grammar, then class, then id, so two runs over
// the same inputs produce rows in the same order.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	oracleDiffSexpCap     = 4096
	oracleDiffInputB64Cap = 4096
	oracleDiffDigestWall  = 30 * time.Second
)

// oracleDiffProgress is what the watchdog reads to find a stuck input.
type oracleDiffProgress struct {
	in    oracleDiffInput
	side  string
	start time.Time
}

var oracleDiffCurrent atomic.Pointer[oracleDiffProgress]

func oracleDiffKey(grammar, class, id string) string { return grammar + "\x00" + class + "\x00" + id }

const oracleDiffHangExit = 75

type oracleDiffInput struct {
	grammar string
	class   string
	id      string
	source  []byte
}

// oracleDiffRow is one JSONL line. Field order is fixed so output is stable.
type oracleDiffRow struct {
	OracleRuntime string `json:"oracle_runtime"`
	Grammar       string `json:"grammar"`
	Class         string `json:"class"`
	ID            string `json:"id"`
	InputBytes    int    `json:"input_bytes"`
	InputSHA256   string `json:"input_sha256"`
	InputB64      string `json:"input_b64,omitempty"`

	CDigest      string `json:"c_digest"`
	CHasError    bool   `json:"c_has_error"`
	CSexpSHA256  string `json:"c_sexp_sha256"`
	CRootType    string `json:"c_root_type"`
	CRootEnd     uint   `json:"c_root_end"`
	CNodes       int    `json:"c_nodes"`
	CErrorNodes  int    `json:"c_error_nodes"`
	CMissingNode int    `json:"c_missing_nodes"`
	CSexp        string `json:"c_sexp"`
	CSexpTrunc   bool   `json:"c_sexp_truncated,omitempty"`
	CError       string `json:"c_error,omitempty"`

	GoDigest      string `json:"go_digest"`
	GoHasError    bool   `json:"go_has_error"`
	GoSexpSHA256  string `json:"go_sexp_sha256"`
	GoRootType    string `json:"go_root_type"`
	GoRootEnd     uint32 `json:"go_root_end"`
	GoNodes       int    `json:"go_nodes"`
	GoErrorNodes  int    `json:"go_error_nodes"`
	GoMissingNode int    `json:"go_missing_nodes"`
	GoStop        string `json:"go_stop,omitempty"`
	GoSexp        string `json:"go_sexp"`
	GoSexpTrunc   bool   `json:"go_sexp_truncated,omitempty"`
	GoError       string `json:"go_error,omitempty"`

	GoEqualsC     bool   `json:"go_equals_c"`
	FirstDivCat   string `json:"first_div_category,omitempty"`
	FirstDivPath  string `json:"first_div_path,omitempty"`
	FirstDivGo    string `json:"first_div_go,omitempty"`
	FirstDivC     string `json:"first_div_c,omitempty"`
	ComparedBoth  bool   `json:"compared"`
	ParseTimedOut bool   `json:"c_timed_out,omitempty"`
}

type oracleDiffMeta struct {
	RunID                string                 `json:"run_id"`
	Inputs               map[string]string      `json:"inputs"`
	OracleRuntimeVersion string                 `json:"oracle_runtime_version"`
	OracleRuntimeCommit  string                 `json:"oracle_runtime_commit"`
	BindingCommit        string                 `json:"binding_commit"`
	TimeoutMillis        int64                  `json:"parse_timeout_ms"`
	HardLimitMillis      int64                  `json:"hard_limit_ms"`
	Grammars             map[string]interface{} `json:"grammar_identity"`
	LoadErrors           map[string]string      `json:"load_errors,omitempty"`
}

// oracleDiffTinyInputs are the empty and one-to-two byte probes.
var oracleDiffTinyInputs = []string{"", " ", "\n", "x", "x\n", "1", ";", "#", "{", ")", "\"", "\t", "\r\n", "\xff"}

// oracleDiffSyntheticSamples mirror cmd/perfcounterledger: these two grammars
// have no committed real sample because the source licenses need review.
var oracleDiffSyntheticSamples = map[string]string{
	"csv":     "name,value\nexample,1\n",
	"enforce": "class Example { int value; }\n",
}

type oracleDiffWitness struct {
	lang   string
	id     string
	source string
}

// oracleDiffWitnesses are the nightly fuzz and census witnesses. The
// JavaScript and HTML ones run on both grammars, because the fuzz target
// exercises them on each.
var oracleDiffWitnesses = func() []oracleDiffWitness {
	var out []oracleDiffWitness
	both := []struct{ id, src string }{
		{"empty_fragment", "<></>0"},
		{"html_nightly_1027", "<html><BodY>00000</0000<html>0"},
		{"html_lower_body", "<html><body>00000</0000<html>0"},
		{"js_nightly_1021", "<html><body>Hello</body></html>0"},
		{"js_1519_a", "<A><A>AAAAA</A></A>0"},
		{"jsx_fragment_unclosed", "<>"},
		{"jsx_text_only", "<a>0</a>"},
	}
	for _, lang := range []string{"javascript", "html"} {
		for _, w := range both {
			out = append(out, oracleDiffWitness{lang: lang, id: w.id, source: w.src})
		}
	}
	own := []oracleDiffWitness{
		{"go", "fuzz_comma", ","},
		{"go", "fuzz_unterminated_string", "package main\n\nfunc main() {\"00000000000\n} "},
		{"go", "fuzz_in_before_func", "package m\nin\n\nfunc main() {\"00000000000\n} "},
		{"haskell", "fuzz_a_colon", "a::()0=0"},
		{"haskell", "fuzz_a0", "a0::()0=\"\" ; "},
		{"haskell", "fuzz_m_bang", "m!0000000000000()00000000A00000000000000! 000000000000=0000000000\"00000\""},
		{"haskell", "f2_where_case", "t=()where\n R=case d of\n      R->()\n      t->case s of\n        g->()\n        ()->()\nr"},
		{"scala", "fuzz_object_def", "object A { def A(A:A),A000 0} "},
		{"scala", "fuzz_object_at", "object @@@@@A { def A(A:A),A000 0},"},
		{"kotlin", "f2_actual_fun", "class r():g by r{actual fun n(e:y){}\nactual fun t(){try{({->})}catch(e:e){f(){}}}\n}"},
		{"agda", "f2_paren", "(I U p)"},
		{"djot", "f2_link_open", "[](ht\n"},
		{"norg", "f2_at_end", "@g\n@end"},
		{"python", "f2_brace", "{"},
		{"python", "f2_rparen", ")"},
		{"python", "f2_quote", "\""},
		{"python", "f2_import", "import"},
		{"python", "f2_dict_missing_value", "x = {1: 2, 3}"},
		{"python", "f2_return_rparen", "return\n)"},
		{"javascript", "f2_let_eq_semi", "let x = ;\n"},
		{"javascript", "f2_latin1_y", "ÿ"},
	}
	return append(out, own...)
}()

// oracleDiffKnown1545 holds the inline inputs of the known differences listed
// in issue 1545. The AWK fixture is loaded from testdata, and the RON case is
// the edit1 row of the ron grammar.
var oracleDiffKnown1545 = []oracleDiffWitness{
	{"html", "html_log_7", "<html><bodyHello</body></html>"},
	{"fidl", "fidl_two_modifiers_with_args", "library test;\ntype Color = strict(removed=2) flexible(added=2) enum {\n    RED = 1;\n};"},
	{"fidl", "fidl_single_modifier_with_args", "library test;\ntype Color = strict(removed=2) enum {\n    RED = 1;\n};"},
	{"fidl", "fidl_three_modifiers_with_args", "library test;\ntype Color = strict(removed=2) flexible(added=2) resource(added=3) enum {\n    RED = 1;\n};"},
	{"bibtex", "bibtex_missing_comma_after_key", "@article{test\n    title     = {title-from-embedded-bibtex-file},\n    author    = {author-from-embedded-bibtex-file},\n}\n"},
	{"bibtex", "bibtex_missing_key", "@techreport{\n  journal = {Wirtschaftsinformatik},\n  author = {Sam and jason},\n}\n"},
}

func oracleDiffBuildInputs(t *testing.T, langs []string, classes map[string]bool) []oracleDiffInput {
	t.Helper()
	want := func(class string) bool { return len(classes) == 0 || classes[class] }
	langSet := make(map[string]bool, len(langs))
	for _, l := range langs {
		langSet[l] = true
	}
	var manifest struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			CommittedPath string `json:"committed_path"`
		} `json:"entries"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatalf("read real corpus manifest: %v", err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode real corpus manifest: %v", err)
	}
	realPath := map[string]string{}
	for _, e := range manifest.Entries {
		if e.Role == "sample" {
			realPath[e.Language] = e.CommittedPath
		}
	}

	var inputs []oracleDiffInput
	add := func(grammar, class, id string, src []byte) {
		if langSet[grammar] && want(class) {
			inputs = append(inputs, oracleDiffInput{grammar: grammar, class: class, id: id, source: src})
		}
	}
	for _, lang := range langs {
		base := grammars.ParseSmokeSample(lang)
		class := "smoke_fallback"
		if hasDedicatedSample[lang] {
			class = "smoke"
		}
		add(lang, class, "smoke_sample", []byte(base))

		if path := realPath[lang]; path != "" {
			b, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", path))
			if err != nil {
				t.Fatalf("%s: read committed real sample %s: %v", lang, path, err)
			}
			add(lang, "real", "real_sample", b)
			add(lang, "edit1", "edit1_source", benchfixtures.EditingSession(b)[0].Source)
		} else if s, ok := oracleDiffSyntheticSamples[lang]; ok {
			add(lang, "real", "real_synthetic", []byte(s))
		}

		for i, s := range oracleDiffTinyInputs {
			add(lang, "tiny", fmt.Sprintf("tiny_%02d", i), []byte(s))
		}

		// Mutations of the smoke sample, as in the F2 census.
		trim := strings.TrimRight(base, " \n\t")
		mid := len(base) / 2
		if strings.HasSuffix(base, "\n") {
			add(lang, "mut", "nonl", []byte(strings.TrimRight(base, "\n")))
		}
		add(lang, "mut", "crlf", []byte(strings.ReplaceAll(base, "\n", "\r\n")))
		add(lang, "mut", "leadws", []byte("\n  "+base))
		if len(trim) > 1 {
			add(lang, "mut", "droplast", []byte(trim[:len(trim)-1]+"\n"))
		}
		if len(base) > 2 {
			add(lang, "mut", "insparen", []byte(base[:mid]+")"+base[mid:]))
			add(lang, "mut", "delmid", []byte(base[:mid]+base[mid+1:]))
			add(lang, "mut", "half", []byte(base[:mid]))
		}
	}
	for i, tc := range parityRecoveryCases {
		add(tc.lang, "recovery_board", fmt.Sprintf("rb_%03d", i), []byte(tc.source))
	}
	for _, w := range oracleDiffWitnesses {
		add(w.lang, "witness", w.id, []byte(w.source))
	}
	for _, w := range oracleDiffKnown1545 {
		add(w.lang, "known1545", w.id, []byte(w.source))
	}
	add("awk", "known1545", "awk_recovered_T_gawk", awkRecoveredFixture(t))
	classOrder := map[string]int{"smoke": 0, "smoke_fallback": 1, "real": 2, "edit1": 3, "recovery_board": 4, "tiny": 5, "mut": 6, "witness": 7, "known1545": 8}
	sort.SliceStable(inputs, func(i, j int) bool {
		a, b := inputs[i], inputs[j]
		if a.grammar != b.grammar {
			return a.grammar < b.grammar
		}
		if a.class != b.class {
			return classOrder[a.class] < classOrder[b.class]
		}
		return a.id < b.id
	})
	return inputs
}

func oracleDiffSHA(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func oracleDiffCap(s string) (string, bool) {
	if len(s) <= oracleDiffSexpCap {
		return strings.ToValidUTF8(s, "�"), false
	}
	return strings.ToValidUTF8(s[:oracleDiffSexpCap], "�"), true
}

// oracleDiffWalkC counts nodes with an iterative cursor walk.
func oracleDiffWalkC(root *sitter.Node) (nodes, errNodes, missing int) {
	cursor := root.Walk()
	defer cursor.Close()
	count := func(n *sitter.Node) {
		nodes++
		if n.IsError() {
			errNodes++
		}
		if n.IsMissing() {
			missing++
		}
	}
	count(root)
	for {
		if cursor.GotoFirstChild() {
			count(cursor.Node())
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				return
			}
		}
		count(cursor.Node())
	}
}

func oracleDiffWalkGo(n *gotreesitter.Node) (nodes, errNodes, missing int) {
	if n == nil {
		return
	}
	nodes = 1
	if n.IsError() {
		errNodes++
	}
	if n.IsMissing() {
		missing++
	}
	for i := 0; i < n.ChildCount(); i++ {
		a, b, c := oracleDiffWalkGo(n.Child(i))
		nodes += a
		errNodes += b
		missing += c
	}
	return
}

func oracleDiffParseC(row *oracleDiffRow, cLang *sitter.Language, src []byte, timeout time.Duration) (cTree *sitter.Tree, cParser *sitter.Parser) {
	defer func() {
		if r := recover(); r != nil {
			row.CError = fmt.Sprintf("panic: %v", r)
		}
	}()
	cParser = sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		row.CError = "SetLanguage: " + err.Error()
		return nil, cParser
	}
	cParser.SetTimeoutMicros(uint64(timeout.Microseconds()))
	cTree = cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		row.CError = "C parser returned nil tree (timeout or cancelled)"
		row.ParseTimedOut = true
		return nil, cParser
	}
	return cTree, cParser
}

// oracleDiffParseGo mirrors parseWithGo and adds a per-parse timeout.
func oracleDiffParseGo(name string, src []byte, timeout time.Duration) (tree *gotreesitter.Tree, lang *gotreesitter.Language, err error) {
	defer func() {
		if r := recover(); r != nil {
			tree, lang, err = nil, nil, fmt.Errorf("panic: %v", r)
		}
	}()
	entry, ok := parityEntriesByName[name]
	if !ok {
		return nil, nil, fmt.Errorf("missing gotreesitter registry entry for %q", name)
	}
	report, ok := paritySupportForName(name)
	if !ok {
		return nil, nil, fmt.Errorf("missing parse support report for %q", name)
	}
	lang = entry.Language()
	parser := gotreesitter.NewParser(lang)
	parser.SetTimeoutMicros(uint64(timeout.Microseconds()))
	switch report.Backend {
	case grammars.ParseBackendTokenSource:
		if entry.TokenSourceFactory == nil {
			return nil, nil, fmt.Errorf("token source backend without factory for %q", name)
		}
		tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(src)
	default:
		return nil, nil, fmt.Errorf("unsupported parse backend %q for %q", report.Backend, name)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("gotreesitter parse error: %w", err)
	}
	if tree == nil || tree.RootNode() == nil {
		return nil, nil, fmt.Errorf("gotreesitter returned nil tree")
	}
	return tree, lang, nil
}

func oracleDiffSetSide(in oracleDiffInput, side string) {
	oracleDiffCurrent.Store(&oracleDiffProgress{in: in, side: side, start: time.Now()})
}

func oracleDiffRun(in oracleDiffInput, cLang *sitter.Language, cLoadErr error, timeout time.Duration, skipC, skipGo bool) oracleDiffRow {
	row := oracleDiffRow{
		OracleRuntime: COracleRuntimeVersion,
		Grammar:       in.grammar,
		Class:         in.class,
		ID:            in.id,
		InputBytes:    len(in.source),
		InputSHA256:   oracleDiffSHA(in.source),
	}
	if len(in.source) <= oracleDiffInputB64Cap {
		row.InputB64 = base64.StdEncoding.EncodeToString(in.source)
	}

	var cRoot *sitter.Node
	if cLoadErr != nil {
		row.CError = "load: " + cLoadErr.Error()
	} else if skipC {
		row.CError = "hang: C parse did not return in an earlier attempt; skipped"
		row.ParseTimedOut = true
	} else {
		oracleDiffSetSide(in, "c")
		cTree, cParser := oracleDiffParseC(&row, cLang, in.source, timeout)
		if cParser != nil {
			defer cParser.Close()
		}
		if cTree != nil {
			defer cTree.Close()
			cRoot = cTree.RootNode()
			row.CHasError = cRoot.HasError()
			row.CRootType = cRoot.Kind()
			row.CRootEnd = cRoot.EndByte()
			sexp := cRoot.ToSexp()
			row.CSexpSHA256 = oracleDiffSHA([]byte(sexp))
			row.CSexp, row.CSexpTrunc = oracleDiffCap(sexp)
			row.CNodes, row.CErrorNodes, row.CMissingNode = oracleDiffWalkC(cRoot)
			digest, err := cOracleDeepDigestWithin(cTree, oracleDiffDigestWall)
			if err != nil {
				row.CError = "digest: " + err.Error()
			} else {
				row.CDigest = digest
			}
		}
	}

	var (
		goTree *gotreesitter.Tree
		goLang *gotreesitter.Language
		err    error
	)
	if skipGo {
		err = fmt.Errorf("hang: Go parse did not return in an earlier attempt; skipped")
	} else {
		oracleDiffSetSide(in, "go")
		goTree, goLang, err = oracleDiffParseGo(in.grammar, in.source, timeout)
	}
	oracleDiffCurrent.Store(nil)
	if err != nil {
		row.GoError = err.Error()
	} else {
		defer releaseGoTree(goTree)
		goRoot := goTree.RootNode()
		row.GoHasError = goRoot.HasError()
		row.GoRootType = goRoot.Type(goLang)
		row.GoRootEnd = goRoot.EndByte()
		row.GoStop = string(goTree.ParseStopReason())
		if goTree.ParseStoppedEarly() {
			row.GoError = "parse stopped early: " + row.GoStop
		}
		sexp := goRoot.SExpr(goLang)
		row.GoSexpSHA256 = oracleDiffSHA([]byte(sexp))
		row.GoSexp, row.GoSexpTrunc = oracleDiffCap(sexp)
		row.GoNodes, row.GoErrorNodes, row.GoMissingNode = oracleDiffWalkGo(goRoot)
		inspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
		if err != nil {
			row.GoError = "digest: " + err.Error()
		} else {
			row.GoDigest = inspection.SHA256
		}
		if cRoot != nil {
			row.ComparedBoth = true
			if div := FirstDivergenceDumpV1(goRoot, goLang, cRoot); div != nil {
				row.FirstDivCat, row.FirstDivPath = div.Category, div.Path
				row.FirstDivGo, row.FirstDivC = div.GoValue, div.CValue
			}
		}
	}
	row.GoEqualsC = row.CError == "" && row.GoError == "" && row.CDigest != "" && row.CDigest == row.GoDigest
	return row
}

func oracleDiffDoneKeys(path string, inputs map[string]string) (map[string]bool, error) {
	done := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil {
		return nil, err
	} else if info.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], info.Size()-1); err != nil || last[0] != '\n' {
			return nil, fmt.Errorf("incomplete final row")
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var row oracleDiffRow
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("invalid existing row: %w", err)
		}
		key := oracleDiffKey(row.Grammar, row.Class, row.ID)
		if done[key] || inputs[key] == "" || row.InputSHA256 != inputs[key] {
			return nil, fmt.Errorf("duplicate or changed input: %s", key)
		}
		done[key] = true
	}
	return done, sc.Err()
}

func oracleDiffHangSides(raw []byte) (map[string]map[string]bool, error) {
	hangs := map[string]map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 || (parts[3] != "c" && parts[3] != "go") {
			return nil, fmt.Errorf("invalid hang entry: %q", line)
		}
		key := oracleDiffKey(parts[0], parts[1], parts[2])
		if hangs[key] == nil {
			hangs[key] = map[string]bool{}
		}
		hangs[key][parts[3]] = true
	}
	return hangs, nil
}

func TestCOracleDiff(t *testing.T) {
	out := strings.TrimSpace(os.Getenv("GTS_ORACLE_DIFF_OUT"))
	if out == "" {
		t.Skip("GTS_ORACLE_DIFF_OUT unset")
	}
	timeout := 20 * time.Second
	if raw := strings.TrimSpace(os.Getenv("GTS_ORACLE_DIFF_TIMEOUT")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			t.Fatalf("GTS_ORACLE_DIFF_TIMEOUT=%q: %v", raw, err)
		}
		timeout = d
	}
	var langs []string
	if raw := strings.TrimSpace(os.Getenv("GTS_ORACLE_DIFF_LANGS")); raw != "" {
		for _, l := range strings.Split(raw, ",") {
			if l = strings.TrimSpace(l); l != "" {
				langs = append(langs, l)
			}
		}
	} else {
		for _, entry := range grammars.AllLanguages() {
			langs = append(langs, entry.Name)
		}
	}
	sort.Strings(langs)
	classes := map[string]bool{}
	for _, c := range strings.Split(os.Getenv("GTS_ORACLE_DIFF_CLASSES"), ",") {
		if c = strings.TrimSpace(c); c != "" {
			classes[c] = true
		}
	}

	hardLimit := 4 * timeout
	if raw := strings.TrimSpace(os.Getenv("GTS_ORACLE_DIFF_HANG")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			t.Fatalf("GTS_ORACLE_DIFF_HANG=%q: %v", raw, err)
		}
		hardLimit = d
	}
	runID := strings.TrimSpace(os.Getenv("GTS_ORACLE_DIFF_RUN_ID"))
	if runID == "" {
		t.Fatal("GTS_ORACLE_DIFF_RUN_ID must identify the exact Go source/build revision")
	}
	inputs := oracleDiffBuildInputs(t, langs, classes)
	meta := oracleDiffMeta{RunID: runID, Inputs: map[string]string{}, OracleRuntimeVersion: COracleRuntimeVersion, OracleRuntimeCommit: COracleRuntimeCommit, BindingCommit: COracleBindingCommit, TimeoutMillis: timeout.Milliseconds(), HardLimitMillis: hardLimit.Milliseconds(), Grammars: map[string]interface{}{}, LoadErrors: map[string]string{}}
	for _, in := range inputs {
		key := oracleDiffKey(in.grammar, in.class, in.id)
		if _, exists := meta.Inputs[key]; exists {
			t.Fatalf("duplicate input %s", key)
		}
		meta.Inputs[key] = oracleDiffSHA(in.source)
	}
	for _, lang := range langs {
		if id, err := COracleIdentity(lang); err == nil {
			meta.Grammars[lang] = id
		} else {
			meta.LoadErrors[lang] = err.Error()
		}
	}
	rawMeta, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	metaPath := strings.TrimSuffix(out, filepath.Ext(out)) + ".meta.json"
	hangPath := out + ".hangs"
	resume := parityEnvBool("GTS_ORACLE_DIFF_RESUME", false)
	done := map[string]bool{}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if resume {
		prev, err := os.ReadFile(metaPath)
		if err != nil || !bytes.Equal(bytes.TrimSpace(prev), rawMeta) {
			t.Fatalf("resume identity missing or changed: %v", err)
		}
		done, err = oracleDiffDoneKeys(out, meta.Inputs)
		if err != nil {
			t.Fatal(err)
		}
		flags = os.O_WRONLY | os.O_APPEND
	} else {
		if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Remove(hangPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	f, err := os.OpenFile(out, flags, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !resume {
		if err := os.WriteFile(metaPath, append(rawMeta, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w := bufio.NewWriterSize(f, 1<<20)
	defer w.Flush()
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	hangs := map[string]map[string]bool{}
	if resume {
		if raw, err := os.ReadFile(hangPath); err == nil {
			hangs, err = oracleDiffHangSides(raw)
			if err != nil {
				t.Fatal(err)
			}
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			cur := oracleDiffCurrent.Load()
			if cur == nil || time.Since(cur.start) < hardLimit {
				continue
			}
			mu.Lock()
			_ = w.Flush()
			if hf, err := os.OpenFile(hangPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				fmt.Fprintf(hf, "%s\t%s\t%s\t%s\n", cur.in.grammar, cur.in.class, cur.in.id, cur.side)
				hf.Close()
			}
			fmt.Fprintf(os.Stderr, "oracle diff watchdog: %s/%s/%s %s parse ran past %s; exiting %d\n",
				cur.in.grammar, cur.in.class, cur.in.id, cur.side, hardLimit, oracleDiffHangExit)
			os.Exit(oracleDiffHangExit)
		}
	}()

	byGrammar := map[string][]oracleDiffInput{}
	for _, in := range inputs {
		byGrammar[in.grammar] = append(byGrammar[in.grammar], in)
	}
	total, equal := 0, 0
	for _, lang := range langs {
		var todo []oracleDiffInput
		for _, in := range byGrammar[lang] {
			if !done[oracleDiffKey(in.grammar, in.class, in.id)] {
				todo = append(todo, in)
			}
		}
		if len(todo) == 0 {
			continue
		}
		cLang, cErr := ParityCLanguage(lang)
		start := time.Now()
		for _, in := range todo {
			sides := hangs[oracleDiffKey(in.grammar, in.class, in.id)]
			row := oracleDiffRun(in, cLang, cErr, timeout, sides["c"], sides["go"])
			mu.Lock()
			err := enc.Encode(row)
			mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			total++
			if row.GoEqualsC {
				equal++
			}
		}
		mu.Lock()
		err := w.Flush()
		mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		debug.FreeOSMemory()
		t.Logf("%s: %d rows in %s", lang, len(todo), time.Since(start).Round(time.Millisecond))
	}
	t.Logf("oracle %s: %d rows, %d Go==C", COracleRuntimeVersion, total, equal)

}

func TestOracleDiffResumeValidation(t *testing.T) {
	key := oracleDiffKey("go", "smoke", "x")
	inputs := map[string]string{key: "abc"}
	path := filepath.Join(t.TempDir(), "rows.jsonl")
	row := oracleDiffRow{Grammar: "go", Class: "smoke", ID: "x", InputSHA256: "abc"}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	done, err := oracleDiffDoneKeys(path, inputs)
	if err != nil || !done[key] {
		t.Fatalf("valid row: %v %v", done, err)
	}
	if err := os.WriteFile(path, append(append(append([]byte{}, raw...), '\n'), append(raw, '\n')...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := oracleDiffDoneKeys(path, inputs); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := oracleDiffDoneKeys(path, map[string]string{key: "changed"}); err == nil {
		t.Fatal("changed source accepted")
	}
	if err := os.WriteFile(path, []byte("{\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := oracleDiffDoneKeys(path, inputs); err == nil {
		t.Fatal("partial row accepted")
	}
}

func TestOracleDiffHangSides(t *testing.T) {
	hangs, err := oracleDiffHangSides([]byte("go\tsmoke\tx\tc\ngo\tsmoke\tx\tgo\n"))
	if err != nil {
		t.Fatal(err)
	}
	sides := hangs[oracleDiffKey("go", "smoke", "x")]
	if !sides["c"] || !sides["go"] {
		t.Fatalf("lost a side: %v", sides)
	}
	if _, err := oracleDiffHangSides([]byte("go\tsmoke\tx\tunknown\n")); err == nil {
		t.Fatal("invalid side accepted")
	}
}
