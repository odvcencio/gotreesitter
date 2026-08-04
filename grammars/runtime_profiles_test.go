package grammars

import (
	"crypto/sha256"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestBuiltinExternalScannerRetryProfilesAttach(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	tests := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "c_sharp", load: CSharpLanguage},
		{name: "crystal", load: CrystalLanguage},
		{name: "dart", load: DartLanguage},
		{name: "kotlin", load: KotlinLanguage},
		{name: "matlab", load: MatlabLanguage},
		{name: "python", load: PythonLanguage},
		{name: "swift", load: SwiftLanguage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := tt.load()
			if lang.ExternalScanner == nil {
				t.Fatal("ExternalScanner = nil, want attached scanner")
			}
			if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetrySkipRepeat {
				t.Fatalf("ExternalScannerFullParseRetryPolicy = %d, want certified skip-repeat policy", got)
			}
		})
	}
}

func TestCrystalAndMatlabKeepAcceptedErrorRetryLadder(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	for _, tt := range []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "crystal", load: CrystalLanguage},
		{name: "matlab", load: MatlabLanguage},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lang := tt.load()
			if got := lang.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want complete ladder", got)
			}
		})
	}
}

func TestBuiltinRuntimeProfilesStayNarrow(t *testing.T) {
	// 42 = the prior 41 plus the F# unary-wrapper materialization profile.
	// Bash, Haskell, and JavaScript reuse their existing exact profiles.
	// Exact blob identity bounds each profile capability.
	// D and Groovy's retry ceilings
	// moved out of parser-core name switches and onto exact-blob profiles. The
	// prior gomod and C additions moved hardcoded compat-tier behavior to profiles.
	// Meson and Enforce add two exact-blob retry policies. JavaScript adds one
	// exact-blob automatic-forest memory allowance. The
	// repetition-conflict helpers were retired in favor of certified
	// ConflictPolicies rows here. AWK and Uxntal add exact-blob automatic forest
	// routing; KDL reuses its existing retry profile. dot's helper was
	// retired outright, not migrated (see the "NOTE on dot" comment above
	// the gomod entry), so it does not add a map entry. Crystal and Matlab add
	// exact-blob external-scanner repeat suppression while retaining the full
	// 44 = the prior 42 plus two new A3 certification-workstream entries:
	// Perl and Ada (spec.campaign.v7, finding
	// tied-election-family-compact-retirement). Apex and Python add compact
	// primary-acceptance-derivation certification (Python also adds
	// converged-split-drop certification); Perl and Ada are new entries
	// carrying both certifications. Kotlin's pre-existing entry stays
	// unchanged in count (it does not add a map entry) but later adds both
	// grants in place once selectCompactAcceptanceDerivation's materiality
	// gate resolves the interaction that withheld them; see the "kotlin" map
	// entry comment.
	// 45 = the prior 44 plus the B3 stage S3 html entry, certifying native
	// strategy-2 error-region recovery for the html_erroneous_end_tag class
	// (spec.compact-recovery-ownership.v1).
	// 46 = the prior 45 plus the powershell entry, declaring backtick as the
	// language's line-continuation escape byte (Language.LineContinuationEscapeByte)
	// so bytesAreParserPadding classifies backtick+newline as padding, matching
	// the C oracle (spore.2026-08-02.birch-g.powershell-bisect).
	if got, want := len(builtinLanguageRuntimeProfiles), 46; got != want {
		t.Fatalf("builtinLanguageRuntimeProfiles has %d entries, want %d", got, want)
	}
	lang := &gotreesitter.Language{ExternalScanner: KotlinExternalScanner{}}
	if attachBuiltinLanguageRuntimeProfile("ruby", [32]byte{}, lang) {
		t.Fatal("unknown runtime profile reported an attachment")
	}
	if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetryDefault {
		t.Fatalf("unknown runtime profile changed policy to %d", got)
	}
	if got := lang.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
		t.Fatalf("unknown runtime profile changed accepted-error retry profile to %+v", got)
	}
	if got := lang.AutomaticForestMemoryAllowanceBytes; got != 0 {
		t.Fatalf("unknown runtime profile changed automatic forest allowance to %d", got)
	}
	if lang.FullParseArenaDensityCapEnabled {
		t.Fatal("unknown runtime profile enabled the full-parse arena density cap")
	}
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("unknown runtime profile enabled compact converged-split drops")
	}
	if lang.CompactEOFAcceptNoActionSiblingsCertified {
		t.Fatal("unknown runtime profile enabled compact EOF sibling drops")
	}
	if lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("unknown runtime profile enabled compact primary derivation selection")
	}
	if lang.ExactStackNodeEquivalenceCertified {
		t.Fatal("unknown runtime profile enabled exact stack-node equivalence")
	}
	if lang.LineContinuationEscapeByte != 0 {
		t.Fatalf("unknown runtime profile set a line-continuation escape byte: %q", lang.LineContinuationEscapeByte)
	}
}

