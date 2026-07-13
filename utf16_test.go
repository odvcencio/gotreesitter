package gotreesitter

import (
	"encoding/binary"
	"errors"
	"testing"
	"unicode/utf16"
)

func utf16Units(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func utf16BytesForTest(t *testing.T, s string, order UTF16ByteOrder) []byte {
	t.Helper()

	var byteOrder binary.ByteOrder
	switch order {
	case UTF16LittleEndian:
		byteOrder = binary.LittleEndian
	case UTF16BigEndian:
		byteOrder = binary.BigEndian
	default:
		t.Fatalf("unsupported test byte order %v", order)
	}

	units := utf16Units(s)
	out := make([]byte, len(units)*2)
	for i, unit := range units {
		byteOrder.PutUint16(out[i*2:], unit)
	}
	return out
}

func assertUTF16UnitsEqual(t *testing.T, got, want []uint16) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("UTF-16 unit length = %d, want %d: got %v want %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("UTF-16 unit %d = %#04x, want %#04x: got %v want %v", i, got[i], want[i], got, want)
		}
	}
}

func dfaTokenSourceFactoryForTest(t *testing.T, parser *Parser) TokenSourceFactory {
	t.Helper()
	factory := parser.dfaReparseFactory()
	if factory == nil {
		t.Fatal("DFA token source factory is nil")
	}
	return factory
}

func TestDecodeUTF16BytesLittleAndBigEndian(t *testing.T) {
	want := utf16Units("a😀\nb")
	tests := []struct {
		name  string
		order UTF16ByteOrder
	}{
		{name: "little", order: UTF16LittleEndian},
		{name: "big", order: UTF16BigEndian},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeUTF16Bytes(utf16BytesForTest(t, "a😀\nb", tc.order), tc.order)
			if err != nil {
				t.Fatalf("DecodeUTF16Bytes failed: %v", err)
			}
			assertUTF16UnitsEqual(t, got, want)
		})
	}
}

func TestDecodeUTF16BytesRejectsInvalidInputs(t *testing.T) {
	if _, err := DecodeUTF16Bytes([]byte{0x31}, UTF16LittleEndian); !errors.Is(err, ErrInvalidUTF16ByteLength) {
		t.Fatalf("odd-length DecodeUTF16Bytes error = %v, want ErrInvalidUTF16ByteLength", err)
	}
	if _, err := DecodeUTF16Bytes([]byte{0x31, 0x00}, UTF16ByteOrder(255)); !errors.Is(err, ErrInvalidUTF16ByteOrder) {
		t.Fatalf("invalid-order DecodeUTF16Bytes error = %v, want ErrInvalidUTF16ByteOrder", err)
	}
}

