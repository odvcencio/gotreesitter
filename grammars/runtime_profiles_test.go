package grammars

import (
	"crypto/sha256"
	"slices"
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

func TestHTMLProfileCertifiesCompleteCompactRecovery(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := HtmlLanguage()
	if !lang.CompactStrategy2ErrorRegionCertified || !lang.CompactMissingTokenInsertionCertified {
		t.Fatal("the HTML profile did not attach both compact recovery capabilities")
	}
	if lang.CompactRecoveryPlainFirstCertified {
		t.Fatal("the HTML profile unexpectedly enabled plain-first recovery")
	}
	if lang.CompactFaithfulS5RecoveryCertified {
		t.Fatal("the HTML profile unexpectedly enabled the Scala S5 route")
	}
	uncertified := &gotreesitter.Language{}
	if attachBuiltinLanguageRuntimeProfile("html", sha256.Sum256([]byte("wrong html blob")), uncertified) ||
		uncertified.CompactStrategy2ErrorRegionCertified || uncertified.CompactMissingTokenInsertionCertified ||
		uncertified.CompactRecoveryPlainFirstCertified {
		t.Fatal("a mismatched HTML blob received compact recovery certification")
	}
}

func TestYAMLProfileCertifiesOnlyExactRecoverEOFArtifact(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := YamlLanguage()
	if !lang.CompactRecoverEOFCertified {
		t.Fatal("the YAML profile did not attach recover_eof certification")
	}
	wantArtifact := gotreesitter.CompactRecoverEOFArtifactReceipt{
		BlobSHA256:        mustRuntimeProfileSHA256("9ac37a326be7a4f4297efa6e43ba00317c5959e7b4909c9262043d408f284b30"),
		TerminalSymbol:    26,
		EOFState:          189,
		EOFByteOffset:     1,
		Passes:            2,
		Elections:         2,
		ActionLookups:     2,
		Dispatches:        1,
		OrdinaryShifts:    1,
		OrdinaryCohorts:   0,
		ExtraShifts:       0,
		ExtraCohorts:      0,
		Reductions:        0,
		Conflicts:         0,
		ConflictActions:   0,
		Forks:             0,
		RepetitionFolds:   0,
		RecoveryWork:      0,
		NoActionDrops:     0,
		ReductionPauses:   0,
		Accepts:           0,
		Canonicalizations: 1,
		PeakHeaders:       1,
	}
	if lang.CompactRecoverEOFArtifactReceipt != wantArtifact {
		t.Fatalf("YAML recover_eof artifact receipt=%+v, want %+v", lang.CompactRecoverEOFArtifactReceipt, wantArtifact)
	}
	if lang.CompactStrategy2ErrorRegionCertified || lang.CompactStackSummaryRecoveryCertified ||
		lang.CompactMissingTokenInsertionCertified {
		t.Fatal("the YAML recover_eof profile enabled another recovery mechanism")
	}
	stale := &gotreesitter.Language{}
	if attachBuiltinLanguageRuntimeProfile("yaml", sha256.Sum256([]byte("stale yaml blob")), stale) ||
		stale.CompactRecoverEOFCertified {
		t.Fatal("a stale YAML blob received recover_eof certification")
	}
	exact := &gotreesitter.Language{}
	blob := BlobByName("yaml")
	if len(blob) == 0 || !attachBuiltinLanguageRuntimeProfile("yaml", sha256.Sum256(blob), exact) ||
		!exact.CompactRecoverEOFCertified || exact.CompactRecoverEOFArtifactReceipt != wantArtifact {
		t.Fatal("the exact YAML blob did not receive recover_eof certification")
	}
}

func TestJavaScriptProfileCertifiesCompactRecoveryFrontier(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := JavascriptLanguage()
	if !lang.CompactStrategy2ErrorRegionCertified || !lang.CompactMissingTokenInsertionCertified ||
		!lang.CompactRecoveryPlainFirstCertified || !lang.CompactRecoveryTrailingLineageRetirementCertified ||
		!lang.CompactRecoveryErrorModeKeywordCaptureCertified {
		t.Fatal("the JavaScript profile did not attach its compact recovery capabilities")
	}
	if lang.CompactFaithfulS5RecoveryCertified {
		t.Fatal("the JavaScript profile unexpectedly enabled the Scala S5 route")
	}
	wantAliases := []gotreesitter.CompactRecoveryTerminalAliasRule{
		{ResumeState: 20, ResumeSymbol: 54, AliasSymbol: 261},
		{ResumeState: 231, ResumeSymbol: 85, AliasSymbol: 261},
		{ResumeState: 1042, ResumeSymbol: 129, AliasSymbol: 261},
		{ResumeState: 1367, ResumeSymbol: 7, AliasSymbol: 261},
	}
	if !slices.Equal(lang.CompactRecoveryTerminalAliasRules, wantAliases) {
		t.Fatalf("terminal alias rules=%+v, want %+v", lang.CompactRecoveryTerminalAliasRules, wantAliases)
	}
	uncertified := &gotreesitter.Language{}
	if attachBuiltinLanguageRuntimeProfile("javascript", sha256.Sum256([]byte("wrong javascript blob")), uncertified) ||
		uncertified.CompactStrategy2ErrorRegionCertified || uncertified.CompactMissingTokenInsertionCertified ||
		uncertified.CompactRecoveryPlainFirstCertified || uncertified.CompactRecoveryTrailingLineageRetirementCertified ||
		uncertified.CompactRecoveryErrorModeKeywordCaptureCertified ||
		len(uncertified.CompactRecoveryTerminalAliasRules) != 0 {
		t.Fatal("a mismatched JavaScript blob received compact recovery certification")
	}
}

func TestScalaProfileCertifiesPackageTwoRecovery(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := ScalaLanguage()
	if !lang.CompactStrategy2ErrorRegionCertified || !lang.CompactMissingTokenInsertionCertified ||
		!lang.CompactFaithfulS5RecoveryCertified ||
		!lang.CompactPrimaryAcceptanceDerivationCertified ||
		!lang.CompactAcceptanceStructuralElectionCertified ||
		!lang.CompactRecoveryTrailingLineageRetirementCertified ||
		!lang.CompactRecoveryErrorModeKeywordCaptureCertified {
		t.Fatal("the Scala profile did not attach its package-two recovery capabilities")
	}
	if lang.CompactStackSummaryRecoveryCertified || lang.CompactRecoverEOFCertified {
		t.Fatal("the Scala package-two profile enabled a later recovery package")
	}
	uncertified := &gotreesitter.Language{}
	if attachBuiltinLanguageRuntimeProfile("scala", sha256.Sum256([]byte("wrong scala blob")), uncertified) ||
		uncertified.CompactStrategy2ErrorRegionCertified ||
		uncertified.CompactMissingTokenInsertionCertified ||
		uncertified.CompactFaithfulS5RecoveryCertified ||
		uncertified.CompactPrimaryAcceptanceDerivationCertified ||
		uncertified.CompactAcceptanceStructuralElectionCertified ||
		uncertified.CompactRecoveryTrailingLineageRetirementCertified ||
		uncertified.CompactRecoveryErrorModeKeywordCaptureCertified {
		t.Fatal("a mismatched Scala blob received package-two recovery certification")
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
	// 47 = the prior 46 plus the V entry. It reuses a certified clean wide
	// result during the accepted-error retry ladder.
	// 48 = the prior 47 plus the ql entry. It carries a
	// ConflictPolicyDeclaredReduceReduceHighestSymbol row that resolves the
	// grammar-declared simpleId/className reduce-reduce conflict to the
	// C-native reading during dispatch.
	// 49 = the prior 48 plus the JSDoc entry. It certifies exact internal-DFA
	// skipped-prefix evidence for compact reduce tiling.
	// 50 = the prior 49 plus the Markdown inline entry. It certifies four exact
	// compact conflict rows while production parsing ignores them.
	// 51 = the prior 50 plus the irreducible YAML recover_eof EOF-root entry.
	// 52 = the prior 51 plus the certified Scala package-two recovery entry.
	if got, want := len(builtinLanguageRuntimeProfiles), 52; got != want {
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
	if lang.CompactLexerSkippedPrefixTilingCertified {
		t.Fatal("unknown runtime profile enabled compact lexer skipped-prefix tiling")
	}
	if lang.ExactStackNodeEquivalenceCertified {
		t.Fatal("unknown runtime profile enabled exact stack-node equivalence")
	}
	if lang.CompactPackedGSSVersionOrderCertified {
		t.Fatal("unknown runtime profile enabled packed GSS version ordering")
	}
	if lang.CompactRecoverEOFCertified {
		t.Fatal("unknown runtime profile enabled recover_eof acceptance")
	}
	if lang.CompactRecoveryPlainFirstCertified {
		t.Fatal("unknown runtime profile enabled compact plain-first recovery")
	}
	if lang.CompactRecoveryTrailingLineageRetirementCertified {
		t.Fatal("unknown runtime profile enabled compact trailing-lineage retirement")
	}
	if lang.CompactRecoveryErrorModeKeywordCaptureCertified {
		t.Fatal("unknown runtime profile enabled compact error-mode keyword capture")
	}
	if len(lang.CompactRecoveryTerminalAliasRules) != 0 {
		t.Fatal("unknown runtime profile attached compact recovery terminal aliases")
	}
	if lang.LineContinuationEscapeByte != 0 {
		t.Fatalf("unknown runtime profile set a line-continuation escape byte: %q", lang.LineContinuationEscapeByte)
	}
}

func TestRuntimeProfileSymbolRejectsDuplicateNames(t *testing.T) {
	lang := &gotreesitter.Language{SymbolNames: []string{"first", "duplicate", "duplicate"}}
	if symbol, ok := runtimeProfileSymbol(lang, "first"); !ok || symbol != 0 {
		t.Fatalf("unique symbol=%d ok=%t, want 0/true", symbol, ok)
	}
	if symbol, ok := runtimeProfileSymbol(lang, "duplicate"); ok {
		t.Fatalf("duplicate symbol=%d ok=true, want fail-closed", symbol)
	}
}

func TestRuntimeProfileTerminalSymbolIgnoresNonterminalDuplicate(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  2,
		SymbolNames: []string{"end", "class", "class"},
	}
	if symbol, ok := runtimeProfileTerminalSymbol(lang, "class"); !ok || symbol != 1 {
		t.Fatalf("terminal symbol=%d ok=%t, want 1/true", symbol, ok)
	}
	lang.SymbolNames[0] = "class"
	if symbol, ok := runtimeProfileTerminalSymbol(lang, "class"); ok {
		t.Fatalf("duplicate terminal symbol=%d ok=true, want fail-closed", symbol)
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

func TestBuiltinCompactEOFProfilesRetireOnlyHTTPAndRobotField(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	for _, test := range []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "http", load: HttpLanguage},
		{name: "robot", load: RobotLanguage},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile, ok := builtinLanguageRuntimeProfiles[test.name]
			if !ok {
				t.Fatal("retired EOF profile lost its exact blob record")
			}
			blob := BlobByName(test.name)
			if len(blob) == 0 || sha256.Sum256(blob) != profile.blobSHA256 {
				t.Fatal("retired EOF profile blob identity changed")
			}
			if language := test.load(); language.CompactEOFAcceptNoActionSiblingsCertified {
				t.Fatal("exact built-in artifact retained the legacy EOF sibling field")
			}

			custom := &gotreesitter.Language{Name: test.name}
			AttachLanguageSupport(test.name, custom)
			if custom.CompactEOFAcceptNoActionSiblingsCertified {
				t.Fatal("same-name custom grammar enabled the retired EOF sibling field")
			}

			wrongIdentity := &gotreesitter.Language{Name: test.name}
			if attachBuiltinLanguageRuntimeProfile(test.name, sha256.Sum256([]byte("uncertified")), wrongIdentity) {
				t.Fatal("wrong blob identity unexpectedly attached a runtime profile")
			}
			if wrongIdentity.CompactEOFAcceptNoActionSiblingsCertified {
				t.Fatal("wrong blob identity enabled the retired EOF sibling field")
			}

			exact := &gotreesitter.Language{Name: test.name}
			if attachBuiltinLanguageRuntimeProfile(test.name, profile.blobSHA256, exact) {
				t.Fatal("exact retired profile unexpectedly changed a language field")
			}
			if exact.CompactEOFAcceptNoActionSiblingsCertified {
				t.Fatal("exact retired profile enabled the legacy EOF sibling field")
			}
		})
	}
}