func TestBuiltinObjcExactStackEquivalenceProfileRequiresExactBlobIdentity(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	builtin := ObjcLanguage()
	if !builtin.ExactStackNodeEquivalenceCertified {
		t.Fatal("exact built-in Objective-C artifact did not receive exact stack-node equivalence")
	}

	custom := &gotreesitter.Language{Name: "objc"}
	AttachLanguageSupport("objc", custom)
	if custom.ExactStackNodeEquivalenceCertified {
		t.Fatal("same-name custom Objective-C grammar enabled exact stack-node equivalence")
	}

	wrongIdentity := &gotreesitter.Language{Name: "objc"}
	if attachBuiltinLanguageRuntimeProfile("objc", sha256.Sum256([]byte("uncertified")), wrongIdentity) {
		t.Fatal("wrong Objective-C blob identity unexpectedly attached a runtime profile")
	}
	if wrongIdentity.ExactStackNodeEquivalenceCertified {
		t.Fatal("wrong Objective-C blob identity enabled exact stack-node equivalence")
	}
}

func TestBuiltinCompactAcceptanceProfilesRequireExactBlobIdentity(t *testing.T) {
	tests := []struct {
		name string
		load func() *gotreesitter.Language
		want func(*gotreesitter.Language) bool
	}{
		{
			name: "http", load: HttpLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactEOFAcceptNoActionSiblingsCertified
			},
		},
		{
			name: "robot", load: RobotLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactEOFAcceptNoActionSiblingsCertified
			},
		},
		{
			name: "meson", load: MesonLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified
			},
		},
		// The tied-election family (A3 certification workstream,
		// spec.campaign.v7, finding
		// tied-election-family-compact-retirement): full-corpus field-aware
		// C-oracle verification certifies primary-acceptance-derivation
		// selection for all five languages. Kotlin's grant lands under
		// selectCompactAcceptanceDerivation's materiality gate
		// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous);
		// see the runtime_profiles.go "kotlin" entry comment.
		{
			name: "python", load: PythonLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified
			},
		},
		{
			name: "apex", load: ApexLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified
			},
		},
		{
			name: "perl", load: PerlLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified
			},
		},
		{
			name: "ada", load: AdaLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified
			},
		},
		{
			name: "kotlin", load: KotlinLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified
			},
		},
	}

	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if lang := tt.load(); !tt.want(lang) {
				t.Fatal("exact built-in artifact did not receive compact acceptance certification")
			}

			custom := &gotreesitter.Language{Name: tt.name}
			AttachLanguageSupport(tt.name, custom)
			if tt.want(custom) {
				t.Fatal("same-name custom grammar enabled compact acceptance certification")
			}

			wrongIdentity := &gotreesitter.Language{Name: tt.name}
			if attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256([]byte("uncertified")), wrongIdentity) {
				t.Fatal("wrong blob identity unexpectedly attached a runtime profile")
			}
			if tt.want(wrongIdentity) {
				t.Fatal("wrong blob identity enabled compact acceptance certification")
			}
		})
	}
}

func TestBuiltinCompactConvergedSplitProfilesRequireExactBlobIdentity(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	tests := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "bash", load: BashLanguage},
		{name: "erlang", load: ErlangLanguage},
		{name: "go", load: GoLanguage},
		{name: "haskell", load: HaskellLanguage},
		{name: "javascript", load: JavascriptLanguage},
		{name: "python", load: PythonLanguage},
		// A3 certification workstream (spec.campaign.v7, finding
		// tied-election-family-compact-retirement): Perl and Ada certify
		// converged-path split-drop acceptance after full-corpus field-aware
		// C-oracle verification. Apex does not need this certification and
		// stays out of this table. Kotlin's grant stays withheld: review
		// found a compact-only divergence class (an annotated extension
		// property with a getter and a trailing comment); see the
		// runtime_profiles.go "kotlin" entry comment.
		{name: "perl", load: PerlLanguage},
		{name: "ada", load: AdaLanguage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builtin := test.load()
			if !builtin.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("exact built-in artifact did not receive compact converged-split certification")
			}

			custom := &gotreesitter.Language{Name: test.name}
			AttachLanguageSupport(test.name, custom)
			if custom.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("same-name custom grammar enabled compact converged-split drops")
			}

			blob := BlobByName(test.name)
			if len(blob) == 0 {
				t.Fatalf("BlobByName(%s) returned no data", test.name)
			}
			stale := &gotreesitter.Language{Name: test.name}
			if attachBuiltinLanguageRuntimeProfile(test.name, sha256.Sum256([]byte("stale")), stale) {
				t.Fatal("stale blob unexpectedly attached a runtime profile")
			}
			if stale.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("stale blob enabled compact converged-split drops")
			}

			exact := &gotreesitter.Language{Name: test.name}
			if !attachBuiltinLanguageRuntimeProfile(test.name, sha256.Sum256(blob), exact) {
				t.Fatal("exact blob did not attach its runtime profile")
			}
			if !exact.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("exact blob did not enable compact converged-split drops")
			}
		})
	}
}