func TestEncodeUTF16ToUTF8WithMapSurrogatePair(t *testing.T) {
	source := utf16Units("a😀\nb")
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)

	if got, want := string(utf8Source), "a😀\nb"; got != want {
		t.Fatalf("UTF-8 source = %q, want %q", got, want)
	}

	tests := []struct {
		byteOffset uint32
		unitOffset uint32
		point      Point
	}{
		{byteOffset: 0, unitOffset: 0, point: Point{Row: 0, Column: 0}},
		{byteOffset: 1, unitOffset: 1, point: Point{Row: 0, Column: 1}},
		{byteOffset: 5, unitOffset: 3, point: Point{Row: 0, Column: 3}},
		{byteOffset: 6, unitOffset: 4, point: Point{Row: 1, Column: 0}},
		{byteOffset: 7, unitOffset: 5, point: Point{Row: 1, Column: 1}},
	}
	for _, tc := range tests {
		unitOffset, ok := sourceMap.byteToUTF16Unit(tc.byteOffset)
		if !ok {
			t.Fatalf("byteToUTF16Unit(%d) returned !ok", tc.byteOffset)
		}
		if unitOffset != tc.unitOffset {
			t.Fatalf("byteToUTF16Unit(%d) = %d, want %d", tc.byteOffset, unitOffset, tc.unitOffset)
		}
		point, ok := sourceMap.pointForByte(tc.byteOffset)
		if !ok {
			t.Fatalf("pointForByte(%d) returned !ok", tc.byteOffset)
		}
		if point != tc.point {
			t.Fatalf("pointForByte(%d) = %+v, want %+v", tc.byteOffset, point, tc.point)
		}
	}

	utf8PointTests := []struct {
		byteOffset uint32
		point      Point
	}{
		{byteOffset: 0, point: Point{Row: 0, Column: 0}},
		{byteOffset: 1, point: Point{Row: 0, Column: 1}},
		{byteOffset: 5, point: Point{Row: 0, Column: 5}},
		{byteOffset: 6, point: Point{Row: 1, Column: 0}},
		{byteOffset: 7, point: Point{Row: 1, Column: 1}},
	}
	for _, tc := range utf8PointTests {
		point, ok := sourceMap.pointForUTF8Byte(tc.byteOffset)
		if !ok {
			t.Fatalf("pointForUTF8Byte(%d) returned !ok", tc.byteOffset)
		}
		if point != tc.point {
			t.Fatalf("pointForUTF8Byte(%d) = %+v, want %+v", tc.byteOffset, point, tc.point)
		}
	}

	for _, unitOffset := range []uint32{0, 1, 3, 4, 5} {
		byteOffset, ok := sourceMap.utf16UnitToByte(unitOffset)
		if !ok {
			t.Fatalf("utf16UnitToByte(%d) returned !ok", unitOffset)
		}
		roundTrip, ok := sourceMap.byteToUTF16Unit(byteOffset)
		if !ok {
			t.Fatalf("byteToUTF16Unit(%d) returned !ok", byteOffset)
		}
		if roundTrip < unitOffset {
			t.Fatalf("round trip for unit %d went backwards to %d via byte %d", unitOffset, roundTrip, byteOffset)
		}
	}
	if _, ok := sourceMap.utf16UnitToByte(2); ok {
		t.Fatal("utf16UnitToByte(2) returned ok for the middle of a surrogate pair")
	}
	for _, byteOffset := range []uint32{2, 3, 4} {
		if _, ok := sourceMap.byteToUTF16Unit(byteOffset); ok {
			t.Fatalf("byteToUTF16Unit(%d) returned ok for the middle of a UTF-8 rune", byteOffset)
		}
	}
}

func TestEncodeUTF16ToUTF8WithMapUnpairedSurrogates(t *testing.T) {
	source := []uint16{'a', 0xD83D, 'b', 0xDE00, 'c'}
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)

	if got, want := string(utf8Source), "a�b�c"; got != want {
		t.Fatalf("UTF-8 source = %q, want %q", got, want)
	}
	for unitOffset := uint32(0); unitOffset <= uint32(len(source)); unitOffset++ {
		byteOffset, ok := sourceMap.utf16UnitToByte(unitOffset)
		if !ok {
			t.Fatalf("utf16UnitToByte(%d) returned !ok", unitOffset)
		}
		roundTrip, ok := sourceMap.byteToUTF16Unit(byteOffset)
		if !ok {
			t.Fatalf("byteToUTF16Unit(%d) returned !ok", byteOffset)
		}
		if roundTrip != unitOffset {
			t.Fatalf("round trip for unit %d = %d via byte %d", unitOffset, roundTrip, byteOffset)
		}
	}
}

func TestUTF16PointsAcrossSurrogateNewline(t *testing.T) {
	source := utf16Units("😀\nβ")
	_, sourceMap := encodeUTF16ToUTF8WithMap(source)

	tests := []struct {
		unit  uint32
		point Point
	}{
		{unit: 0, point: Point{Row: 0, Column: 0}},
		{unit: 2, point: Point{Row: 0, Column: 2}},
		{unit: 3, point: Point{Row: 1, Column: 0}},
		{unit: 4, point: Point{Row: 1, Column: 1}},
	}
	for _, tc := range tests {
		point, ok := sourceMap.pointForUnit(tc.unit)
		if !ok {
			t.Fatalf("pointForUnit(%d) returned !ok", tc.unit)
		}
		if point != tc.point {
			t.Fatalf("pointForUnit(%d) = %+v, want %+v", tc.unit, point, tc.point)
		}
	}
}

