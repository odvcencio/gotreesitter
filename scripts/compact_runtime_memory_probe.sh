#!/usr/bin/env bash
# Run inside the bounded parity container. Each invocation is a fresh process.
# Usage: bash scripts/compact_runtime_memory_probe.sh PARSERS [go|python] [FUNCTIONS]
set -euo pipefail

parsers=${1:-1}
language=${2:-go}
functions=${3:-500}
if [[ ! "$parsers" =~ ^[1-9][0-9]*$ || ! "$functions" =~ ^[1-9][0-9]*$ ]]; then
  echo 'PARSERS and FUNCTIONS must be positive integers' >&2
  exit 2
fi
case "$language" in go|python) ;; *) echo 'language must be go or python' >&2; exit 2 ;; esac

probe_dir=$(mktemp -d)
trap 'rm -rf "$probe_dir"' EXIT
cat > "$probe_dir/main.go" <<'GO'
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "runtime"
    "strconv"
    "strings"

    ts "github.com/odvcencio/gotreesitter"
    "github.com/odvcencio/gotreesitter/grammars"
)

func main() {
    n, _ := strconv.Atoi(os.Args[1])
    name := os.Args[2]
    count, _ := strconv.Atoi(os.Args[3])
    var source strings.Builder
    var lang *ts.Language
    if name == "go" {
        lang = grammars.GoLanguage()
        source.WriteString("package p\n")
    } else {
        lang = grammars.PythonLanguage()
    }
    for i := 0; i < count; i++ {
        if name == "go" {
            fmt.Fprintf(&source, "func f%d() int { v := %d; return v }\n", i, i)
        } else {
            fmt.Fprintf(&source, "def f%d():\n    v = %d\n    return v\n\n", i, i)
        }
    }
    input := []byte(source.String())
    parsers := make([]*ts.Parser, n)
    for i := range parsers {
        p := ts.NewParser(lang)
        p.SetAdmissionCandidateRoute(true)
        parsers[i] = p
        tree, err := p.Parse(input)
        if err != nil { panic(err) }
        if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(input)) {
            panic("incomplete tree")
        }
        tree.Release()
    }
    runtime.GC()
    runtime.GC()
    var memory runtime.MemStats
    runtime.ReadMemStats(&memory)
    routes, fallbacks := ts.AdmissionCandidateCounters()
    if routes != uint64(n) || fallbacks != 0 {
        panic(fmt.Sprintf("routes=%d fallbacks=%d", routes, fallbacks))
    }
    if err := json.NewEncoder(os.Stdout).Encode(map[string]any{
        "parsers": n, "source_bytes": len(input), "heap_alloc": memory.HeapAlloc,
        "heap_inuse": memory.HeapInuse, "heap_sys": memory.HeapSys,
        "routes": routes, "fallbacks": fallbacks,
    }); err != nil { panic(err) }
    runtime.KeepAlive(parsers)
}
GO

export GOWORK=off GOMAXPROCS=1 GTS_ADMISSION_CANDIDATE=1
go build -o "$probe_dir/probe" "$probe_dir/main.go"
/usr/bin/time -v "$probe_dir/probe" "$parsers" "$language" "$functions"
