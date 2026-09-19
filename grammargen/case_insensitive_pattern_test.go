package grammargen

import "testing"

// classContainsRune reports whether the character class node accepts r,
// honoring the class's negate flag.
func classContainsRune(t *testing.T, node *regexNode, r rune) bool {
	t.Helper()
	if node.kind != regexCharClass {
		t.Fatalf("classContainsRune: node kind = %v, want regexCharClass", node.kind)
	}
	inRanges := false
	for _, rr := range node.runes {
		if r >= rr.lo && r <= rr.hi {
			inRanges = true
			break
		}
	}
	if node.negate {
		return !inRanges
	}
	return inRanges
}

// TestMakeCaseInsensitivePatternClassRangeDoesNotOverMatch pins the fix for
// a character-class letter range: "[a-f]" must expand to "[a-fA-F]", not the
// old, wrong "[aA-fF]", which built a single wide range from 'A' (0x41) to
// 'f' (0x66) covering every code point in between — including 'G'-'Z' and
// the punctuation '[', '\', ']', '^', '_', '`'.
func TestMakeCaseInsensitivePatternClassRangeDoesNotOverMatch(t *testing.T) {
	got := makeCaseInsensitivePattern("[a-f]")
	want := "[a-fA-F]"
	if got != want {
		t.Fatalf("makeCaseInsensitivePattern(%q) = %q, want %q", "[a-f]", got, want)
	}

	node, err := parseRegex(got)
	if err != nil {
		t.Fatalf("parseRegex(%q) failed: %v", got, err)
	}

	mustMatch := []rune{'a', 'f', 'A', 'F', 'c', 'C'}
	for _, r := range mustMatch {
		if !classContainsRune(t, node, r) {
			t.Errorf("pattern %q must match %q, but it did not", got, string(r))
		}
	}

	mustNotMatch := []rune{'X', '_', '^', '`', 'g', 'G', 'Z', '['}
	for _, r := range mustNotMatch {
		if classContainsRune(t, node, r) {
			t.Errorf("pattern %q must not match %q, but it did", got, string(r))
		}
	}
}

// TestMakeCaseInsensitivePatternPreservesEscapeSequences pins the fix for
// escape sequences inside and outside character classes: an escape is one
// atomic token, and case-folding does not apply to it. Corrupting it (for
// example by expanding a letter inside \p{Lu}) breaks the pattern.
func TestMakeCaseInsensitivePatternPreservesEscapeSequences(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		want    string
	}{
		{"unicode property braced", `\p{Lu}`, `\p{Lu}`},
		{"unicode property negated braced", `\P{Lu}`, `\P{Lu}`},
		{"unicode property shorthand", `\pL`, `\pL`},
		{"unicode escape braced", `\u{1F600}`, `\u{1F600}`},
		{"unicode escape fixed width", `\uABCD`, `\uABCD`},
		{"long unicode escape fixed width", `\U0001F600`, `\U0001F600`},
		{"hex escape braced", `\x{41}`, `\x{41}`},
		{"hex escape fixed width", `\xFF`, `\xFF`},
		{"digit class escape", `\d`, `\d`},
		{"word class escape", `\w`, `\w`},
		{"newline escape", `\n`, `\n`},
		{"escaped backslash", `\\`, `\\`},
		{"property escape inside class", `[\p{Lu}_]`, `[\p{Lu}_]`},
		{"unicode escape inside class", `[\u{1F600}-\u{1F64F}]`, `[\u{1F600}-\u{1F64F}]`},
		{"property escape mixed with letters", `[\pL\p{Mn}\pN_']`, `[\pL\p{Mn}\pN_']`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := makeCaseInsensitivePattern(tc.pattern)
			if got != tc.want {
				t.Fatalf("makeCaseInsensitivePattern(%q) = %q, want %q (escape sequence must survive intact)",
					tc.pattern, got, tc.want)
			}
			if _, err := parseRegex(got); err != nil {
				t.Fatalf("parseRegex(%q) failed after case-insensitive expansion: %v", got, err)
			}
		})
	}
}

// TestMakeCaseInsensitivePatternOutsideClass pins the existing, unchanged
// behavior for letters outside a character class: each letter still gets
// wrapped in its own [xX] class.
func TestMakeCaseInsensitivePatternOutsideClass(t *testing.T) {
	cases := []struct {
		pattern string
		want    string
	}{
		{"DUP", "[dD][uU][pP]"},
		{"a-f", "[aA]-[fF]"},
		{"if", "[iI][fF]"},
	}
	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			got := makeCaseInsensitivePattern(tc.pattern)
			if got != tc.want {
				t.Fatalf("makeCaseInsensitivePattern(%q) = %q, want %q", tc.pattern, got, tc.want)
			}
		})
	}
}

// TestMakeCaseInsensitivePatternClassSingleLetters pins the existing,
// unchanged behavior for a lone letter inside a class: it expands in place
// to both cases with no extra brackets.
func TestMakeCaseInsensitivePatternClassSingleLetters(t *testing.T) {
	got := makeCaseInsensitivePattern("[eE]")
	want := "[eEeE]"
	if got != want {
		t.Fatalf("makeCaseInsensitivePattern(%q) = %q, want %q", "[eE]", got, want)
	}
	if _, err := parseRegex(got); err != nil {
		t.Fatalf("parseRegex(%q) failed: %v", got, err)
	}
}

// TestMakeCaseInsensitivePatternMultipleRanges exercises a class with more
// than one letter range, mirroring real grammars like hex-digit patterns.
func TestMakeCaseInsensitivePatternMultipleRanges(t *testing.T) {
	got := makeCaseInsensitivePattern("[0-9a-f]")
	node, err := parseRegex(got)
	if err != nil {
		t.Fatalf("parseRegex(%q) failed: %v", got, err)
	}
	for _, r := range []rune{'0', '9', 'a', 'f', 'A', 'F'} {
		if !classContainsRune(t, node, r) {
			t.Errorf("pattern %q (from [0-9a-f]) must match %q, but it did not", got, string(r))
		}
	}
	for _, r := range []rune{'g', 'G', '_', '^'} {
		if classContainsRune(t, node, r) {
			t.Errorf("pattern %q (from [0-9a-f]) must not match %q, but it did", got, string(r))
		}
	}
}