func TestIncludedRangesForUTF16ConvertsAndNormalizes(t *testing.T) {
	source := utf16Units("a😀\nbc")
	ranges, ok := IncludedRangesForUTF16(source, []UTF16Range{
		{StartCodeUnit: 5, EndCodeUnit: 6},
		{StartCodeUnit: 0, EndCodeUnit: 1},
		{StartCodeUnit: 4, EndCodeUnit: 5},
	})
	if !ok {
		t.Fatal("IncludedRangesForUTF16 returned !ok")
	}
	want := []Range{
		{StartByte: 0, EndByte: 1, StartPoint: Point{Row: 0, Column: 0}, EndPoint: Point{Row: 0, Column: 1}},
		{StartByte: 6, EndByte: 8, StartPoint: Point{Row: 1, Column: 0}, EndPoint: Point{Row: 1, Column: 2}},
	}
	if len(ranges) != len(want) {
		t.Fatalf("range len = %d, want %d: got %+v", len(ranges), len(want), ranges)
	}
	for i := range want {
		if ranges[i] != want[i] {
			t.Fatalf("range[%d] = %+v, want %+v", i, ranges[i], want[i])
		}
	}
}

func TestIncludedRangesForUTF16RejectsInvalidBoundaries(t *testing.T) {
	source := utf16Units("a😀b")
	if _, ok := IncludedRangesForUTF16(source, []UTF16Range{{StartCodeUnit: 2, EndCodeUnit: 3}}); ok {
		t.Fatal("IncludedRangesForUTF16 accepted a range starting inside a surrogate pair")
	}
	if _, ok := IncludedRangesForUTF16(source, []UTF16Range{{StartCodeUnit: 3, EndCodeUnit: 1}}); ok {
		t.Fatal("IncludedRangesForUTF16 accepted an inverted range")
	}
	if _, err := IncludedRangesForUTF16Bytes([]byte{0x31}, UTF16LittleEndian, nil); !errors.Is(err, ErrInvalidUTF16ByteLength) {
		t.Fatalf("IncludedRangesForUTF16Bytes odd-length error = %v, want ErrInvalidUTF16ByteLength", err)
	}
	if _, err := IncludedRangesForUTF16Bytes(utf16BytesForTest(t, "a😀b", UTF16LittleEndian), UTF16LittleEndian, []UTF16Range{{StartCodeUnit: 2, EndCodeUnit: 3}}); !errors.Is(err, ErrInvalidUTF16Range) {
		t.Fatalf("IncludedRangesForUTF16Bytes invalid range error = %v, want ErrInvalidUTF16Range", err)
	}
}

func TestParserSetIncludedUTF16Ranges(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := utf16Units("1+2\n3+4")
	if ok := parser.SetIncludedUTF16Ranges(source, []UTF16Range{{StartCodeUnit: 0, EndCodeUnit: 3}}); !ok {
		t.Fatal("SetIncludedUTF16Ranges returned false")
	}
	gotRanges := parser.IncludedRanges()
	if len(gotRanges) != 1 {
		t.Fatalf("IncludedRanges len = %d, want 1: %+v", len(gotRanges), gotRanges)
	}
	if got, want := gotRanges[0], (Range{StartByte: 0, EndByte: 3, StartPoint: Point{Row: 0, Column: 0}, EndPoint: Point{Row: 0, Column: 3}}); got != want {
		t.Fatalf("IncludedRanges[0] = %+v, want %+v", got, want)
	}
	tree, err := parser.ParseUTF16(source)
	if err != nil {
		t.Fatalf("ParseUTF16 failed: %v", err)
	}
	if got, want := tree.RootNode().Text(tree.Source()), "1+2"; got != want {
		t.Fatalf("included UTF16 parse text = %q, want %q", got, want)
	}
	if err := parser.SetIncludedUTF16ByteRanges(utf16BytesForTest(t, "1+2", UTF16BigEndian), UTF16BigEndian, []UTF16Range{{StartCodeUnit: 0, EndCodeUnit: 3}}); err != nil {
		t.Fatalf("SetIncludedUTF16ByteRanges failed: %v", err)
	}
	if ok := parser.SetIncludedUTF16Ranges(utf16Units("a😀b"), []UTF16Range{{StartCodeUnit: 2, EndCodeUnit: 3}}); ok {
		t.Fatal("SetIncludedUTF16Ranges accepted an invalid surrogate boundary")
	}
}

