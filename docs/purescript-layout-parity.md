# PureScript derive and instance layout

The two reported derive/instance witnesses now match the locked C reference
on both the default and compact routes, including node ranges and fields.
The baseline is `5ff20d9`.

```purescript
derive instance t::F i
instance p::A where p=(x)
i::g
```

```purescript
derive instance u::F t
instance a::L where p(L)=(x)
s::r
```

## Cause and change

The baseline DFA/GLR trace emits `where`, `_layout_start`, and `_layout_end`.
At byte 49 in the first witness, it replaces the zero-width
`_layout_semicolon` with `_varid`: three stacks support the separator and six
support the identifier. The missing separator lets the surviving stacks read
the following declaration as a constraint/type continuation. Preserve
zero-width external transitions before considering a consuming DFA candidate.
The synthetic arbitration test also checks that a consuming external candidate
can still yield to a more specific DFA token.

Removing that guard after correcting the scanner is still wrong: the default
route accepts a tree that absorbs the next signature into the instance body,
and compact incremental parsing differs from fresh parsing. Protect both
layout ends and separators, rather than relying on the absence of errors.

Two tree differences remain after preserving the separator. The shipped blob
lacks `ts_non_terminal_alias_map`, so a singleton function-pattern wrapper loses
`pat_parens`. Regenerate the blob from the same pinned `parser.c` to recover
that metadata. The `_exp_apply`/`_fexp` unary conflict also produces singleton
application wrappers. The existing highest-symbol conflict policy selects the
C singleton reading; its generation profile checks the exact source digest,
state and symbol counts, and unary action shape. The application regression
checks that multiple terms still produce `exp_apply`.

The scanner's final unconditional `MarkEnd()` moves layout ends over the next
newline. Preserve the mark selected by the scan for layout ends. A separator
can consume skipped whitespace to avoid rereading it as newline extras; it
must preserve the earlier mark after speculative content reads. This keeps
the instance end at byte 48 in the first witness and byte 51 in the second.

The pinned scanner source digest is
`819914162317e5714253296f4dae278240d570b2ba39ef687a154277c5baea47`.
The scanner token order and valid-symbol mapping are unchanged. Both ports
serialize the indentation stack as 16-bit values. The C port retains its
stack on an empty deserialization while Go clears it; this change leaves
that behavior untouched. The decisive trace difference is token arbitration,
after the scanner has successfully produced the layout tokens.

## Deterministic measurements

| Witness | C tree matches before / after | Tokens before / after | Nodes before / after | Peak stacks before / after |
| --- | --- | --- | --- | --- |
| 53 bytes | 0 / 1 | 24 / 26 | 417 / 383 | 17 / 16 |
| 56 bytes | 0 / 1 | 27 / 29 | 544 / 477 | 31 / 16 |

Both witnesses contain errors before the change and have no errors afterward.
Both roots cover the complete input. The extra tokens perform the layout work
that the baseline discarded.

For the pinned PureScript ledger fixture:

| Route and phase | Tokens before / after | Nodes before / after | Peak versions before / after |
| --- | --- | --- | --- |
| Default full | 170 / 171 | 413 / 414 | 1 / 1 |
| Candidate full | 170 / 171 | 448 / 449 | 1 / 1 |
| Default edit | 170 / 171 | 448 / 450 | 1 / 1 |
| Candidate edit fallback | 170 / 171 | 448 / 450 | 1 / 1 |

The one-token increase is the final newline, retained to preserve the layout
end boundary. Tokens rise 0.59%; the largest node increase is 0.45%. Reused
bytes and block splices remain zero on this fixture. The unmodified counter
ledger passes all 412 route rows for 206 languages.

## Verification

The grammar regression covers both reported witnesses, an indented `where`
block, and a multi-term application. It checks both routes, declaration end
ranges, zero allocations on no-edit reparses, and incremental/fresh equality
through three edits, including byte ranges, points, child field names, and
node flags.
All eight route/case regression subtests fail on the baseline and pass after
the change.

The PureScript R4 invariant session, focused generator/scanner/token tests,
hot-layout telemetry, runtime-profile count, and merge census baseline pass.
The R6 guard reports its unchanged 454 language-name uses and 101 environment
reads. The L0 guard reports 756 root files and 12 unchanged API tag sets.

The broader R4 matrix passes for 196 languages. AWK, JavaScript, Meson, and
Twig fail with the same incremental/fresh tree digests on the baseline.
Elsa, Godot resource, Haskell, Kotlin, Nickel, and PowerShell exceed the same
two-minute per-language timeout on both revisions. These existing failures
remain; this change does not make the whole invariant matrix green.

The pinned incremental invariant ledger and no-edit allocation tests pass.
Full C tree digests match for all four regression cases on both routes.
`go vet ./...` and the generated-package consistency check pass.

## Randomized performance

The benchmark wrapper ran paired, alternating baseline/current processes for
shuffle seeds 1 through 20, with `GOMAXPROCS=1`, `-count=1`, `-benchmem`,
`-benchtime=750ms`, and its default `gts_parsercorephase0` build tag.
These are the primary Go benchmarks, rather than PureScript latency targets.

| Benchmark | Median time before / after | Change | Bytes/op before / after | Allocs/op before / after |
| --- | --- | --- | --- | --- |
| Full DFA | 8.412 ms / 8.564 ms | +1.81% | 1.245 KiB / 1.245 KiB | 8 / 8 |
| Single-byte edit DFA | 179.3 us / 161.7 us | -9.79% | 388 / 387 | 5 / 5 |
| No-edit DFA | 7.863 ns / 8.236 ns | +4.76% | 0 / 0 | 0 / 0 |

Benchstat reports these time differences as significant with 20 samples per
revision. The full and no-edit directional regressions remain below the 10%
ratchet; retaining valid layout transitions justifies this correctness change.
The edit improvement is measured, without attributing it to a particular
instruction. The campaign completes without crashes or allocation regressions.
Its maximum resident set is 1,947,956 KiB, including compilation and the driver;
that number is not a parser-only large-file memory measurement.

The four pinned corpus files have zero exact C matches on main and one match
afterward (`src/Data/Ordering.purs`). The existing mismatches in
`src/Data/Ring/Generic.purs`, `src/Data/Semigroup/First.purs`, and
`src/Data/Semigroup/Last.purs` remain. Their module-header failures precede the
derive/instance layout witnesses; this receipt does not graduate PureScript.

## Regeneration

The grammar source is `postsolar/tree-sitter-purescript` at
`f541f95ffd6852fbbe88636317c613285bc105af`. The pinned `parser.c` digest is
`1d35f0257ef3d9edcd8964ae2762a6e1e830f19d8a8142c4e07e4d237f4f4957`.
Run `cmd/ts2go` on that file and copy its generated PureScript blob into
`grammars/grammar_blobs/purescript.bin`, then run `cmd/gen_grammar_packages`.
The resulting blob digest is
`96cd0da0020f2187bc7a791df0d18ef9f9551b268cdb0903604a35e4580d04c2`.
