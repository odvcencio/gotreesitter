package lexbuild

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// genEnv names the generator binary (tree-sitter 0.27.1) for the differential test.
const genEnv = "LEXBUILD_GEN"

var (
	diffN      = flag.Int("diffn", 300, "number of generated grammars for TestLexDifferential")
	diffSeed   = flag.Int64("diffseed", 1, "first seed for TestLexDifferential")
	diffJobs   = flag.Int("diffjobs", 4, "parallel jobs for TestLexDifferential")
	diffReport = flag.Bool("diffreport", false, "TestLexDifferential: report only, assert nothing (default: any disagreement fails)")
	diffTrace  = flag.Bool("difftrace", false, "print each grammar before it runs (to find the one that kills the process)")
)

// pick returns one of the strings.
func pick(r *rand.Rand, xs ...string) string { return xs[r.Intn(len(xs))] }

var (
	patLiterals = []string{"a", "b", "c", "ab", "xyz", "é", "日", "-", "]", "}", "=", "#", "\\.", "\\-", "\\\"", "\\/", "\\ ", "\\n", "\\t", "\\r", "\\f", "\\v", "\\a", "\\\\", "\\*", "\\(", "\\[", "\\{", "\\|"}
	patEscapes  = []string{`\x41`, `\x{41}`, `\x{000041}`, `\x{0000000041}`, `A`, `\u{41}`, `\u{000000041}`, `\U00000041`, `\U{41}`, `\x{110000}`, `\x{D800}`, `\x4`, `\x{}`, `é`, `\u{1F600}`, `\q`, `\1`, `\8`, `\0`, `\e`, `\d`, `\D`, `\w`, `\W`, `\s`, `\S`, `\pL`, `\PL`, `\p`, `\p{`}
	patLooks    = []string{"^", "$", `\A`, `\z`, `\b`, `\B`, `\<`, `\>`, `\b{start}`, `\b{end}`, `\b{start-half}`, `\b{end-half}`, `\b{foo}`, `\b{`}
	patProps    = []string{`\p{L}`, `\pL`, `\P{Lu}`, `\p{Greek}`, `\p{^L}`, `\P{^L}`, `\p{gc=Lu}`, `\p{General_Category=Lu}`, `\p{gc!=Lu}`, `\p{Script=Greek}`, `\p{Foo}`, `\p{isc}`, `\p{IsC}`, `\p{gc=isc}`, `\p{Any}`, `\P{Any}`, `\p{ASCII}`, `\p{Assigned}`, `\p{Is Letter}`, `\p{XID_Start}`, `\p{White_Space}`, `\p{Age=1.1}`, `\p{sc:Latn}`, `\p{N}`, `\p{Zs}`, `\p{Nd}`, `\p{Decimal_Number}`, `\p{Emoji}`, `\p{Alphabetic}`}
	patClasses  = []string{
		`[abc]`, `[a-z]`, `[^a-z]`, `[]a]`, `[^]a]`, `[-a]`, `[a-]`, `[--a]`, `[a--]`, `[\]]`, `[a[b-d]]`, `[a-z&&[^aeiou]]`, `[a--b]`, `[a~~b]`, `[a&&]`, `[&&a]`, `[[:alpha:]]`, `[[:^digit:]x]`, `[[:foo:]]`,
		`[[:]]`, `[[:lower]]`, `[a-\p{L}]`, `[\p{L}-a]`, `[\p{L}-]`, `[\x41-\x5a]`, `[é-ÿ]`, `[z-a]`, `[\w]`, `[^\d]`, `[\s-]`, `[\b]`, `[\p{Lu}\p{Ll}]`, `[[a-c]--[b]]`, `[+--a]`, `[a-z&&b-d&&c]`, `[]`, `[`, `[^`, `[a`, `[\u{D7FF}-\u{E000}--\u{E000}]`, `[^[\u{D7FF}-\u{E000}--\u{E000}]\u{E000}]`, `[\x{0}-\x{10FFFF}]`, `[^\x{0}-\x{10FFFF}]`, `[\\]`, `[\-]`, `[\^]`, `[ab]-[cd]`,
	}
	patGroupOpen = []string{"(", "(?:", "(?<n>", "(?P<n>", "(?<1a>", "(?<_x.[]1>", "(?<é>", "(?<>", "(?i:", "(?i-s:", "(?s:", "(?R:", "(?U:", "(?x:", "(?-u:", "(?ii:", "(?i-:", "(?i-i:", "(?-:", "(?q:", "(?=", "(?!", "(?<=", "(?<!", "(?#", "(?"}
	patFlagsOnly = []string{"(?i)", "(?s)", "(?m)", "(?R)", "(?U)", "(?u)", "(?x)", "(?-i)", "(?i-s)", "(?ii)", "(?i-i)", "(?i-)", "(?)", "(?-)", "(?is)", "(?-u)"}
	patReps      = []string{"*", "+", "?", "*?", "+?", "??", "{2}", "{0}", "{1}", "{2,}", "{2,4}", "{ 2 }", "{1 , 2 }", "{,3}", "{3,2}", "{", "{x}", "{2", "{4294967296}", "{0,0}", "{1,1}", "{0,1}", "{0,}", "{1,}", "**", "*+", "{2}{3}"}
	patEmpties   = []string{"()", "(?:)", "(?i:)", "(?i)", "^", `\b`, "(?:^)", "(?:\\b)", "(?:|)"}
	patZeroReps  = []string{"{4294967295}", "{4294967295,}", "{0,4294967295}", "{1,4294967295}", "*", "+", "{0}", "{5}", "{2,3}", "?"}
	patPrefixes  = []string{`ab|ac`, `a+b|a+c`, `[ab]c|[ab]d`, `(a)b|(a)c`, `a|ab`, `(?:ab)|(?:ac)`, `ab|ab`, `a|b|c`, `[a-c]|[b-d]|x`, `a|[b]`, `x*y|x*z|x*`, `^a|^b`, `\bfoo|\bbar`}
)