func TestQueryUTF16RangeHelpers(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := utf16Units("1+2")
	tree, err := parser.ParseUTF16(source)
	if err != nil {
		t.Fatalf("ParseUTF16 failed: %v", err)
	}
	q, err := NewQuery(`(NUMBER) @number`, lang)
	if err != nil {
		t.Fatalf("NewQuery failed: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), lang, tree.Source())
	if ok := cursor.SetUTF16Range(tree, 2, 3); !ok {
		t.Fatal("SetUTF16Range returned false")
	}
	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("NextMatch returned !ok")
	}
	if got, want := match.Captures[0].Node.Text(tree.Source()), "2"; got != want {
		t.Fatalf("captured text = %q, want %q", got, want)
	}
	rng, ok := match.Captures[0].UTF16Range(tree)
	if !ok {
		t.Fatal("capture UTF16Range returned !ok")
	}
	if got, want := rng.StartCodeUnit, uint32(2); got != want {
		t.Fatalf("capture UTF16 start = %d, want %d", got, want)
	}
	if got, want := rng.EndCodeUnit, uint32(3); got != want {
		t.Fatalf("capture UTF16 end = %d, want %d", got, want)
	}
	if _, ok := cursor.NextMatch(); ok {
		t.Fatal("SetUTF16Range matched more than the right-hand number")
	}

	if ok := q.Exec(tree.RootNode(), lang, tree.Source()).SetUTF16Range(tree, 3, 2); ok {
		t.Fatal("SetUTF16Range accepted inverted range")
	}
}

func TestHighlighterUTF16(t *testing.T) {
	lang := buildArithmeticLanguage()
	hl, err := NewHighlighter(lang, `(NUMBER) @number`)
	if err != nil {
		t.Fatalf("NewHighlighter failed: %v", err)
	}

	ranges := hl.HighlightUTF16(utf16Units("1+2"))
	if len(ranges) != 2 {
		t.Fatalf("HighlightUTF16 len = %d, want 2: %+v", len(ranges), ranges)
	}
	if got, want := ranges[1].StartCodeUnit, uint32(2); got != want {
		t.Fatalf("right range start = %d, want %d", got, want)
	}
	if got, want := ranges[1].EndCodeUnit, uint32(3); got != want {
		t.Fatalf("right range end = %d, want %d", got, want)
	}

	byteRanges, err := hl.HighlightUTF16Bytes(utf16BytesForTest(t, "1+2", UTF16BigEndian), UTF16BigEndian)
	if err != nil {
		t.Fatalf("HighlightUTF16Bytes failed: %v", err)
	}
	if len(byteRanges) != len(ranges) {
		t.Fatalf("HighlightUTF16Bytes len = %d, want %d", len(byteRanges), len(ranges))
	}
}

func TestHighlighterHighlightTreeUTF16(t *testing.T) {
	lang := buildArithmeticLanguage()
	highlighter, err := NewHighlighter(lang, `(NUMBER) @number`)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := NewParser(lang).ParseUTF16(utf16Units("1+2"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	ranges := highlighter.HighlightTreeUTF16(tree)
	if len(ranges) != 2 {
		t.Fatalf("HighlightTreeUTF16 len = %d, want 2: %+v", len(ranges), ranges)
	}
}

func TestHighlighterIncrementalUTF16(t *testing.T) {
	lang := buildArithmeticLanguage()
	hl, err := NewHighlighter(lang, `(NUMBER) @number`)
	if err != nil {
		t.Fatalf("NewHighlighter failed: %v", err)
	}

	oldSource := utf16Units("1+2")
	_, oldTree := hl.HighlightIncrementalUTF16(oldSource, nil)
	newSource := utf16Units("1+3")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}
	ranges, newTree := hl.HighlightIncrementalUTF16(newSource, oldTree)
	if newTree == nil {
		t.Fatal("HighlightIncrementalUTF16 returned nil tree")
	}
	if len(ranges) != 2 {
		t.Fatalf("HighlightIncrementalUTF16 len = %d, want 2: %+v", len(ranges), ranges)
	}
	if got, want := ranges[1].StartCodeUnit, uint32(2); got != want {
		t.Fatalf("incremental right range start = %d, want %d", got, want)
	}
}

func TestTaggerUTF16(t *testing.T) {
	lang := buildArithmeticLanguage()
	tagger, err := NewTagger(lang, `(NUMBER) @name @definition.number`)
	if err != nil {
		t.Fatalf("NewTagger failed: %v", err)
	}

	tags := tagger.TagUTF16(utf16Units("1+2"))
	if len(tags) != 2 {
		t.Fatalf("TagUTF16 len = %d, want 2: %+v", len(tags), tags)
	}
	if got, want := tags[1].Name, "2"; got != want {
		t.Fatalf("right tag name = %q, want %q", got, want)
	}
	if got, want := tags[1].NameRange.StartCodeUnit, uint32(2); got != want {
		t.Fatalf("right tag name start = %d, want %d", got, want)
	}

	byteTags, err := tagger.TagUTF16Bytes(utf16BytesForTest(t, "1+2", UTF16LittleEndian), UTF16LittleEndian)
	if err != nil {
		t.Fatalf("TagUTF16Bytes failed: %v", err)
	}
	if len(byteTags) != len(tags) {
		t.Fatalf("TagUTF16Bytes len = %d, want %d", len(byteTags), len(tags))
	}
}

func TestTaggerIncrementalUTF16(t *testing.T) {
	lang := buildArithmeticLanguage()
	tagger, err := NewTagger(lang, `(NUMBER) @name @definition.number`)
	if err != nil {
		t.Fatalf("NewTagger failed: %v", err)
	}

	oldSource := utf16Units("1+2")
	_, oldTree := tagger.TagIncrementalUTF16(oldSource, nil)
	newSource := utf16Units("1+4")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}
	tags, newTree := tagger.TagIncrementalUTF16(newSource, oldTree)
	if newTree == nil {
		t.Fatal("TagIncrementalUTF16 returned nil tree")
	}
	if len(tags) != 2 {
		t.Fatalf("TagIncrementalUTF16 len = %d, want 2: %+v", len(tags), tags)
	}
	if got, want := tags[1].Name, "4"; got != want {
		t.Fatalf("incremental right tag name = %q, want %q", got, want)
	}
}

func TestParseUTF16ArithmeticPreservesUTF16Metadata(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := utf16Units("1+2")

	tree, err := parser.ParseUTF16(source)
	if err != nil {
		t.Fatalf("ParseUTF16 failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("ParseUTF16 returned nil root")
	}
	if tree.SourceEncoding() != InputEncodingUTF16 {
		t.Fatalf("SourceEncoding = %s, want utf16", tree.SourceEncoding())
	}
	if got := string(tree.Source()); got != "1+2" {
		t.Fatalf("Source = %q, want %q", got, "1+2")
	}
	if got := tree.SourceUTF16(); len(got) != len(source) || got[0] != source[0] || got[2] != source[2] {
		t.Fatalf("SourceUTF16 = %v, want %v", got, source)
	}

	rng, ok := tree.UTF16RangeForNode(root)
	if !ok {
		t.Fatal("UTF16RangeForNode(root) returned !ok")
	}
	if got, want := rng.StartCodeUnit, uint32(0); got != want {
		t.Fatalf("root UTF16 start = %d, want %d", got, want)
	}
	if got, want := rng.EndCodeUnit, uint32(3); got != want {
		t.Fatalf("root UTF16 end = %d, want %d", got, want)
	}
	if got, want := rng.EndPoint, (Point{Row: 0, Column: 3}); got != want {
		t.Fatalf("root UTF16 end point = %+v, want %+v", got, want)
	}

	num := root.Child(2)
	if num == nil || num.Text(tree.Source()) != "2" {
		t.Fatalf("right child text = %q, want %q", num.Text(tree.Source()), "2")
	}
	numRange, ok := tree.UTF16RangeForNode(num)
	if !ok {
		t.Fatal("UTF16RangeForNode(num) returned !ok")
	}
	if got, want := numRange.StartCodeUnit, uint32(2); got != want {
		t.Fatalf("num UTF16 start = %d, want %d", got, want)
	}
	if got, want := numRange.EndCodeUnit, uint32(3); got != want {
		t.Fatalf("num UTF16 end = %d, want %d", got, want)
	}

	numSource, ok := tree.UTF16SourceForNode(num)
	if !ok {
		t.Fatal("UTF16SourceForNode(num) returned !ok")
	}
	assertUTF16UnitsEqual(t, numSource, utf16Units("2"))
}

func TestParseUTF16Bytes(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	for _, order := range []UTF16ByteOrder{UTF16LittleEndian, UTF16BigEndian} {
		t.Run(order.String(), func(t *testing.T) {
			tree, err := parser.ParseUTF16Bytes(utf16BytesForTest(t, "1+2", order), order)
			if err != nil {
				t.Fatalf("ParseUTF16Bytes failed: %v", err)
			}
			if got, want := tree.SourceEncoding(), InputEncodingUTF16; got != want {
				t.Fatalf("SourceEncoding = %s, want %s", got, want)
			}
			if got, want := string(tree.Source()), "1+2"; got != want {
				t.Fatalf("Source = %q, want %q", got, want)
			}
			assertUTF16UnitsEqual(t, tree.SourceUTF16(), utf16Units("1+2"))
			freshTree, err := parser.ParseUTF16(utf16Units("1+2"))
			if err != nil {
				t.Fatalf("fresh ParseUTF16 failed: %v", err)
			}
			if got, want := tree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("ParseUTF16Bytes SExpr = %q, want %q", got, want)
			}
		})
	}
}

func TestParseUTF16WithTokenSourceFactory(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	baseFactory := dfaTokenSourceFactoryForTest(t, parser)
	var factoryInputs []string
	factory := func(source []byte) (TokenSource, error) {
		factoryInputs = append(factoryInputs, string(source))
		return baseFactory(source)
	}

	source := utf16Units("1+2")
	tree, err := parser.ParseUTF16WithTokenSourceFactory(source, factory)
	if err != nil {
		t.Fatalf("ParseUTF16WithTokenSourceFactory failed: %v", err)
	}
	freshTree, err := parser.ParseUTF16(source)
	if err != nil {
		t.Fatalf("fresh ParseUTF16 failed: %v", err)
	}
	if len(factoryInputs) == 0 || factoryInputs[0] != "1+2" {
		t.Fatalf("factory inputs = %v, want first input %q", factoryInputs, "1+2")
	}
	if got, want := tree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("ParseUTF16WithTokenSourceFactory SExpr = %q, want %q", got, want)
	}
	assertUTF16UnitsEqual(t, tree.SourceUTF16(), source)
}

