# Correct the empty query override contract

Return one valid unified diff. Start with `diff --git `. Do not add fences or prose.

The defect is proven: stripping all of `"hello"` stores `""`, then capture text returns `"hello"`.

The previous proposal was invalid. Its setter did not mark the presence bit. Its separate presence setter was never called. Do not repeat that design.

Change only:

- `query.go`
- `query_reader.go`
- `query_predicates.go`
- `parsercore_phase0_selected_query.go`
- `query_test.go`

Use this exact contract:

1. Add one unexported Boolean presence field to `QueryCapture` and `queryValueCapture`.
2. Change the reader method to `CaptureTextOverride(C) (string, bool)`.
3. Make each reader text setter write both the string and presence field.
4. Make each reader getter return present when the bit is set or the string is non-empty. This preserves direct non-empty public assignments.
5. Make `QueryCapture.Text` use the same rule.
6. Make `captureTextWithReader` return an empty override when present.
7. Do not add a separate presence setter.

Current public methods:

```go
func (c QueryCapture) Text(source []byte) string {
	if c.TextOverride != "" {
		return c.TextOverride
	}
	if c.Node == nil {
		return ""
	}
	return c.Node.Text(source)
}

func (publicQueryReader) CaptureTextOverride(capture QueryCapture) string {
	return capture.TextOverride
}
func (publicQueryReader) SetCaptureTextOverride(capture *QueryCapture, text string) {
	capture.TextOverride = text
}
```

Current selected-store methods:

```go
func (selectedStoreQueryReader) CaptureTextOverride(capture queryValueCapture[core.SelectedNodeID]) string {
	return capture.TextOverride
}
func (selectedStoreQueryReader) SetCaptureTextOverride(capture *queryValueCapture[core.SelectedNodeID], text string) {
	capture.TextOverride = text
}
```

Current broken lookup:

```go
if override := reader.CaptureTextOverride(capture); override != "" {
	return override, true
}
```

Replace that lookup with one tuple read. Do not read the override twice.

Add this regression near the existing strip tests. Use these existing helpers exactly: `leaf`, `mustCompileTestRegex`, `applyStrip`, `captureTextWithReader`, and `publicQueryReader`.

```go
func TestStripEntireCaptureKeepsEmptyOverride(t *testing.T) {
	source := []byte("hello")
	node := leaf(Symbol(1), true, 0, uint32(len(source)))
	predicate := QueryPredicate{
		kind:        predicateStrip,
		leftCapture: "name",
		regex:       mustCompileTestRegex(t, ".+"),
	}
	captures := applyStrip(predicate, []QueryCapture{{Name: "name", Node: node}}, source)
	if got := captures[0].Text(source); got != "" {
		t.Fatalf("fully stripped capture text = %q, want empty", got)
	}
	if got, ok := captureTextWithReader("name", captures, source, publicQueryReader{}); !ok || got != "" {
		t.Fatalf("generic capture text = %q, %v; want empty, true", got, ok)
	}
}
```

Keep all existing tests. Do not change parser, compact replay, grammar, or campaign code.

Validate with:

```text
go test . -run '^(TestStrip|TestQueryCaptureText|TestDiagnosticParserCoreSelectedStoreQueryDirectiveParity)$' -count=20
go test . -run '^(TestStrip|TestQueryCaptureText)$' -race -count=1
go vet ./...
```

Return only the unified diff.
