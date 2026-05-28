// Command grammarblobprobe is a minimal binary that blank-imports the grammars
// package so that whatever grammar blobs are embedded by the active build tags
// are linked into the binary. It exists to measure the embedded-blob payload of
// each grammar-selection build mode (default, grammar_set_core, grammar_subset,
// grammar_blobs_external) — the binary's size is the measurement.
//
//	go build -o /tmp/probe.full                                   ./cmd/grammarblobprobe   # all blobs
//	go build -tags 'grammar_subset grammar_subset_go' -o /tmp/probe.go ./cmd/grammarblobprobe   # go only
//
// A grammar_subset build that embeds only the selected blobs will be
// dramatically smaller than the default all-grammars build.
package main

import _ "github.com/odvcencio/gotreesitter/grammars"

func main() {}