// Valid subsets of the lists above: most generated patterns draw from them, so that most grammars
// reach the comparison of the lexers instead of being refused by both sides.
var (
	goodEscapes = []string{`\x41`, `\x{41}`, `\x{000041}`, `\x{0000000041}`, `\u0041`, `\u{41}`, `\u{000000041}`, `\U00000041`, `\U{41}`, `\u00e9`, `\u{1F600}`, `\d`, `\D`, `\w`, `\W`, `\s`, `\S`, `\pL`, `\PL`}
	goodProps   = []string{`\p{L}`, `\pL`, `\P{Lu}`, `\p{gc=Lu}`, `\p{General_Category=Lu}`, `\p{gc!=Lu}`, `\p{Any}`, `\P{Any}`, `\p{ASCII}`, `\p{Assigned}`, `\p{Is Letter}`, `\p{XID_Start}`, `\p{White_Space}`, `\p{N}`, `\p{Zs}`, `\p{Nd}`, `\p{Decimal_Number}`, `\p{Emoji}`, `\p{Alphabetic}`}
	goodClasses = []string{`[abc]`, `[a-z]`, `[^a-z]`, `[]a]`, `[^]a]`, `[-a]`, `[a-]`, `[--a]`, `[a--]`, `[\]]`, `[a[b-d]]`, `[a-z&&[^aeiou]]`, `[a--b]`, `[a~~b]`, `[a&&]`, `[&&a]`, `[[:alpha:]]`, `[[:^digit:]x]`, `[[:foo:]]`, `[[:]]`, `[[:lower]]`, `[\p{L}-]`, `[\x41-\x5a]`, `[\u00e9-\u00ff]`, `[\w]`, `[^\d]`, `[\s-]`, `[\p{Lu}\p{Ll}]`, `[[a-c]--[b]]`, `[+--a]`, `[a-z&&b-d&&c]`, `[\u{D7FF}-\u{E000}--\u{E000}]`, `[^[\u{D7FF}-\u{E000}--\u{E000}]\u{E000}]`, `[\x{0}-\x{10FFFF}]`, `[\\]`, `[\-]`, `[\^]`}
	goodGroups  = []string{"(", "(?:", "(?<n>", "(?P<n>", "(?<_x.[]1>", "(?<é>", "(?i:", "(?i-s:", "(?s:", "(?R:", "(?U:"}
	goodFlags   = []string{"(?i)", "(?s)", "(?m)", "(?R)", "(?U)", "(?u)", "(?-i)", "(?i-s)", "(?is)"}
	goodReps    = []string{"*", "+", "?", "*?", "+?", "??", "{2}", "{0}", "{1}", "{2,}", "{2,4}", "{ 2 }", "{1 , 2 }", "{0,0}", "{1,1}", "{0,1}", "{0,}", "{1,}"}
)

