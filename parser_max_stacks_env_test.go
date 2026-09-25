package gotreesitter

import "testing"

// TestFullParseInitialMaxStacksKeepsExplicitEnvValue checks that an explicit
// GOT_GLR_MAX_STACKS value wins over the per-language defaults, also when it
// equals the built-in default.
func TestFullParseInitialMaxStacksKeepsExplicitEnvValue(t *testing.T) {
	css := &Language{Name: "css"}
	for _, tc := range []struct {
		env  string
		want int
	}{
		{"", 2},  // no override: the css default applies
		{"8", 8}, // explicit default value: keep it
		{"3", 3}, // explicit other value: keep it
	} {
		t.Setenv("GOT_GLR_MAX_STACKS", tc.env)
		ResetParseEnvConfigCacheForTests()
		if got := fullParseInitialMaxStacks(css, 0, nil); got != tc.want {
			t.Errorf("GOT_GLR_MAX_STACKS=%q: fullParseInitialMaxStacks(css) = %d, want %d", tc.env, got, tc.want)
		}
		if got := fullParseInitialMaxStacks(css, 5, nil); got < 5 {
			t.Errorf("GOT_GLR_MAX_STACKS=%q: conflict width 5 must still raise the floor, got %d", tc.env, got)
		}
	}
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
}
