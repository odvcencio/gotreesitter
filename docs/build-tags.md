# Build tags and environment

## Grammar embedding modes

**External grammar blobs** (avoid embedding in the binary):

```sh
go build -tags grammar_blobs_external
GOTREESITTER_GRAMMAR_BLOB_DIR=/path/to/blobs  # required
GOTREESITTER_GRAMMAR_BLOB_MMAP=false           # disable mmap (Unix only)
```

**Curated language set** (smaller binary):

```sh
go build -tags grammar_set_core  # curated Core100 embedded grammar set
GOTREESITTER_GRAMMAR_SET=go,json,python  # runtime restriction
```

**Selective embedded grammars** (smallest self-contained binary — pick exactly the languages you ship):

```sh
# Embeds ONLY go.bin + java.bin into the binary (everything else is dropped at
# link time). No GOTREESITTER_GRAMMAR_BLOB_DIR needed — still a single static binary.
go build -tags 'grammar_subset grammar_subset_go grammar_subset_java'
```

Add one `grammar_subset_<lang>` tag per grammar you need (names match the
blob file: `grammar_subset_c_sharp`, `grammar_subset_python`, and so on). A
single-language build drops from ~24MB to a few MB. This is finer-grained
than `grammar_set_core` (a fixed set) and, unlike `grammar_blobs_external`,
it keeps the blobs embedded. Pairing `grammar_subset` with
`grammar_blobs_external` instead loads the selected blobs from
`GOTREESITTER_GRAMMAR_BLOB_DIR` at runtime (no embedded blobs at all).

> The four embedding modes are mutually exclusive at the build-tag level:
> default (all embedded) · `grammar_set_core` (Core100 embedded) ·
> `grammar_subset` + `grammar_subset_<lang>` (selected embedded) ·
> `grammar_blobs_external` (none embedded). Regenerate the per-language embed
> files after adding a grammar with `go run ./cmd/gen_subset_blob_embeds`.

## Grammar cache tuning (long-lived processes)

```go
grammars.SetEmbeddedLanguageCacheLimit(8)    // LRU cap
grammars.UnloadEmbeddedLanguage("rust.bin")  // drop one
grammars.PurgeEmbeddedLanguageCache()        // drop all

fmt.Println(lang.Size())                      // approximate decoded table bytes
```

```sh
GOTREESITTER_GRAMMAR_CACHE_LIMIT=8       # LRU cap via env
GOTREESITTER_GRAMMAR_IDLE_TTL=5m         # evict after idle
GOTREESITTER_GRAMMAR_IDLE_SWEEP=30s      # sweep interval
GOTREESITTER_GRAMMAR_COMPACT=true        # loader compaction (default)
GOTREESITTER_GRAMMAR_STRING_INTERN_LIMIT=200000
GOTREESITTER_GRAMMAR_TRANSITION_INTERN_LIMIT=20000
```

## GLR stack cap override

```sh
GOT_GLR_MAX_STACKS=8  # overrides default GLR stack cap (default: 8)
```

The default is tuned for correctness. Increase it only if a grammar or
workload needs more GLR alternatives to preserve parity.

## Legacy benchmark compatibility only

```sh
GOT_PARSE_NODE_LIMIT_SCALE=3
```

`GOT_PARSE_NODE_LIMIT_SCALE` is needed only for comparisons against older
truncation-prone benchmark baselines. Keep it unset on current branches.

## Parser core build tags

- `gts_no_parsercorephase0` builds the emergency legacy parser path and
  drops the phase0 compact core. CI builds and tests this configuration
  directly.
- `gts_parsercorephase0` and `gts_parsercorephase0_selectedstore_onepass`
  select additional phase0 selected-store and admission behavior; see
  [docs/compact-parser-maintenance.md](compact-parser-maintenance.md).
- `gts_workcount` enables the optional GLR/GSS work-count instrumentation
  used for perf attribution; see
  [docs/perf-attribution.md](perf-attribution.md). It is not part of the
  default or CI-gated build.

See [.github/workflows/ci.yml](../.github/workflows/ci.yml) for the exact
tag combinations CI builds and tests on every PR.