func pickGood(r *rand.Rand, good, all []string) string {
	if r.Intn(5) != 0 {
		return pick(r, good...)
	}
	return pick(r, all...)
}

// genPattern builds one pattern from the constructs the reader handles and the
// edge forms found by the reviews. It also mutates the text now and then, which
// produces syntax errors.
func genPattern(r *rand.Rand, depth int) string {
	var atom func(d int) string
	atom = func(d int) string {
		switch k := r.Intn(22); {
		case k < 4:
			return pick(r, patLiterals...)
		case k < 6:
			return pickGood(r, goodEscapes, patEscapes)
		case k < 7:
			if r.Intn(3) != 0 {
				return pick(r, patLiterals...)
			}
			return pick(r, patLooks...)
		case k < 9:
			return pickGood(r, goodProps, patProps)
		case k < 12:
			return pickGood(r, goodClasses, patClasses)
		case k < 13:
			return "."
		case k < 16 && d > 0:
			return pickGood(r, goodGroups, patGroupOpen) + genPattern(r, d-1) + ")"
		case k < 17:
			return pickGood(r, goodFlags, patFlagsOnly)
		case k < 19:
			return pick(r, patEmpties...) + pick(r, patZeroReps...)
		case k < 20:
			return pick(r, patPrefixes...)
		case k < 21:
			return pick(r, "(|a)", "(a|)") + pick(r, "*", "+", "?", "{2}", "{0}", "{1,3}")
		default:
			return pick(r, patLiterals...)
		}
	}
	n := 1 + r.Intn(3)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		a := atom(depth)
		if r.Intn(3) == 0 {
			a += pickGood(r, goodReps, patReps)
		}
		sb.WriteString(a)
		if r.Intn(8) == 0 && i+1 < n {
			sb.WriteString("|")
		}
	}
	s := sb.String()
	if r.Intn(25) == 0 { // mutate
		runes := []rune(s)
		if len(runes) > 0 {
			i := r.Intn(len(runes))
			meta := []rune(`()[]{}|*+?.^$\-&~:<>=!,#`)
			switch r.Intn(3) {
			case 0:
				runes = append(runes[:i], runes[i+1:]...)
			case 1:
				runes[i] = meta[r.Intn(len(meta))]
			default:
				runes = append(runes[:i], append([]rune{meta[r.Intn(len(meta))]}, runes[i:]...)...)
			}
			s = string(runes)
		}
	}
	return s
}

