# compat/smacker

A compatibility shim that lets code written against
[`github.com/smacker/go-tree-sitter`](https://github.com/smacker/go-tree-sitter)
run on the pure-Go [gotreesitter](https://github.com/odvcencio/gotreesitter)
runtime with an **import swap** and no per-node rewrites.

`smacker/go-tree-sitter` is a cgo binding to the C tree-sitter runtime. It has
been unmaintained since August 2024, yet hundreds of modules still depend on
it — including security scanners and language servers that would rather ship a
single static binary that cross-compiles to every `GOOS/GOARCH` without a C
toolchain. This shim is the drop-in path off the dead binding.

## Why a wrapper and not a type alias

smacker's `Node` methods take no language argument because the language is baked
into the C node. gotreesitter threads the `*Language` explicitly for its
pure-Go, arena-allocated nodes. So a straight `type Node = gotreesitter.Node`
re-export is impossible — the method signatures differ. Instead, a `Node` here
carries its owning language (and source), and re-exposes the smacker-shaped,
argument-free API.

## Usage

Replace the import:

```go
// before
import sitter "github.com/smacker/go-tree-sitter"
import "github.com/smacker/go-tree-sitter/golang"

// after
import sitter "github.com/odvcencio/gotreesitter/compat/smacker"
import "github.com/odvcencio/gotreesitter/compat/smacker/golang"
```

The rest of your code — `sitter.ParseCtx`, `sitter.NewQuery`, `NewQueryCursor`,
`node.Type()`, `node.ChildByFieldName(name)`, `node.Content(src)`,
`query.CaptureNameForId(capture.Index)` — is unchanged.

## Covered surface

- Package `ParseCtx(ctx, content, lang) (*Node, error)`.
- Stateful `Parser` (`NewParser`, `SetLanguage`, `ParseCtx`, `Parse`) and `Tree`
  (`RootNode`, `Edit`, `Copy`).
- `Node`: `Type`, `Content`, `String`, `ChildByFieldName`, `FieldNameForChild`,
  `Child`, `NamedChild`, `ChildCount`, `NamedChildCount`, `Parent`, `IsNamed`,
  `IsNull`, `StartByte`, `EndByte`, `StartPoint`, `EndPoint`, `Range`, `Equal`.
- `Query` / `QueryCursor`: `NewQuery`, `NewQueryCursor`, `Exec`, `NextMatch`,
  `CaptureNameForId`, `QueryMatch`, `QueryCapture`.
- Per-grammar subpackages exposing `GetLanguage()`, mirroring smacker's
  layout: `bash`, `c`, `cpp`, `csharp`, `css`, `dockerfile`, `elixir`, `elm`,
  `golang`, `groovy`, `hcl`, `html`, `java`, `javascript`, `kotlin`, `lua`,
  `ocaml`, `php`, `python`, `ruby`, `rust`, `scala`, `sql`, `swift`,
  `typescript/typescript`, `typescript/tsx`.

## Notes

- `Close()` on `Parser`/`Tree`/`Query`/`QueryCursor` is a no-op; gotreesitter
  holds no C resources.
- `IsNull()` is true for both a nil `*Node` and a wrapper around an absent node.
- The included grammar subpackages cover the languages downstream tools use in
  practice; adding another is one file — see any existing subpackage.
