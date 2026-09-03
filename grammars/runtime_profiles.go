package grammars

import (
	"encoding/hex"
	"slices"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// builtinLanguageRuntimeProfile contains narrow runtime decisions certified
// against the checked-in grammar/scanner pair. Keep these policies outside the
// parser core: loading the exact certified blob attaches its profile, while
// caller-constructed and adapted languages retain conservative zero defaults.
type builtinLanguageRuntimeProfile struct {
	blobSHA256                          [32]byte
	externalScannerFullParseRetry       gotreesitter.ExternalScannerFullParseRetryPolicy
	fullParseAcceptedErrorRetryProfile  gotreesitter.FullParseAcceptedErrorRetryProfile
	automaticForestMemoryAllowance      int64
	automaticForestEnabled              bool
	fullParseArenaDensityCap            bool
	fullParseGSSConvergence             bool
	nativeResultCompatibility           gotreesitter.ResultCompatibilityCapability
	nativeUnaryWrapperFlattening        []nativeUnaryWrapperFlatteningProfile
	compactConvergedSplitDrops          bool
	compactEOFAcceptNoActionSiblings    bool
	compactPrimaryAcceptDerivation      bool
	compactAcceptanceStructuralElection bool
	compactMixedGSSMerge                bool
	compactLexerSkippedPrefixTiling     bool
	exactStackNodeEquivalence           bool
	compactPackedGSSVersionOrder        bool
	compactStrategy2ErrorRegion         bool
	compactRecoverEOFArtifact           gotreesitter.CompactRecoverEOFArtifactReceipt
	compactStackSummaryRecovery         bool
	compactMissingTokenInsertion        bool
	compactFaithfulS5Recovery           bool
	compactRecoveryTrailingRetirement   bool
	compactRecoveryErrorModeKeyword     bool
	compactRecoveryTerminalAliases      []compactRecoveryTerminalAliasProfile
	compactRecoveryPlainFirst           bool
	lineContinuationEscapeByte          byte
	conflictPolicies                    []gotreesitter.ConflictPolicy
}

type nativeUnaryWrapperFlatteningProfile struct {
	publicParent        string
	wrapper             string
	leaf                string
	wrapperPreGotoState gotreesitter.StateID
}

type compactRecoveryTerminalAliasProfile struct {
	resumeState  gotreesitter.StateID
	resumeSymbol string
	aliasSymbol  string
}

const (
	csharpAcceptedErrorRetryMaxEntryScratchPeak = 690_365
	csharpFreshErrorNoStacksRetryMaxStacks      = 16
	csharpGSSConvergenceErrorMergePerKey        = 12
	mesonAcceptedErrorRetryMinSourceBytes       = 2 * 1024
	vAcceptedErrorRetryMinSourceBytes           = 128 * 1024
	javascriptAutomaticForestMemoryAllowance    = 128 * 1024 * 1024
)

var builtinLanguageRuntimeProfiles = map[string]builtinLanguageRuntimeProfile{
	// The canonical compact corpus certifies Go's converged-path split drops
	// against the production parser and the tree-sitter C oracle.
	"go": {
		blobSHA256:                 mustRuntimeProfileSHA256("9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08"),
		compactConvergedSplitDrops: true,
	},
	// YAML's irreducible flow opener has one direct no-action EOF lineage whose
	// C result is the recover_eof ERROR root. Keep this gate independent from
	// S3, S4, and S5.
	"yaml": {
		blobSHA256: mustRuntimeProfileSHA256("9ac37a326be7a4f4297efa6e43ba00317c5959e7b4909c9262043d408f284b30"),
		compactRecoverEOFArtifact: gotreesitter.CompactRecoverEOFArtifactReceipt{
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
		},
	},
	"erlang": {
		blobSHA256:                   mustRuntimeProfileSHA256("355deb34ae4b9d8e0bf649c1c36096929d5e403107fa3c8b9c2ee82b138dfdc5"),
		compactConvergedSplitDrops:   true,
		compactPackedGSSVersionOrder: true,
	},
	// F# has one declaration-name state where C omits a same-span unary
	// long_identifier below long_identifier_or_op. Expression identifiers and
	// dotted long identifiers retain the wrapper.
	"fsharp": {
		blobSHA256: mustRuntimeProfileSHA256("409f32a1a287c9f2a385dc96bea03ed8b700bc9bfe92c50f91eed538519475ae"),
		nativeUnaryWrapperFlattening: []nativeUnaryWrapperFlatteningProfile{
			{
				publicParent:        "long_identifier_or_op",
				wrapper:             "long_identifier",
				leaf:                "identifier",
				wrapperPreGotoState: 4258,
			},
		},
	},
	// These exact artifacts previously needed the EOF sibling grant.
	// The authenticated metadata route now proves the locked-C election.
	"http": {
		blobSHA256: mustRuntimeProfileSHA256("332d50a15b3facb407f6c449fe8bbcd2fda55efffefbfd4d8d9ce2c75fbb7bda"),
	},
	"robot": {
		blobSHA256: mustRuntimeProfileSHA256("25075ecf5323eeb88af4f71b55f51867cef38a277aaa60f01b879ee8abb4c74f"),
	},
	// The internal DFA skips each JSDoc line decoration before it produces the
	// next tag token. The compact materializer admits the resulting interior
	// gap only when the accepted terminal carries that exact prefix evidence.
	"jsdoc": {
		blobSHA256:                      mustRuntimeProfileSHA256("e5688e60d41d6ef2d0922de7687b597de0c880761064e9e7b994d401c5508312"),
		compactLexerSkippedPrefixTiling: true,
	},
	// The compact Markdown inline route reduces three proved emphasis rows.
	// It starts an HTML tag through the sole shift in the fourth exact row.
	// All other repetition rows remain outside compact admission.
	"markdown_inline": {
		blobSHA256: mustRuntimeProfileSHA256("6a9064afbce4db62ab6cca8c143a9d3ae465e639a7ed81109eb65afd85469e0d"),
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{State: 18, Lookahead: 56, Kind: gotreesitter.ConflictPolicyRepetitionReduce, CompactOnly: true, CompactMinFrontierHeaders: 2, ReduceSymbols: []gotreesitter.Symbol{107}},
			{State: 18, Lookahead: 50, Kind: gotreesitter.ConflictPolicyRepetitionReduce, CompactOnly: true, CompactMinFrontierHeaders: 2, ReduceSymbols: []gotreesitter.Symbol{107}},
			{State: 18, Lookahead: 36, Kind: gotreesitter.ConflictPolicyRepetitionReduce, CompactOnly: true, CompactMinFrontierHeaders: 2, ReduceSymbols: []gotreesitter.Symbol{107}},
			{State: 209, Lookahead: 50, Kind: gotreesitter.ConflictPolicyShift, CompactOnly: true, ReduceSymbols: []gotreesitter.Symbol{104}},
		},
	},
	// C-oracle parity certifies the complete compact recovery competition for
	// this exact artifact against the ten pinned HTML witnesses.
	"html": {
		blobSHA256:                   mustRuntimeProfileSHA256("76d3d788ec44b5eaeaa0b3b0069bf52ffc4b125791059ff743301b9938dffd3d"),
		compactStrategy2ErrorRegion:  true,
		compactMissingTokenInsertion: true,
	},
	// Objective-C keeps parity-relevant alternatives below the bounded stack
	// comparison frontier. Exact comparison preserves them until generic result
	// selection chooses the C-equivalent alias-target shape.
	"objc": {
		blobSHA256:                mustRuntimeProfileSHA256("c04e841cf500f2b213d699681930a16f3c5f0e74e60f8a4488c36c38015bc09f"),
		exactStackNodeEquivalence: true,
	},
	// JavaScript's speculative automatic forest phase is bounded separately
	// from the production fallback. The exact locked corpus retained identical
	// trees and outcomes across all files while cutting aggregate allocations;
	// explicit forest parsing and non-certified languages keep the full budget.
	//
	// The same exact artifact certifies the complete pinned compact recovery
	// frontier. All eight witnesses route with exact C parity.
	"javascript": {
		blobSHA256:                        mustRuntimeProfileSHA256("6706f93890f24d8ea90d6a140df5dde29c02ec8a3213bae16e8cc4df37e33ee0"),
		automaticForestMemoryAllowance:    javascriptAutomaticForestMemoryAllowance,
		compactConvergedSplitDrops:        true,
		compactStrategy2ErrorRegion:       true,
		compactMissingTokenInsertion:      true,
		compactRecoveryTrailingRetirement: true,
		compactRecoveryErrorModeKeyword:   true,
		compactRecoveryTerminalAliases: []compactRecoveryTerminalAliasProfile{
			{resumeState: 20, resumeSymbol: "class", aliasSymbol: "property_identifier"},
			{resumeState: 231, resumeSymbol: "+", aliasSymbol: "property_identifier"},
			{resumeState: 1042, resumeSymbol: "_automatic_semicolon", aliasSymbol: "property_identifier"},
			{resumeState: 1367, resumeSymbol: "{", aliasSymbol: "property_identifier"},
		},
		compactRecoveryPlainFirst: true,
	},
	// The locked Scala artifact certifies S5 missing insertion through a
	// physical graph-head merge and a per-version lexer split. The profile
	// excludes stack-summary and EOF recovery, which remain separate packages.
	"scala": {
		blobSHA256:                          mustRuntimeProfileSHA256("8bc4a20f983ea8c8873c28430f089ba2bbbf00a995dd29f575bf2bc598d29dfa"),
		compactStrategy2ErrorRegion:         true,
		compactMissingTokenInsertion:        true,
		compactFaithfulS5Recovery:           true,
		compactPrimaryAcceptDerivation:      true,
		compactAcceptanceStructuralElection: true,
		compactRecoveryTrailingRetirement:   true,
		compactRecoveryErrorModeKeyword:     true,
	},
	// These scanner-backed grammars have certified the first retry ladder's
	// selected accepted-error tree as authoritative. Repeating the whole ladder
	// does not improve the selected tree and imposes a full additional parse.
	//
	// Dart also certifies its three profiled repeat-boundary folds (enum
	// bodies, extension bodies, and top-level declaration lists) against this
	// exact blob. The state and reduce-symbol checks preserve the retired
	// helper's scope without a linear scan over every lookahead at each state.
	"dart": {
		blobSHA256:                    mustRuntimeProfileSHA256("06bac15a9921a2e6af2810fb37ecb29a358b120e137345b9af5fb5f6c6632f59"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
		nativeResultCompatibility:     gotreesitter.ResultCompatibilityNativeCollapsedChildren,
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{State: 596, Lookahead: gotreesitter.ConflictPolicyAnyLookahead, Kind: gotreesitter.ConflictPolicyRepetitionShift, ReduceSymbols: []gotreesitter.Symbol{509}},
			{State: 602, Lookahead: gotreesitter.ConflictPolicyAnyLookahead, Kind: gotreesitter.ConflictPolicyRepetitionShift, ReduceSymbols: []gotreesitter.Symbol{512}},
			{State: 479, Lookahead: gotreesitter.ConflictPolicyAnyLookahead, Kind: gotreesitter.ConflictPolicyRepetitionShift, ReduceSymbols: []gotreesitter.Symbol{467}},
		},
	},
	// C#'s certified low-pressure accepted-error trees are authoritative. A
	// fresh no-stacks parse instead benefits from a bounded cap-16 retry; the
	// generic cap-48 ladder exceeds the large-file memory and time budgets.
	"c_sharp": {
		blobSHA256:                    mustRuntimeProfileSHA256("7ad425e89733339dde94e3c03b762ae478fb453b530493f5d62e1ae7537e1784"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
		fullParseGSSConvergence:       true,
		nativeResultCompatibility: gotreesitter.ResultCompatibilityCSharpNativeNotNull |
			gotreesitter.ResultCompatibilityCSharpNativeUnicodeIdentifiers |
			gotreesitter.ResultCompatibilityCSharpNativeScopedLambdaStatements |
			gotreesitter.ResultCompatibilityCSharpNativeScopedLambdaBlocks |
			gotreesitter.ResultCompatibilityCSharpNativeQueryExpressions |
			gotreesitter.ResultCompatibilityNativeRecoveredStructure,
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry:             true,
			SkipCompleteMaxEntryScratchPeak:            csharpAcceptedErrorRetryMaxEntryScratchPeak,
			FreshErrorNoStacksRetryMaxStacks:           csharpFreshErrorNoStacksRetryMaxStacks,
			SkipInitialCompleteAcceptedErrorMergeRetry: true,
			GSSConvergenceAcceptedErrorMergePerKey:     csharpGSSConvergenceErrorMergePerKey,
		},
	},
	// Crystal's external-scanner repeat selects the same tree after the complete
	// accepted-error retry ladder. Keep the full ladder, but do not run it twice
	// for this exact built-in grammar/scanner artifact.
	"crystal": {
		blobSHA256:                    mustRuntimeProfileSHA256("e906c8fec1d2ef49d7dbb349a9ed39fb894f7d8ae0024b2ee45a9956f050bd69"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
	},
	"cue": {
		blobSHA256:                mustRuntimeProfileSHA256("3ff52c09bc788d87116ec56070c4733829513c3d3beadc9028ea4ef7b1d3609a"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	"gitcommit": {
		blobSHA256:                mustRuntimeProfileSHA256("739548172c738dfa86c6f4c3177009ba20e5d3c48f5669d5fab478de1db5cca4"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	// Haxe's accepted-error retry ladder selects the same tree on every pass.
	// Keep the first accepted result instead of running either retry ladder.
	"haxe": {
		blobSHA256: mustRuntimeProfileSHA256("eb39b273148a394f792b322cd30b5483fd6f8ca915b7e15835de4d6482b5a4a7"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	// Large V files can reuse the clean wide pass at the recovery-wide slot.
	// Keep the narrow merge pass because it changes some final V trees.
	"v": {
		blobSHA256: mustRuntimeProfileSHA256("f5dc2cd74426384557116c37f01c95174d516c25c9f9f3d88bc49beb7d9839a7"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			ReuseCleanWideForWideRetry:   true,
			ReuseCleanWideMinSourceBytes: vAcceptedErrorRetryMinSourceBytes,
		},
	},
	// Kotlin certifies CompactPrimaryAcceptanceDerivationCertified only.
	// selectCompactAcceptanceDerivation's materiality gate
	// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) is
	// what makes that grant safe: the object_declaration tied election
	// (object Singleton { fun work() = Unit }, issue #93) declines instead
	// of publishing an unproven positional primary. Full-corpus field-aware
	// C-oracle verification finds zero unadjudicated divergence under that
	// gate for this mechanism alone (A3 certification workstream,
	// spec.campaign.v7; cgo_harness/kotlin_a3_certification_sweep_test.go).
	//
	// CompactConvergedReductionSplitDropsCertified stays withheld. Review
	// found a compact-only divergence class the certification sweep's
	// 3-file real corpus did not surface: an annotated extension property
	// with a getter, followed by a trailing comment (line or block), for
	// example
	//
	//	@Deprecated("old")
	//	val Int.double: Int
	//	    get() = this * 2
	//	// trailing
	//
	// Production is C-exact on this shape. With split-drops alone forced
	// on, the compact route accepts a C-divergent tree: the annotation is
	// torn into a bare prefix_expression and the getter becomes an
	// assignment (first divergence at /source_file, child count 4 vs the
	// C oracle's 3). Primary-acceptance-derivation alone is clean on the
	// same witness: it declines the converged-path split, falling back to
	// production's correct tree. See
	// kotlinA3AdversarialSources's annotated_extension_property_getter_*
	// entries for the pinned regression sweep coverage.
	//
	// A net fidelity ledger over 1,061 truncations of the Kotlin real
	// corpus found split-drops regresses more than it improves (2 improved,
	// 5 regressed) against a clean primary-accept-only baseline (0
	// improved, 0 regressed). The two witnesses split-drops alone would
	// otherwise fix -- the platform-modifier-recovery witness (internal
	// actual fun f(): String = "x") and small__TimeZoneNative.kt -- both
	// decline under primary-accept-only and fall back to production's own
	// (C-divergent, issue #93-adjacent) tree; that reversion to the
	// pre-certification status quo is the accepted, measured cost of
	// withholding this grant. The derivation-selection defect in the
	// converged-path split mechanism itself needs its own repair lane
	// before this grant is reconsidered.
	"kotlin": {
		blobSHA256:                     mustRuntimeProfileSHA256("643a3e6b60d07846dd972849b612159ff9bf09734b09fb00013229c8593a8c78"),
		externalScannerFullParseRetry:  gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
		nativeResultCompatibility:      gotreesitter.ResultCompatibilityNativeCollapsedChildren,
		compactPrimaryAcceptDerivation: true,
	},
	// Apex's tied class-literal election matches the C oracle once the
	// compact route selects the sole primary derivation; the compact tree is
	// strictly more faithful than production plus the compat arm here. Apex
	// has no converged-path split-drop shape, so it does not certify that
	// mechanism. Full-corpus field-aware C-oracle verification certifies this
	// exact blob (A3 certification workstream, spec.campaign.v7).
	"apex": {
		blobSHA256:                     mustRuntimeProfileSHA256("69fc1b577f1f783a204c98719d55d2f15f329d296b9e227d651056ce878c1bd2"),
		nativeResultCompatibility:      gotreesitter.ResultCompatibilityNativeCollapsedChildren,
		compactPrimaryAcceptDerivation: true,
	},
	"elixir": {
		blobSHA256:                mustRuntimeProfileSHA256("9889f5f6704ea87f357c8d65ef3194d88fb5865922b45767fe4df0f2eda7e3f0"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	"hack": {
		blobSHA256:                mustRuntimeProfileSHA256("f7388868d68644eff2ef6aa3dee5d0da1bc4f926ef4a8b04f98274bd471df3e6"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	"ruby": {
		blobSHA256:                mustRuntimeProfileSHA256("9f1dc301142506249e7ac340372671f1d5e9ae76b7d378fc049635259bf8fc7f"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	"rust": {
		blobSHA256:                mustRuntimeProfileSHA256("1f00617f5a6cb9106bb3739d6ab8c592772b87b20d232adff9faf1552fa396fd"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	"r": {
		blobSHA256:                mustRuntimeProfileSHA256("b09226c9eae0afc795d22e3dbed168118554d05b7ab80113bf06bb4315b90c4d"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	// Matlab's external-scanner repeat selects the same tree after the complete
	// accepted-error retry ladder. Keep the full ladder, but do not run it twice
	// for this exact built-in grammar/scanner artifact.
	"matlab": {
		blobSHA256:                    mustRuntimeProfileSHA256("ff3220ac992d281de156b9bd90e0a04e7f8d7015feaf6c356fdb973f15bb434e"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
	},
	// Odin's accepted-error retry ladder selects the same tree on every pass,
	// and its locked large test-vector witness certifies the ASCII full-arena
	// density cap. Both policies remain bound to this exact blob identity.
	"odin": {
		blobSHA256:               mustRuntimeProfileSHA256("9b376bcbbe677780b9031ae84eee4fb59eb37a14fbe169c7c17d35f2b5b776ed"),
		fullParseArenaDensityCap: true,
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	// Python's tuple-assignment and f-string interpolation ties match the C
	// oracle when the compact route applies C's raw subtree ordering. The
	// selection is clean-only and has no recovery authority. Full-corpus,
	// field-aware C-oracle verification certifies this exact blob for the
	// structural election and the existing converged-path split-drop and
	// primary-acceptance mechanisms (A3 certification workstream,
	// spec.campaign.v7). The mixed flat/GSS receiver path is also certified
	// for this exact artifact.
	"python": {
		blobSHA256:                          mustRuntimeProfileSHA256("cde4a67dc6af6e1232dbbd1eab8618478d1d73727020e8a8002542390a452d37"),
		externalScannerFullParseRetry:       gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
		compactConvergedSplitDrops:          true,
		compactPrimaryAcceptDerivation:      true,
		compactAcceptanceStructuralElection: true,
		compactMixedGSSMerge:                true,
	},
	// Perl's tied push-list election matches the C oracle once the compact
	// route accepts after a converged-path split drop and selects the sole
	// primary derivation. Full-corpus field-aware C-oracle verification
	// certifies both mechanisms for this exact blob (A3 certification
	// workstream, spec.campaign.v7).
	"perl": {
		blobSHA256:                     mustRuntimeProfileSHA256("22388f06c2c54bb4748fd5f5f682ed25eecff8115a7e8e6a98f94f9c94bb9820"),
		compactConvergedSplitDrops:     true,
		compactPrimaryAcceptDerivation: true,
	},
	// Ada's tied aggregate elections (positional-array and others-choice)
	// match the C oracle once the compact route accepts after a
	// converged-path split drop and selects the sole primary derivation.
	// Full-corpus field-aware C-oracle verification certifies both
	// mechanisms for this exact blob (A3 certification workstream,
	// spec.campaign.v7).
	//
	// Ada's grammar declares conflicts: [[$.component_choice_list,
	// $.discrete_choice], ...] (tree-sitter-ada grammar.js): a bare
	// "others" choice reduces the shared token as either component_choice_list
	// (162... record-aggregate reading, RM 4.3.1) or discrete_choice
	// (array-aggregate reading, RM 3.8.1), both plain REDUCE actions with no
	// dynamic precedence on either alternative. The locked C oracle keeps
	// the discrete_choice reading; this row-scoped fold (state/lookahead
	// wildcarded, matched only by the declared-conflict symbol pair,
	// component_choice_list=195, discrete_choice=255 in this blob) reproduces
	// that natively, the same mechanism ql's signatureExpr election uses.
	"ada": {
		blobSHA256:                     mustRuntimeProfileSHA256("32f2dd8f0053ffb7e6b7014f6ff2eb7025287c0d5fcdab6ce1f6a694c2d8899e"),
		compactConvergedSplitDrops:     true,
		compactPrimaryAcceptDerivation: true,
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{
				State:         gotreesitter.ConflictPolicyAnyState,
				Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
				Kind:          gotreesitter.ConflictPolicyDeclaredReduceReduceHighestSymbol,
				ReduceSymbols: []gotreesitter.Symbol{195, 255},
			},
		},
	},
	// Swift's low-pressure accepted-error parses select the same tree across
	// the retry ladder. High-pressure parses still benefit from the first
	// ladder, while repeating that ladder for the external scanner does not.
	"swift": {
		blobSHA256:                    mustRuntimeProfileSHA256("be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry:  true,
			SkipCompleteMaxEntryScratchPeak: 3 * 64 * 1024,
		},
	},
	// Large ASM accepted-error parses improve during the same-stack merge
	// retry, but later widened-stack passes do not advance the selected tree.
	// Keep that first retry while avoiding the redundant wider passes.
	"asm": {
		blobSHA256: mustRuntimeProfileSHA256("7001e89cc1c597efce3143c011d39a40855067fb06863b738d2c4d7e595fb71d"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			MinSourceBytes:      11 * 1024,
			InitialStackCeiling: 8,
		},
	},
	// These grammars have error-bearing real-corpus witnesses that legitimately
	// reach EOF. Re-running the accepted-error ladder does not improve their
	// selected trees, so the exact certified blobs keep the first result.
	"bash": {
		blobSHA256:                 mustRuntimeProfileSHA256("a3e898c88f6ad918d4d619dff2a4e74d613bda93c90e4a3f9fb7587c1952f3fb"),
		compactConvergedSplitDrops: true,
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	"caddy": {
		blobSHA256: mustRuntimeProfileSHA256("e1af0dcba90bca6949ac1a2756e1a6db2271061b40570b9a7fa2ada29478f6fa"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	"cpp": {
		blobSHA256: mustRuntimeProfileSHA256("d351f902c8f2ca85257a9296d3c9991862d57701ac6e9006e386ae173fd35178"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	// On the locked large accepted-error witnesses, the non-recovery and
	// recovery-enabled widened-stack passes produce the same complete tree and
	// semantic runtime state. Retain the first result instead of parsing it
	// again; exact blob identity keeps adapted and future grammars conservative.
	"enforce": {
		blobSHA256: mustRuntimeProfileSHA256("9ddb5d9b74c06eb3f3f9eba98be968719eff298d86ee9aa046009ed0a868b676"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			ReuseCleanWideForWideRetry:   true,
			ReuseCleanWideMinSourceBytes: 128 * 1024,
		},
	},
	"kdl": {
		blobSHA256:             mustRuntimeProfileSHA256("ef6d000123c053eddebd200a1cbd44d6df5dcab7c4b3d34ae18acdf2f14989f5"),
		automaticForestEnabled: true,
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	// These exact built-in artifacts have authenticated corpus receipts with no
	// forest/C-oracle divergence on accepted forest files, no
	// routed/production divergence on any file, and an aggregate route wall-time
	// win after conservative production fallbacks.
	"awk": {
		blobSHA256:             mustRuntimeProfileSHA256("925312ca0bc6e279602402c64700b8198c55ed949ac967ce92bae40f7f21cedf"),
		automaticForestEnabled: true,
	},
	"uxntal": {
		blobSHA256:             mustRuntimeProfileSHA256("cca71a0e6385fd9b2791eb6fefc1fe493f93eb2bf58e903f8989096884c31fe4"),
		automaticForestEnabled: true,
	},
	// Meson's tied smoke election compares variableunit with var_unit. The
	// locked C runtime selects variableunit through its raw subtree comparator.
	// The retry ladder still changes selected trees on small error-bearing files.
	"meson": {
		blobSHA256:                          mustRuntimeProfileSHA256("b3b7e74bcd35614419f5359c31eb8a05bd58c0b97529f133f2aea2f40796789d"),
		compactPrimaryAcceptDerivation:      true,
		compactAcceptanceStructuralElection: true,
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
			SkipCompleteMinSourceBytes:     mesonAcceptedErrorRetryMinSourceBytes,
		},
	},
	"rego": {
		blobSHA256: mustRuntimeProfileSHA256("b10816c87dc847492fbbc1fd97c5096ed35d7abe69d0cd2ef5dd7e02aabac25c"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	"scss": {
		blobSHA256: mustRuntimeProfileSHA256("0646d27248a96d865a717a2a020ede70762b8a0542fac32a316b34248af9a50e"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	// Tcl's complete accepted-error trees and first error-bearing no-stacks
	// retry are authoritative for the certified corpus. Later widened passes
	// repeat the same failure without advancing the selected tree.
	"tcl": {
		blobSHA256: mustRuntimeProfileSHA256("4c331e38860001c18b737f6be508f4b09f230c4b9ff95f4b4d12bdb00c176ad7"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
			FreshErrorNoStacksMaxPasses:    1,
		},
	},
	// Haskell has three exact repeat rows where C selects the reduce arm.
	// Retaining both arms grows new graph-structured stack frontiers.
	"haskell": {
		blobSHA256:                 mustRuntimeProfileSHA256("fcfc8794bca4442ebf5688d88e2397c78a22c8f0b585c4e1b868986cfa52dd09"),
		compactConvergedSplitDrops: true,
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{
				State:         9609,
				Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
				Kind:          gotreesitter.ConflictPolicyRepetitionReduce,
				ReduceSymbols: []gotreesitter.Symbol{518},
			},
			{
				State:         10984,
				Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
				Kind:          gotreesitter.ConflictPolicyRepetitionReduce,
				ReduceSymbols: []gotreesitter.Symbol{516},
			},
			{
				State:         11192,
				Lookahead:     4,
				Kind:          gotreesitter.ConflictPolicyRepetitionReduce,
				ReduceSymbols: []gotreesitter.Symbol{500},
			},
		},
	},
	// On large accepted-error Java sources, the cap-16 same-stack merge retry
	// remains authoritative; the subsequent cap-64 clean/recovery passes do not
	// improve the selected tree. Keep this bound pinned to the exact built-in
	// blob so overrides and caller-built languages retain the generic ladder.
	"java": {
		blobSHA256: mustRuntimeProfileSHA256("530c7257b13e1ce356edd251cac347b5e41f04f74343473c72f43bf1177ffa9c"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			MinSourceBytes:      64 * 1024,
			InitialStackCeiling: 14,
		},
	},
	// Large Groovy and D accepted-error parses stay inside their certified
	// initial stack ceilings. Widening does not improve the selected tree and
	// reintroduces their real-corpus time/RSS cliffs. The generic profile gate
	// keeps incremental fallbacks and explicit diagnostic overrides
	// conservative.
	"groovy": {
		blobSHA256: mustRuntimeProfileSHA256("a1d1bb30d9971f1c3d645aab456521943a5f0da419b57a0986fc9b2a502a90d9"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			MinSourceBytes:      64 * 1024,
			InitialStackCeiling: 2,
		},
	},
	"d": {
		blobSHA256: mustRuntimeProfileSHA256("1e2bf6c9d37193dad050a3e2f35d450973245dbee60550ef6cc24fca2b0e0016"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			MinSourceBytes:      64 * 1024,
			InitialStackCeiling: 3,
		},
	},
	// NOTE on dot: no certified row here (unlike gomod/dart/c below) — dot's
	// retired dotRepetitionShiftConflictChoice is NOT migrated. dot isn't in
	// cRepetitionSkipOptOut, so the engine-wide C repetition-skip fold
	// already folded its stmt_list_repeat1 boundary (state 4) with a flat
	// parse stack; the retired helper's only call site was forest-only and
	// dot isn't in builtinForestDefaults, so it never actually ran for this
	// blob. A/B validation found identical accepted trees, but
	// reviving the helper's shift preference as a policy grows the LR stack
	// O(n) with statement count (406 deep on a 400-statement file vs 8 for
	// the fold already in place) for no fork-count benefit (both already
	// MaxStacksSeen=1). Retired outright instead of migrated.
	//
	// go.mod's require-list continuation forks at two states: the top-level
	// source_file list (state 3) and a parenthesized require block (state
	// 37). Replaces gomodRepetitionShiftConflictChoice.
	// Unlike dot, this is not a revival: the retired helper's dispatch call
	// site (conflict_policy.go's gomod special case, still present) ran
	// unconditionally ahead of the reuse gate and the engine-wide fold, so
	// this exact preference was already the live, shipped behavior —
	// confirmed byte-identical MaxStacksSeen/PeakStackDepth before and after
	// this migration on a synthetic 400-require go.mod.
	"gomod": {
		blobSHA256: mustRuntimeProfileSHA256("e7dca79c1b4655caeee59c9bea3befdd199dd568e2e17640e2ff93832839d2c2"),
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{State: 3, Lookahead: gotreesitter.ConflictPolicyAnyLookahead, Kind: gotreesitter.ConflictPolicyRepetitionShift, ReduceSymbols: []gotreesitter.Symbol{50}},
			{State: 37, Lookahead: gotreesitter.ConflictPolicyAnyLookahead, Kind: gotreesitter.ConflictPolicyRepetitionShift, ReduceSymbols: []gotreesitter.Symbol{52}},
		},
	},
	// C's top-level declaration list (translation_unit_repeat1) and
	// preprocessor-conditional body (preproc_if_repeat1) close only on
	// terminators with no continuation shift (EOF; #endif/#elif/#else), so
	// any lookahead offering a continuation shift makes the competing reduce
	// a zero-progress dead end. That C-faithful rule is scoped by reduce
	// symbol identity alone, not by table position: it recurs at hundreds of
	// (state, lookahead) rows across the grammar (a scan of this exact blob
	// found 3242, across 446 states), which is why this is the one certified
	// profile using the ConflictPolicyAnyState/AnyLookahead wildcards instead
	// of an enumerated per-row table. case_statement_repeat1 is deliberately
	// excluded: its list boundary is load-bearing, not a dead-end. Replaces
	// cRepetitionShiftConflictChoice; byte-for-byte C
	// parity held by TestParityCTopLevelDeclAmbiguity /
	// TestParityCPreprocConditional.
	"c": {
		blobSHA256: mustRuntimeProfileSHA256("9aee42825fd1446ce5b754951db26edadcdba5d2f26b61578a30e87ed2dbbd3c"),
		// Measurements on large C witnesses show identical trees before and after
		// the retry ladder. Keep the initial tree and avoid repeated full parses
		// for this exact grammar blob.
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{
				State:         gotreesitter.ConflictPolicyAnyState,
				Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
				Kind:          gotreesitter.ConflictPolicyRepetitionShift,
				ReduceSymbols: []gotreesitter.Symbol{324, 326},
			},
			// Fold aux_sym_enumerator_list_repeat1 at its conflict cell instead
			// of forking. Upstream never executes SHIFT_REPEAT at a conflict cell
			// (lib/src/parser.c, `if (action.shift.repetition) break;`): it
			// reduces, renumbers, and re-dispatches the same lookahead. C is
			// opted out of the engine-wide equivalent (cRepetitionSkipOptOut in
			// conflict_policy.go), so an enumerator list forks and then depends
			// on cap-one convergence. When one branch is in demoted-linear form
			// the GSS merge refuses, the depth tiebreak keeps the unfolded
			// branch, and a three-enumerator list dead-ends on `}` and invents a
			// comma. Folding this one symbol restores the upstream order.
			{
				State:         gotreesitter.ConflictPolicyAnyState,
				Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
				Kind:          gotreesitter.ConflictPolicyRepetitionReduce,
				ReduceSymbols: []gotreesitter.Symbol{343},
			},
		},
	},
	// QL declares conflicts: [[$.simpleId, $.className], ...]. An upper-case
	// identifier in signature position (`implements Foo`, `implements
	// M::Foo`) reduces the shared _upper_id token as either simpleId (162,
	// building moduleExpr) or className (163, building typeExpr); both are
	// live GLR alternatives of signatureExpr's choice(typeExpr, moduleExpr,
	// predicateExpr) with no dynamic precedence on either production. The
	// locked C oracle (tree-sitter-ql 1fd627a4e8bff8c24c11987474bd33112bead857)
	// always keeps the className/typeExpr reading; this row-scoped fold
	// (state/lookahead wildcarded, matched only by the declared-conflict
	// symbol pair) reproduces that natively instead of relying on a
	// post-parse tree rewrite.
	"ql": {
		blobSHA256: mustRuntimeProfileSHA256("f9e092262139d482fef46ef6274a2ac90f9344ea13766d6528819a4e21e7cc84"),
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{
				State:         gotreesitter.ConflictPolicyAnyState,
				Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
				Kind:          gotreesitter.ConflictPolicyDeclaredReduceReduceHighestSymbol,
				ReduceSymbols: []gotreesitter.Symbol{162, 163},
			},
		},
	},
	// PowerShell's backtick immediately followed by a newline is the
	// language's line-continuation escape: the C reference scanner consumes
	// it as ordinary skipped trivia (zero ERROR nodes across the sequence,
	// two children on the enclosing command_argument_sep — the same shape as
	// any other run of whitespace), never as a token of its own. Declaring it
	// here lets bytesAreParserPadding give it the same unconditional
	// treatment already applied to backslash+newline instead of falling
	// through to skipped-gap ERROR materialization. C-oracle verified on the
	// enclosing command_argument_sep shape (TestPowerShellBacktickContinuationIsParserPadding).
	"powershell": {
		blobSHA256:                 mustRuntimeProfileSHA256("8c7a2b47a39efb590cde7f75c9a1135c6423bc07b13b9604f1fa9f0061231687"),
		lineContinuationEscapeByte: '`',
	},
}

func mustRuntimeProfileSHA256(raw string) (sum [32]byte) {
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != len(sum) {
		panic("invalid built-in runtime-profile SHA-256")
	}
	copy(sum[:], decoded)
	return sum
}

func attachBuiltinLanguageRuntimeProfile(name string, blobSHA256 [32]byte, lang *gotreesitter.Language) bool {
	if lang == nil {
		return false
	}
	profile, ok := builtinLanguageRuntimeProfiles[canonicalLanguageName(name)]
	if !ok || blobSHA256 != profile.blobSHA256 {
		return false
	}
	changed := false
	if profile.externalScannerFullParseRetry != gotreesitter.ExternalScannerFullParseRetryDefault &&
		lang.ExternalScanner != nil &&
		lang.ExternalScannerFullParseRetryPolicy != profile.externalScannerFullParseRetry {
		lang.ExternalScannerFullParseRetryPolicy = profile.externalScannerFullParseRetry
		changed = true
	}
	if profile.fullParseAcceptedErrorRetryProfile != (gotreesitter.FullParseAcceptedErrorRetryProfile{}) &&
		lang.FullParseAcceptedErrorRetryProfile != profile.fullParseAcceptedErrorRetryProfile {
		lang.FullParseAcceptedErrorRetryProfile = profile.fullParseAcceptedErrorRetryProfile
		changed = true
	}
	if profile.automaticForestMemoryAllowance > 0 &&
		lang.AutomaticForestMemoryAllowanceBytes != profile.automaticForestMemoryAllowance {
		lang.AutomaticForestMemoryAllowanceBytes = profile.automaticForestMemoryAllowance
		changed = true
	}
	if profile.automaticForestEnabled && !lang.AutomaticForestEnabledByDefault {
		lang.AutomaticForestEnabledByDefault = true
		changed = true
	}
	if profile.fullParseArenaDensityCap && !lang.FullParseArenaDensityCapEnabled {
		lang.FullParseArenaDensityCapEnabled = true
		changed = true
	}
	if profile.fullParseGSSConvergence && !lang.FullParseGSSConvergenceEnabled {
		lang.FullParseGSSConvergenceEnabled = true
		changed = true
	}
	if missing := profile.nativeResultCompatibility &^ lang.NativeResultCompatibility; missing != 0 {
		lang.NativeResultCompatibility |= missing
		changed = true
	}
	if rules := resolveNativeUnaryWrapperFlatteningProfile(lang, profile.nativeUnaryWrapperFlattening); len(rules) > 0 &&
		!slices.Equal(lang.NativeUnaryWrapperFlattening, rules) {
		lang.NativeUnaryWrapperFlattening = rules
		changed = true
	}
	if profile.compactConvergedSplitDrops && !lang.CompactConvergedReductionSplitDropsCertified {
		lang.CompactConvergedReductionSplitDropsCertified = true
		changed = true
	}
	if profile.compactEOFAcceptNoActionSiblings && !lang.CompactEOFAcceptNoActionSiblingsCertified {
		lang.CompactEOFAcceptNoActionSiblingsCertified = true
		changed = true
	}
	if profile.compactPrimaryAcceptDerivation && !lang.CompactPrimaryAcceptanceDerivationCertified {
		lang.CompactPrimaryAcceptanceDerivationCertified = true
		changed = true
	}
	if profile.compactAcceptanceStructuralElection && !lang.CompactAcceptanceStructuralElectionCertified {
		lang.CompactAcceptanceStructuralElectionCertified = true
		changed = true
	}
	if profile.compactMixedGSSMerge && !lang.CompactMixedGSSMergeCertified {
		lang.CompactMixedGSSMergeCertified = true
		changed = true
	}
	if profile.compactLexerSkippedPrefixTiling && !lang.CompactLexerSkippedPrefixTilingCertified {
		lang.CompactLexerSkippedPrefixTilingCertified = true
		changed = true
	}
	if profile.exactStackNodeEquivalence && !lang.ExactStackNodeEquivalenceCertified {
		lang.ExactStackNodeEquivalenceCertified = true
		changed = true
	}
	if profile.compactPackedGSSVersionOrder && !lang.CompactPackedGSSVersionOrderCertified {
		lang.CompactPackedGSSVersionOrderCertified = true
		changed = true
	}
	if profile.compactStrategy2ErrorRegion && !lang.CompactStrategy2ErrorRegionCertified {
		lang.CompactStrategy2ErrorRegionCertified = true
		changed = true
	}
	if profile.compactRecoverEOFArtifact.BlobSHA256 != ([32]byte{}) {
		if lang.CompactRecoverEOFArtifactReceipt != profile.compactRecoverEOFArtifact {
			lang.CompactRecoverEOFArtifactReceipt = profile.compactRecoverEOFArtifact
			changed = true
		}
		if !lang.CompactRecoverEOFCertified {
			lang.CompactRecoverEOFCertified = true
			changed = true
		}
	}
	if profile.compactStackSummaryRecovery && !lang.CompactStackSummaryRecoveryCertified {
		lang.CompactStackSummaryRecoveryCertified = true
		changed = true
	}
	if profile.compactMissingTokenInsertion && !lang.CompactMissingTokenInsertionCertified {
		lang.CompactMissingTokenInsertionCertified = true
		changed = true
	}
	if profile.compactFaithfulS5Recovery && !lang.CompactFaithfulS5RecoveryCertified {
		lang.CompactFaithfulS5RecoveryCertified = true
		changed = true
	}
	if profile.compactRecoveryTrailingRetirement && !lang.CompactRecoveryTrailingLineageRetirementCertified {
		lang.CompactRecoveryTrailingLineageRetirementCertified = true
		changed = true
	}
	if profile.compactRecoveryErrorModeKeyword && !lang.CompactRecoveryErrorModeKeywordCaptureCertified {
		lang.CompactRecoveryErrorModeKeywordCaptureCertified = true
		changed = true
	}
	if rules := resolveCompactRecoveryTerminalAliasProfile(lang, profile.compactRecoveryTerminalAliases); len(rules) > 0 &&
		!slices.Equal(lang.CompactRecoveryTerminalAliasRules, rules) {
		lang.CompactRecoveryTerminalAliasRules = rules
		changed = true
	}
	if profile.compactRecoveryPlainFirst && !lang.CompactRecoveryPlainFirstCertified {
		lang.CompactRecoveryPlainFirstCertified = true
		changed = true
	}
	if profile.lineContinuationEscapeByte != 0 && lang.LineContinuationEscapeByte != profile.lineContinuationEscapeByte {
		lang.LineContinuationEscapeByte = profile.lineContinuationEscapeByte
		changed = true
	}
	for _, policy := range profile.conflictPolicies {
		if languageHasConflictPolicy(lang, policy) {
			continue
		}
		policy.ReduceSymbols = append([]gotreesitter.Symbol(nil), policy.ReduceSymbols...)
		lang.ConflictPolicies = append(lang.ConflictPolicies, policy)
		changed = true
	}
	return changed
}

func resolveNativeUnaryWrapperFlatteningProfile(
	lang *gotreesitter.Language,
	profile []nativeUnaryWrapperFlatteningProfile,
) []gotreesitter.UnaryWrapperFlatteningRule {
	if lang == nil || len(profile) == 0 {
		return nil
	}
	rules := make([]gotreesitter.UnaryWrapperFlatteningRule, 0, len(profile))
	for _, candidate := range profile {
		parent, parentOK := runtimeProfileNamedSymbol(lang, candidate.publicParent)
		wrapper, wrapperOK := runtimeProfileNamedSymbol(lang, candidate.wrapper)
		leaf, leafOK := runtimeProfileNamedSymbol(lang, candidate.leaf)
		if !parentOK || !wrapperOK || !leafOK {
			continue
		}
		rules = append(rules, gotreesitter.UnaryWrapperFlatteningRule{
			PublicParent:        parent,
			Wrapper:             wrapper,
			Leaf:                leaf,
			WrapperPreGotoState: candidate.wrapperPreGotoState,
		})
	}
	return rules
}

func runtimeProfileNamedSymbol(lang *gotreesitter.Language, name string) (gotreesitter.Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	for index, symbolName := range lang.SymbolNames {
		if symbolName == name &&
			index < len(lang.SymbolMetadata) &&
			lang.SymbolMetadata[index].Visible &&
			lang.SymbolMetadata[index].Named {
			return gotreesitter.Symbol(index), true
		}
	}
	return 0, false
}

func resolveCompactRecoveryTerminalAliasProfile(
	lang *gotreesitter.Language,
	profile []compactRecoveryTerminalAliasProfile,
) []gotreesitter.CompactRecoveryTerminalAliasRule {
	if lang == nil || len(profile) == 0 {
		return nil
	}
	rules := make([]gotreesitter.CompactRecoveryTerminalAliasRule, 0, len(profile))
	for _, candidate := range profile {
		resumeSymbol, resumeOK := runtimeProfileTerminalSymbol(lang, candidate.resumeSymbol)
		aliasSymbol, aliasOK := runtimeProfileSymbol(lang, candidate.aliasSymbol)
		if !resumeOK || !aliasOK || uint32(candidate.resumeState) >= lang.StateCount ||
			uint32(resumeSymbol) >= lang.TokenCount || uint32(aliasSymbol) < lang.TokenCount {
			return nil
		}
		rules = append(rules, gotreesitter.CompactRecoveryTerminalAliasRule{
			ResumeState:  candidate.resumeState,
			ResumeSymbol: resumeSymbol,
			AliasSymbol:  aliasSymbol,
		})
	}
	return rules
}

func runtimeProfileTerminalSymbol(lang *gotreesitter.Language, name string) (gotreesitter.Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	limit := len(lang.SymbolNames)
	if uint64(lang.TokenCount) < uint64(limit) {
		limit = int(lang.TokenCount)
	}
	var match gotreesitter.Symbol
	found := false
	for index := 0; index < limit; index++ {
		if lang.SymbolNames[index] != name {
			continue
		}
		if found {
			return 0, false
		}
		match = gotreesitter.Symbol(index)
		found = true
	}
	return match, found
}

func runtimeProfileSymbol(lang *gotreesitter.Language, name string) (gotreesitter.Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	var match gotreesitter.Symbol
	found := false
	for index, symbolName := range lang.SymbolNames {
		if symbolName != name {
			continue
		}
		if found {
			return 0, false
		}
		match = gotreesitter.Symbol(index)
		found = true
	}
	return match, found
}

func languageHasConflictPolicy(lang *gotreesitter.Language, want gotreesitter.ConflictPolicy) bool {
	if lang == nil {
		return false
	}
	for _, policy := range lang.ConflictPolicies {
		if policy.State == want.State &&
			policy.Lookahead == want.Lookahead &&
			policy.Kind == want.Kind &&
			slices.Equal(policy.ReduceSymbols, want.ReduceSymbols) {
			return true
		}
	}
	return false
}