func TestBuiltinCompactAcceptanceProfilesRequireExactBlobIdentity(t *testing.T) {
	tests := []struct {
		name string
		load func() *gotreesitter.Language
		want func(*gotreesitter.Language) bool
	}{
		{
			name: "meson", load: MesonLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified &&
					lang.CompactAcceptanceStructuralElectionCertified
			},
		},
		// The tied-election family (A3 certification workstream,
		// spec.campaign.v7, finding
		// tied-election-family-compact-retirement): full-corpus field-aware
		// C-oracle verification certifies primary-acceptance-derivation
		// selection for all five languages. Python also certifies C's raw
		// subtree ordering for clean ties. Kotlin's grant lands under
		// selectCompactAcceptanceDerivation's materiality gate
		// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous);
		// see the runtime_profiles.go "kotlin" entry comment.
		{
			name: "python", load: PythonLanguage,
			want: func(lang *gotreesitter.Language) bool {
				return lang.CompactPrimaryAcceptanceDerivationCertified &&
					lang.CompactAcceptanceStructuralElectionCertified &&
					lang.CompactMixedGSSMergeCertified
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

func TestBuiltinJsdocLexerSkippedPrefixProfileRequiresExactBlobIdentity(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	builtin := JsdocLanguage()
	if !builtin.CompactLexerSkippedPrefixTilingCertified {
		t.Fatal("exact built-in JSDoc artifact did not receive skipped-prefix certification")
	}

	custom := &gotreesitter.Language{Name: "jsdoc"}
	AttachLanguageSupport("jsdoc", custom)
	if custom.CompactLexerSkippedPrefixTilingCertified {
		t.Fatal("same-name custom JSDoc grammar enabled skipped-prefix certification")
	}

	stale := &gotreesitter.Language{Name: "jsdoc"}
	if attachBuiltinLanguageRuntimeProfile("jsdoc", sha256.Sum256([]byte("stale")), stale) {
		t.Fatal("stale JSDoc blob unexpectedly attached a runtime profile")
	}
	if stale.CompactLexerSkippedPrefixTilingCertified {
		t.Fatal("stale JSDoc blob enabled skipped-prefix certification")
	}

	exact := &gotreesitter.Language{Name: "jsdoc"}
	if !attachBuiltinLanguageRuntimeProfile("jsdoc", sha256.Sum256(BlobByName("jsdoc")), exact) {
		t.Fatal("exact JSDoc blob did not attach its runtime profile")
	}
	if !exact.CompactLexerSkippedPrefixTilingCertified {
		t.Fatal("exact JSDoc blob did not enable skipped-prefix certification")
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

func TestBuiltinCompactPackedGSSVersionOrderProfileRequiresExactBlobIdentity(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	builtin := ErlangLanguage()
	if !builtin.CompactPackedGSSVersionOrderCertified {
		t.Fatal("exact Erlang artifact did not receive packed GSS version-order certification")
	}

	custom := &gotreesitter.Language{Name: "erlang"}
	AttachLanguageSupport("erlang", custom)
	if custom.CompactPackedGSSVersionOrderCertified {
		t.Fatal("same-name custom grammar enabled packed GSS version ordering")
	}

	stale := &gotreesitter.Language{Name: "erlang"}
	if attachBuiltinLanguageRuntimeProfile("erlang", sha256.Sum256([]byte("stale-erlang-blob")), stale) {
		t.Fatal("stale Erlang blob attached the runtime profile")
	}
	if stale.CompactPackedGSSVersionOrderCertified {
		t.Fatal("stale Erlang blob enabled packed GSS version ordering")
	}
}

func TestBuiltinErlangPackedGSSVersionOrderIssue984Tree(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	builtin := ErlangLanguage()
	fresh, err := LoadLanguage("erlang", BlobByName("erlang"))
	if err != nil {
		t.Fatalf("load fresh Erlang language: %v", err)
	}
	for _, test := range []struct {
		name     string
		language *gotreesitter.Language
	}{
		{name: "fresh_decode", language: fresh},
		{name: "cached_builtin", language: builtin},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(test.language)
			parser.SetAdmissionCandidateRoute(false)
			tree, err := parser.Parse([]byte("000\"0A!A \"A\"=0:A0!)A\"0%0000"))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if got, want := tree.RootNode().SExpr(test.language), `(source_file (integer) (string) (var) (string) (integer) (comment))`; got != want {
				t.Fatalf("Erlang issue #984 tree = %s, want %s", got, want)
			}
		})
	}
}

func BenchmarkBuiltinErlangCompactIssue984(b *testing.B) {
	language := ErlangLanguage()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("000\"0A!A \"A\"=0:A0!)A\"0%0000")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}

func TestBuiltinErlangPackedGSSVersionOrderPreservesLinearRecovery(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	const source = "f(X) ->\n" +
		"  try 1 of\n" +
		"    X -> ok;\n" +
		"    Y when Z -> ok\n" +
		"  catch\n" +
		"    Pattern -> ok;\n" +
		"    error:Pattern -> ok;\n" +
		"    throw:Pattern:Stack -> ok;\n" +
		"    exit:Complex + Pattern:Stack -> ok\n" +
		"  after\n" +
		"    ok\n" +
		"  end,\n" +
		"  try _\n" +
		"  catch\n" +
		"  end.\n"
	builtin := ErlangLanguage()
	linear, err := LoadLanguage("erlang", BlobByName("erlang"))
	if err != nil {
		t.Fatalf("load fresh Erlang language: %v", err)
	}
	linear.CompactPackedGSSVersionOrderCertified = false
	var sexprs [2]string
	for i, language := range []*gotreesitter.Language{builtin, linear} {
		parser := gotreesitter.NewParser(language)
		tree, err := parser.Parse([]byte(source))
		if err != nil {
			t.Fatalf("route %d parse: %v", i, err)
		}
		if tree.RootNode().HasError() {
			got := tree.RootNode().SExpr(language)
			tree.Release()
			t.Fatalf("route %d produced an error tree: %s", i, got)
		}
		sexprs[i] = tree.RootNode().SExpr(language)
		tree.Release()
	}
	if sexprs[0] != sexprs[1] {
		t.Fatalf("packed and linear Erlang trees differ:\npacked: %s\nlinear: %s", sexprs[0], sexprs[1])
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

func TestBuiltinMarkdownInlineConflictPolicyRequiresExactBlob(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	builtin := MarkdownInlineLanguage()
	if got := len(builtin.ConflictPolicies); got != 4 {
		t.Fatalf("Markdown inline conflict policies = %d, want 4", got)
	}
	fold := builtin.ConflictPolicies[0]
	if fold.State != 18 || fold.Lookahead != 56 || fold.Kind != gotreesitter.ConflictPolicyRepetitionReduce ||
		!fold.CompactOnly || fold.CompactMinFrontierHeaders != 2 || len(fold.ReduceSymbols) != 1 || fold.ReduceSymbols[0] != 107 {
		t.Fatalf("Markdown inline fold policy = %+v, want state 18 reduce symbol 107 for lookahead 56", fold)
	}
	wordFold := builtin.ConflictPolicies[1]
	if wordFold.State != 18 || wordFold.Lookahead != 50 || wordFold.Kind != gotreesitter.ConflictPolicyRepetitionReduce ||
		!wordFold.CompactOnly || wordFold.CompactMinFrontierHeaders != 2 || len(wordFold.ReduceSymbols) != 1 || wordFold.ReduceSymbols[0] != 107 {
		t.Fatalf("Markdown inline word fold policy = %+v, want state 18 reduce symbol 107 for lookahead 50", wordFold)
	}
	lineEndFold := builtin.ConflictPolicies[2]
	if lineEndFold.State != 18 || lineEndFold.Lookahead != 36 || lineEndFold.Kind != gotreesitter.ConflictPolicyRepetitionReduce ||
		!lineEndFold.CompactOnly || lineEndFold.CompactMinFrontierHeaders != 2 || len(lineEndFold.ReduceSymbols) != 1 || lineEndFold.ReduceSymbols[0] != 107 {
		t.Fatalf("Markdown inline line-end fold policy = %+v, want state 18 reduce symbol 107 for lookahead 36", lineEndFold)
	}
	policy := builtin.ConflictPolicies[3]
	if policy.State != 209 || policy.Lookahead != 50 || policy.Kind != gotreesitter.ConflictPolicyShift ||
		!policy.CompactOnly || len(policy.ReduceSymbols) != 1 || policy.ReduceSymbols[0] != 104 {
		t.Fatalf("Markdown inline conflict policy = %+v, want state 209 shift over symbol 104 for lookahead 50", policy)
	}

	custom := &gotreesitter.Language{Name: "markdown_inline"}
	AttachLanguageSupport("markdown_inline", custom)
	if len(custom.ConflictPolicies) != 0 {
		t.Fatal("same-name custom grammar received the Markdown inline conflict policy")
	}

	stale := &gotreesitter.Language{Name: "markdown_inline"}
	if attachBuiltinLanguageRuntimeProfile("markdown_inline", sha256.Sum256([]byte("stale")), stale) {
		t.Fatal("stale Markdown inline blob reported a runtime-profile attachment")
	}
	if len(stale.ConflictPolicies) != 0 {
		t.Fatal("stale Markdown inline blob received the conflict policy")
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
	if got, want := len(lang.ConflictPolicies), 2; got != want {
		t.Fatalf("c ConflictPolicies = %d rows, want %d", got, want)
	}
	policy := lang.ConflictPolicies[0]
	if policy.State != gotreesitter.ConflictPolicyAnyState || policy.Lookahead != gotreesitter.ConflictPolicyAnyLookahead ||
		policy.Kind != gotreesitter.ConflictPolicyRepetitionShift || len(policy.ReduceSymbols) != 2 ||
		policy.ReduceSymbols[0] != 324 || policy.ReduceSymbols[1] != 326 {
		t.Fatalf("c conflict policy = %+v, want wildcard repetition-shift over translation_unit_repeat1(324)/preproc_if_repeat1(326)", policy)
	}
	// C is held out of the engine-wide repetition fold
	// (cRepetitionSkipConflictChoice), so it carves out the symbols that are
	// proven safe one row at a time, the way haskell does. This row folds
	// aux_sym_enumerator_list_repeat1 and stops a three-enumerator list from
	// inventing a comma; see grammars/c_enum_missing_comma_pin_test.go.
	fold := lang.ConflictPolicies[1]
	if fold.State != gotreesitter.ConflictPolicyAnyState || fold.Lookahead != gotreesitter.ConflictPolicyAnyLookahead ||
		fold.Kind != gotreesitter.ConflictPolicyRepetitionReduce || len(fold.ReduceSymbols) != 1 ||
		fold.ReduceSymbols[0] != 343 {
		t.Fatalf("c conflict policy[1] = %+v, want wildcard repetition-reduce over enumerator_list_repeat1(343)", fold)
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
		{name: "v", load: VLanguage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := tt.load().FullParseAcceptedErrorRetryProfile
			if tt.name == "v" {
				if profile.SkipInitialCompleteAcceptedErrorMergeRetry || !profile.ReuseCleanWideForWideRetry ||
					profile.SkipFreshCompleteAcceptedErrorRetry || profile.SkipCompleteAcceptedErrorRetry {
					t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want clean-wide reuse certification", profile)
				}
			} else if !profile.SkipCompleteAcceptedErrorRetry {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want skip-complete certification", profile)
			}
			if tt.name == "swift" && profile.SkipCompleteMaxEntryScratchPeak != 3*64*1024 {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want first-growth entry-scratch ceiling", profile)
			}
			if tt.name == "meson" && profile.SkipCompleteMinSourceBytes != 2*1024 {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want %d-byte skip minimum", profile, 2*1024)
			}
			if tt.name == "v" && profile.ReuseCleanWideMinSourceBytes != vAcceptedErrorRetryMinSourceBytes {
				t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want %d-byte reuse minimum", profile, vAcceptedErrorRetryMinSourceBytes)
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
		{
			name: "v",
			want: gotreesitter.FullParseAcceptedErrorRetryProfile{
				ReuseCleanWideForWideRetry:   true,
				ReuseCleanWideMinSourceBytes: vAcceptedErrorRetryMinSourceBytes,
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
	for _, name := range []string{"c", "caddy", "c_sharp", "haxe", "kdl", "odin", "rego", "scss", "swift", "tcl", "v"} {
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
			if name == "v" {
				profile := lang.FullParseAcceptedErrorRetryProfile
				if profile.SkipInitialCompleteAcceptedErrorMergeRetry || !profile.ReuseCleanWideForWideRetry {
					t.Fatalf("certified %s retry profile = %+v, want clean-wide reuse certification", name, profile)
				}
			} else if !lang.FullParseAcceptedErrorRetryProfile.SkipCompleteAcceptedErrorRetry {
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