func TestBuiltinOdinArenaDensityProfileRequiresExactBlobIdentity(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	builtin := OdinLanguage()
	if !builtin.FullParseArenaDensityCapEnabled {
		t.Fatal("exact built-in Odin artifact did not receive the arena density certification")
	}

	custom := &gotreesitter.Language{Name: "odin"}
	AttachLanguageSupport("odin", custom)
	if custom.FullParseArenaDensityCapEnabled {
		t.Fatal("same-name custom Odin grammar enabled the arena density cap")
	}

	blob := BlobByName("odin")
	if len(blob) == 0 {
		t.Fatal("BlobByName(odin) returned no data")
	}
	staleBlob := append([]byte(nil), blob...)
	staleBlob[len(staleBlob)-1] ^= 1
	stale := &gotreesitter.Language{Name: "odin"}
	if attachBuiltinLanguageRuntimeProfile("odin", sha256.Sum256(staleBlob), stale) {
		t.Fatal("stale Odin blob unexpectedly attached a runtime profile")
	}
	if stale.FullParseArenaDensityCapEnabled {
		t.Fatal("stale Odin blob enabled the arena density cap")
	}

	exact := &gotreesitter.Language{Name: "odin"}
	if !attachBuiltinLanguageRuntimeProfile("odin", sha256.Sum256(blob), exact) {
		t.Fatal("exact Odin blob did not attach its runtime profile")
	}
	if !exact.FullParseArenaDensityCapEnabled {
		t.Fatal("exact Odin blob did not enable the arena density cap")
	}
}

func TestBuiltinAutomaticForestProfilesRequireExactBlobIdentity(t *testing.T) {
	tests := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "awk", load: AwkLanguage},
		{name: "kdl", load: KdlLanguage},
		{name: "uxntal", load: UxntalLanguage},
	}

	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builtin := tt.load()
			if !builtin.AutomaticForestEnabledByDefault {
				t.Fatal("exact built-in artifact did not receive automatic forest certification")
			}

			wrongIdentity := &gotreesitter.Language{Name: tt.name}
			if attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256([]byte("uncertified")), wrongIdentity) {
				t.Fatal("wrong blob identity unexpectedly attached a runtime profile")
			}
			if wrongIdentity.AutomaticForestEnabledByDefault {
				t.Fatal("wrong blob identity enabled automatic forest routing")
			}

			adapted := &gotreesitter.Language{Name: tt.name}
			AttachLanguageSupport(tt.name, adapted)
			if adapted.AutomaticForestEnabledByDefault {
				t.Fatal("same-name adapted language enabled automatic forest routing")
			}
		})
	}
}

func TestEnforceDuplicateWideProfileRequiresExactBlobIdentity(t *testing.T) {
	want := gotreesitter.FullParseAcceptedErrorRetryProfile{
		ReuseCleanWideForWideRetry:   true,
		ReuseCleanWideMinSourceBytes: 128 * 1024,
	}

	wrongSHA := &gotreesitter.Language{Name: "enforce"}
	if attachBuiltinLanguageRuntimeProfile("enforce", sha256.Sum256([]byte("uncertified")), wrongSHA) {
		t.Fatal("wrong Enforce blob SHA reported a runtime-profile attachment")
	}
	if got := wrongSHA.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
		t.Fatalf("wrong Enforce blob SHA attached retry profile %+v", got)
	}

	adapted := &gotreesitter.Language{Name: "enforce"}
	AttachLanguageSupport("enforce", adapted)
	if got := adapted.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
		t.Fatalf("adapted Enforce language attached retry profile %+v", got)
	}

	blob := BlobByName("enforce")
	if len(blob) == 0 {
		t.Fatal("BlobByName(enforce) returned no data")
	}
	exact := &gotreesitter.Language{Name: "enforce"}
	if !attachBuiltinLanguageRuntimeProfile("enforce", sha256.Sum256(blob), exact) {
		t.Fatal("exact Enforce blob did not attach duplicate-wide profile")
	}
	if got := exact.FullParseAcceptedErrorRetryProfile; got != want {
		t.Fatalf("exact Enforce retry profile = %+v, want %+v", got, want)
	}
}

func TestBuiltinHaskellConflictPolicyAttaches(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	lang := HaskellLanguage()
	want := map[gotreesitter.StateID]struct {
		lookahead gotreesitter.Symbol
		reduce    gotreesitter.Symbol
	}{
		9609:  {lookahead: gotreesitter.ConflictPolicyAnyLookahead, reduce: 518},
		10984: {lookahead: gotreesitter.ConflictPolicyAnyLookahead, reduce: 516},
		11192: {lookahead: 4, reduce: 500},
	}
	seen := make(map[gotreesitter.StateID]bool, len(want))
	for _, policy := range lang.ConflictPolicies {
		expected, ok := want[policy.State]
		if !ok {
			continue
		}
		seen[policy.State] = true
		if policy.Lookahead != expected.lookahead {
			t.Fatalf("Haskell conflict policy at state %d has lookahead %d, want %d", policy.State, policy.Lookahead, expected.lookahead)
		}
		if policy.Kind != gotreesitter.ConflictPolicyRepetitionReduce ||
			len(policy.ReduceSymbols) != 1 || policy.ReduceSymbols[0] != expected.reduce {
			t.Fatalf("Haskell conflict policy at state %d = %+v, want repetition reduce over symbol %d", policy.State, policy, expected.reduce)
		}
	}
	for state := range want {
		if !seen[state] {
			t.Fatalf("Haskell conflict policy for state %d was not attached", state)
		}
	}
}