func TestParseUTF16BytesWithTokenSourceFactory(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	factory := dfaTokenSourceFactoryForTest(t, parser)

	tree, err := parser.ParseUTF16BytesWithTokenSourceFactory(utf16BytesForTest(t, "1+2", UTF16LittleEndian), UTF16LittleEndian, factory)
	if err != nil {
		t.Fatalf("ParseUTF16BytesWithTokenSourceFactory failed: %v", err)
	}
	if got, want := tree.RootNode().Text(tree.Source()), "1+2"; got != want {
		t.Fatalf("UTF16 byte factory root text = %q, want %q", got, want)
	}
	assertUTF16UnitsEqual(t, tree.SourceUTF16(), utf16Units("1+2"))
}

func TestParseIncrementalUTF16WithEdit(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	oldSource := utf16Units("1+2")
	oldTree, err := parser.ParseUTF16(oldSource)
	if err != nil {
		t.Fatalf("ParseUTF16 old failed: %v", err)
	}

	newSource := utf16Units("1+3")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}

	incrTree, err := parser.ParseIncrementalUTF16(newSource, oldTree)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16 failed: %v", err)
	}
	freshTree, err := parser.ParseUTF16(newSource)
	if err != nil {
		t.Fatalf("fresh ParseUTF16 failed: %v", err)
	}

	if got, want := incrTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental UTF16 SExpr mismatch:\n  got:  %s\n  want: %s", got, want)
	}
	if got, want := incrTree.RootNode().Text(incrTree.Source()), "1+3"; got != want {
		t.Fatalf("incremental UTF16 root text = %q, want %q", got, want)
	}
}

