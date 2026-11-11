package lexexact

import (
	"os"
	"strings"
)

func readFile(name string) ([]byte, error) { return os.ReadFile(name) }

// parseInc is Parse for test text that does not start with the header include:
// it adds the include, which Parse requires before any declaration.
func parseInc(c, h string) (*Source, error) {
	if !strings.Contains(c, "#include") {
		c = "#include \"tree_sitter/parser.h\"\n" + c
	}
	return Parse(c, h)
}
