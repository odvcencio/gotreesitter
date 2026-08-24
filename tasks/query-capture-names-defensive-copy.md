# GoTreeSitter task: keep `CaptureNames` caller-owned

Produce one minimal, production-ready unified diff for this repository.

## Evidence-backed defect

`Query` is documented as safe for concurrent execution after construction, but
the public `CaptureNames` accessor returns the backing `q.captures` slice. A
caller can mutate the returned slice and corrupt later public query metadata and
capture names.

At exact base commit `ac90e46ace3c4ac6fb6bbc9f0897e449c949cfad`, this
deterministic probe fails:

```text
--- FAIL: TestProbeCaptureNamesCallerMutationDoesNotAffectQuery (0.00s)
    query_test.go:1135: CaptureNames after caller mutation: got "corrupted" want "ident"
```

The probe compiled `(identifier) @ident`, changed element zero of the slice
returned by `CaptureNames`, then observed both a later `CaptureNames` call and a
normal query match. The existing implementation lets that mutation escape into
the compiled query.

## Required invariant

The slice returned by `CaptureNames` must be caller-owned. Mutating it must not
change:

1. a subsequent `CaptureNames` result;
2. `CaptureNameForID(0)`; or
3. the capture name returned by ordinary query execution.

Preserve existing capture order, deduplication, contents, and all unrelated
behavior. Do not broaden the change into other accessors or alter nil-receiver
semantics.

## Scope

Modify only:

- `query.go`
- `query_test.go`

The expected implementation is small. Add one focused regression beside the
existing capture-name tests and make only the production change required to
establish independent slice ownership. Do not touch parser core, compact parser,
grammars, generated files, campaigns, benchmarks, documentation, or unrelated
formatting.

## Real source context

`query.go` already imports `slices`:

```go
package gotreesitter

import (
	"fmt"
	"regexp"
	"slices"
)
```

The public type contract and storage are:

```go
// Query holds compiled patterns parsed from a tree-sitter .scm query file.
// It can be executed against a syntax tree to find matching nodes and
// return captured names.
//
// Query is safe for concurrent calls to Execute, ExecuteInto, ExecuteNode, and
// Exec after construction. Each goroutine must use its own QueryCursor and any
// ExecuteInto destination slice must remain caller-owned. The mutating methods
// DisableCapture and DisablePattern are NOT safe to call concurrently with
// execution (or with each other); call them before sharing the Query.
type Query struct {
	patterns []Pattern
	captures []string // capture name by index
	strings  []string // string literals by index
	// other fields omitted from this packet
}
```

The accessor block is:

```go
// CaptureCount returns the number of unique capture names in this query.
func (q *Query) CaptureCount() uint32 {
	if q == nil {
		return 0
	}
	return uint32(len(q.captures))
}

// CaptureNames returns the list of unique capture names used in the query.
func (q *Query) CaptureNames() []string {
	return q.captures
}

// CaptureNameForID returns the capture name for the given capture id.
func (q *Query) CaptureNameForID(id uint32) (string, bool) {
	if q == nil || int(id) >= len(q.captures) {
		return "", false
	}
	return q.captures[id], true
}
```

Existing test helpers and the matching tree are available in `query_test.go`:

```go
func queryTestLanguage() *Language
func buildSimpleTree(lang *Language) *Tree
```

`buildSimpleTree` represents `func main() { 42 }` and contains one named
`identifier` node whose source text is `main`.

The existing neighboring test is:

```go
func TestCaptureNames(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`
(identifier) @ident
(number) @number
(true) @bool
`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	names := q.CaptureNames()
	if len(names) != 3 {
		t.Fatalf("CaptureNames: got %d, want 3", len(names))
	}
	expected := []string{"ident", "number", "bool"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("CaptureNames[%d]: got %q, want %q", i, names[i], name)
		}
	}
}
```

Normal execution follows this established shape:

```go
lang := queryTestLanguage()
tree := buildSimpleTree(lang)
q, err := NewQuery(`(identifier) @ident`, lang)
matches := q.Execute(tree)
// matches has one capture whose Name is "ident".
```

## Output contract

Return only a directly applicable unified diff, with normal `diff --git`,
`---`, `+++`, and `@@` headers. No Markdown fences, prose, shell commands,
ellipsis placeholders, new files, or alternate solutions. The diff must be
complete and limited to `query.go` and `query_test.go`.
