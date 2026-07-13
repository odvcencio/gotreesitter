package gotreesitter

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	parseNodeLimitScaleOnce     sync.Once
	parseNodeLimitScale         int
	parseMemoryBudgetOnce       sync.Once
	parseMemoryBudgetMBVal      int
	parseMemoryHardCeilingOnce  sync.Once
	parseMemoryHardCeilingMBVal int
	parseMaxGLRStacksOnce       sync.Once
	parseMaxGLRStacks           int
	parseMaxMergePerKeyOnce     sync.Once
	parseMaxMergePerKey         int
	preMaterializationDiagOnce  sync.Once
	preMaterializationDiag      bool
	parsePhaseTimingOnce        sync.Once
	parsePhaseTiming            bool
	parseReduceTimingOnce       sync.Once
	parseReduceTiming           bool
	parseActionTimingOnce       sync.Once
	parseActionTiming           bool
	parseReduceChainHintsOnce   sync.Once
	parseReduceChainHints       bool
	parseTSLazyCompatOnce       sync.Once
	parseTSLazyCompat           bool
	parseEagerDefaultOnce       sync.Once
	parseEagerDefault           bool
	parseEagerDefaultDebugOnce  sync.Once
	parseEagerDefaultDebug      bool
)

// ResetParseEnvConfigCacheForTests clears memoized parser env config.
//
// Tests in this repo mutate env vars between cases; this helper ensures
// subsequent parses observe the new values in the same process.
func ResetParseEnvConfigCacheForTests() {
	parseNodeLimitScaleOnce = sync.Once{}
	parseNodeLimitScale = 0
	parseMemoryBudgetOnce = sync.Once{}
	parseMemoryBudgetMBVal = 0
	parseMemoryHardCeilingOnce = sync.Once{}
	parseMemoryHardCeilingMBVal = 0
	parseMaxGLRStacksOnce = sync.Once{}
	parseMaxGLRStacks = 0
	parseMaxMergePerKeyOnce = sync.Once{}
	parseMaxMergePerKey = 0
	preMaterializationDiagOnce = sync.Once{}
	preMaterializationDiag = false
	parsePhaseTimingOnce = sync.Once{}
	parsePhaseTiming = false
	parseReduceTimingOnce = sync.Once{}
	parseReduceTiming = false
	parseActionTimingOnce = sync.Once{}
	parseActionTiming = false
	parseReduceChainHintsOnce = sync.Once{}
	parseReduceChainHints = false
	parseTSLazyCompatOnce = sync.Once{}
	parseTSLazyCompat = false
	parseEagerDefaultOnce = sync.Once{}
	parseEagerDefault = false
	parseEagerDefaultDebugOnce = sync.Once{}
	parseEagerDefaultDebug = false
	glrFaithfulCapOneMerge = os.Getenv("GOT_FAITHFUL_CONDENSE") == "1"
}

func parseNodeLimitScaleFactor() int {
	parseNodeLimitScaleOnce.Do(func() {
		parseNodeLimitScale = 1
		raw := strings.TrimSpace(os.Getenv("GOT_PARSE_NODE_LIMIT_SCALE"))
		if raw == "" {
			return
		}
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			parseNodeLimitScale = n
		}
	})
	return parseNodeLimitScale
}

func parseNodeLimitScaleEnvConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOT_PARSE_NODE_LIMIT_SCALE")) != ""
}

func parseMaxGLRStacksValue() int {
	parseMaxGLRStacksOnce.Do(func() {
		parseMaxGLRStacks = maxGLRStacks
		raw := strings.TrimSpace(os.Getenv("GOT_GLR_MAX_STACKS"))
		if raw == "" {
			return
		}
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			parseMaxGLRStacks = n
		}
	})
	return parseMaxGLRStacks
}

func parseMaxMergePerKeyValue() int {
	parseMaxMergePerKeyOnce.Do(func() {
		parseMaxMergePerKey = maxStacksPerMergeKey
		raw := strings.TrimSpace(os.Getenv("GOT_GLR_MAX_MERGE_PER_KEY"))
		if raw == "" {
			return
		}
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			parseMaxMergePerKey = n
		}
	})
	return parseMaxMergePerKey
}

func parseMaxMergePerKeyEnvConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOT_GLR_MAX_MERGE_PER_KEY")) != ""
}

// parseMaxGLRStacksEnvConfigured reports whether GOT_GLR_MAX_STACKS was set
// explicitly. An explicit survivor cap is a diagnostic/experiment contract:
// the retry ladder must treat it as a true ceiling instead of silently
// widening past it (fullParseRetryMaxStacksOverride).
func parseMaxGLRStacksEnvConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOT_GLR_MAX_STACKS")) != ""
}

// glrFaithfulCapOneMerge (GOT_FAITHFUL_CONDENSE=1) makes cap-one condense
// preserve same-key tie readings through multi-link GSS nodes. It defaults off
// while the faithful GLR path is validated.
var glrFaithfulCapOneMerge = os.Getenv("GOT_FAITHFUL_CONDENSE") == "1"

func parseTransientReduceChildrenEnabled() bool {
	return parseTransientReduceEnabled("GOT_TRANSIENT_REDUCE_CHILDREN")
}

func parseTransientReduceParentsEnabled() bool {
	return parseTransientReduceEnabled("GOT_TRANSIENT_REDUCE_PARENTS")
}

func parseTransientReduceCheckpointBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("GOT_TRANSIENT_REDUCE_CHECKPOINT_MB"))
	if raw == "" {
		return 0
	}
	mb, err := strconv.Atoi(raw)
	if err != nil || mb <= 0 {
		return 0
	}
	return int64(mb) << 20
}

func parseCompactFullLeavesEnabled() bool {
	_, enabled := parseCompactFullLeavesEnv()
	return enabled
}

func parseCompactFullLeavesEnv() (configured bool, enabled bool) {
	raw := strings.TrimSpace(os.Getenv("GOT_GLR_V2_COMPACT_FULL_LEAVES"))
	if raw == "" {
		return false, false
	}
	return true, raw != "0" && !strings.EqualFold(raw, "false")
}

func parsePendingParentsEnv() (configured bool, enabled bool) {
	raw := strings.TrimSpace(os.Getenv("GOT_GLR_V2_PENDING_PARENTS"))
	if raw == "" {
		return false, false
	}
	return true, raw != "0" && !strings.EqualFold(raw, "false")
}

func parseFinalChildRefsEnv() (configured bool, enabled bool) {
	raw := strings.TrimSpace(os.Getenv("GOT_GLR_V2_FINAL_CHILD_REFS"))
	if raw == "" {
		return false, false
	}
	return true, raw != "0" && !strings.EqualFold(raw, "false")
}

func parsePreMaterializationDiagEnabled() bool {
	preMaterializationDiagOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("GOT_GLR_V2_PRE_MATERIALIZATION_DIAG"))
		preMaterializationDiag = raw != "" && raw != "0" && !strings.EqualFold(raw, "false")
	})
	return preMaterializationDiag
}

func parsePhaseTimingEnabled() bool {
	parsePhaseTimingOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("GOT_PARSE_PHASE_TIMING"))
		parsePhaseTiming = raw != "" && raw != "0" && !strings.EqualFold(raw, "false")
	})
	return parsePhaseTiming
}

func parseReduceTimingEnabled() bool {
	parseReduceTimingOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("GOT_PARSE_REDUCE_TIMING"))
		parseReduceTiming = raw != "" && raw != "0" && !strings.EqualFold(raw, "false")
	})
	return parseReduceTiming
}

func parseActionTimingEnabled() bool {
	parseActionTimingOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("GOT_PARSE_ACTION_TIMING"))
		parseActionTiming = raw != "" && raw != "0" && !strings.EqualFold(raw, "false")
	})
	return parseActionTiming
}

func parseReduceChainHintsEnabled() bool {
	parseReduceChainHintsOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("GOT_GLR_REDUCE_CHAIN_HINTS"))
		parseReduceChainHints = raw != "" && raw != "0" && !strings.EqualFold(raw, "false")
	})
	return parseReduceChainHints
}

func parseTypeScriptLazyResultCompatibilityEnabled() bool {
	parseTSLazyCompatOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("GOT_TS_LAZY_COMPAT"))
		parseTSLazyCompat = raw == "" || (raw != "0" && !strings.EqualFold(raw, "false"))
	})
	return parseTSLazyCompat
}

