package gotreesitter

import (
	"errors"
	"testing"
	"time"
)

func TestTaggerTimeoutOptionConfiguresParser(t *testing.T) {
	tagger, err := NewTagger(queryTestLanguage(), "", WithTaggerTimeoutMicros(54_321))
	if err != nil {
		t.Fatal(err)
	}
	if got := tagger.parser.TimeoutMicros(); got != 54_321 {
		t.Fatalf("timeout = %d, want 54321", got)
	}
}

func TestTagIncrementalStrictReportsTimeout(t *testing.T) {
	tagger, err := NewTagger(buildArithmeticLanguage(), "",
		WithTaggerTimeoutMicros(100),
		WithTaggerTokenSourceFactory(func(source []byte) TokenSource {
			return &slowArithmeticTokenSource{
				delay: 2 * time.Millisecond,
				tokens: []Token{
					{Symbol: 1, StartByte: 0, EndByte: 1},
					{Symbol: 0, StartByte: 1, EndByte: 1},
				},
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	tags, tree, err := tagger.TagIncrementalStrict([]byte("1"), nil)
	if tree == nil {
		t.Fatal("strict tagger returned nil partial tree")
	}
	defer tree.Release()
	if !errors.Is(err, ErrParseStoppedEarly) {
		t.Fatalf("error = %v, want ErrParseStoppedEarly", err)
	}
	if got := tree.ParseStopReason(); got != ParseStopTimeout {
		t.Fatalf("stop reason = %q, want %q", got, ParseStopTimeout)
	}
	if tags != nil {
		t.Fatalf("tags = %#v, want nil", tags)
	}
}

func TestTagStrictReportsTimeout(t *testing.T) {
	query := `(expression) @definition.expression`
	tagger, err := NewTagger(buildArithmeticLanguage(), query,
		WithTaggerTimeoutMicros(100),
		WithTaggerTokenSourceFactory(func(source []byte) TokenSource {
			return &slowArithmeticTokenSource{
				delay: 2 * time.Millisecond,
				tokens: []Token{
					{Symbol: 1, StartByte: 0, EndByte: 1},
					{Symbol: 0, StartByte: 1, EndByte: 1},
				},
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	tags, err := tagger.TagStrict([]byte("1"))
	var stopped *ParseStoppedEarlyError
	if !errors.As(err, &stopped) {
		t.Fatalf("error = %v, want ParseStoppedEarlyError", err)
	}
	if stopped.Reason != ParseStopTimeout {
		t.Fatalf("stop reason = %q, want %q", stopped.Reason, ParseStopTimeout)
	}
	if tags != nil {
		t.Fatalf("tags = %#v, want nil", tags)
	}

	// Make query execution observable. A stopped strict parse must return
	// before it reaches tagTree, even when the tag query is non-empty.
	tagger.query = nil
	if tags, err := tagger.TagStrict([]byte("1")); !errors.Is(err, ErrParseStoppedEarly) || tags != nil {
		t.Fatalf("second strict call = tags %#v, err %v; want no tags and ErrParseStoppedEarly", tags, err)
	}

	complete, err := NewTagger(buildArithmeticLanguage(), query)
	if err != nil {
		t.Fatal(err)
	}
	tags, err = complete.TagStrict([]byte("1"))
	if err != nil {
		t.Fatalf("complete strict call error = %v", err)
	}
	if len(tags) != 1 || tags[0].Kind != "definition.expression" || tags[0].Name != "1" {
		t.Fatalf("complete strict tags = %#v, want one expression tag for 1", tags)
	}
}

func TestTagStrictPropagatesParserError(t *testing.T) {
	tagger, err := NewTagger(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if tags, err := tagger.TagStrict([]byte("1")); !errors.Is(err, ErrNoLanguage) || tags != nil {
		t.Fatalf("TagStrict = tags %#v, err %v; want nil tags and ErrNoLanguage", tags, err)
	}
}

func TestTaggerBasic(t *testing.T) {
	lang := queryTestLanguage()

	tagger, err := NewTagger(lang, `
(function_declaration name: (identifier) @name) @definition.function
`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tree := buildSimpleTree(lang)
	tags := tagger.TagTree(tree)

	if len(tags) == 0 {
		t.Fatal("expected tags, got none")
	}

	found := false
	for _, tag := range tags {
		if tag.Kind == "definition.function" {
			found = true
			if tag.Name == "" {
				t.Error("definition.function tag has empty Name")
			}
		}
	}
	if !found {
		t.Errorf("no definition.function tag found in %+v", tags)
	}
}

func TestTaggerEmptySource(t *testing.T) {
	lang := queryTestLanguage()
	tagger, err := NewTagger(lang, `(function_declaration) @definition.function`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tags := tagger.Tag(nil)
	if tags != nil {
		t.Errorf("expected nil for nil source, got %+v", tags)
	}

	tags = tagger.Tag([]byte{})
	if tags != nil {
		t.Errorf("expected nil for empty source, got %+v", tags)
	}
}

func TestTaggerInvalidQuery(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewTagger(lang, `(nonexistent_node) @name @definition.function`)
	if err == nil {
		t.Fatal("expected error for invalid query")
	}
}

func TestTaggerWithTokenSourceFactory(t *testing.T) {
	lang := queryTestLanguage()
	factoryCalled := false
	factory := func(source []byte) TokenSource {
		factoryCalled = true
		return &eofTokenSource{pos: uint32(len(source))}
	}

	tagger, err := NewTagger(lang, `(function_declaration) @definition.function`,
		WithTaggerTokenSourceFactory(factory))
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tagger.Tag([]byte("func main() { 42 }"))
	if !factoryCalled {
		t.Error("expected token source factory to be called")
	}
}

func TestTaggerTagTree(t *testing.T) {
	lang := queryTestLanguage()
	tagger, err := NewTagger(lang, `
(function_declaration) @definition.function
`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tree := buildSimpleTree(lang)
	tags := tagger.TagTree(tree)

	if len(tags) == 0 {
		t.Fatal("expected tags from TagTree, got none")
	}
}

// TestExtractTagUsesOffsetAdjustedNameRange proves extractTag reads the
// capture range accessors, so a query with a #offset! directive reports the
// adjusted range in Tag.NameRange and Tag.Name, not the @name node's own
// unmodified range.
func TestExtractTagUsesOffsetAdjustedNameRange(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	tagger, err := NewTagger(lang, `
((function_declaration name: (identifier) @name) @definition.function
  (#offset! @name 0 1 0 -1))
`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tags := tagger.TagTree(tree)
	if len(tags) != 1 {
		t.Fatalf("tags: got %d, want 1: %+v", len(tags), tags)
	}
	tag := tags[0]

	// The identifier node itself still spans bytes [5,9) ("main"); #offset!
	// shrinks the reported name range to bytes [6,8) ("ai").
	if tag.NameRange.StartByte != 6 || tag.NameRange.EndByte != 8 {
		t.Fatalf("NameRange: got [%d,%d), want [6,8)", tag.NameRange.StartByte, tag.NameRange.EndByte)
	}
	if tag.Name != "ai" {
		t.Fatalf("Name: got %q, want %q", tag.Name, "ai")
	}
}

func TestTaggerIncremental(t *testing.T) {
	lang := queryTestLanguage()
	tagger, err := NewTagger(lang, `(function_declaration) @definition.function`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tags, tree := tagger.TagIncremental([]byte("func main() { 42 }"), nil)
	_ = tags
	if tree == nil {
		t.Fatal("TagIncremental returned nil tree")
	}
}
