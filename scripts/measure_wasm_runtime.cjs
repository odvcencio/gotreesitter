// Manual, default-off Go-language runtime observations in Node's WASM engine.
// This is not a browser benchmark, native-C comparison, or graduation gate.
// Build ./wasm/runtime with grammar_subset,grammar_subset_go,grammar_blobs_external.
// Use wasm_exec.js from that build's Go toolchain. Example:
// GTS_WASM_PROGRAM_AUDIT=1 GTS_WASM_FUNCTIONS=4500 node scripts/measure_wasm_runtime.cjs \
//   runtime.wasm wasm_exec.js grammars/grammar_blobs/go.bin
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { performance } = require("node:perf_hooks");

if (process.env.GTS_WASM_PROGRAM_AUDIT !== "1") {
  console.error("Set GTS_WASM_PROGRAM_AUDIT=1 to run this optional observation.");
  process.exit(0);
}
if (process.argv.length !== 5) {
  console.error("Usage: node measure_wasm_runtime.cjs runtime.wasm wasm_exec.js go.bin");
  process.exit(2);
}
const functions = Number(process.env.GTS_WASM_FUNCTIONS || 500);
const repetitions = Number(process.env.GTS_WASM_REPETITIONS || 3);
if (!Number.isSafeInteger(functions) || functions < 2 ||
    !Number.isSafeInteger(repetitions) || repetitions < 1) {
  console.error("Function count must be an integer >=2; repetition count must be >=1.");
  process.exit(2);
}
const emit = (kind, fields) => console.log(JSON.stringify({ kind, ...fields }));
const digest = bytes => crypto.createHash("sha256").update(bytes).digest("hex");
let phase = "initialize";
let source = "";

async function run() {
  globalThis.crypto = crypto.webcrypto;
  const [wasmPath, bootstrapPath, blobPath] = process.argv.slice(2).map(p => path.resolve(p));
  const wasm = fs.readFileSync(wasmPath);
  const bootstrap = fs.readFileSync(bootstrapPath);
  const blob = fs.readFileSync(blobPath);
  emit("identity", { node: process.version, functions, repetitions,
    wasm_sha256: digest(wasm), bootstrap_sha256: digest(bootstrap), blob_sha256: digest(blob),
    wasm_bytes: wasm.length, blob_bytes: blob.length });
  require(bootstrapPath);
  const go = new Go();
  phase = "instantiate";
  const started = performance.now();
  const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
  phase = "start";
  go.run(instance).catch(error => { emit("runtime_error", { error: String(error) }); process.exit(1); });
  const readyMS = performance.now() - started;
  const api = globalThis.gotreesitter;
  if (!api) throw new Error("runtime did not publish gotreesitter");
  const checked = result => {
    if (!result || !result.ok) throw new Error(result?.error || "missing result");
    return result;
  };
  phase = "load_blob";
  const loadStart = performance.now();
  checked(api.loadBlob("go", new Uint8Array(blob), "(identifier) @name", ""));
  emit("startup", { ready_ms: readyMS, blob_load_ms: performance.now() - loadStart });

  phase = "small_document";
  const small = "package main\n// 😀\nfunc F() int { return 1 }\n";
  checked(api.open("go", "audit-small", small));
  const edited = small.replace("return 1", "return 2");
  const update = checked(api.update("audit-small", edited));
  const query = checked(api.queryDocument("audit-small", "(function_declaration name: (identifier) @name)"));
  const parsed = checked(api.parse("go", edited));
  const root = JSON.parse(parsed.tree);
  checked(api.close("audit-small"));
  emit("functional", { source_utf16: edited.length, source_utf8: Buffer.byteLength(edited),
    root_end16: root.end16, has_error: parsed.hasError, query_matches: query.matches.length,
    revision: update.revision, edit: update.edit });
  if (root.end16 !== edited.length || parsed.hasError || query.matches.length !== 1) {
    throw new Error("small-document observation differs from the expected coordinates or query count");
  }

  source = "package main\n";
  for (let i = 0; i < functions; i++) source += `func F${i}() int { return ${i} }\n`;
  const next = source.replace(`return ${functions - 1} }\n`, `return ${functions - 2} }\n`);
  if (next === source) throw new Error("generated edit did not change the source");
  emit("source", { source_bytes: Buffer.byteLength(source), source_sha256: digest(source),
    next_source_sha256: digest(next), highlight_query: "(identifier) @name" });
  for (let rep = 0; rep < repetitions; rep++) {
    phase = "open";
    let start = performance.now();
    const opened = checked(api.open("go", "audit-large", source));
    const openMS = performance.now() - start;
    phase = "update";
    start = performance.now();
    const updated = checked(api.update("audit-large", next));
    const updateMS = performance.now() - start;
    phase = "unchanged_update";
    start = performance.now();
    checked(api.update("audit-large", next));
    const unchangedMS = performance.now() - start;
    emit("document", { rep, source_bytes: Buffer.byteLength(source), open_ms: openMS,
      update_ms: updateMS, unchanged_ms: unchangedMS, edit: updated.edit,
      highlight_count: updated.highlights.length, has_error: updated.hasError,
      linear_memory_bytes: instance.exports.mem.buffer.byteLength, process_rss: process.memoryUsage().rss });
    if (opened.hasError || updated.hasError || !updated.edit) throw new Error("generated clean edit failed");
    phase = "close";
    checked(api.close("audit-large"));
  }
  process.exit(0); // The Go runtime intentionally remains active until its host exits.
}

run().catch(error => {
  emit("failure", { phase, source_bytes: Buffer.byteLength(source), source_sha256: digest(source),
    error: String(error), stack: error.stack });
  process.exit(1);
});
