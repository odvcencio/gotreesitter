package gotreesitter

import "testing"

// TestParseEnvKnobsSnapshotResets checks that the per-parse knob snapshot
// reads the environment once and that the test reset hook refreshes it.
func TestParseEnvKnobsSnapshotResets(t *testing.T) {
	t.Setenv("GOT_PARSE_PROGRESS", "")
	t.Setenv("GOT_C_RECOVERY", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_CHECKPOINT_MB", "")
	t.Setenv("GOT_GLR_V2_PENDING_PARENTS", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	k := envKnobs()
	if k.parseProgress || k.cRecovery != "" || k.transientReduceCheckpointBytes != 0 || k.pendingParents.configured {
		t.Fatalf("empty environment produced %+v", *k)
	}

	t.Setenv("GOT_PARSE_PROGRESS", "1")
	t.Setenv("GOT_C_RECOVERY", "all")
	t.Setenv("GOT_TRANSIENT_REDUCE_CHECKPOINT_MB", "3")
	t.Setenv("GOT_GLR_V2_PENDING_PARENTS", "false")
	if envKnobs().parseProgress {
		t.Fatal("snapshot changed without a reset")
	}
	ResetParseEnvConfigCacheForTests()
	k = envKnobs()
	if !k.parseProgress || k.cRecovery != "all" || k.transientReduceCheckpointBytes != 3<<20 {
		t.Fatalf("reset did not refresh the snapshot: %+v", *k)
	}
	if !k.pendingParents.configured || k.pendingParents.enabled {
		t.Fatalf("pending parents knob = %+v, want configured and disabled", k.pendingParents)
	}
	if got := parseTransientReduceCheckpointBytes(); got != 3<<20 {
		t.Fatalf("parseTransientReduceCheckpointBytes() = %d, want %d", got, 3<<20)
	}
}

// TestParseEnvKnobsTransientReduceFallbacks checks the documented fallback
// names for the transient-reduce knobs.
func TestParseEnvKnobsTransientReduceFallbacks(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	t.Setenv("GOT_PYTHON_TRANSIENT_REDUCE_CHILDREN", "0")
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "go")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if parseTransientReduceChildrenEnabled() || parseTransientReduceParentsEnabled() {
		t.Fatal("the legacy python name must disable both transient-reduce knobs")
	}
	lang := &Language{Name: "go"}
	if !parseTransientReduceChildrenLanguageEnabled(lang) || !parseTransientReduceParentsLanguageEnabled(lang) {
		t.Fatal("GOT_TRANSIENT_REDUCE_LANGS must apply to both language lists")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "rust"}) {
		t.Fatal("a language outside the list must stay disabled")
	}
}
