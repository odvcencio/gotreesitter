//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func loadCSharpCliffFixture(t *testing.T) []byte {
	t.Helper()
	archive, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "testdata", "cliffs", "c_sharp_generated_32k.cs.gz"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	source, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return source
}
