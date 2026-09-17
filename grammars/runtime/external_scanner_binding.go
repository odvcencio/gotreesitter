package grammarruntime

import (
	"fmt"
	"os"
	"sync"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func bindExternalScannerSpec(lang *gotreesitter.Language, spec ExternalScannerSpec, setSymbol func(int, gotreesitter.Symbol)) []int {
	return bindExternalScannerSymbolNames(lang, spec.Externals, setSymbol)
}

// bindExternalScannerSymbolNames binds a hand-written scanner's token slots to a
// Language's external symbols POSITIONALLY: external index i is scanner spec
// token i, for every language provenance.
//
// Why positional and not by name: a hand-written scanner's spec.Externals array
// is ordered by scanner token index, and every Language's ExternalSymbols slice
// is emitted in the same grammar-external order (both the ts2go blobs and the
// grammargen assembler preserve the upstream `externals: [...]` order). Empirically
// spec order == grammar-external order at every index for all five name-binding
// languages (kotlin/python/swift/dart/rust), so positional binding reproduces the
// previous by-name bindings exactly while ALSO rescuing literal-valued externals
// whose Language *display* name differs from the spec *rule* name.
//
// The old by-name algorithm compared spec rule names (e.g. "safe_nav") against
// Language symbol names, but SymbolNames stores DISPLAY text for literal-valued
// tokens (e.g. "\?."), so those externals never matched and were left unbound
// (externalToToken[i] == -1). An unbound external means the scanner is never told
// the token is valid, so the corresponding source mislexes: kotlin `?.`, and
// swift's 18 custom operators (`->`, `.`, `&&`, `||`, `??`, `=`, `==`, `+`, `-`,
// `as`, `as?`, `as!`, `async`, `#`, `#if`, `#elseif`, `#else`, `#endif`). swift's
// were previously rescued only by an incidental GeneratedByGrammargen provenance
// flag; positional binding makes the rescue robust for every provenance.
//
// Name comparison is retained as VERIFICATION ONLY: when the Language symbol name
// at index i disagrees with the spec token name at index i, the binding is still
// made positionally and a drift event is recorded (see noteExternalBindingDrift)
// so a genuine spec/grammar ordering skew is observable instead of silent.
//
// The returned slice maps external index -> scanner token index (or -1 when an
// external has no corresponding scanner token). setSymbol(tokenIdx, sym) records
// the concrete Language Symbol for each bound scanner token.
func bindExternalScannerSymbolNames(lang *gotreesitter.Language, names []string, setSymbol func(int, gotreesitter.Symbol)) []int {
	if lang == nil {
		return nil
	}

	externalToToken := make([]int, len(lang.ExternalSymbols))
	for i := range externalToToken {
		externalToToken[i] = -1
	}

	// Bind min(externals, specTokens): external index i binds to scanner token i.
	bindCount := len(externalToToken)
	if len(names) < bindCount {
		bindCount = len(names)
	}
	for i := 0; i < bindCount; i++ {
		sym := lang.ExternalSymbols[i]
		setSymbol(i, sym)
		externalToToken[i] = i

		// Verification only: flag (but do not act on) a name disagreement between
		// the Language symbol name and the scanner spec token name at this index.
		if want := names[i]; want != "" {
			if got := externalScannerSymbolName(lang, sym); got != "" && got != want {
				noteExternalBindingDrift(i, got, want)
			}
		}
	}

	// Length mismatch preserves the historical structure: externalToToken has one
	// slot per Language external. When the spec declares more tokens than the
	// Language exposes externals, the surplus scanner tokens keep their default
	// symbols (setSymbol is never called for them). When the Language exposes more
	// externals than the spec declares, the surplus externals stay -1 (no token).
	if len(names) != len(externalToToken) {
		noteExternalBindingLengthMismatch(len(names), len(externalToToken), bindCount)
	}

	return externalToToken
}

func externalScannerSymbolName(lang *gotreesitter.Language, sym gotreesitter.Symbol) string {
	if lang == nil || int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
		return ""
	}
	return lang.SymbolNames[sym]
}

// ---- positional-binding drift diagnostics (verification only) ----

// externalBindingDrift records one external index whose Language symbol name
// disagreed with the scanner spec token name at the same index. Positional
// binding is authoritative; drift entries are diagnostic signal only. Benign
// display-vs-rule-name disagreements (e.g. kotlin safe_nav display "\?." vs spec
// "safe_nav", swift's 18 custom operators) drift, and so would a genuine
// spec/grammar ordering skew that warrants investigation.
type externalBindingDrift struct {
	Index int
	Got   string // Language symbol (display/rule) name at this external index
	Want  string // scanner spec token name at this index
}

var (
	externalBindingDriftMu      sync.Mutex
	externalBindingDriftCount   int
	externalBindingDriftCapture bool
	externalBindingDriftLog     []externalBindingDrift

	externalBindingDebugOnce sync.Once
	externalBindingDebug     bool
)

// externalBindingDebugEnabled reports whether GOT_DEBUG_EXTERNAL_BINDING=1 is set,
// matching the repo's env-gated stderr debug idiom (cf. the GOT_DEBUG_* /
// GOT_EAGER_DEFAULT_REDUCE_DEBUG toggles elsewhere in the tree).
func externalBindingDebugEnabled() bool {
	externalBindingDebugOnce.Do(func() {
		externalBindingDebug = os.Getenv("GOT_DEBUG_EXTERNAL_BINDING") == "1"
	})
	return externalBindingDebug
}

func noteExternalBindingDrift(index int, got, want string) {
	externalBindingDriftMu.Lock()
	externalBindingDriftCount++
	if externalBindingDriftCapture {
		externalBindingDriftLog = append(externalBindingDriftLog, externalBindingDrift{Index: index, Got: got, Want: want})
	}
	externalBindingDriftMu.Unlock()

	if externalBindingDebugEnabled() {
		fmt.Fprintf(os.Stderr, "GOT-EXTERNAL-BINDING drift index=%d got=%q want=%q (positional binding kept)\n", index, got, want)
	}
}

func noteExternalBindingLengthMismatch(specTokens, externals, bound int) {
	if externalBindingDebugEnabled() {
		fmt.Fprintf(os.Stderr, "GOT-EXTERNAL-BINDING length-mismatch specTokens=%d externals=%d bound=%d\n", specTokens, externals, bound)
	}
}

// externalBindingDriftBeginCapture starts collecting drift entries into a log for
// inspection and clears any previously captured entries. Test-facing.
func externalBindingDriftBeginCapture() {
	externalBindingDriftMu.Lock()
	externalBindingDriftCapture = true
	externalBindingDriftLog = nil
	externalBindingDriftMu.Unlock()
}

// externalBindingDriftEndCapture stops collecting drift entries and returns those
// captured since the matching begin. Test-facing.
func externalBindingDriftEndCapture() []externalBindingDrift {
	externalBindingDriftMu.Lock()
	out := externalBindingDriftLog
	externalBindingDriftLog = nil
	externalBindingDriftCapture = false
	externalBindingDriftMu.Unlock()
	return out
}

// externalBindingDriftTotal returns the process-wide count of positional bindings
// that disagreed with their spec token name. Test-facing.
func externalBindingDriftTotal() int {
	externalBindingDriftMu.Lock()
	defer externalBindingDriftMu.Unlock()
	return externalBindingDriftCount
}
