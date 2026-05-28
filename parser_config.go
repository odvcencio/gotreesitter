package gotreesitter

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	parseNodeLimitScaleOnce    sync.Once
	parseNodeLimitScale        int
	parseMemoryBudgetOnce      sync.Once
	parseMemoryBudgetMBVal     int
	parseMaxGLRStacksOnce      sync.Once
	parseMaxGLRStacks          int
	parseMaxMergePerKeyOnce    sync.Once
	parseMaxMergePerKey        int
	preMaterializationDiagOnce sync.Once
	preMaterializationDiag     bool
	parsePhaseTimingOnce       sync.Once
	parsePhaseTiming           bool
	parseReduceTimingOnce      sync.Once
	parseReduceTiming          bool
	parseActionTimingOnce      sync.Once
	parseActionTiming          bool
	parseReduceChainHintsOnce  sync.Once
	parseReduceChainHints      bool
	parseTSLazyCompatOnce      sync.Once
	parseTSLazyCompat          bool
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

func parseTransientReduceChildrenEnabled() bool {
	return parseTransientReduceEnabled("GOT_TRANSIENT_REDUCE_CHILDREN")
}

func parseTransientReduceParentsEnabled() bool {
	return parseTransientReduceEnabled("GOT_TRANSIENT_REDUCE_PARENTS")
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

func parseTransientReduceParentsLanguageEnabled(lang *Language) bool {
	return parseTransientReduceLanguageEnabled(lang, "GOT_TRANSIENT_REDUCE_PARENTS_LANGS")
}

func parseTransientReduceLanguageEnabled(lang *Language, envName string) bool {
	if lang == nil {
		return false
	}
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOT_TRANSIENT_REDUCE_LANGS"))
	}
	if raw == "" {
		return false
	}
	return transientReduceLanguageListMatches(raw, lang.Name)
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
