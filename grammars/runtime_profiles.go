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
	blobSHA256                         [32]byte
	externalScannerFullParseRetry      gotreesitter.ExternalScannerFullParseRetryPolicy
	fullParseAcceptedErrorRetryProfile gotreesitter.FullParseAcceptedErrorRetryProfile
	automaticForestMemoryAllowance     int64
	automaticForestEnabled             bool
	fullParseArenaDensityCap           bool
	nativeResultCompatibility          gotreesitter.ResultCompatibilityCapability
	conflictPolicies                   []gotreesitter.ConflictPolicy
}

const (
	csharpAcceptedErrorRetryMaxEntryScratchPeak = 690_365
	csharpFreshErrorNoStacksRetryMaxStacks      = 16
	mesonAcceptedErrorRetryMinSourceBytes       = 2 * 1024
	javascriptAutomaticForestMemoryAllowance    = 128 * 1024 * 1024
)

var builtinLanguageRuntimeProfiles = map[string]builtinLanguageRuntimeProfile{
	// JavaScript's speculative automatic forest phase is bounded separately
	// from the production fallback. The exact locked corpus retained identical
	// trees and outcomes across all files while cutting aggregate allocations;
	// explicit forest parsing and non-certified languages keep the full budget.
	"javascript": {
		blobSHA256:                     mustRuntimeProfileSHA256("6706f93890f24d8ea90d6a140df5dde29c02ec8a3213bae16e8cc4df37e33ee0"),
		automaticForestMemoryAllowance: javascriptAutomaticForestMemoryAllowance,
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
		nativeResultCompatibility: gotreesitter.ResultCompatibilityCSharpNativeNotNull |
			gotreesitter.ResultCompatibilityCSharpNativeUnicodeIdentifiers |
			gotreesitter.ResultCompatibilityCSharpNativeScopedLambdaStatements |
			gotreesitter.ResultCompatibilityCSharpNativeScopedLambdaBlocks |
			gotreesitter.ResultCompatibilityCSharpNativeQueryExpressions,
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry:             true,
			SkipCompleteMaxEntryScratchPeak:            csharpAcceptedErrorRetryMaxEntryScratchPeak,
			FreshErrorNoStacksRetryMaxStacks:           csharpFreshErrorNoStacksRetryMaxStacks,
			SkipInitialCompleteAcceptedErrorMergeRetry: true,
		},
	},
	// Crystal's external-scanner repeat selects the same tree after the complete
	// accepted-error retry ladder. Keep the full ladder, but do not run it twice
	// for this exact built-in grammar/scanner artifact.
	"crystal": {
		blobSHA256:                    mustRuntimeProfileSHA256("e906c8fec1d2ef49d7dbb349a9ed39fb894f7d8ae0024b2ee45a9956f050bd69"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
	},
	// Haxe's accepted-error retry ladder selects the same tree on every pass.
	// Keep the first accepted result instead of running either retry ladder.
	"haxe": {
		blobSHA256: mustRuntimeProfileSHA256("eb39b273148a394f792b322cd30b5483fd6f8ca915b7e15835de4d6482b5a4a7"),
		fullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
		},
	},
	"kotlin": {
		blobSHA256:                    mustRuntimeProfileSHA256("643a3e6b60d07846dd972849b612159ff9bf09734b09fb00013229c8593a8c78"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
		nativeResultCompatibility:     gotreesitter.ResultCompatibilityNativeCollapsedChildren,
	},
	"apex": {
		blobSHA256:                mustRuntimeProfileSHA256("69fc1b577f1f783a204c98719d55d2f15f329d296b9e227d651056ce878c1bd2"),
		nativeResultCompatibility: gotreesitter.ResultCompatibilityNativeCollapsedChildren,
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
	"python": {
		blobSHA256:                    mustRuntimeProfileSHA256("cde4a67dc6af6e1232dbbd1eab8618478d1d73727020e8a8002542390a452d37"),
		externalScannerFullParseRetry: gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
	},
	// Swift's low-pressure accepted-error parses select the same tree across
	// the retry ladder. High-pressure parses still benefit from the first
	// ladder, while repeating that ladder for the external scanner does not.
	"swift": {
		blobSHA256:                    mustRuntimeProfileSHA256("d0bb8834f9a93fee2a268c81c04e9a36dde5f725db2e3ebc785ab43f79d2ae6a"),
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
		blobSHA256: mustRuntimeProfileSHA256("a3e898c88f6ad918d4d619dff2a4e74d613bda93c90e4a3f9fb7587c1952f3fb"),
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
	// Meson's retry ladder changes selected trees on small error-bearing files,
	// but is redundant for complete accepted-error sources at or above this
	// conservative exact-blob corpus-certified floor.
	"meson": {
		blobSHA256: mustRuntimeProfileSHA256("b3b7e74bcd35614419f5359c31eb8a05bd58c0b97529f133f2aea2f40796789d"),
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
	// Haskell's expression-list repeat has one exact comma row where C folds
	// the reduce/repetition-shift pair deterministically. Retaining both arms
	// grows a new GSS frontier for every list element.
	"haskell": {
		blobSHA256: mustRuntimeProfileSHA256("fcfc8794bca4442ebf5688d88e2397c78a22c8f0b585c4e1b868986cfa52dd09"),
		conflictPolicies: []gotreesitter.ConflictPolicy{
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
		conflictPolicies: []gotreesitter.ConflictPolicy{
			{
				State:         gotreesitter.ConflictPolicyAnyState,
				Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
				Kind:          gotreesitter.ConflictPolicyRepetitionShift,
				ReduceSymbols: []gotreesitter.Symbol{324, 326},
			},
		},
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
	if missing := profile.nativeResultCompatibility &^ lang.NativeResultCompatibility; missing != 0 {
		lang.NativeResultCompatibility |= missing
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
