// Optional complete-operation observations in Node, Chromium, and Firefox.
// Build ./wasm/runtime with grammar_subset,grammar_subset_go,grammar_blobs_external.
// Use wasm_exec.js from that build's Go toolchain. Browser hosts need Playwright.
// GTS_WASM_PROGRAM_AUDIT=1 GTS_WASM_HOST=node node scripts/measure_wasm_runtime.cjs \
//   runtime.wasm wasm_exec.js grammars/grammar_blobs/go.bin
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const zlib = require("node:zlib");

function functionSource(functions) {
  let source = "package main\n";
  for (let i = 0; i < functions; i++) source += `func F${i}() int { return ${i} }\n`;
  return source;
}
function sizedSource(bytes, deep = false) {
  let source;
  let next;
  let functions = 0;
  if (deep) {
    source = "package main\nvar X = " + "(".repeat(12000) + "1" + ")".repeat(12000) + "\n";
    next = source.replace("1)", "2)");
  } else {
    source = "package main\n";
    while (true) {
      const line = `func F${functions}() int { return ${functions} }\n`;
      if (source.length + line.length > bytes - 3) break;
      source += line;
      functions++;
    }
    next = source.replace(`return ${functions - 1} }\n`, `return ${functions - 2} }\n`);
  }
  const padding = "//" + " ".repeat(bytes - source.length - 3) + "\n";
  return { source: source + padding, next: next + padding, functions };
}
const pinnedSources = {
  "functions-64k": ["787a5f02172d7d89be5f339e9653c3785e28d4004520489a5320dccb54323da5", "7657c34e622332ad1d0a7128c4717006dffb9b56ce48e1d20b3feb8eb9f4d9f8"],
  "functions-4500": ["a3e6f63a616dfb7c9ee1b739c9082e49066fd00d494fc6691cc4b713efdbe833", "1b72a6905f29c3811dc67b751d6359b4a44e75719eda77f574b2db77519447ee"],
  "functions-1m": ["66e6e2d95fc2b124fe7b5050dc3228d1ea01c79d10c47e47118a230a6146861d", "55ad4e2a55248392bf829aa7945615fb9f77d92a79d1e6a6441236083019b2cd"],
  "nested-1m": ["91f2a2bbd6706b84f381429d4ffa3c71a84170ef00e489f5e2d3d45b816e2717", "4c72e7a70806a9b788186aa5a75b76d07d17e0a4e98a8c5c00f8866df0ded69d"],
};
const digest = bytes => crypto.createHash("sha256").update(bytes).digest("hex");
// Sample the browser process and its descendants together. Linux process RSS
// includes shared pages in each process, so the aggregate is an upper bound.
function processTreeRSS(pid) {
  const pending = [pid];
  const seen = new Set();
  let bytes = 0;
  while (pending.length) {
    const current = pending.pop();
    if (seen.has(current)) continue;
    seen.add(current);
    try {
      const status = fs.readFileSync(`/proc/${current}/status`, "utf8");
      bytes += Number(status.match(/^VmRSS:\s+(\d+)/m)?.[1] || 0) * 1024;
      for (const thread of fs.readdirSync(`/proc/${current}/task`)) {
        const children = fs.readFileSync(`/proc/${current}/task/${thread}/children`, "utf8").trim();
        if (children) pending.push(...children.split(/\s+/).map(Number));
      }
    } catch {} // A browser helper may exit between the two reads.
  }
  return bytes;
}
function workloads() {
  if (process.env.GTS_WASM_FUNCTIONS) {
    const functions = Number(process.env.GTS_WASM_FUNCTIONS);
    if (!Number.isSafeInteger(functions) || functions < 2) throw new Error("invalid function count");
    const source = functionSource(functions);
    return [{ id: `functions-${functions}`, source,
      next: source.replace(`return ${functions - 1} }\n`, `return ${functions - 2} }\n`), functions }];
  }
  return [
    { id: "functions-64k", ...sizedSource(65536) },
    { id: "functions-4500", source: functionSource(4500), next: functionSource(4500).replace("return 4499 }\n", "return 4498 }\n"), functions: 4500 },
    { id: "functions-1m", ...sizedSource(1048576) },
    { id: "nested-1m", ...sizedSource(1048576, true) },
  ];
}

