package lexbuild

import (
	"math/big"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// Tests of the counters and of the guard arithmetic added with the fifth
// review of draft pull request 1618.

// ---- groups: the next three runes, not the remainder ----

// groupHeadOld is the earlier version of groupHead: the whole remainder.
func groupHeadOld(p *patParser) string { return string(p.src[p.pos:]) }

// randGroupPattern draws a pattern of pieces that start groups, flags, repetitions and classes.
func randGroupPattern(rng *rand.Rand) string {
	pieces := []string{"a", "b", "(", ")", "(", ")", "(?:", "(?i)", "(?i:", "(?-i:", "(?s", "(?P<n", "(?P<x>", "(?<y>", "(?<",
		"(?=", "(?!", "(?<=", "(?<!", "(?", "(?P", "?", "*", "+", "{2}", "{1,3}", "|", "[ab]", "[a-c]", "\\d", "é", "(?x)", "(?U:"}
	var sb strings.Builder
	for k := 1 + rng.Intn(14); k > 0; k-- {
		sb.WriteString(pieces[rng.Intn(len(pieces))])
	}
	return sb.String()
}

// randBalanced draws a pattern whose groups are closed: capture groups, (?:...),
// named groups, flag groups, (?i) directives, and the look-around forms that
// the parser refuses, mixed with literals, repetitions and classes.
func randBalanced(rng *rand.Rand, depth int) string {
	var sb strings.Builder
	for k := 1 + rng.Intn(4); k > 0; k-- {
		switch c := rng.Intn(14); {
		case c < 3 || depth == 0:
			sb.WriteString([]string{"a", "b", "é", "[ab]", "\\d", "x{2}", "a|b"}[rng.Intn(7)])
		case c < 5:
			sb.WriteString("(" + randBalanced(rng, depth-1) + ")")
		case c < 6:
			sb.WriteString("(?:" + randBalanced(rng, depth-1) + ")")
		case c < 7:
			sb.WriteString("(?P<n" + string(rune('a'+rng.Intn(3))) + ">" + randBalanced(rng, depth-1) + ")")
		case c < 8:
			sb.WriteString("(?<m" + string(rune('a'+rng.Intn(3))) + ">" + randBalanced(rng, depth-1) + ")")
		case c < 9:
			sb.WriteString("(?" + []string{"i", "s", "-i", "is", "U", "x"}[rng.Intn(6)] + ":" + randBalanced(rng, depth-1) + ")")
		case c < 10:
			sb.WriteString("(?" + []string{"i", "s", "-i", "m"}[rng.Intn(4)] + ")")
		case c < 11:
			sb.WriteString([]string{"(?=a)", "(?!a)", "(?<=a)", "(?<!a)"}[rng.Intn(4)])
		case c < 13:
			sb.WriteString("(" + randBalanced(rng, depth-1) + ")" + []string{"?", "*", "+", "{2}", "{1,2}"}[rng.Intn(5)])
		default:
			sb.WriteString("()")
		}
	}
	return sb.String()
}

func TestGroupHeadMatchesWholeRemainder(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	groupHeadOfSaved := groupHeadOf
	t.Cleanup(func() { groupHeadOf = groupHeadOfSaved })
	parsed, refused := 0, 0
	for n := 0; n < 20000; n++ {
		pat := randGroupPattern(rng)
		if n%2 == 0 {
			pat = randBalanced(rng, 3)
		}
		for _, ci := range []bool{false, true} {
			groupHeadOf = groupHeadOld
			wantH, wantErr := parsePattern(pat, ci)
			groupHeadOf = groupHeadOfSaved
			gotH, gotErr := parsePattern(pat, ci)
			if (wantErr == nil) != (gotErr == nil) || (wantErr != nil && wantErr.Error() != gotErr.Error()) {
				t.Fatalf("pattern %q: error %v, want %v", pat, gotErr, wantErr)
			}
			if wantErr == nil {
				parsed++
				if !reflect.DeepEqual(gotH, wantH) {
					t.Fatalf("pattern %q: tree differs from the earlier version", pat)
				}
			} else {
				refused++
			}
		}
	}
	t.Logf("%d parses equal, %d refusals equal", parsed, refused)
	if parsed < 3000 || refused < 5000 {
		t.Fatalf("the draw is too narrow: %d parsed, %d refused", parsed, refused)
	}
}

// Finding 1 of the fifth review: four patterns of 131,000 empty groups and a
// letter (1,048,185 bytes of grammar). The earlier code converted the whole
// remainder of the pattern for each group: 4 x 131,000 x 131,001 = 68,644,524,000
// runes.
func TestManyGroupsInspectThreeRunesEach(t *testing.T) {
	const groups = 131000
	var members []string
	for _, c := range "abcd" {
		members = append(members, `{"type":"PATTERN","value":"`+strings.Repeat("()", groups)+string(c)+`"}`)
	}
	g := []byte(`{"name":"t","rules":{"source":{"type":"CHOICE","members":[` + strings.Join(members, ",") + `]}}}`)
	if len(g) != 1048185 {
		t.Fatalf("grammar of %d bytes, want 1,048,185", len(g))
	}
	got := counted(t, "Prepare, 4 patterns of 131,000 groups", &ops.groupRunes, func() {
		if _, err := Prepare(g); err != nil {
			t.Fatal(err)
		}
	})
	if got > 3*4*groups {
		t.Fatalf("%d runes inspected, want at most 3 for each of the %d groups", got, 4*groups)
	}
	const old int64 = 4 * groups * (groups + 1) // 68,644,524,000
	atMost(t, "runes inspected by parseGroup", got, 3*4*groups, old)
}

// A counted repetition built its "unclosed" message, which quotes the whole
// pattern, before it knew it needed one. 60,000 repetitions of 4 bytes each
// would quote 240,000 bytes 60,000 times (1.4 x 10^10 bytes).
func TestCountedRepetitionsBuildNoMessage(t *testing.T) {
	const n = 60000
	got := counted(t, "Prepare, 60,000 counted repetitions", &ops.errBytes, func() {
		if _, err := Prepare(sizeGrammar(strings.Repeat("a{1}", n))); err != nil {
			t.Fatal(err)
		}
	})
	if got != 0 {
		t.Fatalf("%d bytes of pattern quoted into messages for a pattern that parses", got)
	}
	// A refused pattern quotes its source once.
	pat := strings.Repeat("a{1}", 1000) + "a{"
	got = counted(t, "Prepare, refused pattern", &ops.errBytes, func() {
		if _, err := Prepare(sizeGrammar(pat)); err == nil {
			t.Fatal("want an error")
		}
	})
	if got != int64(len(pat)) {
		t.Fatalf("%d bytes quoted, want %d (once)", got, len(pat))
	}
}

// ---- arithmetic of the budget checks on a 32-bit int ----

// mul32 multiplies in 32-bit ints, as a 386 build multiplies int; the host does not matter.
func mul32(a, b int32) int32 { return a * b }

// old32 is the guard of the 4th round (build.go: rangesCost), computed in
// 32-bit ints as a 386 build computes it. It does not depend on the host.
func rangesCostOld32(ranges, transitions int32) int32 {
	return (2*ranges + 4) * (transitions + 1)
}

func bigRangesCost(ranges, transitions int64) *big.Int {
	x := big.NewInt(2*ranges + 4)
	return x.Mul(x, big.NewInt(transitions+1))
}

// Finding 2 of the fifth review: 80,500 ranges and 15,001 transitions.
func TestRangesCostDoesNotWrap(t *testing.T) {
	cases := []struct {
		ranges      int64
		transitions int
	}{
		{80500, 15001},      // the review's state: 2,415,382,008
		{2, 2147483647 / 8}, // a product just below 2^31
		{(1 << 31) / 4, 4},  // a product just above 2^31
		{bLexRanges.limit, int(bLexAdvances.limit)}, // the caps of one table: 8,388,608 ranges, 1,048,576 transitions
		{bNFARanges.limit, int(bTransIDs.limit)},
		{1<<23 + 1, 1<<20 + 1}, // one past
		{1 << 32, 1 << 20},     // far past 2^32
		{0, 0}, {1, 0}, {0, 1},
	}
	if got := rangesCostOld32(80500, 15001); got >= 0 || got >= int32(bRangesWork.limit) {
		t.Fatalf("the 32-bit computation of the review's numbers gave %d: it must wrap negative and pass the limit", got)
	}
	for _, c := range cases {
		want := bigRangesCost(c.ranges, int64(c.transitions))
		got := rangesCostOf(c.ranges, c.transitions)
		if want.Cmp(big.NewInt(got)) != 0 || got < 0 {
			t.Errorf("ranges %d, transitions %d: cost %d, want %s", c.ranges, c.transitions, got, want)
		}
	}
	// The wiring: a state of 100 ranges in two advances and 3 planned transitions.
	st := &lexState{advances: []charAction{{chars: CharSet{r: make([]urange, 60)}}, {chars: CharSet{r: make([]urange, 40)}}}}
	if got, want := rangesCost(st, &statePlan{transitions: make([]plannedTransition, 3)}), int64((2*100+4)*4); got != want {
		t.Errorf("rangesCost = %d, want %d", got, want)
	}
}

// The review's grammar through BuildUnmerged: alternatives rune(0x10000+3i)+"a"
// for 15,000 values of i, and a class of 65,500 code points followed by "b".
// The starting state has 80,500 ranges and 15,001 planned transitions; the
// guard must refuse it with a *SizeError on every int width.
func TestRangeWorkGuardRefusesTheReviewsGrammar(t *testing.T) {
	var alts []string
	for i := 0; i < 15000; i++ {
		alts = append(alts, string(rune(0x10000+3*i))+"a")
	}
	var cls strings.Builder
	cls.WriteString("[")
	for i := 0; i < 65500; i++ {
		cls.WriteRune(rune(0x20000 + 2*i))
	}
	cls.WriteString("]b")
	g := choiceGrammar(strings.Join(alts, "|"), cls.String())
	p, err := Prepare(g)
	if err != nil {
		t.Fatal(err)
	}
	var ts TokenSet
	ts.Insert(terminalSym(0))
	ts.Insert(terminalSym(1))
	resetPeaks()
	_, _, err = p.BuildUnmerged([]TokenSet{ts})
	wantSize(t, err, "list the ranges of the lex states")
	if peak := bRangesWork.peak.Load(); peak != 2415382008 && peak < 2415382008 {
		t.Errorf("the guard counted %d, want at least 2,415,382,008", peak)
	}
	t.Logf("range work counted %d against the limit %d", bRangesWork.peak.Load(), bRangesWork.limit)
}

// splitStateIDGroups: the product (group size - 1) x (1 + advances of a state)
// for 4,096 states of 1,048,576 advances each is 4,294,... : above 2^32, so a
// 32-bit int gives a small or negative number.
func TestMinimizationProductDoesNotWrap(t *testing.T) {
	const states, adv = 4096, 1 << 20
	shared := make([]charAction, adv)
	ls := make([]lexState, states)
	ids := make([]uint32, states)
	for i := range ls {
		ls[i].advances = shared
		ids[i] = uint32(i)
	}
	if got := mul32(states-1, 1+adv); got >= int32(bMinCompares.limit) {
		t.Fatalf("the 32-bit product is %d: it must wrap below the limit for this test to show the defect", got)
	}
	groups := [][]uint32{nil, ids}
	groupOf := make([]uint32, states)
	var compares int64
	resetPeaks()
	err := guardSize(func() {
		splitStateIDGroups(ls, &groups, groupOf, 1, lexStatesDiffer, &compares)
	})
	wantSize(t, err, "comparisons while minimizing")
	want := int64(states-1) * int64(1+adv)
	if peak := bMinCompares.peak.Load(); peak < want {
		t.Errorf("the guard counted %d, want at least %d", peak, want)
	}
}

// getFollowingChars: 4,096 tokens whose starting sets share 1,000,000 ranges
// each join 4.1 x 10^9 ranges, above 2^32 and so wrapped by a 32-bit int.
func TestFollowingCharsWorkDoesNotWrap(t *testing.T) {
	const tokens, rangesEach = 4096, 1000000
	shared := make([]urange, rangesEach)
	starting := make([]CharSet, tokens)
	var follow TokenSet
	for i := range starting {
		starting[i] = CharSet{r: shared}
		follow.Insert(terminalSym(i))
	}
	if got := mul32(tokens, rangesEach); got >= int32(bFollowChars.limit) {
		t.Fatalf("the 32-bit sum is %d: it must wrap below the limit for this test to show the defect", got)
	}
	resetPeaks()
	err := guardSize(func() { getFollowingChars(starting, []TokenSet{follow}) })
	wantSize(t, err, "starting characters of the tokens that follow")
	if peak := bFollowChars.peak.Load(); peak < int64(tokens)*int64(rangesEach) {
		t.Errorf("the guard counted %d, want at least %d", peak, int64(tokens)*int64(rangesEach))
	}
}

// flattenGrammar: reserved sets x (tokens + externals).
func TestReservedCellsDoNotWrap(t *testing.T) {
	const sets, tokens = 1 << 14, 1<<18 + 1 // the product is 2^32 + 16,384: a 32-bit int wraps it to 16,384
	if got := mul32(sets, tokens); got >= int32(bSymbolCells.limit) {
		t.Fatalf("the 32-bit product is %d: it must wrap below the limit for this test to show the defect", got)
	}
	m := &extractedMeta{reservedSets: make([]reservedSet, sets), lexicalVariables: make([]LexicalToken, tokens)}
	err := guardSize(func() { _, _ = flattenGrammar(&InputGrammar{Pool: &RulePool{}}, m) })
	wantSize(t, err, "matrix")
}

// planState: len(large) x (1 + ranges of an advance). 65,536 large sets and an
// advance of 65,536 ranges give 4.3 x 10^9, which a 32-bit int wraps.
func TestRenderingProductDoesNotWrap(t *testing.T) {
	const sets, ranges = 1 << 16, 1 << 16
	if got := mul32(sets, 1+ranges); got >= int32(bRenderWork.limit) {
		t.Fatalf("the 32-bit product is %d: it must wrap below the limit for this test to show the defect", got)
	}
	shared := make([]urange, 2*largeCharacterRangeCount)
	for i := range shared {
		shared[i] = urange{uint32(10 * i), uint32(10*i + 1)}
	}
	large := make([]largeSet, sets)
	for i := range large {
		large[i].set = CharSet{r: shared}
	}
	adv := make([]urange, ranges)
	for i := range adv {
		adv[i] = urange{uint32(0x20000 + 4*i), uint32(0x20000 + 4*i + 1)}
	}
	st := &lexState{advances: []charAction{{chars: CharSet{r: adv}}}}
	var work int64
	resetPeaks()
	err := guardSize(func() { planState(st, large, &work) })
	wantSize(t, err, "rendering lex states")
	if want := int64(sets) * int64(1+ranges); bRenderWork.peak.Load() < want {
		t.Errorf("the guard counted %d, want at least %d", bRenderWork.peak.Load(), want)
	}
}

// mulWork at the caps, with the sizes of the three products of the rendering
// and minimization counters (collectLargeCharacterSets, planState,
// splitStateIDGroups).
func TestMulWorkAtTheCaps(t *testing.T) {
	for _, c := range [][2]int{
		{1 << 17, 1<<23 + 1}, // lex states x ranges
		{1 << 16, 1<<16 + 1}, // 4.3 x 10^9
		{46341, 46341},       // just above 2^31
		{1 << 20, 1<<20 + 1}, // transitions x transitions
		{1<<31 - 1, 2},       // the largest 32-bit operand
		{0, 1 << 30}, {1, 1},
	} {
		want := new(big.Int).Mul(big.NewInt(int64(c[0])), big.NewInt(int64(c[1])))
		if got := mulWork(c[0], c[1]); want.Cmp(big.NewInt(got)) != 0 {
			t.Errorf("mulWork(%d, %d) = %d, want %s", c[0], c[1], got, want)
		}
	}
}