func TestBuiltinHaskellConflictPolicyRequiresCertifiedBlobAndAttachesOnce(t *testing.T) {
	lang := &gotreesitter.Language{Name: "haskell"}
	if attachBuiltinLanguageRuntimeProfile("haskell", sha256.Sum256([]byte("uncertified")), lang) {
		t.Fatal("uncertified Haskell blob reported a runtime-profile attachment")
	}
	if len(lang.ConflictPolicies) != 0 {
		t.Fatalf("uncertified Haskell blob attached %d conflict policies", len(lang.ConflictPolicies))
	}

	blob := BlobByName("haskell")
	if len(blob) == 0 {
		t.Fatal("BlobByName(haskell) returned no data")
	}
	sum := sha256.Sum256(blob)
	if !attachBuiltinLanguageRuntimeProfile("haskell", sum, lang) {
		t.Fatal("certified Haskell blob did not attach its runtime profile")
	}
	if got := len(lang.ConflictPolicies); got != 3 {
		t.Fatalf("certified Haskell conflict policies = %d, want 3", got)
	}
	if attachBuiltinLanguageRuntimeProfile("haskell", sum, lang) {
		t.Fatal("reattaching the same Haskell profile reported a change")
	}
	if got := len(lang.ConflictPolicies); got != 3 {
		t.Fatalf("reattached Haskell conflict policies = %d, want 3", got)
	}
}

// TestBuiltinDartConflictPoliciesAttach covers the three certified dart
// repeat-boundary rows (enum bodies, extension bodies, top-level declaration
// lists) that replaced the hardcoded dartRepetitionShiftConflictChoice helper
// through the generic policy path.
func TestBuiltinDartConflictPoliciesAttach(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	lang := DartLanguage()
	wantStates := map[gotreesitter.StateID]gotreesitter.Symbol{596: 509, 602: 512, 479: 467}
	gotStates := map[gotreesitter.StateID]bool{}
	for _, policy := range lang.ConflictPolicies {
		wantReduce, known := wantStates[policy.State]
		if !known {
			continue
		}
		gotStates[policy.State] = true
		if policy.Lookahead != gotreesitter.ConflictPolicyAnyLookahead ||
			policy.Kind != gotreesitter.ConflictPolicyRepetitionShift ||
			len(policy.ReduceSymbols) != 1 || policy.ReduceSymbols[0] != wantReduce {
			t.Fatalf("dart conflict policy at state %d = %+v, want repetition-shift over reduce symbol %d", policy.State, policy, wantReduce)
		}
	}
	for state := range wantStates {
		if !gotStates[state] {
			t.Fatalf("dart conflict policy for state %d was not attached", state)
		}
	}
	if got, want := len(lang.ConflictPolicies), 3; got != want {
		t.Fatalf("dart ConflictPolicies = %d rows, want %d", got, want)
	}
}

// TestBuiltinGomodConflictPoliciesAttach covers the certified go.mod
// require-list rows that replaced gomodRepetitionShiftConflictChoice
// through the generic policy path.
func TestBuiltinGomodConflictPoliciesAttach(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	lang := GomodLanguage()
	var state3, state37 int
	for _, policy := range lang.ConflictPolicies {
		if policy.Kind != gotreesitter.ConflictPolicyRepetitionShift {
			t.Fatalf("gomod conflict policy = %+v, want repetition-shift kind", policy)
		}
		if policy.Lookahead != gotreesitter.ConflictPolicyAnyLookahead {
			t.Fatalf("gomod conflict policy = %+v, want wildcard lookahead", policy)
		}
		switch policy.State {
		case 3:
			if len(policy.ReduceSymbols) != 1 || policy.ReduceSymbols[0] != 50 {
				t.Fatalf("gomod state-3 conflict policy = %+v, want source_file_repeat1 (symbol 50)", policy)
			}
			state3++
		case 37:
			if len(policy.ReduceSymbols) != 1 || policy.ReduceSymbols[0] != 52 {
				t.Fatalf("gomod state-37 conflict policy = %+v, want require_directive_repeat1 (symbol 52)", policy)
			}
			state37++
		default:
			t.Fatalf("unexpected gomod conflict policy state %d", policy.State)
		}
	}
	if state3 != 1 {
		t.Fatalf("gomod state-3 conflict policies = %d, want 1", state3)
	}
	if state37 != 1 {
		t.Fatalf("gomod state-37 conflict policies = %d, want 1", state37)
	}
}

// TestBuiltinDotHasNoConflictPolicies documents that dot's retired
// compat-tier helper (dotRepetitionShiftConflictChoice) was NOT migrated to a
// certified ConflictPolicy: dot already relied on the engine-wide C
// repetition-skip fold (it is not in cRepetitionSkipOptOut), and A/B testing
// found the helper's repetition-shift preference grows the LR parse-stack
// depth O(n) with statement count for no behavioral benefit over that
// already-flat fold (see the "NOTE on dot" comment in runtime_profiles.go).
func TestBuiltinDotHasNoConflictPolicies(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	if got := len(DotLanguage().ConflictPolicies); got != 0 {
		t.Fatalf("dot ConflictPolicies = %d rows, want 0 (helper retired, not migrated)", got)
	}
}