// This function runs unchanged in every host. All timed calls include the
// UTF-16 bridge, parsing, analysis, result conversion, and return to JavaScript.
async function observe({ wasm, blob, cases, repetitions, seed, host }) {
  const emit = (kind, fields) => console.log(JSON.stringify({ kind, host, seed, ...fields }));
  let phase = "instantiate";
  let workload = "startup";
  try {
    const go = new Go();
    const start = performance.now();
    const { instance } = await WebAssembly.instantiate(new Uint8Array(wasm), go.importObject);
    go.run(instance).catch(error => emit("runtime_error", { error: String(error) }));
    const api = globalThis.gotreesitter;
    if (!api) throw new Error("runtime did not publish gotreesitter");
    const checked = result => {
      if (!result || !result.ok) throw new Error(result?.error || "missing result");
      return result;
    };
    const loadStart = performance.now();
    checked(api.loadBlob("go", new Uint8Array(blob), "(identifier) @name", ""));
    emit("startup", { ready_ms: loadStart - start, blob_load_ms: performance.now() - loadStart });
    phase = "small_document";
    const small = "package main\n// 😀\nfunc F() int { return 1 }\n";
    checked(api.open("go", "audit-small", small));
    const edited = small.replace("return 1", "return 2");
    const update = checked(api.update("audit-small", edited));
    const query = checked(api.queryDocument("audit-small", "(function_declaration name: (identifier) @name)"));
    const parsed = checked(api.parse("go", edited));
    const root = JSON.parse(parsed.tree);
    checked(api.close("audit-small"));
    if (root.end16 !== edited.length || parsed.hasError || query.matches.length !== 1 || update.revision !== 2) {
      throw new Error("UTF-16 document coordinates or query count differ");
    }
    emit("functional", { source_utf16: edited.length, root_end16: root.end16, has_error: parsed.hasError, query_matches: query.matches.length });
    let random = seed >>> 0;
    for (let i = cases.length - 1; i > 0; i--) {
      random = (Math.imul(random, 1664525) + 1013904223) >>> 0;
      const j = random % (i + 1);
      [cases[i], cases[j]] = [cases[j], cases[i]];
    }
    for (const entry of cases) {
      workload = entry.id;
      emit("source", { workload, source_bytes: entry.source.length, source_sha256: entry.sha256,
        next_source_sha256: entry.next_sha256, functions: entry.functions });
      for (let rep = 0; rep < repetitions; rep++) {
        phase = "open";
        let start = performance.now();
        const opened = checked(api.open("go", "audit", entry.source));
        const openMS = performance.now() - start;
        phase = "update";
        start = performance.now();
        const updated = checked(api.update("audit", entry.next));
        const updateMS = performance.now() - start;
        phase = "unchanged_update";
        start = performance.now();
        checked(api.update("audit", entry.next));
        const unchangedMS = performance.now() - start;
        if (opened.hasError || updated.hasError || !updated.edit) throw new Error("clean edit failed");
        phase = "query";
        const query = checked(api.queryDocument("audit", "(identifier) @name"));
        if (!query.matches.length) throw new Error("query returned no matches");
        phase = "parse";
        const parsed = checked(api.parse("go", entry.next));
        const root = JSON.parse(parsed.tree);
        if (parsed.hasError || root.start !== 0 || root.end !== entry.next.length || root.end16 !== entry.next.length) {
          throw new Error("fresh parse does not cover the clean input");
        }
        emit("document", { workload, rep, source_bytes: entry.source.length, open_ms: openMS,
          update_ms: updateMS, unchanged_ms: unchangedMS, edit: updated.edit,
          highlight_count: updated.highlights.length, has_error: updated.hasError,
          root_end: root.end, structured_truncated: !!root.truncated,
          query_matches: query.matches.length, query_truncated: query.truncated,
          linear_memory_bytes: instance.exports.mem.buffer.byteLength,
          process_rss: typeof process === "undefined" || typeof process.memoryUsage !== "function" ? null : process.memoryUsage().rss });
        phase = "close";
        checked(api.close("audit"));
        // Independent editor transactions yield to the host between calls.
        // This also lets Go's scheduled callbacks and JS finalizers run.
        await new Promise(resolve => setTimeout(resolve, 0));
      }
    }
    emit("pass", { workloads: cases.length });
    return true;
  } catch (error) {
    emit("failure", { phase, workload, error: String(error), stack: error.stack });
    return false;
  }
}
async function main() {
  if (process.env.GTS_WASM_PROGRAM_AUDIT !== "1") {
    console.error("Set GTS_WASM_PROGRAM_AUDIT=1 to run this optional observation.");
    return;
  }
  if (process.argv.length !== 5) throw new Error("Usage: node measure_wasm_runtime.cjs runtime.wasm wasm_exec.js go.bin");
  const host = process.env.GTS_WASM_HOST || "node";
  const repetitions = Number(process.env.GTS_WASM_REPETITIONS || 1);
  const seed = Number(process.env.GTS_WASM_SEED || 1);
  if (!Number.isSafeInteger(repetitions) || repetitions < 1 || !Number.isSafeInteger(seed)) throw new Error("invalid repetitions or seed");
  const [wasmPath, bootstrapPath, blobPath] = process.argv.slice(2).map(p => path.resolve(p));
  const wasm = fs.readFileSync(wasmPath);
  const bootstrap = fs.readFileSync(bootstrapPath);
  const blob = fs.readFileSync(blobPath);
  let cases = workloads();
  if (process.env.GTS_WASM_CASE) cases = cases.filter(entry => entry.id === process.env.GTS_WASM_CASE);
  if (!cases.length) throw new Error("no workload selected");
  for (const entry of cases) {
    entry.sha256 = digest(entry.source);
    entry.next_sha256 = digest(entry.next);
    const pin = pinnedSources[entry.id];
    if (pin && (pin[0] !== entry.sha256 || pin[1] !== entry.next_sha256)) throw new Error(`workload digest differs: ${entry.id}`);
  }
  console.log(JSON.stringify({ kind: "identity", host, node: process.version, repetitions, seed,
    wasm_sha256: digest(wasm), bootstrap_sha256: digest(bootstrap), blob_sha256: digest(blob),
    wasm_bytes: wasm.length, wasm_gzip_bytes: zlib.gzipSync(wasm, { level: 9 }).length,
    bootstrap_gzip_bytes: zlib.gzipSync(bootstrap, { level: 9 }).length,
    blob_bytes: blob.length, blob_gzip_bytes: zlib.gzipSync(blob, { level: 9 }).length }));
  let passed;
  if (host === "node") {
    globalThis.crypto = crypto.webcrypto;
    require(bootstrapPath);
    passed = await observe({ wasm, blob, cases, repetitions, seed, host });
  } else {
    const playwright = require("playwright");
    if (host !== "chromium" && host !== "firefox") throw new Error("host must be node, chromium or firefox");
    const http = require("node:http");
    const server = http.createServer((req, res) => {
      const assets = { "/runtime.wasm": [wasm, "application/wasm"], "/wasm_exec.js": [bootstrap, "text/javascript"], "/go.bin": [blob, "application/octet-stream"] };
      const [body, type] = assets[req.url] || ["<!doctype html><script src='/wasm_exec.js'></script>", "text/html"];
      res.setHeader("Content-Type", type);
      res.end(body);
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    let browser;
    let browserServer;
    let sampleTimer;
    let peakRSS = 0;
    try {
      browserServer = await playwright[host].launchServer({ headless: true });
      const browserPID = browserServer.process().pid;
      const sampleRSS = () => { peakRSS = Math.max(peakRSS, processTreeRSS(browserPID)); };
      sampleRSS();
      sampleTimer = setInterval(sampleRSS, 100);
      browser = await playwright[host].connect(browserServer.wsEndpoint());
      console.log(JSON.stringify({ kind: "browser", host, version: browser.version() }));
      const page = await browser.newPage();
      page.on("console", message => { if (message.type() === "log") console.log(message.text()); });
      page.on("pageerror", error => console.error(String(error)));
      await page.goto(`http://127.0.0.1:${server.address().port}/`);
      // Fetch binary assets inside the browser rather than sending a large JSON array.
      await page.evaluate(async () => {
        globalThis.auditWasm = new Uint8Array(await (await fetch("/runtime.wasm")).arrayBuffer());
        globalThis.auditBlob = new Uint8Array(await (await fetch("/go.bin")).arrayBuffer());
      });
      await page.addScriptTag({ content: `globalThis.observe = ${observe.toString()}` });
      passed = await page.evaluate(args => observe({ ...args, wasm: auditWasm, blob: auditBlob }), { cases, repetitions, seed, host });
      sampleRSS();
      console.log(JSON.stringify({ kind: "browser_memory", host, seed, peak_process_tree_rss_bytes: peakRSS, sampling_ms: 100 }));
    } finally {
      clearInterval(sampleTimer);
      if (browser) await browser.close();
      if (browserServer) await browserServer.close();
      await new Promise(resolve => server.close(resolve));
    }
  }
  process.exit(passed ? 0 : 1); // The Go runtime remains active until its host exits.
}
if (require.main === module) main().catch(error => { console.error(error.stack); process.exit(1); });
module.exports = { functionSource, sizedSource, workloads };