func TestInputEditForUTF16MeasuresReplacementSegment(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	oldTree, err := parser.ParseUTF16(utf16Units("1+2"))
	if err != nil {
		t.Fatalf("ParseUTF16 old source failed: %v", err)
	}

	inputEdit, ok := oldTree.InputEditForUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 5,
	}, utf16Units("1+3\n4"))
	if !ok {
		t.Fatal("InputEditForUTF16 returned !ok")
	}
	want := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  5,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 1, Column: 1},
	}
	if inputEdit != want {
		t.Fatalf("InputEditForUTF16 = %+v, want %+v", inputEdit, want)
	}

	inputEdit, ok = oldTree.InputEditForUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 4,
	}, utf16Units("1+😀"))
	if !ok {
		t.Fatal("InputEditForUTF16 surrogate replacement returned !ok")
	}
	if got, want := inputEdit.NewEndByte, uint32(6); got != want {
		t.Fatalf("surrogate replacement NewEndByte = %d, want %d", got, want)
	}
	if got, want := inputEdit.NewEndPoint, (Point{Row: 0, Column: 6}); got != want {
		t.Fatalf("surrogate replacement NewEndPoint = %+v, want %+v", got, want)
	}
}

func TestDescendantForUTF16Range(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := utf16Units("1+2")
	tree, err := parser.ParseUTF16(source)
	if err != nil {
		t.Fatalf("ParseUTF16 failed: %v", err)
	}

	if got := tree.RootNode().DescendantForByteRange(0, uint32(len(source))*2); got != nil {
		t.Fatalf("DescendantForByteRange with UTF16 byte offsets = %s, want nil", got.Type(lang))
	}

	root := tree.DescendantForUTF16Range(0, uint32(len(source)))
	if root == nil {
		t.Fatal("DescendantForUTF16Range returned nil for full source")
	}
	if got, want := root.Type(lang), "expression"; got != want {
		t.Fatalf("full-source descendant type = %q, want %q", got, want)
	}

	number := tree.NamedDescendantForUTF16Range(2, 3)
	if number == nil {
		t.Fatal("NamedDescendantForUTF16Range returned nil for right-hand number")
	}
	if got, want := number.Type(lang), "NUMBER"; got != want {
		t.Fatalf("named descendant type = %q, want %q", got, want)
	}
	if got, want := number.Text(tree.Source()), "2"; got != want {
		t.Fatalf("named descendant text = %q, want %q", got, want)
	}

	if got := tree.DescendantForUTF16Range(3, 2); got != nil {
		t.Fatalf("DescendantForUTF16Range accepted inverted range: %s", got.Type(lang))
	}
	if got := tree.DescendantForUTF16Range(0, uint32(len(source)+1)); got != nil {
		t.Fatalf("DescendantForUTF16Range accepted out-of-range end: %s", got.Type(lang))
	}

	utf8Tree, err := parser.Parse([]byte("1+2"))
	if err != nil {
		t.Fatalf("Parse UTF8 failed: %v", err)
	}
	if got := utf8Tree.DescendantForUTF16Range(0, 3); got != nil {
		t.Fatalf("DescendantForUTF16Range worked on UTF8 tree: %s", got.Type(lang))
	}
}