// TestBuiltinCConflictPolicyAttachesWildcard covers the certified C
// translation_unit_repeat1/preproc_if_repeat1 fold, the one profile using the
// ConflictPolicyAnyState/AnyLookahead wildcards because the C-faithful rule
// is scoped by reduce-symbol identity alone (thousands of real table rows).
// Replaces cRepetitionShiftConflictChoice.
func TestBuiltinCConflictPolicyAttachesWildcard(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	lang := CLanguage()
	if got, want := len(lang.ConflictPolicies), 1; got != want {
		t.Fatalf("c ConflictPolicies = %d rows, want %d", got, want)
	}
	policy := lang.ConflictPolicies[0]
	if policy.State != gotreesitter.ConflictPolicyAnyState || policy.Lookahead != gotreesitter.ConflictPolicyAnyLookahead ||
		policy.Kind != gotreesitter.ConflictPolicyRepetitionShift || len(policy.ReduceSymbols) != 2 ||
		policy.ReduceSymbols[0] != 324 || policy.ReduceSymbols[1] != 326 {
		t.Fatalf("c conflict policy = %+v, want wildcard repetition-shift over translation_unit_repeat1(324)/preproc_if_repeat1(326)", policy)
	}
}

// TestBuiltinGomodCConflictPoliciesRequireCertifiedBlob mirrors the Haskell
// fail-closed coverage for the two new certified profiles: an uncertified
// blob identity must not attach any ConflictPolicies.
func TestBuiltinGomodCConflictPoliciesRequireCertifiedBlob(t *testing.T) {
	for _, name := range []string{"gomod", "c"} {
		t.Run(name, func(t *testing.T) {
			lang := &gotreesitter.Language{Name: name}
			if attachBuiltinLanguageRuntimeProfile(name, sha256.Sum256([]byte("uncertified")), lang) {
				t.Fatalf("uncertified %s blob reported a runtime-profile attachment", name)
			}
			if len(lang.ConflictPolicies) != 0 {
				t.Fatalf("uncertified %s blob attached %d conflict policies", name, len(lang.ConflictPolicies))
			}

			blob := BlobByName(name)
			if len(blob) == 0 {
				t.Fatalf("BlobByName(%s) returned no data", name)
			}
			sum := sha256.Sum256(blob)
			if !attachBuiltinLanguageRuntimeProfile(name, sum, lang) {
				t.Fatalf("certified %s blob did not attach its runtime profile", name)
			}
			if len(lang.ConflictPolicies) == 0 {
				t.Fatalf("certified %s blob attached no conflict policies", name)
			}
		})
	}
}

func TestBuiltinCompleteAcceptedErrorRetryProfilesAttach(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	tests := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "bash", load: BashLanguage},
		{name: "c", load: CLanguage},
		{name: "caddy", load: CaddyLanguage},
		{name: "c_sharp", load: CSharpLanguage},
		{name: "cpp", load: CppLanguage},
		{name: "haxe", load: HaxeLanguage},
		{name: "kdl", load: KdlLanguage},
		{name: "meson", load: MesonLanguage},
		{name: "odin", load: OdinLanguage},
		{name: "rego", load: RegoLanguage},
		{name: "scss", load: ScssLanguage},
		{name: "swift", load: SwiftLanguage},
		{name: "tcl", load: TclLanguage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := tt.load().FullParseAcceptedErrorRetryProfile
			if !profile.SkipCompleteAcceptedErrorRetry {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want skip-complete certification", profile)
			}
			if tt.name == "swift" && profile.SkipCompleteMaxEntryScratchPeak != 3*64*1024 {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want first-growth entry-scratch ceiling", profile)
			}
			if tt.name == "meson" && profile.SkipCompleteMinSourceBytes != 2*1024 {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want %d-byte skip minimum", profile, 2*1024)
			}
			if tt.name == "c_sharp" && (profile.SkipCompleteMaxEntryScratchPeak != csharpAcceptedErrorRetryMaxEntryScratchPeak ||
				profile.FreshErrorNoStacksRetryMaxStacks != csharpFreshErrorNoStacksRetryMaxStacks ||
				profile.GSSConvergenceAcceptedErrorMergePerKey != csharpGSSConvergenceErrorMergePerKey ||
				!profile.SkipInitialCompleteAcceptedErrorMergeRetry) {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want bounded C# retry profile", profile)
			}
			if tt.name == "tcl" && profile.FreshErrorNoStacksMaxPasses != 1 {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want one fresh no-stacks retry", profile)
			}
		})
	}
}