func jsonQuoteOf(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// genGrammar returns the text of one grammar.json and a label of its kind.
func genGrammar(r *rand.Rand) (string, string) {
	if r.Intn(4) != 0 {
		pat := genPattern(r, 2)
		flags := ""
		if r.Intn(8) == 0 {
			flags = `,"flags":"i"`
		}
		return `{"name":"t","rules":{"source":{"type":"SYMBOL","name":"tok"},"tok":{"type":"PATTERN","value":` + jsonQuoteOf(pat) + flags + `}}}`, "pattern"
	}
	return genJSONVariant(r), "json"
}

func deep(n int) string { return strings.Repeat("[", n) + "0" + strings.Repeat("]", n) }

// genJSONVariant builds a grammar.json with an odd JSON form around a valid grammar.
func genJSONVariant(r *rand.Rand) string {
	tok := `{"type":"STRING","value":"x"}`
	rule := func(extra string) string { return `{"type":"PATTERN","value":"a+"` + extra + `}` }
	switch r.Intn(26) {
	case 0:
		return `{"name":"t","rules":{"source":` + tok + `,"source":{"type":"STRING","value":"y"}}}`
	case 1:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x","value":"y"}}}`
	case 2:
		return `{"name":"t","rules":{"source":` + tok + `},"ignored":1,"ignored":2}`
	case 3:
		return `{"name":"t","rules":{"source":` + tok + `},"ignored":"\ud800"}`
	case 4:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x","z":"\ud800"}}}`
	case 5:
		return `{"name":"t","rules":{"source":` + rule(`,"z":1e400`) + `}}`
	case 6:
		return `{"name":"t","rules":{"source":` + rule(`,"z":1e400,"z":1`) + `}}`
	case 7:
		return `{"name":"t","ignored":1e400,"rules":{"source":` + tok + `}}`
	case 8:
		n := 120 + r.Intn(12)
		return `{"name":"t","rules":{"source":` + rule(`,"z":`+deep(n)) + `}}`
	case 9:
		n := 120 + r.Intn(12)
		return `{"name":"t","ignored":` + deep(n) + `,"rules":{"source":` + tok + `}}`
	case 10:
		return `{"name":"t","rules":{"source":{"type":"TOKEN","content":{"type":"PREC_DYNAMIC","value":` + pick(r, "-0", "0", "1.0", "1e2", "4294967296", "-2147483648", "2147483648", "null", `"1"`) + `,"content":` + tok + `}}}}`
	case 11:
		return `{"name":"t","rules":{"source":{"type":"TOKEN","content":{"type":"PREC","value":` + pick(r, "-0", "0", "1.0", "1e2", "4294967296", "null", `"p"`, `""`) + `,"content":` + tok + `}}}}`
	case 12:
		return `{"name":"t","rules":{"source":{"type":"TOKEN","content":{"type":"PREC_LEFT","value":` + pick(r, "-0", "-1", "3") + `,"content":` + tok + `}}}}`
	case 13:
		return `{"name":"t","rules":{"source":{"type":"WOBBLE"}}}`
	case 14:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":null}}}`
	case 15:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x"}},"extras":[{"type":"STRING","value":"y","value":"z"}]}`
	case 16:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x"}},"extras":[{"type":"STRING","value":"y","zz":1,"zz":2}]}`
	case 17:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x"}},"extras":[{"type":"STRING","value":"y","z":"\ud800"}]}`
	case 18:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x😀"}}}`
	case 19:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x\ud83d"}}}`
	case 20:
		return `{"name":"t","word":null,"extras":null,"rules":{"source":` + tok + `}}`
	case 21:
		return `{"name":"t","rules":{"source":` + tok + `,"other":{"type":"STRING","value":"q"}},"conflicts":[]}`
	case 22:
		return `{"rules":{"source":` + tok + `}}`
	case 23:
		return `{"name":"t","rules":{"source":{"type":"ALIAS","named":` + pick(r, "true", "1", `"x"`, "null") + `,"value":"v","content":` + tok + `}}}`
	case 24:
		return `{"name":"t","rules":{"source":{"type":"PATTERN","value":"a","flags":` + pick(r, `"i"`, `"u"`, `"iu"`, "null", "1") + `}}}`
	default:
		return `{"name":"t","rules":{"source":{"type":"STRING","value":"x"},"source":{"type":"STRING","value":"y"}},"reserved":{"g":[],"g":[]}}`
	}
}

type diffOutcome struct {
	bucket string
	text   string // grammar.json
	detail string
}

// runGenerator runs the generator binary on grammar text in dir. ok is false when it refused or failed.
func runGenerator(ctx context.Context, bin, dir, text string) (ok bool, msg string) {
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		return false, err.Error()
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "grammar.json"), []byte(text), 0o644); err != nil {
		return false, err.Error()
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "generate", "-o", "gen", "src/grammar.json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if cctx.Err() != nil {
		return false, "generator timeout"
	}
	if err != nil {
		return false, string(out)
	}
	if os.Rename(filepath.Join(dir, "gen", "parser.c"), filepath.Join(dir, "parser.c")) != nil {
		return false, "no parser.c"
	}
	os.MkdirAll(filepath.Join(dir, "tree_sitter"), 0o755)
	os.Rename(filepath.Join(dir, "gen", "tree_sitter", "parser.h"), filepath.Join(dir, "tree_sitter", "parser.h"))
	return true, ""
}

