# Return an error only when a nil language must resolve query names

Repository: `github.com/odvcencio/gotreesitter`

Exact base: `74543caa8bb894d9ede163ec8f6625bb266ae788`

License evidence: the repository root contains an MIT `LICENSE` file. Its
SHA-256 is `b174fbe1e1cffafb096528de3e8361c90a0d97e7e3f1a32061aae51e983a7cfc`.

## Correction reason

The first response added an unconditional nil-language guard to
`NewQueryWithOptions`. Its constructor regression passed, but the full test
gate found an existing compatibility contract:

```go
func TestTagStrictPropagatesParserError(t *testing.T) {
	tagger, err := NewTagger(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if tags, err := tagger.TagStrict([]byte("1")); !errors.Is(err, ErrNoLanguage) || tags != nil {
		t.Fatalf("TagStrict = tags %#v, err %v; want nil tags and ErrNoLanguage", tags, err)
	}
}
```

The first patch made this test fail during `NewTagger` construction:

```text
--- FAIL: TestTagStrictPropagatesParserError
    tagger_test.go:105: NewQueryWithOptions: language is nil
```

An empty query does not need language metadata. It must remain valid with a nil
language. A wildcard such as `(_)` also does not need a symbol lookup. A named
node type such as `(identifier)` does need language metadata. Today that lookup
panics in `(*Language).buildSymbolMaps`.

## Required behavior

- Keep `NewQuery("", nil)` valid.
- Keep the existing `NewTagger(nil, "")` behavior and test valid.
- Do not add an unconditional nil-language guard to `NewQueryWithOptions`.
- Make node-type resolution with a nil language return an error, not a panic.
- Make field-name resolution with a nil language return an error, not a panic.
- Preserve wildcard, `ERROR`, `MISSING`, string, and non-nil-language behavior.
- Add one focused table-driven regression in `query_test.go`.
- The regression must call both `NewQuery` and `NewQueryWithOptions` with the
  source `(identifier) @name` and a nil language.
- Require a nil query and a non-nil error. Do not require exact error text.
- Touch only `query_compile_helpers.go` and `query_test.go`.
- Do not change tagger code, parser code, matching, ranges, UTF-16,
  highlighting, injection parsing, or generated files.

## Relevant constructor source

From `query.go` on the exact base:

```go
func NewQuery(source string, lang *Language) (*Query, error) {
	return NewQueryWithOptions(source, lang)
}

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

## Relevant lookup source

From `query_compile_helpers.go`:

```go
// resolveSymbol looks up a node type name in the language, returning the
// symbol ID and whether it's a named symbol. Uses Language.SymbolByName
// for O(1) lookup.
func (p *queryParser) resolveSymbol(name string) (Symbol, bool, error) {
	if name == "_" {
		return 0, false, nil
	}
	if name == "ERROR" {
		return errorSymbol, true, nil
	}
	// "MISSING" is handled specially by stepFromIdentifierName /
	// stepFromMissingKeyword before resolveSymbol is ever reached for that
	// name; it is a query-language keyword, not a grammar node type.

	sym, ok := p.lang.symbolByNamePreferNamed(name)
	if !ok {
		if idx := strings.LastIndex(name, "/"); idx >= 0 && idx+1 < len(name) {
			if fallback, fallbackOK := p.lang.symbolByNamePreferNamed(name[idx+1:]); fallbackOK {
				sym = fallback
				ok = true
			}
		}
	}
	if !ok {
		return 0, false, queryUnknownNodeTypeError{name: name}
	}
	isNamed := false
	if int(sym) < len(p.lang.SymbolMetadata) {
		isNamed = p.lang.SymbolMetadata[sym].Named
	}
	return sym, isNamed, nil
}

// resolveField looks up a field name in the language with compatibility
// fallbacks for grammar/query naming drift.
func (p *queryParser) resolveField(name string, parentSymbol Symbol, parentSymbolHint Symbol) (FieldID, error) {
	fid, ok := p.lang.FieldByName(name)
	if ok {
		return fid, nil
	}

	seenSymbols := map[Symbol]struct{}{}
	for _, sym := range []Symbol{parentSymbol, parentSymbolHint} {
		if _, seen := seenSymbols[sym]; seen {
			continue
		}
		seenSymbols[sym] = struct{}{}
		if int(sym) < 0 || int(sym) >= len(p.lang.SymbolNames) {
			continue
		}
		parentName := p.lang.SymbolNames[sym]
		if parentName == "" {
			continue
		}
		if prefixed, found := p.lang.FieldByName(parentName + "_" + name); found {
			return prefixed, nil
		}
	}

	return 0, fmt.Errorf("query: unknown field %q", name)
}
```

`query_compile_helpers.go` already imports `fmt` and `strings`.

The test file is package `gotreesitter` and already imports `testing`. Existing
tests use plain `testing` checks. Add the regression near the first query parse
tests. Do not change `TestTagStrictPropagatesParserError`; it is the existing
compatibility gate.

## Output contract

Return one complete patch against the exact base. Return only an applicable
unified diff. The first bytes must be `diff --git `. Do not use Markdown fences
or commentary. Use the exact existing filenames. Include complete hunk ranges
and enough unchanged context for `git apply`.