func TestBuiltinBoundedAcceptedErrorRetryProfilesAttach(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	tests := []struct {
		name          string
		load          func() *gotreesitter.Language
		want          gotreesitter.FullParseAcceptedErrorRetryProfile
		wantNoScanner bool
	}{
		{
			name: "asm",
			load: AsmLanguage,
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{
				MinSourceBytes:      11 * 1024,
				InitialStackCeiling: 8,
			},
		},
		{
			name: "java",
			load: JavaLanguage,
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{
				MinSourceBytes:      64 * 1024,
				InitialStackCeiling: 14,
			},
			wantNoScanner: true,
		},
		{
			name: "groovy",
			load: GroovyLanguage,
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{
				MinSourceBytes:      64 * 1024,
				InitialStackCeiling: 2,
			},
			wantNoScanner: true,
		},
		{
			name: "d",
			load: DLanguage,
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{
				MinSourceBytes:      64 * 1024,
				InitialStackCeiling: 3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := tt.load()
			if tt.wantNoScanner && lang.ExternalScanner != nil {
				t.Fatalf("%s ExternalScanner != nil; profile must not depend on scanner attachment", tt.name)
			}
			if got := lang.FullParseAcceptedErrorRetryProfile; got != tt.want {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuiltinExternalScannerRetryProfilesRequireCertifiedBlob(t *testing.T) {
	tests := []struct {
		name    string
		scanner gotreesitter.ExternalScanner
	}{
		{name: "crystal", scanner: CrystalExternalScanner{}},
		{name: "kotlin", scanner: KotlinExternalScanner{}},
		{name: "matlab", scanner: MatlabExternalScanner{}},
		{name: "swift", scanner: SwiftExternalScanner{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := &gotreesitter.Language{Name: tt.name, ExternalScanner: tt.scanner}
			if attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256([]byte("uncertified")), lang) {
				t.Fatal("uncertified blob reported a runtime-profile attachment")
			}
			if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetryDefault {
				t.Fatalf("uncertified blob changed policy to %d", got)
			}

			blob := BlobByName(tt.name)
			if len(blob) == 0 {
				t.Fatalf("BlobByName(%s) returned no data", tt.name)
			}
			if !attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256(blob), lang) {
				t.Fatal("certified blob did not attach its runtime profile")
			}
			if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetrySkipRepeat {
				t.Fatalf("certified blob policy = %d, want skip-repeat", got)
			}
		})
	}
}

func TestBuiltinCSharpRetryProfileRequiresCertifiedBlob(t *testing.T) {
	lang := &gotreesitter.Language{ExternalScanner: CSharpExternalScanner{}}
	if attachBuiltinLanguageRuntimeProfile("c_sharp", sha256.Sum256([]byte("uncertified")), lang) {
		t.Fatal("uncertified C# blob reported a runtime-profile attachment")
	}
	if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetryDefault {
		t.Fatalf("uncertified C# blob changed scanner retry policy to %d", got)
	}
	if got := lang.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
		t.Fatalf("uncertified C# blob changed retry profile to %+v", got)
	}
	if lang.FullParseGSSConvergenceEnabled {
		t.Fatal("uncertified C# blob enabled full-parse GSS convergence")
	}

	blob := BlobByName("c_sharp")
	if len(blob) == 0 {
		t.Fatal("BlobByName(c_sharp) returned no data")
	}
	if !attachBuiltinLanguageRuntimeProfile("c_sharp", sha256.Sum256(blob), lang) {
		t.Fatal("certified C# blob did not attach its runtime profile")
	}
	if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetrySkipRepeat {
		t.Fatalf("certified C# scanner retry policy = %d, want skip-repeat", got)
	}
	want := gotreesitter.FullParseAcceptedErrorRetryProfile{
		SkipCompleteAcceptedErrorRetry:             true,
		SkipCompleteMaxEntryScratchPeak:            csharpAcceptedErrorRetryMaxEntryScratchPeak,
		FreshErrorNoStacksRetryMaxStacks:           csharpFreshErrorNoStacksRetryMaxStacks,
		SkipInitialCompleteAcceptedErrorMergeRetry: true,
		GSSConvergenceAcceptedErrorMergePerKey:     csharpGSSConvergenceErrorMergePerKey,
	}
	if got := lang.FullParseAcceptedErrorRetryProfile; got != want {
		t.Fatalf("certified C# retry profile = %+v, want %+v", got, want)
	}
	if !lang.FullParseGSSConvergenceEnabled {
		t.Fatal("certified C# blob did not enable full-parse GSS convergence")
	}
}

func TestBuiltinBoundedAcceptedErrorRetryProfilesRequireCertifiedBlob(t *testing.T) {
	tests := []struct {
		name string
		want gotreesitter.FullParseAcceptedErrorRetryProfile
	}{
		{name: "asm", want: gotreesitter.FullParseAcceptedErrorRetryProfile{MinSourceBytes: 11 * 1024, InitialStackCeiling: 8}},
		{name: "java", want: gotreesitter.FullParseAcceptedErrorRetryProfile{MinSourceBytes: 64 * 1024, InitialStackCeiling: 14}},
		{name: "groovy", want: gotreesitter.FullParseAcceptedErrorRetryProfile{MinSourceBytes: 64 * 1024, InitialStackCeiling: 2}},
		{name: "d", want: gotreesitter.FullParseAcceptedErrorRetryProfile{MinSourceBytes: 64 * 1024, InitialStackCeiling: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := &gotreesitter.Language{Name: tt.name}
			if attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256([]byte("uncertified")), lang) {
				t.Fatalf("uncertified %s blob reported a runtime-profile attachment", tt.name)
			}
			if got := lang.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
				t.Fatalf("uncertified %s blob changed retry profile to %+v", tt.name, got)
			}

			blob := BlobByName(tt.name)
			if len(blob) == 0 {
				t.Fatalf("BlobByName(%s) returned no data", tt.name)
			}
			if !attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256(blob), lang) {
				t.Fatalf("certified %s blob did not attach its runtime profile", tt.name)
			}
			if got := lang.FullParseAcceptedErrorRetryProfile; got != tt.want {
				t.Fatalf("certified %s retry profile = %+v, want %+v", tt.name, got, tt.want)
			}
		})
	}
}

