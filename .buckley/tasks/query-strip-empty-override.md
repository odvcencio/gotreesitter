# Preserve an empty query text override

Return one valid unified diff. Start the response with `diff --git `. Do not add fences, analysis, or alternatives.

Fix one query directive defect. A `#strip!` directive can remove all captured text. The capture stores `""`, but current readers use `""` as the unset sentinel. They then return the original node text.

This focused probe fails on current `main`:

```text
--- FAIL: TestProbeStripEntireCaptureKeepsEmptyOverride (0.00s)
    query_strip_empty_probe_test.go:15: fully stripped capture text = "hello", want empty
```

Use a general override-presence bit. Do not special-case `#strip!` during text reads. Preserve compatibility for callers that assign a non-empty public `TextOverride` directly.

Change only these files:

- `query.go`
- `query_reader.go`
- `query_predicates.go`
- `parsercore_phase0_selected_query.go`
- `query_test.go`

Do not change parser, compact replay, grammar, or campaign code.

The current public capture is:

```go
type QueryCapture struct {
	Name string
	Node *Node
	TextOverride string
}

func (c QueryCapture) Text(source []byte) string {
	if c.TextOverride != "" {
		return c.TextOverride
	}
	if c.Node == nil {
		return ""
	}
	return c.Node.Text(source)
}
```

The generic reader seam and compact capture are:

```go
type queryNodeReader[N comparable, C any] interface {
	// Other methods are unchanged.
	CaptureTextOverride(C) string
	SetCaptureTextOverride(*C, string)
}

type queryValueCapture[N comparable] struct {
	Name         string
	Node         N
	TextOverride string
}
```

The public reader methods are:

```go
func (publicQueryReader) CaptureTextOverride(capture QueryCapture) string {
	return capture.TextOverride
}
func (publicQueryReader) SetCaptureTextOverride(capture *QueryCapture, text string) {
	capture.TextOverride = text
}
```

The selected-store reader has the same methods for `queryValueCapture[core.SelectedNodeID]`:

```go
func (selectedStoreQueryReader) CaptureTextOverride(capture queryValueCapture[core.SelectedNodeID]) string {
	return capture.TextOverride
}
func (selectedStoreQueryReader) SetCaptureTextOverride(capture *queryValueCapture[core.SelectedNodeID], text string) {
	capture.TextOverride = text
}
```

The directive correctly calls the setter even when its result is empty:

```go
func applyStripWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, reader R) []C {
	if pred.regex == nil {
		return captures
	}
	for i := range captures {
		node := reader.CaptureNode(captures[i])
		if reader.CaptureName(captures[i]) != pred.leftCapture || reader.IsNil(node) {
			continue
		}
		text := reader.Text(node, source)
		if stripped := pred.regex.ReplaceAllString(text, ""); stripped != text {
			reader.SetCaptureTextOverride(&captures[i], stripped)
		}
	}
	return captures
}
```

The broken generic read is:

```go
func captureTextWithReader[N comparable, C any, R queryNodeReader[N, C]](name string, captures []C, source []byte, reader R) (string, bool) {
	if source == nil {
		return "", false
	}
	for _, capture := range captures {
		if reader.CaptureName(capture) != name {
			continue
		}
		if override := reader.CaptureTextOverride(capture); override != "" {
			return override, true
		}
		node := reader.CaptureNode(capture)
		if reader.IsNil(node) {
			return "", false
		}
		return reader.Text(node, source), true
	}
	return "", false
}
```

Requirements:

- Record whether an override is present separately from its string value.
- Make both public and selected-store reader specializations preserve an empty override.
- Make `QueryCapture.Text` return an empty directive result.
- Keep direct non-empty `TextOverride` assignments working.
- Keep an unchanged capture reading its original node text.
- Add the failing regression to `query_test.go` near the existing strip tests.
- Add a focused assertion that generic predicate text lookup also observes the empty override.
- Do not export a new API unless it is necessary.
- Do not weaken or delete tests.

Validate with:

```text
go test . -run '^(TestStrip|TestQueryCaptureText|TestDiagnosticParserCoreSelectedStoreQueryDirectiveParity)$' -count=20
go test . -run '^(TestStrip|TestQueryCaptureText)$' -race -count=1
go vet ./...
```

Return only the unified diff.
