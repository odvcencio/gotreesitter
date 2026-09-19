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
	parseNodeLimitScaleEnvSet   bool
	parseMemoryBudgetOnce       sync.Once
	parseMemoryBudgetMBVal      int
	parseMemoryBudgetMBFromEnv  bool
	parseMemoryHardCeilingOnce  sync.Once
	parseMemoryHardCeilingMBVal int
	parseMemoryHardCeilingEnv   bool
	parseMaxGLRStacksOnce       sync.Once
	parseMaxGLRStacks           int
	parseMaxGLRStacksEnvSet     bool
	parseMaxMergePerKeyOnce     sync.Once
	parseMaxMergePerKey         int
	parseMaxMergePerKeyEnvSet   bool
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
//
// glrFaithfulCapOneMerge is intentionally excluded: unlike the values below,
// it is a process-start mode rather than a memoized config value. Tests that
// override it must restore the variable directly. Mixing that mode into this
// cache reset made cleanup order observable when t.Setenv restored the
// environment after ResetParseEnvConfigCacheForTests ran.
func ResetParseEnvConfigCacheForTests() {
	parseEnvKnobsOnce = sync.Once{}
	parseEnvKnobsVal = parseEnvKnobs{}
	parseNodeLimitScaleOnce = sync.Once{}
	parseNodeLimitScale = 0
	parseNodeLimitScaleEnvSet = false
	parseMemoryBudgetOnce = sync.Once{}
	parseMemoryBudgetMBVal = 0
	parseMemoryBudgetMBFromEnv = false
	parseMemoryHardCeilingOnce = sync.Once{}
	parseMemoryHardCeilingMBVal = 0
	parseMemoryHardCeilingEnv = false
	parseMaxGLRStacksOnce = sync.Once{}
	parseMaxGLRStacks = 0
	parseMaxGLRStacksEnvSet = false
	parseMaxMergePerKeyOnce = sync.Once{}
	parseMaxMergePerKey = 0
	parseMaxMergePerKeyEnvSet = false
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
}

