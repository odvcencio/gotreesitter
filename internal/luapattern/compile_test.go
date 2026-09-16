package luapattern

import "testing"

func TestCompileUppercaseClassesAndNonGreedy(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		matches   []string
		nonMatch  []string
		findInput string
		findWant  string
	}{
		{
			name:     "non-digit",
			pattern:  `^%D+$`,
			matches:  []string{"abc", "_-"},
			nonMatch: []string{"123", "abc1"},
		},
		{
			name:     "non-space",
			pattern:  `^%S+$`,
			matches:  []string{"abc"},
			nonMatch: []string{"a b", "\t"},
		},
		{
			name:     "non-word",
			pattern:  `^%W+$`,
			matches:  []string{"-!", "_"},
			nonMatch: []string{"abc", "A1"},
		},
		{
			name:      "non-greedy",
			pattern:   `a.-b`,
			findInput: "a123b456b",
			findWant:  "a123b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rx, err := Compile(tt.pattern)
			if err != nil {
				t.Fatalf("Compile(%q): %v", tt.pattern, err)
			}
			for _, input := range tt.matches {
				if !rx.MatchString(input) {
					t.Fatalf("Compile(%q).MatchString(%q) = false, want true", tt.pattern, input)
				}
			}
			for _, input := range tt.nonMatch {
				if rx.MatchString(input) {
					t.Fatalf("Compile(%q).MatchString(%q) = true, want false", tt.pattern, input)
				}
			}
			if tt.findInput != "" {
				if got := rx.FindString(tt.findInput); got != tt.findWant {
					t.Fatalf("Compile(%q).FindString(%q) = %q, want %q", tt.pattern, tt.findInput, got, tt.findWant)
				}
			}
		})
	}
}