func TestEditUTF16RejectsInvalidBoundaries(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())

	surrogateSource := utf16Units("1+😀")
	surrogateTree, err := parser.ParseUTF16(surrogateSource)
	if err != nil {
		t.Fatalf("ParseUTF16 surrogate source failed: %v", err)
	}
	if ok := surrogateTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  3,
		OldEndCodeUnit: 4,
		NewEndCodeUnit: 4,
	}, surrogateSource); ok {
		t.Fatal("EditUTF16 accepted an old-source offset inside a surrogate pair")
	}

	oldSource := utf16Units("1+2")
	oldTree, err := parser.ParseUTF16(oldSource)
	if err != nil {
		t.Fatalf("ParseUTF16 old source failed: %v", err)
	}
	newSource := utf16Units("1+😀")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); ok {
		t.Fatal("EditUTF16 accepted a new-source offset inside a surrogate pair")
	}
}

func TestParseIncrementalUTF16BytesWithEdit(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	oldSource := utf16Units("1+2")
	oldTree, err := parser.ParseUTF16(oldSource)
	if err != nil {
		t.Fatalf("ParseUTF16 old failed: %v", err)
	}

	newSource := utf16Units("1+4")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}

	incrTree, err := parser.ParseIncrementalUTF16Bytes(utf16BytesForTest(t, "1+4", UTF16BigEndian), oldTree, UTF16BigEndian)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16Bytes failed: %v", err)
	}
	if got, want := incrTree.RootNode().Text(incrTree.Source()), "1+4"; got != want {
		t.Fatalf("incremental UTF16 byte root text = %q, want %q", got, want)
	}
	assertUTF16UnitsEqual(t, incrTree.SourceUTF16(), newSource)
}

