package gotreesitter

import "testing"

func setGLRFaithfulCapOneMergeForTest(t *testing.T, enabled bool) {
	t.Helper()
	previous := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = enabled
	t.Cleanup(func() {
		glrFaithfulCapOneMerge = previous
	})
}

func TestResetParseEnvConfigCacheDoesNotOwnFaithfulCondenseMode(t *testing.T) {
	setGLRFaithfulCapOneMergeForTest(t, true)
	t.Setenv("GOT_FAITHFUL_CONDENSE", "")

	ResetParseEnvConfigCacheForTests()

	if !glrFaithfulCapOneMerge {
		t.Fatal("ResetParseEnvConfigCacheForTests changed the process-start faithful-condense mode")
	}
}

func TestParseLimitEnvPresenceSharesValueSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		defaultVal int
		value      func() int
		configured func() bool
	}{
		{
			name:       "node_limit_scale",
			env:        "GOT_PARSE_NODE_LIMIT_SCALE",
			defaultVal: 1,
			value:      parseNodeLimitScaleFactor,
			configured: parseNodeLimitScaleEnvConfigured,
		},
		{
			name:       "max_glr_stacks",
			env:        "GOT_GLR_MAX_STACKS",
			defaultVal: maxGLRStacks,
			value:      parseMaxGLRStacksValue,
			configured: parseMaxGLRStacksEnvConfigured,
		},
		{
			name:       "max_merge_per_key",
			env:        "GOT_GLR_MAX_MERGE_PER_KEY",
			defaultVal: maxStacksPerMergeKey,
			value:      parseMaxMergePerKeyValue,
			configured: parseMaxMergePerKeyEnvConfigured,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.env, "")
			ResetParseEnvConfigCacheForTests()
			t.Cleanup(ResetParseEnvConfigCacheForTests)

			if got := test.value(); got != test.defaultVal {
				t.Fatalf("default value = %d, want %d", got, test.defaultVal)
			}
			if test.configured() {
				t.Fatal("empty environment reported an explicit override")
			}

			t.Setenv(test.env, "7")
			if got := test.value(); got != test.defaultVal {
				t.Fatalf("cached value = %d, want %d", got, test.defaultVal)
			}
			if test.configured() {
				t.Fatal("presence changed outside the cached value snapshot")
			}

			ResetParseEnvConfigCacheForTests()
			if got := test.value(); got != 7 {
				t.Fatalf("reset value = %d, want 7", got)
			}
			if !test.configured() {
				t.Fatal("reset omitted the explicit override")
			}

			t.Setenv(test.env, "")
			if !test.configured() {
				t.Fatal("presence changed outside the cached value snapshot")
			}
		})
	}
}

func TestTransientReduceLanguageDefaultsToDisabled(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "python"}) {
		t.Fatal("python transient reduce children enabled by default")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "java"}) {
		t.Fatal("java transient reduce children enabled by default")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "go"}) {
		t.Fatal("go transient reduce children enabled without source-gated default")
	}
	if parseTransientReduceParentsLanguageEnabled(&Language{Name: "go"}) {
		t.Fatal("go transient reduce parents enabled without source-gated default")
	}
}

func TestTransientReduceGoDefaultLargeSourceOnly(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	p := &Parser{language: &Language{Name: "go"}}
	small := make([]byte, defaultTransientReduceGoMinSourceLen-1)
	large := make([]byte, defaultTransientReduceGoMinSourceLen)
	if p.shouldUseTransientReduceChildren(small, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce children enabled below large-source threshold")
	}
	if p.shouldUseTransientReduceParents(small, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce parents enabled below large-source threshold")
	}
	if !p.shouldUseTransientReduceChildren(large, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce children disabled at large-source threshold")
	}
	if !p.shouldUseTransientReduceParents(large, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce parents disabled at large-source threshold")
	}

	p.noResultCompatibilityBenchmarkOnly = true
	if p.shouldUseTransientReduceChildren(large, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce children enabled in no-result-compat benchmark mode")
	}
	if p.shouldUseTransientReduceParents(large, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce parents enabled in no-result-compat benchmark mode")
	}
}

func TestTransientReduceLanguageAllowlist(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "java, typescript")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	if !parseTransientReduceChildrenLanguageEnabled(&Language{Name: "java"}) {
		t.Fatal("java transient reduce children disabled by allowlist")
	}
	if !parseTransientReduceParentsLanguageEnabled(&Language{Name: "typescript"}) {
		t.Fatal("typescript transient reduce parents disabled by allowlist")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "python"}) {
		t.Fatal("python transient reduce children enabled outside allowlist")
	}
}

func TestTransientReduceGoAllowlistBypassesLargeSourceDefault(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "go")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	p := &Parser{language: &Language{Name: "go"}}
	src := []byte("package p\n")
	if !p.shouldUseTransientReduceChildren(src, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce children explicit allowlist ignored below threshold")
	}
	if !p.shouldUseTransientReduceParents(src, nil, nil, arenaClassFull) {
		t.Fatal("go transient reduce parents explicit allowlist ignored below threshold")
	}
}

func TestTransientReduceLanguageSpecificOverride(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "all")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "kotlin")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "none")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	if !parseTransientReduceChildrenLanguageEnabled(&Language{Name: "kotlin"}) {
		t.Fatal("kotlin transient reduce children disabled by specific allowlist")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "java"}) {
		t.Fatal("java transient reduce children enabled despite specific override")
	}
	if parseTransientReduceParentsLanguageEnabled(&Language{Name: "kotlin"}) {
		t.Fatal("kotlin transient reduce parents enabled despite specific none override")
	}
}

func TestTransientReduceLegacyDisable(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_PYTHON_TRANSIENT_REDUCE_CHILDREN", "false")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	if parseTransientReduceChildrenEnabled() {
		t.Fatal("legacy transient reduce disable ignored")
	}
	if parseTransientReduceParentsEnabled() {
		t.Fatal("legacy transient reduce parent disable ignored")
	}
}

func TestTransientReducePathDisable(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "0")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "false")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_PYTHON_TRANSIENT_REDUCE_CHILDREN", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	if parseTransientReduceChildrenEnabled() {
		t.Fatal("transient reduce children disable ignored")
	}
	if parseTransientReduceParentsEnabled() {
		t.Fatal("transient reduce parent disable ignored")
	}
}

func TestTransientReduceScratchNoAliasLargeOnly(t *testing.T) {
	if parseShouldUseTransientReduceScratchNoAlias(50 * 1024) {
		t.Fatal("scratch no-alias transient reduce enabled for 50KB input")
	}
	if !parseShouldUseTransientReduceScratchNoAlias(256 * 1024) {
		t.Fatal("scratch no-alias transient reduce disabled at large-file threshold")
	}
}

func TestTransientReduceCheckpointBytes(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHECKPOINT_MB", "128")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	if got, want := parseTransientReduceCheckpointBytes(), int64(128<<20); got != want {
		t.Fatalf("parseTransientReduceCheckpointBytes() = %d, want %d", got, want)
	}
	t.Setenv("GOT_TRANSIENT_REDUCE_CHECKPOINT_MB", "off")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	if got := parseTransientReduceCheckpointBytes(); got != 0 {
		t.Fatalf("parseTransientReduceCheckpointBytes() for invalid value = %d, want 0", got)
	}
}