func parseNodeLimitScaleFactor() int {
	parseNodeLimitScaleOnce.Do(func() {
		parseNodeLimitScale = 1
		raw := strings.TrimSpace(os.Getenv("GOT_PARSE_NODE_LIMIT_SCALE"))
		parseNodeLimitScaleEnvSet = raw != ""
		if !parseNodeLimitScaleEnvSet {
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
	_ = parseNodeLimitScaleFactor()
	return parseNodeLimitScaleEnvSet
}

func parseMaxGLRStacksValue() int {
	parseMaxGLRStacksOnce.Do(func() {
		parseMaxGLRStacks = maxGLRStacks
		raw := strings.TrimSpace(os.Getenv("GOT_GLR_MAX_STACKS"))
		parseMaxGLRStacksEnvSet = raw != ""
		if !parseMaxGLRStacksEnvSet {
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
		parseMaxMergePerKeyEnvSet = raw != ""
		if !parseMaxMergePerKeyEnvSet {
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
	_ = parseMaxMergePerKeyValue()
	return parseMaxMergePerKeyEnvSet
}

// parseMaxGLRStacksEnvConfigured reports whether GOT_GLR_MAX_STACKS was set
// explicitly. An explicit survivor cap is a diagnostic/experiment contract:
// the retry ladder must treat it as a true ceiling instead of silently
// widening past it (fullParseRetryMaxStacksOverride).
func parseMaxGLRStacksEnvConfigured() bool {
	_ = parseMaxGLRStacksValue()
	return parseMaxGLRStacksEnvSet
}

// glrFaithfulCapOneMerge (GOT_FAITHFUL_CONDENSE=1) makes cap-one condense
// preserve same-key tie readings through multi-link GSS nodes. It defaults off
// while the faithful GLR path is validated.
var glrFaithfulCapOneMerge = os.Getenv("GOT_FAITHFUL_CONDENSE") == "1"

// parseEnvKnobs holds the environment values that the parser reads on each
// parse or token source. The process reads them once. os.Getenv takes a
// process-wide lock, so a per-parse read costs latency on every keystroke.
// ResetParseEnvConfigCacheForTests clears the snapshot.
type parseEnvKnobs struct {
	transientReduceCheckpointBytes int64
	compactFullLeaves              envBoolKnob
	pendingParents                 envBoolKnob
	finalChildRefs                 envBoolKnob
	// transientReduceRaw maps an env name to its trimmed value, with the
	// documented fallback names already applied.
	transientReduceRaw map[string]string
	parseProgress      bool
	cRecovery          string
}

// envBoolKnob records whether an env value is set and how it parses.
type envBoolKnob struct {
	configured bool
	enabled    bool
}

var (
	parseEnvKnobsOnce sync.Once
	parseEnvKnobsVal  parseEnvKnobs
)

func envKnobs() *parseEnvKnobs {
	parseEnvKnobsOnce.Do(loadParseEnvKnobs)
	return &parseEnvKnobsVal
}

func loadParseEnvKnobs() {
	k := parseEnvKnobs{transientReduceRaw: make(map[string]string, 5)}
	if raw := strings.TrimSpace(os.Getenv("GOT_TRANSIENT_REDUCE_CHECKPOINT_MB")); raw != "" {
		if mb, err := strconv.Atoi(raw); err == nil && mb > 0 {
			k.transientReduceCheckpointBytes = int64(mb) << 20
		}
	}
	k.compactFullLeaves = readEnvBoolKnob("GOT_GLR_V2_COMPACT_FULL_LEAVES")
	k.pendingParents = readEnvBoolKnob("GOT_GLR_V2_PENDING_PARENTS")
	k.finalChildRefs = readEnvBoolKnob("GOT_GLR_V2_FINAL_CHILD_REFS")
	pythonFallback := strings.TrimSpace(os.Getenv("GOT_PYTHON_TRANSIENT_REDUCE_CHILDREN"))
	for _, name := range []string{"GOT_TRANSIENT_REDUCE_CHILDREN", "GOT_TRANSIENT_REDUCE_PARENTS"} {
		raw := strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			raw = pythonFallback
		}
		k.transientReduceRaw[name] = raw
	}
	langsFallback := strings.TrimSpace(os.Getenv("GOT_TRANSIENT_REDUCE_LANGS"))
	for _, name := range []string{"GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "GOT_TRANSIENT_REDUCE_PARENTS_LANGS"} {
		raw := strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			raw = langsFallback
		}
		k.transientReduceRaw[name] = raw
	}
	k.parseProgress = strings.TrimSpace(os.Getenv("GOT_PARSE_PROGRESS")) == "1"
	k.cRecovery = os.Getenv("GOT_C_RECOVERY")
	parseEnvKnobsVal = k
}

func readEnvBoolKnob(name string) envBoolKnob {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return envBoolKnob{}
	}
	return envBoolKnob{configured: true, enabled: raw != "0" && !strings.EqualFold(raw, "false")}
}

func parseTransientReduceChildrenEnabled() bool {
	return parseTransientReduceEnabled("GOT_TRANSIENT_REDUCE_CHILDREN")
}

func parseTransientReduceParentsEnabled() bool {
	return parseTransientReduceEnabled("GOT_TRANSIENT_REDUCE_PARENTS")
}

func parseTransientReduceCheckpointBytes() int64 {
	return envKnobs().transientReduceCheckpointBytes
}

func parseCompactFullLeavesEnabled() bool {
	_, enabled := parseCompactFullLeavesEnv()
	return enabled
}

func parseCompactFullLeavesEnv() (configured bool, enabled bool) {
	k := envKnobs().compactFullLeaves
	return k.configured, k.enabled
}

func parsePendingParentsEnv() (configured bool, enabled bool) {
	k := envKnobs().pendingParents
	return k.configured, k.enabled
}

func parseFinalChildRefsEnv() (configured bool, enabled bool) {
	k := envKnobs().finalChildRefs
	return k.configured, k.enabled
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
	raw := envKnobs().transientReduceRaw[envName]
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
	raw := envKnobs().transientReduceRaw[envName]
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
			parseMemoryBudgetMBFromEnv = true
		}
	})
	return parseMemoryBudgetMBVal
}

// parseMemoryBudgetFixedByEnv reports whether GOT_PARSE_MEMORY_BUDGET_MB sets
// a fixed budget. A fixed budget does not scale with the input size.
func parseMemoryBudgetFixedByEnv() bool {
	parseMemoryBudgetMB()
	return parseMemoryBudgetMBFromEnv
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
			parseMemoryHardCeilingEnv = true
		}
	})
	return parseMemoryHardCeilingMBVal
}

// parseMemoryHardCeilingFixedByEnv reports whether
// GOT_PARSE_MEMORY_HARD_CEILING_MB sets a fixed ceiling. A fixed ceiling does
// not scale with the parse budget.
func parseMemoryHardCeilingFixedByEnv() bool {
	parseMemoryHardCeilingMB()
	return parseMemoryHardCeilingEnv
}

func parseMemoryHardCeilingBytes() int64 {
	mb := parseMemoryHardCeilingMB()
	if mb <= 0 {
		return 0
	}
	return int64(mb) * 1024 * 1024
}
