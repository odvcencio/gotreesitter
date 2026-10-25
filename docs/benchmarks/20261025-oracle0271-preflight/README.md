# C 0.27.1 default-API preflight

At source `9580313574e273cd0e920cd120c6afe5dc357c8e`, a private Go-binding adaptation compiles and executes against upstream tree-sitter **0.27.1**, commit `10eb5ac119eec9cb4267a8e035066473983f5f3a`. The bounded Linux amd64 run passes **3 private top-level tests / 12 including subtests**, plus **4 Go top-level tests / 36 including subtests**. Both selections finish with zero failures or skips. This is a compatibility preflight, not oracle adoption.

The private link probe calls `ts_language_is_parseable`, absent from the old runtime. It verifies ABI 15 / minimum ABI 13 and logs the linked runtime, adapted binding and Go grammar identities. The Go cases cover a clean fresh/incremental edit on both routes, malformed declaration prefixes, numeric repair/reuse and an unterminated-string case. The checks compare fresh/incremental Go with C using structural axes, error flags and deep digests.

| Attempt | Result | Explanation |
| --- | --- | --- |
| Initial | Build failed; no tests ran | Go 1.26 rejects overlay replacements beneath `GOMODCACHE`. |
| V2 | Build failed; no tests ran | A second read-only bind moved the same dependency outside the cache, but cgo rejected `.go.txt` backing names. |
| V3 | Build and both test selections passed | Fourteen cgo inputs use their original `.go` basenames. Their bodies are byte-identical to the frozen adaptation; the adjacent allocator header is authenticated. |

V3 does not introduce another runtime patch. Its temporary module file redirects the existing v0.25.0 binding through the second read-only bind. All translation units select the authenticated 0.27.1 headers and amalgamated runtime. Deprecated language-version access uses the retained ABI API. Six removed timeout/cancellation methods and cancellable deprecated `ParseCtx` fail explicitly; seven rejection subtests exercise those cases. This intentionally does not claim complete v0.25 binding API compatibility.

The runtime source archive SHA-256 is `71bbc0a7a98013b519b104e281e6ad98524a438c8c822a095e6f27de05d15562`. Cargo and API-header Git blob hashes are verified against the pinned source; every extracted runtime file is rechecked. Runtime and dependency licenses, all three control sets, exact Docker commands/workers, raw logs, input checks and source manifests are retained. The successful build's before/after canonical source manifests are identical and Git status is clean. The container uses Go 1.26.6, one CPU, 4 GiB memory, `GOMAXPROCS=1`, no network and the shared measurement lock.

The repository's ordinary harness still pins **C 0.25.1** (`f5afe475deb7c0bae6407fb776c76824f717bb61`). Its dependency declarations and runtime metadata are unchanged. The binding module remains v0.25.0; it is the private adaptation that changes the runtime selected for this binary. Original 0.25 sentinels and preparation files in the archive describe their original control scope, not the runtime observed by the successful V3 link probe. Frozen preparation manifests saying unbuilt/unexecuted are preserved; this receipt records the later execution separately.

No 206-grammar qualification, general binding compatibility, other-platform coverage, performance, dependency upgrade, graduation or merge authorization is claimed. Private corpus payloads, caches, shared objects and executables are excluded; only the test binary's hash is retained.

[Verification](verification.json), [source identities](source-identities.json), [evidence archive](evidence.tar.gz) and [archive checksum](evidence.sha256) provide the receipt. To inspect and reproduce, extract the archive into a new directory and verify `payload.sha256`. `reproduce.py` authenticates source 95803135 and prints the V3 Docker command. Supply an existing read-only module cache, the pinned prebuilt Go C grammar cache, a shared runtime directory and a new output directory; use `--execute` only when the shared measurement slot is available:

```sh
python3 reproduce.py --repo /path/to/gotreesitter --module-cache /path/to/go-mod --c-cache /path/to/c-cache --runtime /home/draco/work/.runtime --output /path/to/new-results --cpu 6
```

The precompiled Go grammar must match `f17d674be6e3e7a0edb521defa31d3174c877b01414fd758a4988673423573f7` and grammar commit `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`. Cache misses fail closed; the replay performs no network download or grammar build. Reproduction still requires the public pinned dependencies identified by the source manifests; the archive is an evidence/control bundle, not a complete project or dependency checkout.

The separately queued expanded Go check uses that exact authenticated V3 binary. Its terminal process status is `{"go_parity_query_exit": 1, "go_regression_exit": 1}`, with 112 passing test records, 3 failing records and 0 skipped records. The expanded check does not qualify; its failures, skips or incomplete output remain in the archive. It covers additional Go separator/recovery/reuse regressions and the selected Go fresh, incremental, no-error, highlight and query tests. This remains Go-only partial coverage, with no fleet or general oracle-adoption claim. Add `--expanded-go` to the replay command to run those same selectors after the preflight.

The expanded regression selection has 76 PASS records and one FAIL: `TestGoNextLiveArmProbe` stops at its original C0.25.1 identity sentinel before executing its witnesses. The actual C0.27.1 identity is correct for this private binary. The parity/query selection has 36 PASS records and two FAIL records (one parent and its child): `(_simple_type/type_identifier) @t` reports matching captures, which trips the strict requirement to remove its prior known-divergence annotation. Both failures remain unchanged. The cause of the query expectation change is not attributed to the new C version without a matched old-C comparison. These results extend the retained evidence but do not turn the expanded suite green.

The supplemental V4 Go qualification now passes **20 top-level tests / 121 records including subtests**, with zero failures or skips: 14 / 83 in the regression selection and 6 / 38 in the parity/highlight/query selection. The source remains 95803135 and the V3 binding/runtime adaptation is unchanged. A new binary is built with only two runtime-identity constants changed to C0.27.1/10eb and the single agreeing Go query's known-divergence note removed. The existing exact compiler/capture comparator, query text, source inputs, grammar/artifact hashes, tree digests and pass assertions remain unchanged. The diff guard proves these are the only test-source changes. This makes equality mandatory for that query; it does not relax a threshold.

The [supplemental verification](go-qualification-verification.json), [source identities](go-qualification-source-identities.json), [Go qualification archive](go-qualification.tar.gz) and [checksum](go-qualification.sha256) retain V4 controls, both raw selections, exact commands/worker, input authentication and binary hash. The original archive/checksum and original verification/source identities remain unchanged, including the earlier three FAIL records. The reason captures now agree is still unattributed without a matched old-C comparison. This is bounded Go coverage, with no oracle adoption, general binding compatibility, fleet, other-platform, performance or full graduation claim.

For reproduction, extract `evidence.tar.gz` first, then `go-qualification.tar.gz` into the same new directory. Verify `payload.sha256` and `go-qualification-payload.sha256`. The supplement preserves the outside-cache binding mount and uses these explicit build flags:

```sh
GOFLAGS='-p=1 -mod=readonly -modfile=/controls/oracle0271-module-bind-v3/go.mod -overlay=/controls/oracle0271-go-pins-v4/overlay.json'
python3 reproduce-go-qualification.py --repo /path/to/gotreesitter --module-cache /path/to/go-mod --c-cache /path/to/c-cache --runtime /home/draco/work/.runtime --output /path/to/new-go-results --cpu 6
```

The replay prints its bounded Docker command; add `--execute` only when ready to acquire the shared measurement lock. Its source, dependency, cache and pinned-Go-artifact prerequisites are the same as the original preflight.
