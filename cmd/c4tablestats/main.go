// Command c4tablestats is the C4 stage-2 table-shape analyzer.
//
// spec.c4-bytecode-isa.v1 section 2 marks every static table-shape number in
// the design brief an estimate and makes running this analyzer the stage-2
// precondition: "Stage 2 runs it and pins the outputs before any
// superinstruction lands." This command produces the committed receipt.
//
// Run it against the grammar blobs the spec names:
//
//	go run ./cmd/c4tablestats -out testdata/c4_table_shape.json \
//	    grammars/grammar_blobs/go.bin \
//	    grammars/grammar_blobs/javascript.bin \
//	    grammars/grammar_blobs/json.bin
//
// With -check the command re-measures and compares against the committed
// receipt instead of rewriting it, so CI can prove the pinned numbers still
// describe the shipped tables.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gts "github.com/odvcencio/gotreesitter"
)

// receipt is the committed artifact. Schema and blob digests are part of it so
// a row can never be read as describing tables it was not measured against.
type receipt struct {
	Schema   string                             `json:"schema"`
	Spec     string                             `json:"spec"`
	Grammars []gts.ParserCoreCorridorTableShape `json:"grammars"`
}

const receiptSchema = "gts-c4-table-shape/v1"
const receiptSpec = "spec.c4-bytecode-isa.v1"

func main() {
	out := flag.String("out", "", "write the receipt to this path (default: stdout)")
	check := flag.Bool("check", false, "compare against the committed receipt at -out instead of writing it")
	flag.Parse()

	blobs := flag.Args()
	if len(blobs) == 0 {
		fmt.Fprintln(os.Stderr, "c4tablestats: give at least one grammar blob path")
		os.Exit(2)
	}

	measured := receipt{Schema: receiptSchema, Spec: receiptSpec}
	for _, path := range blobs {
		shape, err := analyze(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4tablestats: %s: %v\n", path, err)
			os.Exit(1)
		}
		measured.Grammars = append(measured.Grammars, shape)
	}

	encoded, err := json.MarshalIndent(measured, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4tablestats: encode receipt: %v\n", err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')

	if *out == "" {
		os.Stdout.Write(encoded)
		return
	}
	if *check {
		committed, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4tablestats: read committed receipt: %v\n", err)
			os.Exit(1)
		}
		if !bytes.Equal(committed, encoded) {
			fmt.Fprintf(os.Stderr, "c4tablestats: measured table shape differs from the committed receipt %s\n", *out)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "c4tablestats: %s matches the measured table shape\n", *out)
		return
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "c4tablestats: write receipt: %v\n", err)
		os.Exit(1)
	}
}

func analyze(path string) (gts.ParserCoreCorridorTableShape, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return gts.ParserCoreCorridorTableShape{}, err
	}
	lang, err := gts.LoadLanguage(data)
	if err != nil {
		return gts.ParserCoreCorridorTableShape{}, fmt.Errorf("decode blob: %w", err)
	}
	if lang.Name == "" {
		lang.Name = strings.TrimSuffix(filepath.Base(path), ".bin")
	}
	shape, err := gts.AnalyzeParserCoreCorridorTables(lang)
	if err != nil {
		return gts.ParserCoreCorridorTableShape{}, err
	}
	shape.BlobSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	return shape, nil
}