func parseTransientReduceEnabled(envName string) bool {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOT_PYTHON_TRANSIENT_REDUCE_CHILDREN"))
	}
	if raw == "" {
		return true
	}
	return raw != "0" && !strings.EqualFold(raw, "false")
}

func parseTransientReduceChildrenLanguageEnabled(lang *Language) bool {
	return parseTransientReduceLanguageEnabled(lang, "GOT_TRANSIENT_REDUCE_CHILDREN_LANGS")
}

func parseTransientReduceChildrenLanguageEnabledForSource(lang *Language, sourceLen int) bool {
	return parseTransientReduceLanguageEnabledForSource(lang, sourceLen, "GOT_TRANSIENT_REDUCE_CHILDREN_LANGS")
}

func parseTransientReduceParentsLanguageEnabled(lang *Language) bool {
	return parseTransientReduceLanguageEnabled(lang, "GOT_TRANSIENT_REDUCE_PARENTS_LANGS")
}

func parseTransientReduceParentsLanguageEnabledForSource(lang *Language, sourceLen int) bool {
	return parseTransientReduceLanguageEnabledForSource(lang, sourceLen, "GOT_TRANSIENT_REDUCE_PARENTS_LANGS")
}

func parseTransientReduceLanguageEnabled(lang *Language, envName string) bool {
	return parseTransientReduceLanguageEnabledForSource(lang, 0, envName)
}

func parseTransientReduceLanguageEnabledForSource(lang *Language, sourceLen int, envName string) bool {
	if lang == nil {
		return false
	}
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOT_TRANSIENT_REDUCE_LANGS"))
	}
	if raw == "" {
		return defaultTransientReduceLanguageEnabledForSource(lang.Name, sourceLen)
	}
	return transientReduceLanguageListMatches(raw, lang.Name)
}

const defaultTransientReduceGoMinSourceLen = 64 * 1024

func defaultTransientReduceLanguageEnabledForSource(name string, sourceLen int) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "go":
		return sourceLen >= defaultTransientReduceGoMinSourceLen
	default:
		return false
	}
}

func transientReduceLanguageListMatches(raw, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		switch part {
		case "", "0", "false", "off", "none":
			continue
		case "1", "true", "on", "all", "*":
			return true
		case name:
			return true
		}
	}
	return false
}

func parseMemoryBudgetMB() int {
	parseMemoryBudgetOnce.Do(func() {
		// Default to a bounded per-parse ceiling so runaway GLR/arena growth
		// stops before multi-GB RSS while ordinary full parses still complete.
		parseMemoryBudgetMBVal = 512
		raw := strings.TrimSpace(os.Getenv("GOT_PARSE_MEMORY_BUDGET_MB"))
		if raw == "" {
			return
		}
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 0 {
			parseMemoryBudgetMBVal = n
		}
	})
	return parseMemoryBudgetMBVal
}

// parseMemoryHardCeilingMB returns the absolute, decoupled heap-growth stop
// that fires regardless of the soft per-parse budget's poll-mask overshoot
// tolerance (see the 2026-07-12 budget-containment RCA). The default (2GiB)
// sits comfortably above the known-good Poppler JS witness completion point
// (~1.67GiB heap growth at the default soft budget), so legitimate large
// parses that rely on bounded overshoot to finish are unaffected; it exists
// to catch pathological lanes (e.g. the GLR merge survivor grind) that can
// otherwise balloon to multiple GB before any soft-budget poll site is
// reached. Set GOT_PARSE_MEMORY_HARD_CEILING_MB=0 to disable it entirely.
func parseMemoryHardCeilingMB() int {
	parseMemoryHardCeilingOnce.Do(func() {
		parseMemoryHardCeilingMBVal = 2048
		raw := strings.TrimSpace(os.Getenv("GOT_PARSE_MEMORY_HARD_CEILING_MB"))
		if raw == "" {
			return
		}
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 0 {
			parseMemoryHardCeilingMBVal = n
		}
	})
	return parseMemoryHardCeilingMBVal
}

func parseMemoryHardCeilingBytes() int64 {
	mb := parseMemoryHardCeilingMB()
	if mb <= 0 {
		return 0
	}
	return int64(mb) * 1024 * 1024
}