// diffOne compares the generator and the port on one grammar.
func diffOne(ctx context.Context, bin, root string, id int, text string) diffOutcome {
	name := fmt.Sprintf("g%d", id)
	dir := filepath.Join(root, name)
	defer os.RemoveAll(dir)
	genOK, genMsg := runGenerator(ctx, bin, dir, text)
	if genMsg == "generator timeout" {
		return diffOutcome{"generator timeout", text, genMsg}
	}
	_, perr := Prepare([]byte(text))
	var np *NotPortedError
	var se *SizeError
	switch {
	case errors.As(perr, &np):
		return diffOutcome{"not ported", text, np.Error()}
	case errors.As(perr, &se) && genOK:
		return diffOutcome{"port refuses: size limit", text, se.Error()}
	case perr != nil && !genOK:
		return diffOutcome{"both refuse", text, ""}
	case perr != nil && genOK:
		return diffOutcome{"port refuses only", text, perr.Error()}
	case perr == nil && !genOK:
		return diffOutcome{"generator refuses only", text, oneLine(genMsg, 200)}
	}
	line, cat := censusLineCat(root, name)
	return classifyCensus(line, cat, text)
}

// classifyCensus maps a census result to a differential bucket. The category
// comes from the result of the port, not from its text.
func classifyCensus(line, cat, text string) diffOutcome {
	switch {
	case cat == catSizeLimit:
		// A deliberate difference: the generator has no size limit (see doc.go).
		return diffOutcome{"port refuses: size limit", text, line}
	case cat == catReferenceUnread:
		return diffOutcome{"reference unreadable", text, line}
	case strings.HasSuffix(line, "result equal"):
		return diffOutcome{"equal", text, ""}
	}
	return diffOutcome{"differ", text, line}
}

// TestLexDifferential generates grammars from a seed, runs the generator binary and the port on each, and
// compares: accepted or refused by each, and, when both accept, the complete lexer transitions through the
// reference reader as the census does. It prints the buckets and the first disagreements. It fails if any
// grammar ends as "differ", "port refuses only", "generator refuses only" or "reference unreadable", and if
// no grammar was compared. With -diffreport it only reports ("report only, nothing asserted"). Grammars that
// the port refuses as not ported yet or for its size limit are counted and do not fail the test.
func TestLexDifferential(t *testing.T) {
	bin := os.Getenv(genEnv)
	if bin == "" {
		t.Skipf("%s is not set: no generator binary to compare with", genEnv)
	}
	root := t.TempDir()
	type job struct {
		id   int
		text string
		kind string
	}
	jobs := make(chan job)
	var mu sync.Mutex
	buckets := map[string]int{}
	var disagree []diffOutcome
	var wg sync.WaitGroup
	for w := 0; w < *diffJobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if *diffTrace {
					fmt.Fprintf(os.Stderr, "DIFFSTART %d %s\n", j.id, j.text)
				}
				o := diffOne(context.Background(), bin, root, j.id, j.text)
				mu.Lock()
				buckets[o.bucket]++
				buckets["kind:"+j.kind]++
				switch o.bucket {
				case "differ", "port refuses only", "generator refuses only", "reference unreadable":
					disagree = append(disagree, o)
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < *diffN; i++ {
		r := rand.New(rand.NewSource(*diffSeed + int64(i)))
		text, kind := genGrammar(r)
		jobs <- job{int(*diffSeed) + i, text, kind}
	}
	close(jobs)
	wg.Wait()
	var keys []string
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("DIFFERENTIAL seed %d n %d:", *diffSeed, *diffN)
	for _, k := range keys {
		fmt.Printf(" %s=%d", strings.ReplaceAll(k, " ", "_"), buckets[k])
	}
	fmt.Println()
	sort.Slice(disagree, func(i, j int) bool { return disagree[i].text < disagree[j].text })
	for i, o := range disagree {
		if i >= 400 {
			break
		}
		fmt.Printf("DISAGREE %s | %s | %s\n", strings.ReplaceAll(o.bucket, " ", "_"), o.text, oneLine(o.detail, 160))
	}
	if *diffReport {
		fmt.Println("DIFFERENTIAL report only, nothing asserted")
		return
	}
	for i, o := range disagree {
		if i >= 20 {
			t.Errorf("... and %d more disagreements", len(disagree)-20)
			break
		}
		t.Errorf("%s: %s | %s", o.bucket, o.text, oneLine(o.detail, 160))
	}
	if buckets["equal"] == 0 {
		t.Errorf("no grammar was compared (-diffn %d): the test asserts nothing; use -diffreport to only report", *diffN)
	}
}
