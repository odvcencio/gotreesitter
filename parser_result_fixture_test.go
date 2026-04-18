package gotreesitter

import (
	"os"
	"path/filepath"
	"testing"
)

func mustReadParserResultFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "parser_result", name))
	if err != nil {
		t.Fatalf("read parser result fixture %q: %v", name, err)
	}
	return data
}
