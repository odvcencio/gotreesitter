# Locked C oracle sources

The separate cgo harness replaces `github.com/tree-sitter/go-tree-sitter`
with `internal/coracle`. The production Go binding sources come from the
commit in `COracleBindingCommit`, without resolving the vanished release tag.
The C runtime sources and headers come independently from the commit in
`COracleRuntimeCommit`. Grammar sources still come from `grammars/languages.lock`.
The root module has no dependency on this module or on cgo.

Run `GOWORK=off python3 cgo_harness/oracle/refresh_sources.py` from the
repository root to reproduce the sources from those commits. The script
fetches each commit, verifies its full hash, copies production binding and
runtime inputs, and applies `binding_compat.patch`. The local module excludes
upstream test grammar dependencies. `upstream.json` records the upstream
runtime tree object IDs, every compiled source hash, and the binding patch
hash. `--verify` checks the local sources without fetching anything.
`TestCOracleVendoredSources` checks the same bytes against the manifest
embedded in the compiled binding. `COracleIdentity` rejects a manifest whose
pins differ from the harness contract and reports its SHA-256.

The binding patch keeps parser timeouts and cancellation through upstream
progress callbacks, which replace the removed C API functions. It also
releases synchronous callback and logger handles, checks the result of
assigning a language, and uses the supported language ABI accessor.
The unused deprecated query timeout methods are omitted; queries retain
their explicit progress callback API. The vendored C runtime stays unpatched. `LICENSE` preserves the binding
notice, and `RUNTIME_LICENSE` preserves the upstream runtime notice.
Diagnostic work-count builds apply their separate versioned instrumentation
patch to a copy of the runtime.