func TestResidualRetryProfilesRequireExactBlobIdentity(t *testing.T) {
	tests := []struct {
		name string
		want gotreesitter.FullParseAcceptedErrorRetryProfile
	}{
		{
			name: "groovy",
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{MinSourceBytes: 64 * 1024, InitialStackCeiling: 2},
		},
		{
			name: "d",
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{MinSourceBytes: 64 * 1024, InitialStackCeiling: 3},
		},
		{
			name: "c_sharp",
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{
				SkipCompleteAcceptedErrorRetry:             true,
				SkipCompleteMaxEntryScratchPeak:            csharpAcceptedErrorRetryMaxEntryScratchPeak,
				FreshErrorNoStacksRetryMaxStacks:           csharpFreshErrorNoStacksRetryMaxStacks,
				SkipInitialCompleteAcceptedErrorMergeRetry: true,
				GSSConvergenceAcceptedErrorMergePerKey:     csharpGSSConvergenceErrorMergePerKey,
			},
		},
		{
			name: "meson",
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{
				SkipCompleteAcceptedErrorRetry: true,
				SkipCompleteMinSourceBytes:     2 * 1024,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrongSHA := &gotreesitter.Language{Name: tt.name}
			if attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256([]byte("uncertified")), wrongSHA) {
				t.Fatal("wrong blob SHA reported a runtime-profile attachment")
			}
			if got := wrongSHA.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
				t.Fatalf("wrong blob SHA attached retry profile %+v", got)
			}

			adapted := &gotreesitter.Language{Name: tt.name}
			AttachLanguageSupport(tt.name, adapted)
			if got := adapted.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
				t.Fatalf("adapted same-name language attached retry profile %+v", got)
			}

			blob := BlobByName(tt.name)
			if len(blob) == 0 {
				t.Fatal("BlobByName returned no data")
			}
			exact := &gotreesitter.Language{Name: tt.name}
			if !attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256(blob), exact) {
				t.Fatal("exact blob did not attach its runtime profile")
			}
			if got := exact.FullParseAcceptedErrorRetryProfile; got != tt.want {
				t.Fatalf("exact blob retry profile = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCSharpNativeResultCompatibilityRequiresExactBlobIdentity(t *testing.T) {
	want := gotreesitter.ResultCompatibilityCSharpNativeNotNull |
		gotreesitter.ResultCompatibilityCSharpNativeUnicodeIdentifiers |
		gotreesitter.ResultCompatibilityCSharpNativeScopedLambdaStatements |
		gotreesitter.ResultCompatibilityCSharpNativeScopedLambdaBlocks |
		gotreesitter.ResultCompatibilityCSharpNativeQueryExpressions |
		gotreesitter.ResultCompatibilityNativeRecoveredStructure

	wrongSHA := &gotreesitter.Language{Name: "c_sharp"}
	if attachBuiltinLanguageRuntimeProfile("c_sharp", sha256.Sum256([]byte("uncertified")), wrongSHA) {
		t.Fatal("wrong blob SHA reported a runtime-profile attachment")
	}
	if got := wrongSHA.NativeResultCompatibility; got != 0 {
		t.Fatalf("wrong blob SHA attached native result compatibility %#x", got)
	}

	adapted := &gotreesitter.Language{Name: "c_sharp"}
	AttachLanguageSupport("c_sharp", adapted)
	if got := adapted.NativeResultCompatibility; got != 0 {
		t.Fatalf("AttachLanguageSupport certified native result compatibility without blob identity: %#x", got)
	}

	blob := BlobByName("c_sharp")
	if len(blob) == 0 {
		t.Fatal("BlobByName(c_sharp) returned no data")
	}
	exact := &gotreesitter.Language{Name: "c_sharp"}
	if !attachBuiltinLanguageRuntimeProfile("c_sharp", sha256.Sum256(blob), exact) {
		t.Fatal("exact C# blob did not attach its runtime profile")
	}
	if got := exact.NativeResultCompatibility; got != want {
		t.Fatalf("exact profile NativeResultCompatibility = %#x, want %#x", got, want)
	}

	loaded, err := LoadLanguage("c_sharp", blob)
	if err != nil {
		t.Fatalf("LoadLanguage(c_sharp): %v", err)
	}
	if got := loaded.NativeResultCompatibility; got != want {
		t.Fatalf("LoadLanguage NativeResultCompatibility = %#x, want %#x", got, want)
	}
	if got := CSharpLanguage().NativeResultCompatibility; got != want {
		t.Fatalf("embedded CSharpLanguage NativeResultCompatibility = %#x, want %#x", got, want)
	}
}

func TestCollapsedChildNativeCapabilityRequiresExactBlobIdentity(t *testing.T) {
	want := gotreesitter.ResultCompatibilityNativeCollapsedChildren
	tests := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "apex", load: ApexLanguage},
		{name: "dart", load: DartLanguage},
		{name: "elixir", load: ElixirLanguage},
		{name: "hack", load: HackLanguage},
		{name: "kotlin", load: KotlinLanguage},
		{name: "ruby", load: RubyLanguage},
	}
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrongSHA := &gotreesitter.Language{Name: tt.name}
			if attachBuiltinLanguageRuntimeProfile(tt.name, sha256.Sum256([]byte("uncertified")), wrongSHA) {
				t.Fatal("wrong blob SHA reported a runtime-profile attachment")
			}
			if wrongSHA.NativeResultCompatibility&want != 0 {
				t.Fatal("wrong blob SHA attached collapsed-child capability")
			}

			adapted := &gotreesitter.Language{Name: tt.name}
			AttachLanguageSupport(tt.name, adapted)
			if adapted.NativeResultCompatibility&want != 0 {
				t.Fatal("same-name adapted language attached collapsed-child capability")
			}

			exact := tt.load()
			if exact.NativeResultCompatibility&want == 0 {
				t.Fatal("exact built-in artifact omitted collapsed-child capability")
			}
		})
	}
}

