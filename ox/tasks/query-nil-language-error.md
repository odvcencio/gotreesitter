# Return an error for a nil query language

Repository: `github.com/odvcencio/gotreesitter`

Exact base: `f285a87e68926f767979b8824350367eec542a9c`

License evidence: the repository root contains an MIT `LICENSE` file. Its
SHA-256 is `b174fbe1e1cffafb096528de3e8361c90a0d97e7e3f1a32061aae51e983a7cfc`.

## Defect

`NewQuery` and `NewQueryWithOptions` return `(*Query, error)`. A caller that
passes a nil language receives a process panic instead of an error. The panic
starts in `(*Language).buildSymbolMaps` after query parsing resolves the first
node type.

This focused probe fails on the exact base:

```go
func TestProbeNewQueryRejectsNilLanguage(t *testing.T) {
	if _, err := NewQuery(`(identifier) @name`, nil); err == nil {
		t.Fatal("NewQuery returned no error for a nil language")
	}
}
```

Observed result:

```text
panic: runtime error: invalid memory address or nil pointer dereference
github.com/odvcencio/gotreesitter.(*Language).buildSymbolMaps
github.com/odvcencio/gotreesitter.(*queryParser).resolveSymbol
github.com/odvcencio/gotreesitter.NewQueryWithOptions
github.com/odvcencio/gotreesitter.NewQuery
```

`NewTagger` also delegates to `NewQuery`, so the constructor guard protects
that path without a tagger-specific change.

## Required behavior

- Make `NewQuery(source, nil)` return a non-nil error and no query.
- Make `NewQueryWithOptions(source, nil, opts...)` return a non-nil error and
  no query.
- Do not recover panics. Reject the invalid argument before parsing starts.
- Preserve all behavior for a non-nil language.
- Add one focused table-driven regression in `query_test.go` that calls both
  public constructors.
- Do not require an exact error string in the test. Require only a non-nil
  error and a nil query.
- Touch only `query.go` and `query_test.go`.
- Do not change query matching, predicates, capture storage, ranges, UTF-16,
  highlighting, injection parsing, parser code, or generated files.

## Relevant production source

From `query.go`:

```go
// NewQuery compiles query source (tree-sitter .scm format) against a language.
// It returns an error if the query syntax is invalid or references unknown
// node types or field names.
//
// NewQuery is a thin call to NewQueryWithOptions with no options; its
// signature and behavior are unchanged from before NewQueryWithOptions
// existed. See NewQueryWithOptions to opt into WithStrictPatternValidation.
func NewQuery(source string, lang *Language) (*Query, error) {
	return NewQueryWithOptions(source, lang)
}

// NewQueryWithOptions compiles query source the same way NewQuery does, plus
// any QueryOptions. opts is empty in every existing call site through
// NewQuery and defaults to NewQuery's exact behavior; see
// WithStrictPatternValidation for the one option currently defined.
func NewQueryWithOptions(source string, lang *Language, opts ...QueryOption) (*Query, error) {
	var cfg queryCompileOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	p := &queryParser{
		input: source,
		lang:  lang,
		q: &Query{
			captures: []string{},
		},
	}
	if err := p.parse(); err != nil {
		return nil, err
	}
	p.q.buildAlternationIndices()
	p.q.buildRootPatternIndex()
	if cfg.strictPatternValidation {
		if issues := ValidateQueryPatterns(lang, p.q); len(issues) > 0 {
			return nil, fmt.Errorf("%s", issues[0].String())
		}
	}
	return p.q, nil
}
```

`query.go` already imports `fmt`.

The test file is in package `gotreesitter`, imports `testing`, and defines the
`queryTestLanguage()` helper. Existing tests use plain `testing` checks. For
example:

```go
func TestParseSimpleNodeType(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @ident`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
}
```

## Output contract

Return only one applicable unified diff. Do not use Markdown fences. Use the
existing filenames `query.go` and `query_test.go`. Do not add a new file. Keep
the patch small. Include enough unchanged context for `git apply`.
