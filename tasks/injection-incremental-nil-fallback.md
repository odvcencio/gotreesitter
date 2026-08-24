# Task: make nil incremental injection input an initial parse

Return one unified diff and nothing else. Do not use Markdown fences. The
response must end with a newline. Modify only these existing files:

- `injection.go`
- `injection_test.go`

## Repository checkpoint

- Exact clean HEAD: `83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`
- Root license: MIT, `LICENSE` SHA-256
  `b174fbe1e1cffafb096528de3e8361c90a0d97e7e3f1a32061aae51e983a7cfc`
- Baseline focused injection tests pass:
  `go test . -run '^TestInjectionParser' -count=1`

## Deterministic defect

The public byte-oriented API
`(*InjectionParser).ParseIncremental(source, parentLang, oldResult)` panics
when `oldResult == nil`. Its UTF-16 counterpart already treats a nil prior
result as an initial parse. A nil prior result must be accepted by the byte API
with the same fallback semantics.

This probe fails on the checkpoint above with a nil-pointer panic at
`injection.go:215`, where `oldResult.Tree` is dereferenced:

```go
func TestProbeInjectionParserParseIncrementalNilOldResult(t *testing.T) {
	ip := NewInjectionParser()
	ip.RegisterLanguage("container", buildContainerLanguage())

	result, err := ip.ParseIncremental([]byte("[value]"), "container", nil)
	if err != nil {
		t.Fatalf("ParseIncremental with nil old result: %v", err)
	}
	if result == nil || result.Tree == nil {
		t.Fatal("ParseIncremental with nil old result returned no tree")
	}
}
```

Observed failure:

```text
panic: runtime error: invalid memory address or nil pointer dereference
github.com/odvcencio/gotreesitter.(*InjectionParser).ParseIncremental
    injection.go:215
```

## Required invariant

When `oldResult` is nil, `ParseIncremental` must behave as the corresponding
initial `Parse` call for the same source and parent language. It must return a
usable result rather than panic. Preserve all existing behavior for non-nil
prior results, including previous-result release/reuse ownership. Do not add a
new API or special-case tests without fixing the production method itself.

Add a focused regression to `injection_test.go` using the real existing
`buildContainerLanguage` helper. The regression must call `ParseIncremental`
with a nil prior result and assert at least:

- no error;
- a non-nil parent tree/root;
- the returned tree represents the supplied source (use existing Node/Text
  APIs or another direct observable assertion).

## Exact production context: `injection.go`

```go
// Parse parses source as parentLang, then recursively parses injected regions.
func (ip *InjectionParser) Parse(source []byte, parentLang string) (*InjectionResult, error) {
	// Release previous result to allow arena reuse.
	releaseResult(ip.prevResult)
	ip.prevResult = nil

	lang, ok := ip.languages[parentLang]
	if !ok {
		return nil, fmt.Errorf("injection: language %q not registered", parentLang)
	}

	parser := ip.getParser(parentLang, lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("injection: parsing %q: %w", parentLang, err)
	}

	injections, err := ip.findAndParseInjections(source, parentLang, tree, 0)
	if err != nil {
		return nil, err
	}

	ip.prevResult = &InjectionResult{
		Tree:       tree,
		Injections: injections,
	}

	return ip.prevResult, nil
}

// ParseIncremental re-parses after edits, reusing unchanged child trees.
func (ip *InjectionParser) ParseIncremental(source []byte, parentLang string,
	oldResult *InjectionResult) (*InjectionResult, error) {

	// Detach prevResult now; release it after parsing so that oldResult.Tree
	// (which may be the same object) remains valid throughout the parse.
	prev := ip.prevResult
	ip.prevResult = nil
	defer func() {
		releaseResultExcept(prev, ip.prevResult)
	}()

	lang, ok := ip.languages[parentLang]
	if !ok {
		return nil, fmt.Errorf("injection: language %q not registered", parentLang)
	}

	parser := ip.getParser(parentLang, lang)
	newTree, err := parser.ParseIncremental(source, oldResult.Tree)
	if err != nil {
		return nil, fmt.Errorf("injection: incremental parsing %q: %w", parentLang, err)
	}

	// Determine which ranges changed between old and new parent trees.
	changedRanges := DiffChangedRanges(oldResult.Tree, newTree)

	// Re-detect injections from the new parent tree.
	newDetected, err := ip.detectInjections(source, parentLang, newTree)
	if err != nil {
		return nil, err
	}

	// The remainder reuses or reparses child injections, then assigns:
	// ip.prevResult = &InjectionResult{Tree: newTree, Injections: injections}
	// and returns ip.prevResult.
}

// ParseIncrementalUTF16 re-parses UTF-16 source after edits, reusing unchanged
// child trees. Call oldResult.Tree.EditUTF16 before calling this.
func (ip *InjectionParser) ParseIncrementalUTF16(source []uint16, parentLang string,
	oldResult *UTF16InjectionResult) (*UTF16InjectionResult, error) {

	if oldResult == nil {
		return ip.ParseUTF16(source, parentLang)
	}
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)
	byteResult := oldResult.byteResult
	if byteResult == nil {
		byteResult = oldResult.toByteResult()
	}
	result, err := ip.ParseIncremental(utf8Source, parentLang, byteResult)
	if err != nil {
		return nil, err
	}
	attachUTF16SourceToInjectionResult(result, source, sourceMap)
	return ip.utf16InjectionResult(result)
}
```

## Existing regression style: `injection_test.go`

`injection_test.go` is in package `gotreesitter`, imports `testing`, and already
provides `buildContainerLanguage()`. Existing tests construct and register it
like this:

```go
func TestInjectionParserEmptySource(t *testing.T) {
	parentLang := buildContainerLanguage()
	ip := NewInjectionParser()
	ip.RegisterLanguage("container", parentLang)
	// ...
}
```

Keep the patch narrow. Do not modify parser core, grammar blobs, generated
files, documentation, task files, or unrelated behavior.