func TestBuiltinCompleteAcceptedErrorRetryProfileRequiresCertifiedBlob(t *testing.T) {
	for _, name := range []string{"c", "caddy", "c_sharp", "haxe", "kdl", "odin", "rego", "scss", "swift", "tcl"} {
		t.Run(name, func(t *testing.T) {
			lang := &gotreesitter.Language{Name: name}
			if attachBuiltinLanguageRuntimeProfile(name, sha256.Sum256([]byte("uncertified")), lang) {
				t.Fatalf("uncertified %s blob reported a runtime-profile attachment", name)
			}
			if got := lang.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
				t.Fatalf("uncertified %s blob changed retry profile to %+v", name, got)
			}

			blob := BlobByName(name)
			if len(blob) == 0 {
				t.Fatalf("BlobByName(%s) returned no data", name)
			}
			if !attachBuiltinLanguageRuntimeProfile(name, sha256.Sum256(blob), lang) {
				t.Fatalf("certified %s blob did not attach its runtime profile", name)
			}
			if !lang.FullParseAcceptedErrorRetryProfile.SkipCompleteAcceptedErrorRetry {
				t.Fatalf("certified %s retry profile = %+v, want skip-complete certification", name, lang.FullParseAcceptedErrorRetryProfile)
			}
		})
	}
}

func TestAttachLanguageSupportDoesNotCertifyWithoutBlobIdentity(t *testing.T) {
	base := KotlinLanguage()
	lang := &gotreesitter.Language{
		Name:            base.Name,
		ExternalSymbols: append([]gotreesitter.Symbol(nil), base.ExternalSymbols...),
		SymbolNames:     append([]string(nil), base.SymbolNames...),
	}
	if !AttachLanguageSupport("kotlin", lang) {
		t.Fatal("AttachLanguageSupport(kotlin) did not attach scanner support")
	}
	if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetryDefault {
		t.Fatalf("AttachLanguageSupport certified policy without blob identity: %d", got)
	}

	java := &gotreesitter.Language{Name: "java"}
	AttachLanguageSupport("java", java)
	if got := java.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
		t.Fatalf("AttachLanguageSupport certified Java retry profile without blob identity: %+v", got)
	}

	rego := &gotreesitter.Language{Name: "rego"}
	AttachLanguageSupport("rego", rego)
	if got := rego.FullParseAcceptedErrorRetryProfile; got != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) {
		t.Fatalf("AttachLanguageSupport certified Rego retry profile without blob identity: %+v", got)
	}
}

func TestNativeUnaryWrapperFlatteningProfileCensus(t *testing.T) {
	configuredLanguages := make(map[string]int)
	for _, entry := range AllLanguages() {
		profile, ok := builtinLanguageRuntimeProfiles[entry.Name]
		if !ok || len(profile.nativeUnaryWrapperFlattening) == 0 {
			continue
		}
		configuredLanguages[entry.Name] = len(profile.nativeUnaryWrapperFlattening)
	}
	if len(configuredLanguages) != 1 || configuredLanguages["fsharp"] != 1 {
		t.Fatalf("native unary-wrapper profile census = %v, want fsharp:1", configuredLanguages)
	}

	language := FsharpLanguage()
	if len(language.NativeUnaryWrapperFlattening) != 1 {
		t.Fatalf("F# native unary-wrapper rules = %d, want 1", len(language.NativeUnaryWrapperFlattening))
	}
	rule := language.NativeUnaryWrapperFlattening[0]
	for symbol, want := range map[gotreesitter.Symbol]string{
		rule.PublicParent: "long_identifier_or_op",
		rule.Wrapper:      "long_identifier",
		rule.Leaf:         "identifier",
	} {
		if int(symbol) >= len(language.SymbolNames) || language.SymbolNames[symbol] != want {
			t.Fatalf("F# unary-wrapper symbol %d = %q, want %q", symbol, language.SymbolNames[symbol], want)
		}
	}
	if got, want := rule.WrapperPreGotoState, gotreesitter.StateID(4258); got != want {
		t.Fatalf("F# wrapper pre-goto state = %d, want %d", got, want)
	}

	stale := &gotreesitter.Language{
		Name:           language.Name,
		StateCount:     language.StateCount,
		SymbolNames:    language.SymbolNames,
		SymbolMetadata: language.SymbolMetadata,
	}
	if attachBuiltinLanguageRuntimeProfile("fsharp", [32]byte{}, stale) {
		t.Fatal("stale F# blob identity attached the unary-wrapper profile")
	}
	if len(stale.NativeUnaryWrapperFlattening) != 0 {
		t.Fatalf("stale F# unary-wrapper rules = %v, want none", stale.NativeUnaryWrapperFlattening)
	}
}
