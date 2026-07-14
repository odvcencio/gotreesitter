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
		{name: "dart", load: DartLanguage},
		{name: "kotlin", load: KotlinLanguage},
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

func TestBuiltinRuntimeProfilesStayNarrow(t *testing.T) {
	// 24 = the prior 19 plus D and Groovy, whose retry ceilings moved out of
	// parser-core name switches and onto exact-blob profiles. The prior gomod
	// and C additions moved hardcoded compat-tier behavior to profiles. Meson
	// and Enforce add two exact-blob retry policies. JavaScript adds one
	// exact-blob automatic-forest memory allowance. The
	// repetition-conflict helpers were retired in favor of certified
	// ConflictPolicies rows here. dot's helper was
	// retired outright, not migrated (see the "NOTE on dot" comment above
	// the gomod entry), so it does not add a map entry.
	if got, want := len(builtinLanguageRuntimeProfiles), 24; got != want {
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
	for _, policy := range lang.ConflictPolicies {
		if policy.State != 11192 || policy.Lookahead != 4 {
			continue
		}
		if policy.Kind != gotreesitter.ConflictPolicyRepetitionReduce ||
			len(policy.ReduceSymbols) != 1 || policy.ReduceSymbols[0] != 500 {
			t.Fatalf("Haskell conflict policy = %+v, want certified expression-list reduce", policy)
		}
		return
	}
	t.Fatal("Haskell expression-list conflict policy was not attached")
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
	if got := len(lang.ConflictPolicies); got != 1 {
		t.Fatalf("certified Haskell conflict policies = %d, want 1", got)
	}
	if attachBuiltinLanguageRuntimeProfile("haskell", sum, lang) {
		t.Fatal("reattaching the same Haskell profile reported a change")
	}
	if got := len(lang.ConflictPolicies); got != 1 {
		t.Fatalf("reattached Haskell conflict policies = %d, want 1", got)
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
	lang := &gotreesitter.Language{ExternalScanner: KotlinExternalScanner{}}
	if attachBuiltinLanguageRuntimeProfile("kotlin", sha256.Sum256([]byte("uncertified")), lang) {
		t.Fatal("uncertified Kotlin blob reported a runtime-profile attachment")
	}
	if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetryDefault {
		t.Fatalf("uncertified Kotlin blob changed policy to %d", got)
	}

	blob := BlobByName("kotlin")
	if len(blob) == 0 {
		t.Fatal("BlobByName(kotlin) returned no data")
	}
	if !attachBuiltinLanguageRuntimeProfile("kotlin", sha256.Sum256(blob), lang) {
		t.Fatal("certified Kotlin blob did not attach its runtime profile")
	}
	if got := lang.ExternalScannerFullParseRetryPolicy; got != gotreesitter.ExternalScannerFullParseRetrySkipRepeat {
		t.Fatalf("certified Kotlin blob policy = %d, want skip-repeat", got)
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
	}
	if got := lang.FullParseAcceptedErrorRetryProfile; got != want {
		t.Fatalf("certified C# retry profile = %+v, want %+v", got, want)
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
		gotreesitter.ResultCompatibilityCSharpNativeQueryExpressions

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

func TestBuiltinCompleteAcceptedErrorRetryProfileRequiresCertifiedBlob(t *testing.T) {
	for _, name := range []string{"caddy", "c_sharp", "haxe", "kdl", "odin", "rego", "scss", "swift", "tcl"} {
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