func TestParseIncrementalUTF16WithTokenSourceFactory(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	factory := dfaTokenSourceFactoryForTest(t, parser)

	oldSource := utf16Units("1+2")
	oldTree, err := parser.ParseUTF16WithTokenSourceFactory(oldSource, factory)
	if err != nil {
		t.Fatalf("ParseUTF16WithTokenSourceFactory old failed: %v", err)
	}

	newSource := utf16Units("1+5")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}

	incrTree, err := parser.ParseIncrementalUTF16WithTokenSourceFactory(newSource, oldTree, factory)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16WithTokenSourceFactory failed: %v", err)
	}
	freshTree, err := parser.ParseUTF16WithTokenSourceFactory(newSource, factory)
	if err != nil {
		t.Fatalf("fresh ParseUTF16WithTokenSourceFactory failed: %v", err)
	}
	if got, want := incrTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental UTF16 factory SExpr mismatch:\n  got:  %s\n  want: %s", got, want)
	}
	assertUTF16UnitsEqual(t, incrTree.SourceUTF16(), newSource)
}

func TestParseUTF16WithTokenSourceFactoryRejectsNilFactory(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	if _, err := parser.ParseUTF16WithTokenSourceFactory(utf16Units("1+2"), nil); !errors.Is(err, ErrNoTokenSourceFactory) {
		t.Fatalf("nil UTF16 token source factory error = %v, want ErrNoTokenSourceFactory", err)
	}
}

func TestParseWithTokenSourceRejectsNilTokenSource(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	if _, err := parser.ParseWithTokenSource([]byte("1+2"), nil); !errors.Is(err, ErrNoTokenSource) {
		t.Fatalf("nil token source error = %v, want ErrNoTokenSource", err)
	}
	if _, err := parser.ParseWithTokenSourceFactory([]byte("1+2"), func([]byte) (TokenSource, error) {
		return nil, nil
	}); !errors.Is(err, ErrNoTokenSource) {
		t.Fatalf("nil factory token source error = %v, want ErrNoTokenSource", err)
	}
}
