package gotreesitter

import "testing"

func TestGoIncrementalMergePolicyPrecedence(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = false
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })
	for _, tc := range []struct {
		name, language, env    string
		incremental, certified bool
		override, wantCap      int
		wantFaithful           bool
	}{
		{"go incremental default", "go", "", true, false, 0, 1, true},
		{"go fresh default", "go", "", false, false, 0, 3, false},
		{"go explicit wide", "go", "4", true, false, 0, 4, false},
		{"go explicit one", "go", "1", true, false, 0, 1, true},
		{"go retry widen", "go", "", true, false, 4, 4, false},
		{"go retry exact", "go", "", true, false, -3, 3, false},
		{"other incremental default", "other", "", true, false, 0, maxStacksPerMergeKey, false},
		{"other incremental explicit one", "other", "1", true, false, 0, 1, false},
		{"certified incremental exact one", "other", "", true, true, -1, 1, false},
		{"certified fresh default", "other", "", false, true, 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", tc.env)
			ResetParseEnvConfigCacheForTests()
			t.Cleanup(ResetParseEnvConfigCacheForTests)
			p := &Parser{language: &Language{Name: tc.language, FullParseGSSConvergenceEnabled: tc.certified}}
			var reuse *reuseCursor
			class := arenaClassFull
			if tc.incremental {
				reuse = &reuseCursor{}
				class = arenaClassIncremental
			}
			var scratch parserScratch
			caps := p.configureParseCaps([]byte("source"), reuse, class, &scratch, 0, 0, tc.override)
			if caps.mergePerKeyCap != tc.wantCap || scratch.merge.faithfulCapOne != tc.wantFaithful {
				t.Fatalf("cap=%d faithful=%t, want cap=%d faithful=%t", caps.mergePerKeyCap, scratch.merge.faithfulCapOne, tc.wantCap, tc.wantFaithful)
			}
		})
	}
}
