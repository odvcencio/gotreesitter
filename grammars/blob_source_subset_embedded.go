//go:build grammar_subset && !grammar_blobs_external && !grammar_set_core

package grammars

import (
	"embed"
	"fmt"
)

// This file provides the embedded-but-selective grammar blob source for
// grammar_subset builds (issue #88). Under a `grammar_subset` build that is NOT
// `grammar_blobs_external` and NOT `grammar_set_core`, the all-grammars wildcard
// embed in blob_source_embedded.go is compiled out (its build tag excludes
// grammar_subset). Instead, each selected language contributes its own blob via
// a generated z_subset_blob_embed_<lang>.go file (tagged grammar_subset_<lang>),
// whose init() registers its embed.FS here. The result is a self-contained
// binary that embeds ONLY the blobs for the selected grammar_subset_<lang> tags.
//
//	go build -tags 'grammar_subset grammar_subset_go grammar_subset_python'
//
// embeds only go.bin and python.bin (~hundreds of KB) instead of all ~206
// blobs (~20MB), with no GOTREESITTER_GRAMMAR_BLOB_DIR required at runtime.
//
// Build-tag matrix (exactly one readGrammarBlob is compiled per build):
//
//	grammar_blobs_external                                   -> blob_source_external.go        (no embed; loads from dir)
//	!external && grammar_set_core                            -> blob_source_embedded_core.go   (Core100 embedded)
//	!external && !core && grammar_subset                     -> THIS FILE                      (selected blobs embedded)
//	!external && !core && !grammar_subset                    -> blob_source_embedded.go        (all blobs embedded)

// subsetEmbeddedBlobs holds the embed.FS for each grammar blob selected via a
// grammar_subset_<lang> build tag. Populated by z_subset_blob_embed_<lang>.go
// init() functions; package-level var init runs before any init(), so the map
// is ready when those init()s register into it.
var subsetEmbeddedBlobs = map[string]embed.FS{}

// registerSubsetEmbeddedBlob records a selected language's embedded blob FS.
// blobName is the bare file name (e.g. "go.bin"); fs embeds
// "grammar_blobs/<blobName>".
func registerSubsetEmbeddedBlob(blobName string, fs embed.FS) {
	subsetEmbeddedBlobs[blobName] = fs
}

func readGrammarBlob(blobName string) (grammarBlob, error) {
	fs, ok := subsetEmbeddedBlobs[blobName]
	if !ok {
		return grammarBlob{}, fmt.Errorf(
			"grammar blob %q is not embedded in this grammar_subset build: add its "+
				"grammar_subset_<lang> build tag, or build without grammar_subset", blobName)
	}
	data, err := fs.ReadFile("grammar_blobs/" + blobName)
	if err != nil {
		return grammarBlob{}, err
	}
	return grammarBlob{data: data}, nil
}
